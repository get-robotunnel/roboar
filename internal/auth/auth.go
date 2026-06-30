// Package auth implements the registry's three credential types (spec §2.2):
// owner JWTs (issued after an Ed25519 challenge-response login), platform tokens
// (bcrypt-hashed at rest), and raw Ed25519 signature verification.
package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrChallengeNotFound = errors.New("no pending challenge for this key")
	ErrChallengeExpired  = errors.New("challenge expired")
	ErrChallengeMismatch = errors.New("challenge does not match")
	ErrBadSignature      = errors.New("signature verification failed")
	ErrBadPublicKey      = errors.New("invalid public key")
)

const (
	challengeTTL = 5 * time.Minute
	accessTTL    = 24 * time.Hour
)

// Manager issues and verifies credentials. It is safe for concurrent use.
//
// Owner JWTs are signed with EdDSA (Ed25519) so that any party — including
// robot-side agents that never contact the registry per request — can verify a
// token locally with only the registry's public key, published at
// /.well-known/jwks.json. The signing key is derived deterministically from the
// configured JWT_SIGNING_KEY, so no extra configuration is required.
type Manager struct {
	signingPriv ed25519.PrivateKey
	signingPub  ed25519.PublicKey

	mu         sync.Mutex
	challenges map[string]pendingChallenge // keyed by owner public key (hex)
}

type pendingChallenge struct {
	value   string
	expires time.Time
}

func NewManager(signingKey string) *Manager {
	seed := sha256.Sum256([]byte(signingKey))
	priv := ed25519.NewKeyFromSeed(seed[:])
	return &Manager{
		signingPriv: priv,
		signingPub:  priv.Public().(ed25519.PublicKey),
		challenges:  make(map[string]pendingChallenge),
	}
}

// NewChallenge creates and stores a one-time login challenge for the given owner
// public key. The caller signs the returned string's bytes with the matching
// private key and submits the signature to Verify.
func (m *Manager) NewChallenge(publicKeyHex string) (string, error) {
	if _, err := decodePublicKey(publicKeyHex); err != nil {
		return "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	challenge := hex.EncodeToString(buf)

	m.mu.Lock()
	m.challenges[publicKeyHex] = pendingChallenge{value: challenge, expires: time.Now().Add(challengeTTL)}
	m.mu.Unlock()
	return challenge, nil
}

// VerifyChallenge checks a signed challenge and consumes it (one-time use).
func (m *Manager) VerifyChallenge(publicKeyHex, challenge, signatureHex string) error {
	m.mu.Lock()
	pc, ok := m.challenges[publicKeyHex]
	if ok {
		delete(m.challenges, publicKeyHex)
	}
	m.mu.Unlock()

	if !ok {
		return ErrChallengeNotFound
	}
	if time.Now().After(pc.expires) {
		return ErrChallengeExpired
	}
	if pc.value != challenge {
		return ErrChallengeMismatch
	}
	return VerifyEd25519(publicKeyHex, []byte(challenge), signatureHex)
}

// VerifyEd25519 verifies that signatureHex is a valid signature of message under
// the public key publicKeyHex.
func VerifyEd25519(publicKeyHex string, message []byte, signatureHex string) error {
	pub, err := decodePublicKey(publicKeyHex)
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(signatureHex)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pub, message, sig) {
		return ErrBadSignature
	}
	return nil
}

// ValidatePublicKey reports whether s is a well-formed Ed25519 public key (hex).
func ValidatePublicKey(s string) error {
	_, err := decodePublicKey(s)
	return err
}

func decodePublicKey(publicKeyHex string) (ed25519.PublicKey, error) {
	raw, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, ErrBadPublicKey
	}
	return ed25519.PublicKey(raw), nil
}

// IssueJWT returns a signed access token (EdDSA) for the owner, valid 24h.
func (m *Manager) IssueJWT(ownerID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   ownerID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(m.signingPriv)
}

// ParseJWT validates an access token and returns the owner id (subject).
func (m *Manager) ParseJWT(token string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.signingPub, nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid token")
	}
	return claims.Subject, nil
}

// PublicJWK returns the registry's owner-JWT verification key as a JWK (RFC 8037
// OKP / Ed25519). Published at /.well-known/jwks.json so agents can verify owner
// JWTs locally.
func (m *Manager) PublicJWK() map[string]string {
	return map[string]string{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(m.signingPub),
		"use": "sig",
		"alg": "EdDSA",
		"kid": "owner-jwt",
	}
}

// HashPlatformToken returns a bcrypt hash of a platform token for storage.
func HashPlatformToken(token string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPlatformToken reports whether token matches the stored bcrypt hash.
func CheckPlatformToken(hash, token string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
}
