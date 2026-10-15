package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"mime"
	"strings"
)

const (
	oldAAD  = "artex-proxy-pool-v1"
	newAAD  = "yunkun-proxy-pool-v2"
	oldMIME = "application/x-artex-extracted-text"
	newMIME = "application/x-yunkun-extracted-text"
)

func convertCredential(key []byte, raw string) (string, bool, error) {
	if len(key) != 32 {
		return "", false, errors.New("invalid key")
	}
	if raw == "" {
		return raw, false, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", false, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", false, err
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) < aead.NonceSize()+aead.Overhead() {
		return "", false, errors.New("invalid ciphertext")
	}
	nonce, sealed := b[:aead.NonceSize()], b[aead.NonceSize():]
	if _, err := aead.Open(nil, nonce, sealed, []byte(newAAD)); err == nil {
		return raw, false, nil
	}
	plain, err := aead.Open(nil, nonce, sealed, []byte(oldAAD))
	if err != nil {
		return "", false, errors.New("cannot decrypt credential")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(plain, &payload); err != nil || payload == nil {
		return "", false, errors.New("invalid credential payload")
	}
	nextNonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nextNonce); err != nil {
		return "", false, err
	}
	next := aead.Seal(nextNonce, nextNonce, plain, []byte(newAAD))
	return base64.StdEncoding.EncodeToString(next), true, nil
}

func convertMIME(raw string) string {
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != oldMIME {
		return raw
	}
	if semi := strings.IndexByte(raw, ';'); semi >= 0 {
		return newMIME + raw[semi:]
	}
	return newMIME
}
