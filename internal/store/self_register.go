package store

import (
	"context"
	"errors"

	"github.com/RussellTNY/robot-agent-registry/internal/ids"
	"github.com/RussellTNY/robot-agent-registry/internal/model"
	"github.com/jackc/pgx/v5"
)

// SelfRegisterRequest is the payload for SelfRegister.
type SelfRegisterRequest struct {
	PublicKey    string
	Name         string
	Description  string
	AgentType    string
	Version      string
	Capabilities []model.Capability
}

// SelfRegisterResult is returned by SelfRegister.
type SelfRegisterResult struct {
	AgentID       string
	WalletAddress string
	ClaimStatus   string
	OwnerID       string
	IsNew         bool
}

// SelfRegister idempotently creates or retrieves an agent by its Ed25519 public
// key (spec §1.2 steps 1-4). On first call it creates an auto-owner (keyed by
// the agent's own public key), an "auto" platform, and the agent record. On
// subsequent calls it returns the existing agent without modification.
//
// walletAddress must be pre-computed by the caller (auth.DeriveWalletAddress).
func (s *Store) SelfRegister(ctx context.Context, req SelfRegisterRequest, walletAddress string) (*SelfRegisterResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Fast-path: agent already registered under this public key.
	var existing struct {
		agentID       string
		walletAddress string
		claimStatus   string
		ownerID       string
	}
	err = tx.QueryRow(ctx,
		`SELECT agent_id, wallet_address, claim_status, owner_id FROM agents WHERE public_key=$1`,
		req.PublicKey,
	).Scan(&existing.agentID, &existing.walletAddress, &existing.claimStatus, &existing.ownerID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return &SelfRegisterResult{
			AgentID:       existing.agentID,
			WalletAddress: existing.walletAddress,
			ClaimStatus:   existing.claimStatus,
			OwnerID:       existing.ownerID,
			IsNew:         false,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// First registration: create auto-owner keyed by the agent's public key.
	ownerID := ids.Owner()
	var actualOwnerID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO owners (owner_id, public_key, display_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (public_key) DO UPDATE SET display_name = EXCLUDED.display_name
		 RETURNING owner_id`,
		ownerID, req.PublicKey, req.Name,
	).Scan(&actualOwnerID); err != nil {
		return nil, err
	}

	// Create an "auto" platform for this agent (one per agent, no token needed).
	platformID := ids.Platform()
	tokenHash := "auto:" + req.PublicKey // never used for auth; Agent-Signature replaces it
	if _, err := tx.Exec(ctx,
		`INSERT INTO platforms (platform_id, owner_id, platform_type, display_name,
		     platform_token_hash, public_key, tags)
		 VALUES ($1, $2, 'auto', $3, $4, $5, '{}')`,
		platformID, actualOwnerID, req.Name, tokenHash, req.PublicKey,
	); err != nil {
		return nil, err
	}

	// Create the agent record.
	agentID := ids.Agent()
	if _, err := tx.Exec(ctx,
		`INSERT INTO agents (agent_id, platform_id, owner_id, name, description,
		     agent_type, version, visibility, identity_kind,
		     public_key, wallet_address, claim_status, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'public','service',$8,$9,'unclaimed','{}')`,
		agentID, platformID, actualOwnerID,
		req.Name, req.Description, req.AgentType, req.Version,
		req.PublicKey, walletAddress,
	); err != nil {
		return nil, err
	}

	// Upsert capabilities declared at startup.
	for i := range req.Capabilities {
		cap := &req.Capabilities[i]
		cap.CapabilityID = ids.Capability()
		cap.AgentID = agentID
		if _, err := tx.Exec(ctx,
			`INSERT INTO capabilities (capability_id, agent_id, name, display_name, description,
			     interface_type, permission, pricing, authorized_agents,
			     input_schema, output_schema, ros2_metadata, enabled)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,'{}','{}','{}',NULL,true)
			 ON CONFLICT (agent_id, name) DO NOTHING`,
			cap.CapabilityID, agentID, cap.Name, cap.DisplayName, cap.Description,
			cap.InterfaceType, cap.Permission, jsonbArg(cap.Pricing),
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &SelfRegisterResult{
		AgentID:       agentID,
		WalletAddress: walletAddress,
		ClaimStatus:   "unclaimed",
		OwnerID:       actualOwnerID,
		IsNew:         true,
	}, nil
}

// AgentHeartbeat updates the heartbeat timestamp on the agent's auto-platform
// and refreshes the mcp_endpoint and tunnel_endpoint on the agent record.
func (s *Store) AgentHeartbeat(ctx context.Context, agentID, tunnelEndpoint, mcpEndpoint string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var platformID string
	if err := tx.QueryRow(ctx,
		`SELECT platform_id FROM agents WHERE agent_id=$1`, agentID,
	).Scan(&platformID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE platforms SET last_seen_at=NOW(), online=TRUE WHERE platform_id=$1`,
		platformID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE agents SET
		     tunnel_endpoint = NULLIF($2,''),
		     mcp_endpoint    = NULLIF($3,''),
		     updated_at      = NOW()
		 WHERE agent_id=$1`,
		agentID, tunnelEndpoint, mcpEndpoint,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetAgentPublicKey returns the Ed25519 public key for an agent. Used by
// Agent-Signature middleware to retrieve the stored key for verification.
func (s *Store) GetAgentPublicKey(ctx context.Context, agentID string) (string, error) {
	var pk string
	err := s.Pool.QueryRow(ctx,
		`SELECT COALESCE(public_key,'') FROM agents WHERE agent_id=$1`, agentID,
	).Scan(&pk)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return pk, err
}

// ClaimAgent binds an unclaimed agent to a human owner (spec §3.2).
// The caller must have already verified the owner's Ed25519 signature over the
// agent_id before calling this.
func (s *Store) ClaimAgent(ctx context.Context, agentID, ownerPublicKey, ownerDisplayName string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Verify agent exists and is unclaimed.
	var claimStatus, platformID string
	if err := tx.QueryRow(ctx,
		`SELECT claim_status, platform_id FROM agents WHERE agent_id=$1`, agentID,
	).Scan(&claimStatus, &platformID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if claimStatus == "claimed" {
		return errors.New("agent already claimed")
	}

	// Upsert the real owner by their public key.
	ownerID := ids.Owner()
	var actualOwnerID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO owners (owner_id, public_key, display_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (public_key) DO UPDATE SET display_name = EXCLUDED.display_name
		 RETURNING owner_id`,
		ownerID, ownerPublicKey, ownerDisplayName,
	).Scan(&actualOwnerID); err != nil {
		return err
	}

	// Bind agent and its auto-platform to the real owner.
	if _, err := tx.Exec(ctx,
		`UPDATE agents SET owner_id=$2, claim_status='claimed', updated_at=NOW()
		 WHERE agent_id=$1`, agentID, actualOwnerID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE platforms SET owner_id=$2 WHERE platform_id=$1`,
		platformID, actualOwnerID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
