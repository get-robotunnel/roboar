package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"
)

// VerifyAgentRequest validates the four Agent-Signature headers on an incoming
// request (spec §1.4). It checks the 60-second replay window, then verifies the
// Ed25519 signature over (agent_id + nonce + body_hash).
//
// Parameters mirror the HTTP headers:
//
//	agentID    ← X-Agent-ID
//	nonce      ← X-Agent-Nonce
//	sigHex     ← X-Agent-Signature
//	tsStr      ← X-Agent-Timestamp
//	body       ← raw request body bytes
//	pubKeyHex  ← the agent's stored Ed25519 public key (retrieved from registry)
func VerifyAgentRequest(agentID, nonce, sigHex, tsStr string, body []byte, pubKeyHex string) error {
	if agentID == "" || nonce == "" || sigHex == "" || tsStr == "" {
		return errors.New("missing agent signature headers")
	}
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return errors.New("invalid X-Agent-Timestamp")
	}
	diff := time.Now().Unix() - ts
	if diff > 60 || diff < -60 {
		return errors.New("request timestamp out of 60-second window")
	}
	bodyHash := hex.EncodeToString(sha256Sum(body))
	msg := agentID + nonce + bodyHash
	return VerifyEd25519(pubKeyHex, []byte(msg), sigHex)
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
