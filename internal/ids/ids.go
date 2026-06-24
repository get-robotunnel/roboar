// Package ids generates prefixed, URL-safe identifiers for registry entities.
package ids

import gonanoid "github.com/matoous/go-nanoid/v2"

// size is the length of the random portion of an id. 16 chars of the default
// nanoid alphabet gives ~95 bits of entropy, which is plenty for our scale.
const size = 16

func newID(prefix string) string {
	id, err := gonanoid.New(size)
	if err != nil {
		// gonanoid.New only errors if the OS CSPRNG fails; that is fatal-class.
		panic("ids: failed to generate nanoid: " + err.Error())
	}
	return prefix + "_" + id
}

// Owner returns a new owner id, e.g. "usr_V1StGXR8Z5jdHi6B".
func Owner() string { return newID("usr") }

// Platform returns a new platform id, e.g. "plt_...".
func Platform() string { return newID("plt") }

// Agent returns a new agent id, e.g. "agt_...".
func Agent() string { return newID("agt") }

// Capability returns a new capability id, e.g. "cap_...".
func Capability() string { return newID("cap") }

// PlatformToken returns a fresh secret platform token, e.g. "ptk_<secret>".
// The plaintext is shown to the caller exactly once; only its bcrypt hash is
// persisted (spec §9.3).
func PlatformToken() string {
	secret, err := gonanoid.New(32)
	if err != nil {
		panic("ids: failed to generate platform token: " + err.Error())
	}
	return "ptk_" + secret
}
