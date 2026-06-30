package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/get-robotunnel/roboar/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) discoverAgents(c *gin.Context) {
	f := store.DiscoverFilter{
		Q:            c.Query("q"),
		PlatformType: c.Query("platform_type"),
		Capability:   c.Query("capability"),
		Permission:   c.Query("permission"),
		OwnerID:      c.Query("owner_id"),
		OnlineOnly:   c.Query("online") == "true",
		Limit:        atoiDefault(c.Query("limit"), 20),
		Offset:       atoiDefault(c.Query("offset"), 0),
	}
	if tags := c.Query("tags"); tags != "" {
		f.Tags = strings.Split(tags, ",")
	}

	agents, total, err := s.store.DiscoverAgents(c, f, s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusInternalServerError, "discovery failed")
		return
	}
	if agents == nil {
		agents = []store.DiscoveredAgent{}
	}
	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// jwks publishes the registry's owner-JWT verification key (Ed25519) so agents
// can verify owner JWTs locally without contacting the registry per request.
func (s *Server) jwks(c *gin.Context) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, gin.H{"keys": []map[string]string{s.auth.PublicJWK()}})
}

func (s *Server) discoverAgent(c *gin.Context) {
	a, err := s.store.GetDiscoverAgent(c, c.Param("agent_id"), s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusNotFound, "agent not found")
		return
	}
	c.JSON(http.StatusOK, a)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
