package auth

import (
	"strings"
	"testing"
)

func TestDeriveWalletAddress(t *testing.T) {
	// Zero key: known-stable derivation — just verify format and determinism.
	zeroPub := strings.Repeat("00", 32)
	addr, err := DeriveWalletAddress(zeroPub)
	if err != nil {
		t.Fatalf("DeriveWalletAddress: %v", err)
	}
	if !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
		t.Fatalf("unexpected address format: %q", addr)
	}

	// Determinism: same key → same address.
	addr2, _ := DeriveWalletAddress(zeroPub)
	if addr != addr2 {
		t.Fatalf("not deterministic: %q vs %q", addr, addr2)
	}

	// Different key → different address.
	otherPub := strings.Repeat("ff", 32)
	addrOther, _ := DeriveWalletAddress(otherPub)
	if addr == addrOther {
		t.Fatalf("collision between zero-key and ff-key addresses")
	}

	// Invalid key.
	if _, err := DeriveWalletAddress("notvalid"); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestEIP55Checksum(t *testing.T) {
	// EIP-55 specifies that 0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed
	// is the checksummed form of 5aaeb6053f3e94c9b9a09f33669435e7ef1beaed.
	addr := eip55([]byte{
		0x5a, 0xae, 0xb6, 0x05, 0x3f, 0x3e, 0x94, 0xc9, 0xb9, 0xa0,
		0x9f, 0x33, 0x66, 0x94, 0x35, 0xe7, 0xef, 0x1b, 0xea, 0xed,
	})
	want := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	if addr != want {
		t.Fatalf("EIP-55 mismatch:\n got  %q\n want %q", addr, want)
	}
}
