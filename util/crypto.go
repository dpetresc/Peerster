package util

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
)

func GetPrivateKey(name string) *rsa.PrivateKey{
	return nil
}

func GetPublicKey(name string) *rsa.PublicKey{
	return nil
}

/*
 *	Sign signs a message given a private key.
 *	message 	[]byte is the message that need to be signed
 *	privateKey 	*rsa.PrivateKey is the key needed for the signature
 */
func Sign(message []byte, privateKey *rsa.PrivateKey) []byte {
	hashed := sha256.Sum256(message)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	CheckError(err)
	return signature
}

/*
 *	Verify verifies that the signature of the message made with the key corresponding to the given public key
 *	is correct.
 *	message 	[]byte is the message that need to be signed
 *	signature	[]byte is the signature of the message.
 *	publicKey 	*rsa.PublicKey is the public key needed for the signature
 */
func Verify(message, signature []byte, publicKey *rsa.PublicKey) bool{
	hashed := sha256.Sum256(message)
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	return err == nil
}