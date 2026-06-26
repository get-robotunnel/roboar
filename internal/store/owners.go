package store

import (
	"context"
	"errors"

	"github.com/get-robotunnel/roboar/internal/model"
	"github.com/jackc/pgx/v5"
)

// CreateOwner inserts a new owner. The caller assigns OwnerID and CreatedAt is
// set by the database.
func (s *Store) CreateOwner(ctx context.Context, o *model.Owner) error {
	return s.Pool.QueryRow(ctx,
		`INSERT INTO owners (owner_id, public_key, display_name, email)
		 VALUES ($1, $2, $3, NULLIF($4, ''))
		 RETURNING created_at`,
		o.OwnerID, o.PublicKey, o.DisplayName, o.Email,
	).Scan(&o.CreatedAt)
}

func (s *Store) GetOwnerByID(ctx context.Context, ownerID string) (*model.Owner, error) {
	return s.scanOwner(s.Pool.QueryRow(ctx, ownerSelect+` WHERE owner_id=$1`, ownerID))
}

func (s *Store) GetOwnerByPublicKey(ctx context.Context, publicKey string) (*model.Owner, error) {
	return s.scanOwner(s.Pool.QueryRow(ctx, ownerSelect+` WHERE public_key=$1`, publicKey))
}

const ownerSelect = `SELECT owner_id, public_key, display_name, COALESCE(email,''), created_at FROM owners`

func (s *Store) scanOwner(row pgx.Row) (*model.Owner, error) {
	var o model.Owner
	if err := row.Scan(&o.OwnerID, &o.PublicKey, &o.DisplayName, &o.Email, &o.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}
