package auth

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"
)

func TestChallengeRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	m := NewManager("secret")

	challenge, err := m.NewChallenge(pubHex)
	if err != nil {
		t.Fatalf("NewChallenge: %v", err)
	}
	sig := hex.EncodeToString(ed25519.Sign(priv, []byte(challenge)))

	if err := m.VerifyChallenge(pubHex, challenge, sig); err != nil {
		t.Fatalf("VerifyChallenge: %v", err)
	}
	// Challenge is one-time: a replay must fail.
	if err := m.VerifyChallenge(pubHex, challenge, sig); err == nil {
		t.Fatal("expected replay to fail")
	}
}

func TestVerifyChallengeWrongKey(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(nil)
	_, attacker, _ := ed25519.GenerateKey(nil)
	pubHex := hex.EncodeToString(pub)
	m := NewManager("secret")

	challenge, _ := m.NewChallenge(pubHex)
	badSig := hex.EncodeToString(ed25519.Sign(attacker, []byte(challenge)))
	if err := m.VerifyChallenge(pubHex, challenge, badSig); err == nil {
		t.Fatal("expected signature mismatch to fail")
	}
}

func TestJWTRoundTrip(t *testing.T) {
	m := NewManager("secret")
	token, err := m.IssueJWT("usr_abc")
	if err != nil {
		t.Fatalf("IssueJWT: %v", err)
	}
	owner, err := m.ParseJWT(token)
	if err != nil {
		t.Fatalf("ParseJWT: %v", err)
	}
	if owner != "usr_abc" {
		t.Fatalf("owner = %q, want usr_abc", owner)
	}
	// A token signed with a different key must be rejected.
	if _, err := NewManager("other").ParseJWT(token); err == nil {
		t.Fatal("expected verification with wrong key to fail")
	}
}

func TestPlatformTokenHash(t *testing.T) {
	hash, err := HashPlatformToken("ptk_secret")
	if err != nil {
		t.Fatalf("HashPlatformToken: %v", err)
	}
	if !CheckPlatformToken(hash, "ptk_secret") {
		t.Fatal("expected token to match its hash")
	}
	if CheckPlatformToken(hash, "ptk_wrong") {
		t.Fatal("expected wrong token to be rejected")
	}
}
