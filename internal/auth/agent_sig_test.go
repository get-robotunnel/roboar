package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestVerifyAgentRequest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)

	agentID := "agt_test123"
	nonce := hex.EncodeToString(make([]byte, 32)) // all zeros nonce
	body := []byte(`{"status":"online"}`)
	bodyHash := hex.EncodeToString(sha256Sum(body))
	ts := fmt.Sprintf("%d", time.Now().Unix())

	msg := agentID + nonce + bodyHash
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte(msg)))

	if err := VerifyAgentRequest(agentID, nonce, sig, ts, body, pubHex); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	// Bad signature
	if err := VerifyAgentRequest(agentID, nonce, "aabbcc", ts, body, pubHex); err == nil {
		t.Fatal("expected error for bad signature")
	}

	// Stale timestamp
	oldTS := fmt.Sprintf("%d", time.Now().Unix()-120)
	if err := VerifyAgentRequest(agentID, nonce, sig, oldTS, body, pubHex); err == nil {
		t.Fatal("expected error for stale timestamp")
	}

	// Missing headers
	if err := VerifyAgentRequest("", nonce, sig, ts, body, pubHex); err == nil {
		t.Fatal("expected error for missing agentID")
	}
}

func TestSha256Sum(t *testing.T) {
	b := []byte("hello")
	got := sha256Sum(b)
	want := sha256.Sum256(b)
	if hex.EncodeToString(got) != hex.EncodeToString(want[:]) {
		t.Fatal("sha256Sum mismatch")
	}
}
