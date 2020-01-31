package gossip

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"net/http"
	"strings"
	"sync"
	"time"
)

type HSDescriptor struct {
	PublicKey []byte
	IPIdentity string
	OnionAddress string
	Signature []byte
}

type CAHSHashMap struct {
	// onion address => hsDescriptor
	HS        map[string]*HSDescriptor
	Signature []byte
}

type LockHS struct {
	// my hidden services : onion address => private key
	MPrivateKeys map[string]*rsa.PrivateKey
	// consensus services : onion address => public key
	HashMap map[string]*HSDescriptor

	// if this node is an IP or the HS node
	// onion addr to corresponding circuit id
	OnionAddrToCircuit map[string]uint32

	// onion address to html page
	HTMLForGui map[string]string

	sync.RWMutex
}

type HSConnections struct {
	sync.RWMutex
	hsCos map[uint64]*HSConnection
}

type HSConnection struct{
	OnionAddr string
	SharedKey []byte
	RDVPoint string
}

func NewHSConnections() *HSConnections{
	return &HSConnections{
		RWMutex:           sync.RWMutex{},
		hsCos: make(map[uint64]*HSConnection),
	}
}

/*
 * sendHSDescriptorToConsensus send the HS Descriptor to the CA
 */
func (hsDescriptor *HSDescriptor) sendHSDescriptorToConsensus() {
	buf := new(bytes.Buffer)
	err := json.NewEncoder(buf).Encode(*hsDescriptor)
	util.CheckError(err)
	r, err := http.Post("http://"+util.CAAddress+"/hsDescriptor", "application/json; charset=utf-8", buf)
	util.CheckError(err)
	util.CheckHttpError(r)
}

/*
 * sendHSDescriptorToConsensus send the HS Descriptor to the CA
 */
func (gossiper *Gossiper) getHSFromConsensus() {
	r, err := http.Get("http://" + util.CAAddress + "/hsConsensus")
	util.CheckError(err)
	util.CheckHttpError(r)
	var CAHashMap CAHSHashMap
	err = json.NewDecoder(r.Body).Decode(&CAHashMap)
	util.CheckError(err)
	r.Body.Close()

	hashMapBytes, err := json.Marshal(CAHashMap.HS)
	gossiper.LConsensus.Lock()
	if !util.VerifyRSASignature(hashMapBytes, CAHashMap.Signature, gossiper.LConsensus.CAKey) {
		err = errors.New("CA corrupted")
		gossiper.LConsensus.Unlock()
		util.CheckError(err)
		return
	}
	gossiper.LConsensus.Unlock()

	gossiper.LHS.Lock()
	gossiper.LHS.HashMap = CAHashMap.HS
	gossiper.LHS.Unlock()
}

/*
 * Derive the onion address from the public key
 */
func generateOnionAddr(publicKeyPKCS1 []byte) string {
	// sha1 hash
	sha := sha1.Sum(publicKeyPKCS1)
	base32Str := base32.StdEncoding.EncodeToString(sha[:])
	return strings.ToLower(base32Str[:len(base32Str)/2]) + ".onion"
}

func (gossiper *Gossiper) createHS(packet *util.Message) {
	privateKey := util.GetPrivateKey(util.KeysFolderPath, *packet.HSPort)
	publicKey := privateKey.PublicKey
	publicKeyPKCS1 := x509.MarshalPKCS1PublicKey(&publicKey)
	gossiper.LConsensus.RLock()
	// Introduction Point node
	ip := gossiper.selectRandomNodeFromConsensus("")
	gossiper.LConsensus.RUnlock()

	if ip == "" {
		fmt.Println("Consensus does not have enough nodes, retry later")
		return
	}
	onionAddr := generateOnionAddr(publicKeyPKCS1)

	pKIP := append(publicKeyPKCS1[:], []byte(ip)...)
	pkIpOnionAddr := append(pKIP[:], []byte(onionAddr)...)
	signature := util.SignRSA(pkIpOnionAddr, privateKey)

	hsDescriptor := &HSDescriptor{
		PublicKey:    publicKeyPKCS1,
		IPIdentity:   ip,
		OnionAddress: onionAddr,
		Signature:    signature,
	}
	gossiper.LHS.Lock()
	gossiper.LHS.MPrivateKeys[onionAddr] = privateKey
	gossiper.LHS.Unlock()
	hsDescriptor.sendHSDescriptorToConsensus()

	// open connection with IP
	privateMessage := &util.PrivateMessage{
		Origin:      "",
		ID:          0,
		Text:        "",
		Destination: "",
		HopLimit:    0,
		HsFlag:      util.IPRequest,
		RDVPoint:    "",
		OnionAddr:   onionAddr,
		IPIdentity:  "",
		Cookie:      0,
		PublicDH:    nil,
		SignatureDH: nil,
	}
	gossiper.HandlePrivateMessageToSend(ip, privateMessage)

	// Keep alive
	go gossiper.keepAlive(ip)
}

func (gossiper *Gossiper) keepAlive(ip string) {
	ticker := time.NewTicker(time.Duration((torTimeoutCircuits-1) * time.Minute))
	for {
		select {
		case <-ticker.C:
			gossiper.LConsensus.Lock()
			gossiper.lCircuits.Lock()
			// open connection with IP
			privateMessage := &util.PrivateMessage{
				Origin:      "",
				ID:          0,
				Text:        "",
				Destination: "",
				HopLimit:    0,
				HsFlag:      util.KeepAlive,
				RDVPoint:    "",
				OnionAddr:   "",
				IPIdentity:  "",
				Cookie:      0,
				PublicDH:    nil,
				SignatureDH: nil,
			}
			gossiper.HandlePrivateMessageToSend(ip, privateMessage)
			gossiper.LConsensus.Unlock()
			gossiper.lCircuits.Unlock()
		}
	}
}