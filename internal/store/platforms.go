package store

import (
	"context"
	"errors"

	"github.com/get-robotunnel/roboar/internal/model"
	"github.com/jackc/pgx/v5"
)

// CreatePlatform inserts a platform. tokenHash is the bcrypt hash of the
// platform token; the plaintext is never stored.
func (s *Store) CreatePlatform(ctx context.Context, p *model.Platform, tokenHash string) error {
	return s.Pool.QueryRow(ctx,
		`INSERT INTO platforms (platform_id, owner_id, platform_type, display_name, description, platform_token_hash, public_key, tags)
		 VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7, $8)
		 RETURNING created_at`,
		p.PlatformID, p.OwnerID, p.PlatformType, p.DisplayName, p.Description, tokenHash, p.PublicKey, p.Tags,
	).Scan(&p.CreatedAt)
}

// platformSelect computes effective online from heartbeat recency so a missed
// sweeper never leaves a stale "online" flag. $1 is the offline-after seconds.
const platformSelect = `SELECT platform_id, owner_id, platform_type, display_name, COALESCE(description,''),
	COALESCE(public_key,''), last_seen_at,
	(last_seen_at IS NOT NULL AND last_seen_at > NOW() - make_interval(secs => $1)) AS online,
	tags, created_at
	FROM platforms`

func (s *Store) GetPlatform(ctx context.Context, platformID string, offlineSecs int) (*model.Platform, error) {
	return s.scanPlatform(s.Pool.QueryRow(ctx, platformSelect+` WHERE platform_id=$2`, offlineSecs, platformID))
}

func (s *Store) ListPlatformsByOwner(ctx context.Context, ownerID string, offlineSecs int) ([]model.Platform, error) {
	rows, err := s.Pool.Query(ctx, platformSelect+` WHERE owner_id=$2 ORDER BY created_at`, offlineSecs, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Platform
	for rows.Next() {
		p, err := s.scanPlatformRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPlatformTokenHash returns the stored bcrypt hash and owner id for auth.
func (s *Store) GetPlatformTokenHash(ctx context.Context, platformID string) (hash, ownerID string, err error) {
	err = s.Pool.QueryRow(ctx,
		`SELECT platform_token_hash, owner_id FROM platforms WHERE platform_id=$1`, platformID,
	).Scan(&hash, &ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return hash, ownerID, err
}

// UpdatePlatform patches mutable platform fields. Nil pointers are left unchanged.
func (s *Store) UpdatePlatform(ctx context.Context, platformID string, displayName, description *string, tags *[]string) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE platforms SET
			display_name = COALESCE($2, display_name),
			description  = COALESCE($3, description),
			tags         = COALESCE($4, tags)
		 WHERE platform_id=$1`,
		platformID, displayName, description, tags,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeletePlatform(ctx context.Context, platformID string) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM platforms WHERE platform_id=$1`, platformID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Heartbeat marks the platform seen now and records its public key on first
// contact (the platform generates its keypair locally and uploads it here).
func (s *Store) Heartbeat(ctx context.Context, platformID, publicKey string) error {
	ct, err := s.Pool.Exec(ctx,
		`UPDATE platforms SET
			last_seen_at = NOW(),
			online = TRUE,
			public_key = CASE WHEN $2 <> '' THEN $2 ELSE public_key END
		 WHERE platform_id=$1`,
		platformID, publicKey,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanPlatform(row pgx.Row) (*model.Platform, error) {
	p, err := s.scanPlatformRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) scanPlatformRow(row pgx.Row) (*model.Platform, error) {
	var p model.Platform
	if err := row.Scan(&p.PlatformID, &p.OwnerID, &p.PlatformType, &p.DisplayName, &p.Description,
		&p.PublicKey, &p.LastSeenAt, &p.Online, &p.Tags, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}
