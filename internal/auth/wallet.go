package auth

import (
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/sha3"
)

// DeriveWalletAddress deterministically derives an EVM address from an Ed25519
// public key using the same rule as the spec §0:
//
//	keccak256(pubkey_bytes)[12:] → 20-byte address → EIP-55 checksum hex
//
// The derivation is purely mathematical — same public key always yields the
// same wallet address, no external service required.
func DeriveWalletAddress(publicKeyHex string) (string, error) {
	raw, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(raw) != 32 {
		return "", errors.New("invalid Ed25519 public key")
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(raw)
	full := h.Sum(nil) // 32 bytes
	addr := full[12:]  // last 20 bytes
	return eip55(addr), nil
}

// eip55 encodes a 20-byte address as a checksummed "0x…" hex string.
func eip55(addr []byte) string {
	lower := hex.EncodeToString(addr) // 40 lowercase hex chars
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(lower))
	csum := h.Sum(nil)

	result := make([]byte, 42)
	result[0] = '0'
	result[1] = 'x'
	for i := range 40 {
		c := lower[i]
		if c >= 'a' && c <= 'f' {
			nibble := csum[i/2]
			if i%2 == 0 {
				nibble >>= 4
			}
			if nibble&0xf >= 8 {
				c -= 32 // uppercase
			}
		}
		result[i+2] = c
	}
	return string(result)
}
