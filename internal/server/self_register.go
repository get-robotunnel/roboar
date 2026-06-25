package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/RussellTNY/robot-agent-registry/internal/auth"
	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/RussellTNY/robot-agent-registry/internal/store"
	"github.com/gin-gonic/gin"
)

// ── POST /v1/agents/self-register ────────────────────────────────────────────

type selfRegisterCapability struct {
	Name          string          `json:"name"`
	DisplayName   string          `json:"display_name"`
	Description   string          `json:"description"`
	InterfaceType string          `json:"interface_type"`
	Permission    string          `json:"permission"`
	Pricing       json.RawMessage `json:"pricing"`
}

type selfRegisterReq struct {
	PublicKey    string                   `json:"public_key"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	AgentType    string                   `json:"agent_type"`
	Version      string                   `json:"version"`
	Capabilities []selfRegisterCapability `json:"capabilities"`
}

// selfRegister handles POST /v1/agents/self-register (no auth — public key is
// the identity credential). The endpoint is idempotent: sending the same public
// key again returns the existing agent_id without creating a new record.
func (s *Server) selfRegister(c *gin.Context) {
	var req selfRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.PublicKey == "" || req.Name == "" {
		abort(c, http.StatusBadRequest, "public_key and name are required")
		return
	}
	if err := auth.ValidatePublicKey(req.PublicKey); err != nil {
		abort(c, http.StatusBadRequest, "public_key must be a 32-byte Ed25519 key in hex")
		return
	}

	wallet, err := auth.DeriveWalletAddress(req.PublicKey)
	if err != nil {
		abort(c, http.StatusBadRequest, "could not derive wallet address")
		return
	}

	caps := make([]model.Capability, 0, len(req.Capabilities))
	for _, rc := range req.Capabilities {
		caps = append(caps, model.Capability{
			Name:          rc.Name,
			DisplayName:   orDefault(rc.DisplayName, rc.Name),
			Description:   rc.Description,
			InterfaceType: orDefault(rc.InterfaceType, "mcp_tool"),
			Permission:    orDefault(rc.Permission, "public"),
			Pricing:       rc.Pricing,
		})
	}

	result, err := s.store.SelfRegister(c, store.SelfRegisterRequest{
		PublicKey:    req.PublicKey,
		Name:         req.Name,
		Description:  req.Description,
		AgentType:    orDefault(req.AgentType, "robot"),
		Version:      orDefault(req.Version, "1.0"),
		Capabilities: caps,
	}, wallet)
	if err != nil {
		abort(c, http.StatusInternalServerError, "registration failed")
		return
	}

	status := http.StatusCreated
	if !result.IsNew {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{
		"agent_id":       result.AgentID,
		"wallet_address": result.WalletAddress,
		"status":         result.ClaimStatus,
		"owner_id":       result.OwnerID,
	})
}

// ── Agent-Signature middleware ────────────────────────────────────────────────

// agentSigAuth verifies the four Agent-Signature headers on requests that an
// agent sends to prove it is the key-holder (spec §1.4).
func (s *Server) agentSigAuth(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		abort(c, http.StatusBadRequest, "missing agent_id")
		return
	}

	// Read body without consuming the stream.
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		abort(c, http.StatusBadRequest, "could not read body")
		return
	}
	// Re-inject for downstream handlers.
	c.Request.Body = io.NopCloser(bytesReader(body))

	pubKey, err := s.store.GetAgentPublicKey(c, agentID)
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		abort(c, http.StatusInternalServerError, "lookup failed")
		return
	}
	if pubKey == "" {
		abort(c, http.StatusUnauthorized, "agent has no public key (not self-registered)")
		return
	}

	if err := auth.VerifyAgentRequest(
		agentID,
		c.GetHeader("X-Agent-Nonce"),
		c.GetHeader("X-Agent-Signature"),
		c.GetHeader("X-Agent-Timestamp"),
		body,
		pubKey,
	); err != nil {
		abort(c, http.StatusUnauthorized, "invalid agent signature: "+err.Error())
		return
	}

	c.Set(ctxAgentID, agentID)
	c.Next()
}

// ── POST /v1/agents/:agent_id/heartbeat ──────────────────────────────────────

type agentHeartbeatReq struct {
	Status         string `json:"status"`
	TunnelEndpoint string `json:"tunnel_endpoint"`
	MCPEndpoint    string `json:"mcp_endpoint"`
}

// agentHeartbeat handles POST /v1/agents/:agent_id/heartbeat (Agent-Signature
// auth). It updates the platform last_seen_at and the agent's connection info.
func (s *Server) agentHeartbeat(c *gin.Context) {
	agentID := c.GetString(ctxAgentID)
	var req agentHeartbeatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.AgentHeartbeat(c, agentID, req.TunnelEndpoint, req.MCPEndpoint); errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "agent not found")
		return
	} else if err != nil {
		abort(c, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── POST /v1/agents/:agent_id/claim ──────────────────────────────────────────

type claimAgentReq struct {
	// Owner's Ed25519 public key (hex).
	OwnerPublicKey string `json:"owner_public_key"`
	// DisplayName is the owner's chosen name, used to create/look up the owner record.
	DisplayName string `json:"display_name"`
	// SignatureOverAgentID is Ed25519(owner_private_key).sign(agent_id bytes).
	SignatureOverAgentID string `json:"signature_over_agent_id"`
}

// claimAgent handles POST /v1/agents/:agent_id/claim (spec §3.2). The caller
// proves ownership by signing the agent_id with their Ed25519 private key.
func (s *Server) claimAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	var req claimAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.OwnerPublicKey == "" || req.SignatureOverAgentID == "" {
		abort(c, http.StatusBadRequest, "owner_public_key and signature_over_agent_id are required")
		return
	}
	if err := auth.ValidatePublicKey(req.OwnerPublicKey); err != nil {
		abort(c, http.StatusBadRequest, "owner_public_key must be a 32-byte Ed25519 key in hex")
		return
	}
	if err := auth.VerifyEd25519(req.OwnerPublicKey, []byte(agentID), req.SignatureOverAgentID); err != nil {
		abort(c, http.StatusUnauthorized, "signature verification failed")
		return
	}

	err := s.store.ClaimAgent(c, agentID, req.OwnerPublicKey, orDefault(req.DisplayName, "owner"))
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil && err.Error() == "agent already claimed" {
		abort(c, http.StatusConflict, "agent already claimed")
		return
	}
	if err != nil {
		abort(c, http.StatusInternalServerError, "claim failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "agent_id": agentID})
}

// ── helpers ───────────────────────────────────────────────────────────────────

type byteReader struct{ b []byte; pos int }

func (r *byteReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n = copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

func bytesReader(b []byte) io.Reader { return &byteReader{b: b} }
