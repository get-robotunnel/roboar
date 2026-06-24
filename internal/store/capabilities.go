package store

import (
	"context"
	"encoding/json"

	"github.com/RussellTNY/robot-agent-registry/internal/model"
)

// UpsertCapability registers or idempotently updates a capability, keyed by
// (agent_id, name).
func (s *Store) UpsertCapability(ctx context.Context, c *model.Capability) error {
	return s.Pool.QueryRow(ctx,
		`INSERT INTO capabilities (capability_id, agent_id, name, display_name, description, interface_type,
			permission, pricing, authorized_agents, input_schema, output_schema, ros2_metadata, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::jsonb,$11::jsonb,$12::jsonb,$13)
		 ON CONFLICT (agent_id, name) DO UPDATE SET
			display_name      = EXCLUDED.display_name,
			description       = EXCLUDED.description,
			interface_type    = EXCLUDED.interface_type,
			permission        = EXCLUDED.permission,
			pricing           = EXCLUDED.pricing,
			authorized_agents = EXCLUDED.authorized_agents,
			input_schema      = EXCLUDED.input_schema,
			output_schema     = EXCLUDED.output_schema,
			ros2_metadata     = EXCLUDED.ros2_metadata,
			enabled           = EXCLUDED.enabled
		 RETURNING capability_id, created_at`,
		c.CapabilityID, c.AgentID, c.Name, c.DisplayName, c.Description, c.InterfaceType,
		c.Permission, jsonbArg(c.Pricing), c.AuthorizedAgents, jsonbObj(c.InputSchema),
		jsonbObj(c.OutputSchema), jsonbArg(c.ROS2), c.Enabled,
	).Scan(&c.CapabilityID, &c.CreatedAt)
}

func (s *Store) UpdateCapability(ctx context.Context, capabilityID string, permission *string, enabled *bool) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE capabilities SET
			permission = COALESCE($2, permission),
			enabled    = COALESCE($3, enabled)
		 WHERE capability_id=$1`,
		capabilityID, permission, enabled,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteCapability(ctx context.Context, capabilityID string) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM capabilities WHERE capability_id=$1`, capabilityID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// capabilitiesByAgent returns capabilities grouped by agent_id for a set of
// agents, in a single query to avoid N+1 loads.
func (s *Store) capabilitiesByAgent(ctx context.Context, agentIDs []string) (map[string][]model.Capability, error) {
	out := make(map[string][]model.Capability)
	if len(agentIDs) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT capability_id, agent_id, name, display_name, COALESCE(description,''), interface_type,
			permission, pricing::text, authorized_agents, input_schema::text, output_schema::text,
			ros2_metadata::text, enabled, created_at
		 FROM capabilities WHERE agent_id = ANY($1) ORDER BY created_at`,
		agentIDs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c model.Capability
		var pricing, input, output, ros2 *string
		if err := rows.Scan(&c.CapabilityID, &c.AgentID, &c.Name, &c.DisplayName, &c.Description,
			&c.InterfaceType, &c.Permission, &pricing, &c.AuthorizedAgents, &input, &output, &ros2,
			&c.Enabled, &c.CreatedAt); err != nil {
			return nil, err
		}
		if pricing != nil {
			c.Pricing = json.RawMessage(*pricing)
		}
		if input != nil {
			c.InputSchema = json.RawMessage(*input)
		}
		if output != nil {
			c.OutputSchema = json.RawMessage(*output)
		}
		if ros2 != nil {
			c.ROS2 = json.RawMessage(*ros2)
		}
		out[c.AgentID] = append(out[c.AgentID], c)
	}
	return out, rows.Err()
}
