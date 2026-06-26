package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/get-robotunnel/roboar/internal/auth"
	"github.com/get-robotunnel/roboar/internal/ids"
	"github.com/get-robotunnel/roboar/internal/store"
	"github.com/gin-gonic/gin"
)

// ── JSON-RPC 2.0 types ───────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ── MCP tool definitions ─────────────────────────────────────────────────────

type mcpTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

var mcpToolList = []mcpTool{
	{
		Name:        "search_robot_agents",
		Description: "Search public robot service agents in the registry by text query, status, or capability.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":         map[string]any{"type": "string", "description": "Full-text search on agent name and description"},
				"online_only":   map[string]any{"type": "boolean", "description": "Return only currently-online agents"},
				"platform_type": map[string]any{"type": "string", "description": "Filter by platform type (e.g. raspberry_pi, ros2_robot)"},
				"capability":    map[string]any{"type": "string", "description": "Filter by capability name"},
				"limit":         map[string]any{"type": "integer", "description": "Max results (default 20, max 100)"},
			},
		},
	},
	{
		Name:        "get_agent_details",
		Description: "Fetch full details of a robot agent by its agent_id, including capabilities and connection info.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"agent_id"},
			"properties": map[string]any{
				"agent_id": map[string]any{"type": "string", "description": "The agt_… ID of the agent"},
			},
		},
	},
	{
		Name:        "get_connection_info",
		Description: "Get the tunnel connection info for an agent (tunnel_endpoint, mcp_endpoint, supported channels). Pass the tunnel_endpoint to a tunnel daemon to open a connection.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"agent_id"},
			"properties": map[string]any{
				"agent_id": map[string]any{"type": "string", "description": "The agt_… ID of the agent"},
			},
		},
	},
	{
		Name:        "check_capability_price",
		Description: "Retrieve the x402 pricing terms for a specific agent capability.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"capability_id"},
			"properties": map[string]any{
				"capability_id": map[string]any{"type": "string", "description": "The cap_… ID of the capability"},
			},
		},
	},
	{
		Name:        "register_identity",
		Description: "Register a principal identity in the registry (for AI agents, CLI users, or mobile apps). Returns an agent_id that can be authorized to use capabilities.",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"public_key", "display_name"},
			"properties": map[string]any{
				"public_key":   map[string]any{"type": "string", "description": "Ed25519 public key in hex (32 bytes)"},
				"display_name": map[string]any{"type": "string", "description": "Human-readable name for this identity"},
				"kind": map[string]any{
					"type":        "string",
					"enum":        []string{"principal", "service"},
					"default":     "principal",
					"description": "principal = consumer identity; service = robot/service-provider",
				},
			},
		},
	},
}

// ── MCP HTTP handler ─────────────────────────────────────────────────────────

// handleMCP serves the MCP server endpoint at POST /mcp.
// It implements the MCP 2024-11-05 protocol over streamable HTTP.
func (s *Server) handleMCP(c *gin.Context) {
	var req rpcRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rpcResponse{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: -32700, Message: "parse error"},
		})
		return
	}

	// Notifications have no id and require no response.
	if req.ID == nil && req.Method != "" {
		c.Status(http.StatusAccepted)
		return
	}

	var result any
	var rpcErr *rpcError

	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "roboar", "version": "1.0"},
		}
	case "tools/list":
		result = map[string]any{"tools": mcpToolList}
	case "tools/call":
		result, rpcErr = s.dispatchMCPTool(c, req.Params)
	default:
		rpcErr = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}

	c.JSON(http.StatusOK, rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	})
}

func (s *Server) dispatchMCPTool(c *gin.Context, raw json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid params"}
	}
	args := p.Arguments
	if args == nil {
		args = map[string]any{}
	}

	switch p.Name {
	case "search_robot_agents":
		return s.mcpSearchAgents(c, args)
	case "get_agent_details":
		return s.mcpGetAgentDetails(c, args)
	case "get_connection_info":
		return s.mcpGetConnectionInfo(c, args)
	case "check_capability_price":
		return s.mcpCheckCapabilityPrice(c, args)
	case "register_identity":
		return s.mcpRegisterIdentity(c, args)
	default:
		return nil, &rpcError{Code: -32601, Message: "unknown tool: " + p.Name}
	}
}

