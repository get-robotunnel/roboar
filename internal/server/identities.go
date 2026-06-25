package server

import (
	"net/http"

	"github.com/RussellTNY/robot-agent-registry/internal/auth"
	"github.com/RussellTNY/robot-agent-registry/internal/ids"
	"github.com/gin-gonic/gin"
)

type quickIdentityReq struct {
	PublicKey   string `json:"public_key"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
}

// quickIdentity handles POST /v1/identities/quick.
//
// One-call registration path for human / CLI identities: it creates (or
// retrieves) an owner by public key, creates a "personal" platform, and
// registers a principal agent — all in a single transaction.  The plaintext
// platform token is returned exactly once; the DB stores only its bcrypt hash.
func (s *Server) quickIdentity(c *gin.Context) {
	var req quickIdentityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.DisplayName == "" {
		abort(c, http.StatusBadRequest, "display_name is required")
		return
	}
	if err := auth.ValidatePublicKey(req.PublicKey); err != nil {
		abort(c, http.StatusBadRequest, "public_key must be a 32-byte Ed25519 key in hex")
		return
	}
	kind := orDefault(req.Kind, "principal")
	if kind != "principal" && kind != "service" {
		abort(c, http.StatusBadRequest, "kind must be 'principal' or 'service'")
		return
	}

	token := ids.PlatformToken()
	hash, err := auth.HashPlatformToken(token)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not hash token")
		return
	}

	result, err := s.store.QuickIdentity(c, req.PublicKey, req.DisplayName, kind, hash)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not create identity")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"agent_id":       result.AgentID,
		"owner_id":       result.OwnerID,
		"platform_id":    result.PlatformID,
		"platform_token": token,
	})
}
