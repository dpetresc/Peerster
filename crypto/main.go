package main

import (
	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/dpetresc/Peerster/util"
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
 */

func main(){
	//Signature("YOOO", "../keys/CA")
	//GenerateAndStoreRSAKeys("../keys/CA")
	SignAndVerifyKey("../keys/own", "../keys/CA")
}

func SignAndVerifyKey(pathKeyToSign, pathPrivateKey string){
	//Node's side...
	_, publicKey := ImportRSAKeys(pathKeyToSign)
	pemPublicBlock := &pem.Block{
		Type:    "RSA PRIVATE KEY",
		Bytes:   x509.MarshalPKCS1PublicKey(publicKey),
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

func GenerateAndStoreRSAKeys(path string){
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	util.CheckError(err)

	//export key
	pemPrivateFile, err := os.Create(path+"/private.pem")
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

	privateKeyFile, err := os.Open(path+"/private.pem")
	util.CheckError(err)

	pemInfo, err := privateKeyFile.Stat()
	util.CheckError(err)
	pemBytes := make([]byte, pemInfo.Size())

	buffer := bufio.NewReader(privateKeyFile)
	_,err = buffer.Read(pemBytes)
	util.CheckError(err)
	data, _ := pem.Decode([]byte(pemBytes))

	err = privateKeyFile.Close()
	util.CheckError(err)

	privateKey, err := x509.ParsePKCS1PrivateKey(data.Bytes)
	util.CheckError(err)

	publicKey := &privateKey.PublicKey

	return privateKey, publicKey
}

func Signature(message, path string){

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
