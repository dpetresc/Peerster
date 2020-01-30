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
	sync.RWMutex
}

type HSConnections struct {
	sync.RWMutex
	hsCos map[uint64]*HSConnetion
}

type HSConnetion struct{
	SharedKey []byte
	RDVPoint string
}

func NewHSConnections() *HSConnections{
	return &HSConnections{
		RWMutex:           sync.RWMutex{},
		hsCos: make(map[uint64]*HSConnetion),
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
	gossiper.lConsensus.Lock()
	if !util.VerifyRSASignature(hashMapBytes, CAHashMap.Signature, gossiper.lConsensus.CAKey) {
		err = errors.New("CA corrupted")
		gossiper.lConsensus.Unlock()
		util.CheckError(err)
		return
	}
	gossiper.lConsensus.Unlock()

	gossiper.lHS.Lock()
	gossiper.lHS.HashMap = CAHashMap.HS
	gossiper.lHS.Unlock()
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
	gossiper.lConsensus.RLock()
	// Introduction Point node
	ip := gossiper.selectRandomNodeFromConsensus("")
	gossiper.lConsensus.RUnlock()

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
	gossiper.lHS.Lock()
	gossiper.lHS.MPrivateKeys[onionAddr] = privateKey
	gossiper.lHS.Unlock()
	hsDescriptor.sendHSDescriptorToConsensus()

	// TODO
	// Keep alive
}