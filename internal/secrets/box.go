// Package secrets keeps the secrets gitd stores on disk — the tokens and private keys of
// connections made through the API — sealed with a key kept apart, when one is given.
//
// Without a key a secret is kept as it is, and only the file's permissions protect it.
// With one, a copy of the state directory or a backup of it opens nothing without the key.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// sealedMark starts a sealed secret, saying how it was sealed.
const sealedMark = "aes256gcm:"

// keySize is the size of the key, in bytes: AES-256.
const keySize = 32

// Box seals and opens secrets with one key, or keeps them as they are without one.
type Box struct {
	aead cipher.AEAD
}

// Open returns the box for the key in keyFile, or a box without a key when keyFile is
// empty. The key is 32 bytes, written raw, in hex or in base64.
func Open(keyFile string) (*Box, error) {
	if keyFile == "" {
		return &Box{}, nil
	}
	read, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("the secrets key: %w", err)
	}
	key, err := decodeKey(read)
	if err != nil {
		return nil, fmt.Errorf("the secrets key in %s: %w", keyFile, err)
	}
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

// Seal returns the secret as it is kept: sealed when the box has a key.
func (box *Box) Seal(secret []byte) (string, error) {
	if box.aead == nil {
		return string(secret), nil
	}
	nonce := make([]byte, box.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := box.aead.Seal(nonce, nonce, secret, nil)
	return sealedMark + base64.StdEncoding.EncodeToString(sealed), nil
}

// Unseal returns the secret a kept value holds. A value kept before a key was given opens
// as it is; a sealed one needs the key it was sealed with.
func (box *Box) Unseal(kept string) ([]byte, error) {
	encoded, sealed := strings.CutPrefix(kept, sealedMark)
	if !sealed {
		return []byte(kept), nil
	}
	if box.aead == nil {
		return nil, errors.New("this secret is sealed, and gitd was given no secrets_key_file to open it")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < box.aead.NonceSize() {
		return nil, errors.New("this sealed secret is damaged")
	}
	nonce, sealedData := data[:box.aead.NonceSize()], data[box.aead.NonceSize():]
	secret, err := box.aead.Open(nil, nonce, sealedData, nil)
	if err != nil {
		return nil, errors.New("this secret was sealed with another key")
	}
	return secret, nil
}

// decodeKey reads a key written raw, in hex or in base64.
func decodeKey(read []byte) ([]byte, error) {
	if len(read) == keySize {
		return read, nil
	}
	text := strings.TrimSpace(string(read))
	if decoded, err := hex.DecodeString(text); err == nil && len(decoded) == keySize {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil && len(decoded) == keySize {
		return decoded, nil
	}
	return nil, fmt.Errorf("it must be %d bytes, raw, in hex or in base64", keySize)
}
