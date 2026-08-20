// Package secretbox encrypts DB-resident secrets with the installation key.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

type Box struct{ aead cipher.AEAD }

func New(key []byte) (*Box, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(plaintext string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	data := b.aead.Seal(nonce, nonce, []byte(plaintext), []byte("igame:v1"))
	return "v1:" + base64.RawURLEncoding.EncodeToString(data), nil
}

func (b *Box) Open(ciphertext string) (string, error) {
	if len(ciphertext) < 4 || ciphertext[:3] != "v1:" {
		return "", fmt.Errorf("unsupported ciphertext")
	}
	data, err := base64.RawURLEncoding.DecodeString(ciphertext[3:])
	if err != nil || len(data) < b.aead.NonceSize() {
		return "", fmt.Errorf("invalid ciphertext")
	}
	nonce := data[:b.aead.NonceSize()]
	plain, err := b.aead.Open(nil, nonce, data[b.aead.NonceSize():], []byte("igame:v1"))
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plain), nil
}
