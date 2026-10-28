package history

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
)

type Cipher struct{ aead cipher.AEAD }

func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, errors.New("la clé KOL doit contenir 32 octets")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead}, nil
}

func (c *Cipher) Seal(plain []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plain, []byte("KOL/v1")), nil
}

func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(sealed) < n+c.aead.Overhead() {
		return nil, errors.New("données KOL tronquées")
	}
	return c.aead.Open(nil, sealed[:n], sealed[n:], []byte("KOL/v1"))
}
