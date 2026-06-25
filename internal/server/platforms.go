package server

import (
	"errors"
	"net/http"

	"github.com/RussellTNY/robot-agent-registry/internal/auth"
	"github.com/RussellTNY/robot-agent-registry/internal/ids"
	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/RussellTNY/robot-agent-registry/internal/store"
	"github.com/gin-gonic/gin"
)

type createPlatformReq struct {
	PlatformType string   `json:"platform_type"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	PublicKey    string   `json:"public_key"`
}

func (s *Server) createPlatform(c *gin.Context) {
	ownerID := c.GetString(ctxOwnerID)
	var req createPlatformReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.PlatformType == "" || req.DisplayName == "" {
		abort(c, http.StatusBadRequest, "platform_type and display_name are required")
		return
	}

	token := ids.PlatformToken()
	hash, err := auth.HashPlatformToken(token)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not hash token")
		return
	}
	p := &model.Platform{
		PlatformID:   ids.Platform(),
		OwnerID:      ownerID,
		PlatformType: req.PlatformType,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		PublicKey:    req.PublicKey,
		Tags:         orEmpty(req.Tags),
	}
	if err := s.store.CreatePlatform(c, p, hash); err != nil {
		abort(c, http.StatusInternalServerError, "could not create platform")
		return
	}
	// platform_token plaintext is returned exactly once here (spec §9.3).
	c.JSON(http.StatusCreated, gin.H{"platform": p, "platform_token": token})
}

func (s *Server) listPlatforms(c *gin.Context) {
	ownerID := c.GetString(ctxOwnerID)
	platforms, err := s.store.ListPlatformsByOwner(c, ownerID, s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not list platforms")
		return
	}
	c.JSON(http.StatusOK, gin.H{"platforms": orEmptyPlatforms(platforms)})
}

// loadOwnedPlatform fetches a platform and verifies it belongs to the caller,
// returning nil (and writing a 404) otherwise.
func (s *Server) loadOwnedPlatform(c *gin.Context) *model.Platform {
	ownerID := c.GetString(ctxOwnerID)
	p, err := s.store.GetPlatform(c, c.Param("platform_id"), s.cfg.OfflineAfterSecs)
	if errors.Is(err, store.ErrNotFound) || (p != nil && p.OwnerID != ownerID) {
		abort(c, http.StatusNotFound, "platform not found")
		return nil
	}
	if err != nil {
		abort(c, http.StatusInternalServerError, "lookup failed")
		return nil
	}
	return p
}

func (s *Server) getPlatform(c *gin.Context) {
	if p := s.loadOwnedPlatform(c); p != nil {
		c.JSON(http.StatusOK, p)
	}
}

type patchPlatformReq struct {
	DisplayName *string   `json:"display_name"`
	Description *string   `json:"description"`
	Tags        *[]string `json:"tags"`
}

func (s *Server) patchPlatform(c *gin.Context) {
	p := s.loadOwnedPlatform(c)
	if p == nil {
		return
	}
	var req patchPlatformReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.UpdatePlatform(c, p.PlatformID, req.DisplayName, req.Description, req.Tags); err != nil {
		abort(c, http.StatusInternalServerError, "could not update platform")
		return
	}
	updated, _ := s.store.GetPlatform(c, p.PlatformID, s.cfg.OfflineAfterSecs)
	c.JSON(http.StatusOK, updated)
}

func (s *Server) deletePlatform(c *gin.Context) {
	p := s.loadOwnedPlatform(c)
	if p == nil {
		return
	}
	if err := s.store.DeletePlatform(c, p.PlatformID); err != nil {
		abort(c, http.StatusInternalServerError, "could not delete platform")
		return
	}
	c.Status(http.StatusNoContent)
}

type agentTunnelUpdate struct {
	AgentID        string   `json:"agent_id"`
	TunnelEndpoint string   `json:"tunnel_endpoint"`
	TunnelSupports []string `json:"tunnel_supports"`
}

type heartbeatReq struct {
	PublicKey string               `json:"public_key"`
	Status    string               `json:"status"`
	Agents    []agentTunnelUpdate  `json:"agents"`
}

func (s *Server) heartbeat(c *gin.Context) {
	platformID := c.GetString(ctxPlatformID)
	var req heartbeatReq
	// Body is optional; ignore decode errors for an empty heartbeat.
	_ = c.ShouldBindJSON(&req)
	if err := s.store.Heartbeat(c, platformID, req.PublicKey); err != nil {
		abort(c, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	for _, u := range req.Agents {
		if u.AgentID == "" {
			continue
		}
		_ = s.store.UpdateAgentTunnel(c, platformID, u.AgentID, u.TunnelEndpoint, u.TunnelSupports)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyPlatforms(p []model.Platform) []model.Platform {
	if p == nil {
		return []model.Platform{}
	}
	return p
}
