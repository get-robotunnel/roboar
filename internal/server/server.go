// Package server wires the registry HTTP API: routing, auth middleware, and the
// request handlers for owners, platforms, agents, capabilities, and discovery.
package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/get-robotunnel/roboar/internal/auth"
	"github.com/get-robotunnel/roboar/internal/config"
	"github.com/get-robotunnel/roboar/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

type Server struct {
	cfg    *config.Config
	store  *store.Store
	auth   *auth.Manager
	engine *gin.Engine
}

func New(cfg *config.Config, st *store.Store, am *auth.Manager) *Server {
	s := &Server{cfg: cfg, store: st, auth: am, engine: gin.New()}
	s.engine.Use(gin.Recovery())
	s.routes()
	return s
}

func (s *Server) Engine() *gin.Engine { return s.engine }

func (s *Server) Run() error { return s.engine.Run(":" + s.cfg.Port) }

func (s *Server) routes() {
	s.engine.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// MCP server — AI agent entry point (no auth; tools enforce their own access rules)
	s.engine.POST("/mcp", s.handleMCP)

	v1 := s.engine.Group("/v1")

	// Owners
	v1.POST("/owners", s.createOwner)
	v1.POST("/owners/auth/challenge", s.authChallenge)
	v1.POST("/owners/auth/verify", s.authVerify)
	v1.GET("/owners/me", s.ownerAuth, s.getMe)

	// Platforms (owner JWT)
	v1.POST("/platforms", s.ownerAuth, s.createPlatform)
	v1.GET("/platforms", s.ownerAuth, s.listPlatforms)
	v1.GET("/platforms/:platform_id", s.ownerAuth, s.getPlatform)
	v1.PATCH("/platforms/:platform_id", s.ownerAuth, s.patchPlatform)
	v1.DELETE("/platforms/:platform_id", s.ownerAuth, s.deletePlatform)

	// Heartbeat + agent registration (platform token)
	v1.POST("/platforms/:platform_id/heartbeat", s.platformAuthByParam, s.heartbeat)
	v1.POST("/platforms/:platform_id/agents", s.platformAuthByParam, s.createAgent)
	v1.GET("/platforms/:platform_id/agents", s.platformAuthByParam, s.listAgents)

	// Agent + capability management (platform token, resolved via agent_id)
	v1.GET("/agents/:agent_id", s.platformAuthByAgent, s.getAgent)
	v1.PATCH("/agents/:agent_id", s.platformAuthByAgent, s.patchAgent)
	v1.DELETE("/agents/:agent_id", s.platformAuthByAgent, s.deleteAgent)
	v1.POST("/agents/:agent_id/capabilities", s.platformAuthByAgent, s.createCapability)
	v1.PATCH("/agents/:agent_id/capabilities/:capability_id", s.platformAuthByAgent, s.patchCapability)
	v1.DELETE("/agents/:agent_id/capabilities/:capability_id", s.platformAuthByAgent, s.deleteCapability)

	// Agent self-registration (spec §1): no platform token needed — agent's Ed25519
	// key is its identity. Heartbeat uses Agent-Signature; claim uses owner Ed25519.
	v1.POST("/agents", s.selfRegister)
	v1.POST("/agents/:agent_id/heartbeat", s.agentSigAuth, s.agentHeartbeat)
	v1.POST("/agents/:agent_id/claim", s.claimAgent)

	// Identity register (no auth — public key is the identity credential)
	v1.POST("/identities", s.quickIdentity)

	// Discovery (public, no auth)
	v1.GET("/discover/agents", s.discoverAgents)
	v1.GET("/discover/agents/:agent_id", s.discoverAgent)
}

// --- middleware ---

const (
	ctxOwnerID    = "owner_id"
	ctxPlatformID = "platform_id"
	ctxAgentID    = "agent_id"
)

func (s *Server) ownerAuth(c *gin.Context) {
	token, ok := bearerToken(c, "Bearer")
	if !ok {
		abort(c, http.StatusUnauthorized, "missing bearer token")
		return
	}
	ownerID, err := s.auth.ParseJWT(token)
	if err != nil {
		abort(c, http.StatusUnauthorized, "invalid token")
		return
	}
	c.Set(ctxOwnerID, ownerID)
	c.Next()
}

func (s *Server) platformAuthByParam(c *gin.Context) {
	s.authenticatePlatform(c, c.Param("platform_id"))
}

func (s *Server) platformAuthByAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	platformID, err := s.store.GetAgentPlatformID(c, agentID)
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		abort(c, http.StatusInternalServerError, "lookup failed")
		return
	}
	c.Set(ctxAgentID, agentID)
	s.authenticatePlatform(c, platformID)
}

// authenticatePlatform validates an `Authorization: Platform <token>` header
// against the stored bcrypt hash for platformID, then continues the chain.
func (s *Server) authenticatePlatform(c *gin.Context, platformID string) {
	token, ok := bearerToken(c, "Platform")
	if !ok {
		abort(c, http.StatusUnauthorized, "missing platform token")
		return
	}
	hash, ownerID, err := s.store.GetPlatformTokenHash(c, platformID)
	if errors.Is(err, store.ErrNotFound) {
		abort(c, http.StatusNotFound, "platform not found")
		return
	}
	if err != nil {
		abort(c, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !auth.CheckPlatformToken(hash, token) {
		abort(c, http.StatusUnauthorized, "invalid platform token")
		return
	}
	c.Set(ctxPlatformID, platformID)
	c.Set(ctxOwnerID, ownerID)
	c.Next()
}

// --- helpers ---

func bearerToken(c *gin.Context, scheme string) (string, bool) {
	h := c.GetHeader("Authorization")
	prefix := scheme + " "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
