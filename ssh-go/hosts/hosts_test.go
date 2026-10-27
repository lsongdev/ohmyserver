package hosts

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestNormalize(t *testing.T) {
	h := Host{Name: " nas ", Address: "10.0.0.2", User: "root", Password: "x"}
	if err := h.Normalize(); err != nil || h.Port != 22 || h.Auth != AuthPassword || h.Name != "nas" {
		t.Fatalf("%+v %v", h, err)
	}
	for _, bad := range []Host{
		{Address: "a", User: "u", Password: "x"},
		{Name: "n", User: "u", Password: "x"},
		{Name: "n", Address: "a b", User: "u", Password: "x"},
		{Name: "n", Address: "a", Password: "x"},
		{Name: "n", Address: "a", User: "u"},
		{Name: "n", Address: "a", User: "u", Auth: AuthKey, PrivateKey: "junk"},
		{Name: "n", Address: "a", User: "u", Auth: "agent"},
		{Name: "n", Address: "a", User: "u", Password: "x", Port: 70000},
	} {
		if err := bad.Normalize(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestEncryptedKeyNeedsPassphrase(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	h := Host{Name: "n", Address: "a", User: "u", Auth: AuthKey, PrivateKey: string(pem.EncodeToMemory(block))}
	if err := h.Normalize(); err == nil || !strings.Contains(err.Error(), "passphrase") {
		t.Fatalf("without passphrase: %v", err)
	}
	h.Passphrase = "pw"
	if err := h.Normalize(); err != nil {
		t.Fatal(err)
	}
}

func TestPanelKey(t *testing.T) {
	private, err := NewKey("ssh-go")
	if err != nil {
		t.Fatal(err)
	}
	line, signer, err := PublicKey(private, "ssh-go")
	if err != nil || !strings.HasPrefix(line, "ssh-ed25519 ") || !strings.HasSuffix(line, " ssh-go") || signer == nil {
		t.Fatalf("%q %v", line, err)
	}
}
