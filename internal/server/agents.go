package server

import (
	"encoding/json"
	"net/http"

	"github.com/RussellTNY/robot-agent-registry/internal/ids"
	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/gin-gonic/gin"
)

type createAgentReq struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	AgentType   string           `json:"agent_type"`
	Version     string           `json:"version"`
	Visibility  string           `json:"visibility"`
	Connection  model.Connection `json:"connection"`
	Metadata    json.RawMessage  `json:"metadata"`
}

func (s *Server) createAgent(c *gin.Context) {
	platformID := c.GetString(ctxPlatformID)
	ownerID := c.GetString(ctxOwnerID)
	var req createAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" || req.AgentType == "" || req.Version == "" {
		abort(c, http.StatusBadRequest, "name, agent_type and version are required")
		return
	}
	a := &model.Agent{
		AgentID:     ids.Agent(),
		PlatformID:  platformID,
		OwnerID:     ownerID,
		Name:        req.Name,
		Description: req.Description,
		AgentType:   req.AgentType,
		Version:     req.Version,
		Visibility:  orDefault(req.Visibility, "public"),
		Connection:  req.Connection,
		Metadata:    req.Metadata,
	}
	if err := s.store.UpsertAgent(c, a); err != nil {
		abort(c, http.StatusInternalServerError, "could not register agent")
		return
	}
	full, err := s.store.GetAgent(c, a.AgentID, s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not load agent")
		return
	}
	c.JSON(http.StatusCreated, full)
}

func (s *Server) listAgents(c *gin.Context) {
	platformID := c.GetString(ctxPlatformID)
	agents, err := s.store.ListAgentsByPlatform(c, platformID, s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusInternalServerError, "could not list agents")
		return
	}
	if agents == nil {
		agents = []model.Agent{}
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (s *Server) getAgent(c *gin.Context) {
	a, err := s.store.GetAgent(c, c.Param("agent_id"), s.cfg.OfflineAfterSecs)
	if err != nil {
		abort(c, http.StatusNotFound, "agent not found")
		return
	}
	c.JSON(http.StatusOK, a)
}

type patchAgentReq struct {
	Description *string `json:"description"`
	Version     *string `json:"version"`
	Visibility  *string `json:"visibility"`
}

func (s *Server) patchAgent(c *gin.Context) {
	var req patchAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	agentID := c.Param("agent_id")
	if err := s.store.UpdateAgent(c, agentID, req.Description, req.Version, req.Visibility); err != nil {
		abort(c, http.StatusInternalServerError, "could not update agent")
		return
	}
	a, _ := s.store.GetAgent(c, agentID, s.cfg.OfflineAfterSecs)
	c.JSON(http.StatusOK, a)
}

func (s *Server) deleteAgent(c *gin.Context) {
	if err := s.store.DeleteAgent(c, c.Param("agent_id")); err != nil {
		abort(c, http.StatusInternalServerError, "could not delete agent")
		return
	}
	c.Status(http.StatusNoContent)
}

type createCapabilityReq struct {
	Name             string          `json:"name"`
	DisplayName      string          `json:"display_name"`
	Description      string          `json:"description"`
	InterfaceType    string          `json:"interface_type"`
	Permission       string          `json:"permission"`
	Pricing          json.RawMessage `json:"pricing"`
	AuthorizedAgents []string        `json:"authorized_agents"`
	InputSchema      json.RawMessage `json:"input_schema"`
	OutputSchema     json.RawMessage `json:"output_schema"`
	ROS2             json.RawMessage `json:"ros2"`
	Enabled          *bool           `json:"enabled"`
}

func (s *Server) createCapability(c *gin.Context) {
	agentID := c.Param("agent_id")
	var req createCapabilityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" || req.DisplayName == "" || req.InterfaceType == "" {
		abort(c, http.StatusBadRequest, "name, display_name and interface_type are required")
		return
	}
	cap := &model.Capability{
		CapabilityID:     ids.Capability(),
		AgentID:          agentID,
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      req.Description,
		InterfaceType:    req.InterfaceType,
		Permission:       orDefault(req.Permission, "public"),
		Pricing:          req.Pricing,
		AuthorizedAgents: orEmpty(req.AuthorizedAgents),
		InputSchema:      req.InputSchema,
		OutputSchema:     req.OutputSchema,
		ROS2:             req.ROS2,
		Enabled:          req.Enabled == nil || *req.Enabled,
	}
	if err := s.store.UpsertCapability(c, cap); err != nil {
		abort(c, http.StatusInternalServerError, "could not register capability")
		return
	}
	c.JSON(http.StatusCreated, cap)
}

type patchCapabilityReq struct {
	Permission *string `json:"permission"`
	Enabled    *bool   `json:"enabled"`
}

func (s *Server) patchCapability(c *gin.Context) {
	var req patchCapabilityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		abort(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.UpdateCapability(c, c.Param("capability_id"), req.Permission, req.Enabled); err != nil {
		abort(c, http.StatusInternalServerError, "could not update capability")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) deleteCapability(c *gin.Context) {
	if err := s.store.DeleteCapability(c, c.Param("capability_id")); err != nil {
		abort(c, http.StatusInternalServerError, "could not delete capability")
		return
	}
	c.Status(http.StatusNoContent)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
