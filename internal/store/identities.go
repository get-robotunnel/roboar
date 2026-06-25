package store

import (
	"context"

	"github.com/RussellTNY/robot-agent-registry/internal/ids"
)

// QuickIdentityResult is returned by QuickIdentity.
type QuickIdentityResult struct {
	AgentID    string
	OwnerID    string
	PlatformID string
}

// QuickIdentity atomically creates or retrieves the owner identified by
// publicKey, creates a new "personal" platform under that owner, and registers
// an agent of the given identity kind (principal or service).
//
// tokenHash is the bcrypt hash of the platform token the caller generated;
// the plaintext is returned by the caller to the HTTP client.
func (s *Store) QuickIdentity(ctx context.Context, publicKey, displayName, kind, tokenHash string) (*QuickIdentityResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Upsert owner by public key (idempotent across devices).
	ownerID := ids.Owner()
	var actualOwnerID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO owners (owner_id, public_key, display_name)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (public_key) DO UPDATE SET
		     display_name = CASE WHEN EXCLUDED.display_name <> '' THEN EXCLUDED.display_name
		                         ELSE owners.display_name END
		 RETURNING owner_id`,
		ownerID, publicKey, displayName,
	).Scan(&actualOwnerID); err != nil {
		return nil, err
	}

	// 2. Create a "personal" platform for this device/session.
	platformID := ids.Platform()
	if _, err := tx.Exec(ctx,
		`INSERT INTO platforms (platform_id, owner_id, platform_type, display_name, platform_token_hash, public_key, tags)
		 VALUES ($1, $2, 'personal', $3, $4, $5, '{}')`,
		platformID, actualOwnerID, displayName, tokenHash, publicKey,
	); err != nil {
		return nil, err
	}

	// 3. Register the identity agent under the personal platform.
	visibility := "private"
	if kind == "service" {
		visibility = "public"
	}
	agentID := ids.Agent()
	if _, err := tx.Exec(ctx,
		`INSERT INTO agents (agent_id, platform_id, owner_id, name, description, agent_type, version,
		     identity_kind, visibility, metadata)
		 VALUES ($1, $2, $3, $4, '', 'cli', '1.0', $5, $6, '{}')`,
		agentID, platformID, actualOwnerID, displayName, kind, visibility,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &QuickIdentityResult{
		AgentID:    agentID,
		OwnerID:    actualOwnerID,
		PlatformID: platformID,
	}, nil
}
