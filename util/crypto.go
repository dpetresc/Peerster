package util

import (
	"bufio"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"github.com/monnand/dhkx"
	"io"
	"os"
)

// TODO change timer
var ConsensusTimerMin uint32 = 15
var CAAddress = "127.0.0.1:4343"

// folder where the keys needed by this node are stored
var KeysFolderPath = "./_MyKeys/"
var KeysHS = "./_HSKeys/"

var HSHTML = "./_HS/index.html"

// DH group used in all the Peerster crypto
var DHGroupID = 0

/*
 * Check if two []byte are equal
 */
func Equals(tab1, tab2 []byte) bool{
	if len(tab1) != len(tab2){
		return false
	}

	for i := range tab1 {
		if tab1[i] != tab2[i]{
			return false
		}
	}
	return true
}

/****************************** RSA ************************************/

func readKeyFromFile(path string) *pem.Block {
	privateKeyFile, err := os.Open(path)
	CheckError(err)

	pemInfo, err := privateKeyFile.Stat()
	CheckError(err)
	pemBytes := make([]byte, pemInfo.Size())

	buffer := bufio.NewReader(privateKeyFile)
	_, err = buffer.Read(pemBytes)
	CheckError(err)
	data, _ := pem.Decode([]byte(pemBytes))

	err = privateKeyFile.Close()
	CheckError(err)
	return data
}

/*
 * Read rsa.PrivateKey from path
 */
func ReadRSAPrivateKey(path string) *rsa.PrivateKey {
	data := readKeyFromFile(path + "private.pem")
	privateKey, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	CheckError(err)

	return privateKey
}

/*
 * Get the public key of the CA
 */
func GetCAKey(path string) *rsa.PublicKey {
	CAKeyPath := path + "CA/"

	data := readKeyFromFile(CAKeyPath + "public.pem")

	publicKey, err := x509.ParsePKCS1PublicKey(data.Bytes)
	CheckError(err)

	return publicKey
}

/*
 * Writes the publickey into a file if it doesn't already exists
 */
func SavePublicKey(path string, publicKey *rsa.PublicKey) {
	publicPath := path + "public.pem"
	if _, err := os.Stat(publicPath); err != nil && os.IsNotExist(err) {
		var pemkey = &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: x509.MarshalPKCS1PublicKey(publicKey),
		}

		pemfile, err := os.Create(publicPath)
		CheckError(err)
		defer pemfile.Close()

		err = pem.Encode(pemfile, pemkey)
		CheckError(err)
	}
}

/*
 * Generate private RSA key or reads it from file
 * if one already exists for this username
 */
func GetPrivateKey(path, name string) *rsa.PrivateKey {
	myPrivateKeyPath := path + name + "/"

	var privateKey *rsa.PrivateKey
	var err error

	if _, err = os.Stat(myPrivateKeyPath); err == nil {
		privateKey = ReadRSAPrivateKey(myPrivateKeyPath)
	} else if os.IsNotExist(err) {
		os.MkdirAll(myPrivateKeyPath, 0777)
		privateKey, err = rsa.GenerateKey(rand.Reader, 4096)
		CheckError(err)

		pemPrivateFile, err := os.Create(myPrivateKeyPath + "/private.pem")
		CheckError(err)

		pemPrivateBlock := &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		}

		err = pem.Encode(pemPrivateFile, pemPrivateBlock)
		CheckError(err)

		err = pemPrivateFile.Close()
		CheckError(err)
	}
	return privateKey
}

/*
 * Encrypt a message with a private key
 */
func EncryptRSA(message []byte, publicKey *rsa.PublicKey) []byte {
	encryption, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, message, nil)

	CheckError(err)

	return encryption
}

/*
 * Decrypt a message with a private key
 */
func DecryptRSA(message []byte, privateKey *rsa.PrivateKey) []byte {
	message, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, message, nil)

	CheckError(err)

	return message
}

/*
 * Sign a message with a private key
 */
func SignRSA(message []byte, privateKey *rsa.PrivateKey) []byte {
	hashed := sha256.Sum256(message)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	CheckError(err)

	return signature
}

/*
 *	Verifies that the signature of the message made with the key corresponding to the given public key
 *	is correct.
 */
func VerifyRSASignature(message, signature []byte, publicKey *rsa.PublicKey) bool {
	hashed := sha256.Sum256(message)
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	return err == nil
}

/****************************** DH + GCM ************************************/
/*
 * CreateDHPartialKey creates a partial key for DH protocol
 */
func CreateDHPartialKey() (*dhkx.DHKey, []byte) {
	g, err := dhkx.GetGroup(DHGroupID)
	CheckError(err)

	privateDH, err := g.GeneratePrivateKey(nil)
	CheckError(err)

	publicDH := privateDH.Bytes()
	return privateDH, publicDH
}

/*
 *	CreateDHSharedKey creates shared key from
 *	dhPublic the public partial key
 *	privateDH the private partial key
 */
func CreateDHSharedKey(dhPublic []byte, privateDH *dhkx.DHKey) []byte {
	g, err := dhkx.GetGroup(DHGroupID)
	CheckError(err)

	sharedKey, err := g.ComputeKey(dhkx.NewPublicKey(dhPublic), privateDH)
	CheckError(err)
	shaKeyShared := sha256.Sum256(sharedKey.Bytes())
	shaKeySharedBytes := shaKeyShared[:]
	return shaKeySharedBytes
}

/*
 *	EncryptGCM encrypts the given plaintext using GCM encryption.
 *	plaintext []byte is the message that must be encrypted
 *	sharedKey []byte is the key used for the encryption
 *	It returns a pair of ciphertext and nonce.
 */
func EncryptGCM(plaintext, sharedKey []byte) ([]byte, []byte) {
	//Encrypt
	block, err := aes.NewCipher(sharedKey)
	CheckError(err)

	nonce := make([]byte, 12)
	_, err = io.ReadFull(rand.Reader, nonce)
	CheckError(err)

	aesgcm, err := cipher.NewGCM(block)
	CheckError(err)
	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	return ciphertext, nonce
}

/*
 *	DecryptGCM decrypts the given ciphertext that was encrypted using the given nonce and sharedKey.
 *	ciphertext 	[]byte is the ciphertext that needs to be decrypted
 *	nonce		[]byte is the nonce used to encrypt the message
 *	sharedKey	[]byte is the shared key used to encrypt the message
 *	It returns the plaintext as a slice of bytes.
 */
func DecryptGCM(ciphertext, nonce, sharedKey []byte) []byte {
	block, err := aes.NewCipher(sharedKey)
	CheckError(err)

	aesgcm, err := cipher.NewGCM(block)
	CheckError(err)

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	CheckError(err)
	return plaintext
}
