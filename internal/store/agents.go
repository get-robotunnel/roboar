package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/jackc/pgx/v5"
)

// UpsertAgent registers or idempotently updates an agent, keyed by
// (platform_id, name). On conflict the existing agent_id is returned and the
// runtime status column is left untouched.
func (s *Store) UpsertAgent(ctx context.Context, a *model.Agent) error {
	supports := a.Connection.Supports
	if supports == nil {
		supports = []string{}
	}
	return s.Pool.QueryRow(ctx,
		`INSERT INTO agents (agent_id, platform_id, owner_id, name, description, agent_type, version,
			status, visibility, identity_kind, tunnel_endpoint, mcp_endpoint, rest_endpoint,
			tunnel_supports, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'offline',$8,$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),
		         $13,$14::jsonb)
		 ON CONFLICT (platform_id, name) DO UPDATE SET
			description     = EXCLUDED.description,
			agent_type      = EXCLUDED.agent_type,
			version         = EXCLUDED.version,
			visibility      = EXCLUDED.visibility,
			tunnel_endpoint = EXCLUDED.tunnel_endpoint,
			mcp_endpoint    = EXCLUDED.mcp_endpoint,
			rest_endpoint   = EXCLUDED.rest_endpoint,
			tunnel_supports = EXCLUDED.tunnel_supports,
			metadata        = EXCLUDED.metadata,
			updated_at      = NOW()
		 RETURNING agent_id, status, created_at, updated_at`,
		a.AgentID, a.PlatformID, a.OwnerID, a.Name, a.Description, a.AgentType, a.Version,
		a.Visibility, orDefaultStr(a.IdentityKind, "service"),
		a.Connection.TunnelEndpoint, a.Connection.MCPEndpoint, a.Connection.RestEndpoint,
		supports, jsonbObj(a.Metadata),
	).Scan(&a.AgentID, &a.Status, &a.CreatedAt, &a.UpdatedAt)
}

func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// UpdateAgentTunnel sets the tunnel_endpoint and tunnel_supports for a single
// agent, keyed by (platform_id, agent_id) so the platform token can't update
// agents it doesn't own.
func (s *Store) UpdateAgentTunnel(ctx context.Context, platformID, agentID, tunnelEndpoint string, supports []string) error {
	if supports == nil {
		supports = []string{}
	}
	ct, err := s.Pool.Exec(ctx,
		`UPDATE agents SET
			tunnel_endpoint = NULLIF($3, ''),
			tunnel_supports = $4,
			updated_at      = NOW()
		 WHERE agent_id=$2 AND platform_id=$1`,
		platformID, agentID, tunnelEndpoint, supports,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetAgentPlatformID resolves an agent's owning platform, used by auth on
// agent/capability endpoints that are keyed only by agent_id.
func (s *Store) GetAgentPlatformID(ctx context.Context, agentID string) (string, error) {
	var pid string
	err := s.Pool.QueryRow(ctx, `SELECT platform_id FROM agents WHERE agent_id=$1`, agentID).Scan(&pid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return pid, err
}

// agentBaseSelect projects model.Agent fields with an effective status derived
// from heartbeat recency. $1 is the offline-after seconds.
const agentBaseSelect = `SELECT a.agent_id, a.platform_id, a.owner_id, a.name, COALESCE(a.description,''),
	a.agent_type, a.version,
	CASE WHEN a.status='error' THEN 'error'
	     WHEN (p.last_seen_at IS NOT NULL AND p.last_seen_at > NOW() - make_interval(secs => $1)) THEN 'online'
	     ELSE 'offline' END AS status,
	a.visibility, a.identity_kind,
	COALESCE(a.tunnel_endpoint,''), COALESCE(a.mcp_endpoint,''), COALESCE(a.rest_endpoint,''),
	a.tunnel_supports, a.metadata::text, a.created_at, a.updated_at
	FROM agents a JOIN platforms p ON p.platform_id = a.platform_id`

func (s *Store) loadAgents(ctx context.Context, offlineSecs int, where string, args ...interface{}) ([]model.Agent, error) {
	full := append([]interface{}{offlineSecs}, args...)
	rows, err := s.Pool.Query(ctx, agentBaseSelect+" "+where, full...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []model.Agent
	var ids []string
	for rows.Next() {
		var a model.Agent
		var meta string
		if err := rows.Scan(&a.AgentID, &a.PlatformID, &a.OwnerID, &a.Name, &a.Description,
			&a.AgentType, &a.Version, &a.Status, &a.Visibility, &a.IdentityKind,
			&a.Connection.TunnelEndpoint, &a.Connection.MCPEndpoint, &a.Connection.RestEndpoint,
			&a.Connection.Supports, &meta, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Metadata = json.RawMessage(meta)
		a.Capabilities = []model.Capability{}
		agents = append(agents, a)
		ids = append(ids, a.AgentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	caps, err := s.capabilitiesByAgent(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range agents {
		if c := caps[agents[i].AgentID]; c != nil {
			agents[i].Capabilities = c
		}
	}
	return agents, nil
}

func (s *Store) GetAgent(ctx context.Context, agentID string, offlineSecs int) (*model.Agent, error) {
	agents, err := s.loadAgents(ctx, offlineSecs, `WHERE a.agent_id=$2`, agentID)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return nil, ErrNotFound
	}
	return &agents[0], nil
}

func (s *Store) ListAgentsByPlatform(ctx context.Context, platformID string, offlineSecs int) ([]model.Agent, error) {
	return s.loadAgents(ctx, offlineSecs, `WHERE a.platform_id=$2 ORDER BY a.created_at`, platformID)
}

// UpdateAgent patches mutable agent fields. Nil pointers are left unchanged.
func (s *Store) UpdateAgent(ctx context.Context, agentID string, description, version, visibility *string) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE agents SET
			description = COALESCE($2, description),
			version     = COALESCE($3, version),
			visibility  = COALESCE($4, visibility),
			updated_at  = NOW()
		 WHERE agent_id=$1`,
		agentID, description, version, visibility,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteAgent(ctx context.Context, agentID string) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM agents WHERE agent_id=$1`, agentID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
