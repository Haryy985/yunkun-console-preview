package proxypool

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Vault struct{ aead cipher.AEAD }

const vaultAAD = "yunkun-proxy-pool-v2"

func NewVault(key []byte) (*Vault, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func LoadVault(dir string) (*Vault, error) {
	path := filepath.Join(dir, "proxy-pool.key")
	key, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(e) {
			key, err = os.ReadFile(path)
		} else if e != nil {
			return nil, e
		} else {
			_, err = f.Write(key)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid proxy pool encryption key")
	}
	return NewVault(key)
}

func (v *Vault) Seal(s Secrets) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(v.aead.Seal(nonce, nonce, b, []byte(vaultAAD))), nil
}

func (v *Vault) Open(raw string) (Secrets, error) {
	var s Secrets
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(b) < v.aead.NonceSize() {
		return s, errors.New("proxy credentials unavailable")
	}
	plain, err := v.aead.Open(nil, b[:v.aead.NonceSize()], b[v.aead.NonceSize():], []byte(vaultAAD))
	if err != nil {
		return s, errors.New("proxy credentials unavailable")
	}
	if err := json.Unmarshal(plain, &s); err != nil {
		return Secrets{}, errors.New("proxy credentials unavailable")
	}
	return s, nil
}
