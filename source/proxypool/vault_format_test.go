package proxypool

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestVaultCurrentFormatAndIntegrity(t *testing.T) {
	v, err := NewVault(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	want := Secrets{ProxyUsername: "test-user", ProxyPassword: "test-password"}
	sealed, err := v.Seal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Open(sealed)
	if err != nil || got != want {
		t.Fatalf("roundtrip failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatal(err)
	}
	n := v.aead.NonceSize()
	if _, err := v.aead.Open(nil, raw[:n], raw[n:], []byte(vaultAAD)); err != nil {
		t.Fatalf("current format: %v", err)
	}
	wrong, _ := NewVault(bytes.Repeat([]byte{1}, 32))
	if _, err := wrong.Open(sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	raw[len(raw)-1] ^= 1
	if _, err := v.Open(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
