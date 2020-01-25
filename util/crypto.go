package util

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"io"
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

/*
 *	EncryptGCM encrypts the given plaintext using GCM encryption.
 *	plaintext []byte is the message that must be encrypted
 *	sharedKey []byte is the key used for the encryption
 *	It returns a pair of ciphertext and nonce.
 */
func EncryptGCM(plaintext, sharedKey []byte)([]byte,[]byte){
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
func DecryptGCM(ciphertext, nonce, sharedKey []byte)[]byte{
	block, err := aes.NewCipher(sharedKey)
	CheckError(err)

	aesgcm, err := cipher.NewGCM(block)
	CheckError(err)

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	CheckError(err)
	return plaintext
}