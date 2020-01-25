package main

import (
	"bufio"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/dpetresc/Peerster/util"
	"github.com/monnand/dhkx"
	"io"
	"os"
)

/*
 *	This main is not meant to be kept in the final version. It is only for an illustration and testing purpose
 *	for all the cryptographic operations needed in the project, i.e., symmetric encryption using GCM (AEAD),
 *	PKCS#1 v1.5 (RSA) signatures and Diffie-Hellman key exchange.
 *
 *	How to generate RSA keys:
 *	private: openssl genrsa -out private.pem 2048
 *	public: openssl rsa -in private.pem -outform PEM -pubout -out public.pem
 *	see https://www.openssl.org/docs/man1.0.2/man1/genrsa.html
 *
 *	How to import RSA keys and generate RSA keys in go:
 *	see https://medium.com/@Raulgzm/export-import-pem-files-in-go-67614624adc7
 *
 *	Code for Diffie-Hellman comes from:
 *	https://github.com/monnand/dhkx
 *	Notice that the created key is 256 bits longs
 *
 *	Example code for GCM comes from:
 *	https://golang.org/pkg/crypto/cipher/#AEAD
 */

func main() {
	//Signature("YOOO", "../keys/CA")
	//GenerateAndStoreRSAKeys("../keys/CA")
	//SignAndVerifyKey("../keys/own", "../keys/CA")
	GCM()
}

func SignAndVerifyKey(pathKeyToSign, pathPrivateKey string) {
	//Node's side...
	_, publicKey := ImportRSAKeys(pathKeyToSign)
	pemPublicBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PublicKey(publicKey),
	}
	//send pemPublicBlock to CA...

	//CA's side...
	privateCA, publicCA := ImportRSAKeys(pathPrivateKey)
	hashed := sha256.Sum256(pemPublicBlock.Bytes)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateCA, crypto.SHA256, hashed[:])
	util.CheckError(err)
	fmt.Printf("Signature: %x\n", signature)
	//send signature to Node...

	//Other node
	err = rsa.VerifyPKCS1v15(publicCA, crypto.SHA256, hashed[:], signature)
	util.CheckError(err)
	fmt.Println("Signature verified")

}

func GenerateAndStoreRSAKeys(path string) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	util.CheckError(err)

	//export key
	pemPrivateFile, err := os.Create(path + "/private.pem")
	util.CheckError(err)

	pemPrivateBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	err = pem.Encode(pemPrivateFile, pemPrivateBlock)
	util.CheckError(err)

	err = pemPrivateFile.Close()
	util.CheckError(err)
}

func ImportRSAKeys(path string) (*rsa.PrivateKey, *rsa.PublicKey) {

	privateKeyFile, err := os.Open(path + "/private.pem")
	util.CheckError(err)

	pemInfo, err := privateKeyFile.Stat()
	util.CheckError(err)
	pemBytes := make([]byte, pemInfo.Size())

	buffer := bufio.NewReader(privateKeyFile)
	_, err = buffer.Read(pemBytes)
	util.CheckError(err)
	data, _ := pem.Decode([]byte(pemBytes))

	err = privateKeyFile.Close()
	util.CheckError(err)

	privateKey, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	util.CheckError(err)

	publicKey := &privateKey.PublicKey

	return privateKey, publicKey
}

func Signature(message, path string) {

	private, public := ImportRSAKeys(path)

	/******* SIGN MESSAGE *******/
	//see https://golang.org/pkg/crypto/rsa/#SignPKCS1v15
	//Need to hash because only small messages can be directly signed!
	hashed := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, private, crypto.SHA256, hashed[:])
	util.CheckError(err)

	fmt.Printf("Signature of %s: %x\n", message, signature)

	/******* Verify Signature *******/
	//see https://golang.org/pkg/crypto/rsa/#VerifyPKCS1v15
	// err == nil => signature was correctly verified
	err = rsa.VerifyPKCS1v15(public, crypto.SHA256, hashed[:], signature)
	util.CheckError(err)
	fmt.Println("Signature verified")

}

func GCM(){
	key, _ := hex.DecodeString("6368616e676520746869732070617373776f726420746f206120736563726574")
	plaintext := []byte("exampleplaintext")

	//Encrypt
	block, err := aes.NewCipher(key)
	util.CheckError(err)

	nonce := make([]byte, 12)
	_, err = io.ReadFull(rand.Reader, nonce)
	util.CheckError(err)

	aesgcm, err := cipher.NewGCM(block)
	util.CheckError(err)

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)
	fmt.Printf("%x\n", ciphertext)

	//Decrypt
	pt, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	util.CheckError(err)

	fmt.Printf("%s\n", pt)
}

func DH() []byte {
	//Here is the code on Alice's side:

	// Get a group. Use the default one would be enough.
	g, err := dhkx.GetGroup(0)
	util.CheckError(err)

	// Generate a private key from the group.
	// Use the default random number generator.
	privateKey, err := g.GeneratePrivateKey(nil)
	util.CheckError(err)

	// Get the public key from the private key.
	pub := privateKey.Bytes()
	fmt.Println("Length: ",len(pub)," => ", pub)

	// Send the public key to Bob.

	// Receive a slice of bytes from Bob, which contains Bob's public key
	b := make([]byte, 32)
	_, err = rand.Read(b)
	util.CheckError(err)

	// Recover Bob's public key
	bobPubKey := dhkx.NewPublicKey(pub)

	// Compute the key
	k, _ := g.ComputeKey(bobPubKey, privateKey)

	// Get the key in the form of []byte
	key := k.Bytes()

	return key
}