// mcpText wraps a string as an MCP text content block (success result).
func mcpText(s string) any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": s}},
		"isError": false,
	}
}

func mcpJSON(v any) any {
	b, _ := json.Marshal(v)
	return mcpText(string(b))
}

func mcpErr(msg string) any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": msg}},
		"isError": true,
	}
}

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func boolArg(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func intArg(args map[string]any, key string, def int) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return def
}

// ── Tool implementations ─────────────────────────────────────────────────────

func (s *Server) mcpSearchAgents(c *gin.Context, args map[string]any) (any, *rpcError) {
	f := store.DiscoverFilter{
		Q:            stringArg(args, "query"),
		PlatformType: stringArg(args, "platform_type"),
		Capability:   stringArg(args, "capability"),
		OnlineOnly:   boolArg(args, "online_only"),
		Limit:        intArg(args, "limit", 20),
	}
	agents, total, err := s.store.DiscoverAgents(c, f, s.cfg.OfflineAfterSecs)
	if err != nil {
		return mcpErr("search failed: " + err.Error()), nil
	}
	return mcpJSON(map[string]any{"agents": agents, "total": total}), nil
}

func (s *Server) mcpGetAgentDetails(c *gin.Context, args map[string]any) (any, *rpcError) {
	agentID := stringArg(args, "agent_id")
	if agentID == "" {
		return mcpErr("agent_id is required"), nil
	}
	agent, err := s.store.GetDiscoverAgent(c, agentID, s.cfg.OfflineAfterSecs)
	if err != nil {
		return mcpErr(fmt.Sprintf("agent %s not found", agentID)), nil
	}
	return mcpJSON(agent), nil
}

func (s *Server) mcpGetConnectionInfo(c *gin.Context, args map[string]any) (any, *rpcError) {
	agentID := stringArg(args, "agent_id")
	if agentID == "" {
		return mcpErr("agent_id is required"), nil
	}
	agent, err := s.store.GetDiscoverAgent(c, agentID, s.cfg.OfflineAfterSecs)
	if err != nil {
		return mcpErr(fmt.Sprintf("agent %s not found", agentID)), nil
	}
	return mcpJSON(map[string]any{
		"agent_id":   agent.AgentID,
		"connection": agent.Connection,
		"online":     agent.Online,
	}), nil
}

func (s *Server) mcpCheckCapabilityPrice(c *gin.Context, args map[string]any) (any, *rpcError) {
	capID := stringArg(args, "capability_id")
	if capID == "" {
		return mcpErr("capability_id is required"), nil
	}
	cap, err := s.store.GetCapability(c, capID)
	if err != nil {
		return mcpErr(fmt.Sprintf("capability %s not found", capID)), nil
	}
	return mcpJSON(map[string]any{
		"capability_id": cap.CapabilityID,
		"name":          cap.Name,
		"permission":    cap.Permission,
		"pricing":       cap.Pricing,
	}), nil
}

func (s *Server) mcpRegisterIdentity(c *gin.Context, args map[string]any) (any, *rpcError) {
	publicKey := stringArg(args, "public_key")
	displayName := stringArg(args, "display_name")
	kind := stringArg(args, "kind")
	if kind == "" {
		kind = "principal"
	}
	if publicKey == "" || displayName == "" {
		return mcpErr("public_key and display_name are required"), nil
	}
	if err := auth.ValidatePublicKey(publicKey); err != nil {
		return mcpErr("public_key must be a 32-byte Ed25519 key in hex"), nil
	}
	if kind != "principal" && kind != "service" {
		return mcpErr("kind must be 'principal' or 'service'"), nil
	}

	token := ids.PlatformToken()
	hash, err := auth.HashPlatformToken(token)
	if err != nil {
		return mcpErr("internal error"), nil
	}
	result, err := s.store.QuickIdentity(c, publicKey, displayName, kind, hash)
	if err != nil {
		return mcpErr("could not create identity: " + err.Error()), nil
	}
	return mcpJSON(map[string]any{
		"agent_id":    result.AgentID,
		"owner_id":    result.OwnerID,
		"platform_id": result.PlatformID,
	}), nil
}
