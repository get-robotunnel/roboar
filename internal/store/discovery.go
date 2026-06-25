package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/RussellTNY/robot-agent-registry/internal/model"
)

// DiscoverFilter holds the optional query parameters of GET /discover/agents.
type DiscoverFilter struct {
	Q            string
	PlatformType string
	Capability   string
	Permission   string
	Tags         []string
	OwnerID      string
	OnlineOnly   bool
	Limit        int
	Offset       int
}

// DiscoveredAgent is the flattened public view returned by discovery (spec §3.4).
type DiscoveredAgent struct {
	AgentID          string             `json:"agent_id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	PlatformType     string             `json:"platform_type"`
	OwnerDisplayName string             `json:"owner_display_name"`
	Online           bool               `json:"online"`
	Visibility       string             `json:"visibility"`
	Tags             []string           `json:"tags"`
	Capabilities     []model.Capability `json:"capabilities"`
	Connection       model.Connection   `json:"connection"`
}

const discoverSelect = `SELECT a.agent_id, a.name, COALESCE(a.description,''), p.platform_type, o.display_name,
	(p.last_seen_at IS NOT NULL AND p.last_seen_at > NOW() - make_interval(secs => $1)) AS online,
	a.visibility, p.tags,
	COALESCE(a.tunnel_endpoint,''), COALESCE(a.mcp_endpoint,''), COALESCE(a.rest_endpoint,''),
	a.tunnel_supports
	FROM agents a
	JOIN platforms p ON p.platform_id = a.platform_id
	JOIN owners o ON o.owner_id = a.owner_id`

const discoverCountFrom = `FROM agents a
	JOIN platforms p ON p.platform_id = a.platform_id
	JOIN owners o ON o.owner_id = a.owner_id`

// DiscoverAgents runs a filtered, paginated search over public agents and
// returns the page plus the total match count.
func (s *Store) DiscoverAgents(ctx context.Context, f DiscoverFilter, offlineSecs int) ([]DiscoveredAgent, int, error) {
	args := []interface{}{offlineSecs}
	add := func(v interface{}) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	onlineExpr := "(p.last_seen_at IS NOT NULL AND p.last_seen_at > NOW() - make_interval(secs => $1))"

	conds := []string{"a.visibility='public'", "a.identity_kind='service'"}
	if f.Q != "" {
		p := add("%" + f.Q + "%")
		conds = append(conds, fmt.Sprintf("(a.name ILIKE %s OR a.description ILIKE %s)", p, p))
	}
	if f.PlatformType != "" {
		conds = append(conds, "p.platform_type="+add(f.PlatformType))
	}
	if f.Capability != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM capabilities c WHERE c.agent_id=a.agent_id AND c.name="+add(f.Capability)+")")
	}
	if f.Permission != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM capabilities c WHERE c.agent_id=a.agent_id AND c.permission="+add(f.Permission)+")")
	}
	if len(f.Tags) > 0 {
		conds = append(conds, "p.tags && "+add(f.Tags))
	}
	if f.OwnerID != "" {
		conds = append(conds, "a.owner_id="+add(f.OwnerID))
	}
	if f.OnlineOnly {
		conds = append(conds, onlineExpr)
	}
	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.Pool.QueryRow(ctx, "SELECT COUNT(*) "+discoverCountFrom+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	query := discoverSelect + " " + where + " ORDER BY a.created_at DESC LIMIT " + add(limit) + " OFFSET " + add(offset)

	agents, err := s.scanDiscovered(ctx, query, args)
	if err != nil {
		return nil, 0, err
	}
	return agents, total, nil
}

// GetDiscoverAgent returns a single agent's public view. Visible for public and
// unlisted agents (unlisted is reachable by direct id but excluded from search);
// private agents return ErrNotFound to anonymous callers.
func (s *Store) GetDiscoverAgent(ctx context.Context, agentID string, offlineSecs int) (*DiscoveredAgent, error) {
	args := []interface{}{offlineSecs, agentID}
	query := discoverSelect + ` WHERE a.agent_id=$2 AND a.visibility IN ('public','unlisted')`
	agents, err := s.scanDiscovered(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, ErrNotFound
	}
	return &agents[0], nil
}

func (s *Store) scanDiscovered(ctx context.Context, query string, args []interface{}) ([]DiscoveredAgent, error) {
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiscoveredAgent
	var ids []string
	for rows.Next() {
		var d DiscoveredAgent
		if err := rows.Scan(&d.AgentID, &d.Name, &d.Description, &d.PlatformType, &d.OwnerDisplayName,
			&d.Online, &d.Visibility, &d.Tags,
			&d.Connection.TunnelEndpoint, &d.Connection.MCPEndpoint, &d.Connection.RestEndpoint,
			&d.Connection.Supports); err != nil {
			return nil, err
		}
		d.Capabilities = []model.Capability{}
		out = append(out, d)
		ids = append(ids, d.AgentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	caps, err := s.capabilitiesByAgent(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if c := caps[out[i].AgentID]; c != nil {
			out[i].Capabilities = c
		}
	}
	return out, nil
}
