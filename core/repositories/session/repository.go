package sessionrepo

import (
	"context"
	"time"

	"arkive/core/database"
)

type Repository struct {
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateSession(ctx context.Context, db database.PgExecutor, userID string, tokenHash []byte, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`, tokenHash, userID, expiresAt)
	return err
}

func (r *Repository) DeleteSession(ctx context.Context, db database.PgExecutor, tokenHash []byte) error {
	query := `DELETE FROM
		sessions
	WHERE
		token_hash = $1`
	_, err := db.Exec(ctx, query, tokenHash)
	return err
}

func (r *Repository) DeleteSessionsByUserID(ctx context.Context, db database.PgExecutor, userID string) error {
	query := `DELETE FROM
		sessions
	WHERE
		user_id = $1`
	_, err := db.Exec(ctx, query, userID)
	return err
}

func (r *Repository) GetSessionByTokenHash(ctx context.Context, db database.PgExecutor, tokenHash []byte) (string, time.Time, error) {
	var userID string
	var expiresAt time.Time
	query := `SELECT
		user_id, expires_at
	FROM
		sessions
	WHERE
		token_hash = $1 AND expires_at > now()`
	if err := db.QueryRow(ctx, query, tokenHash).Scan(&userID, &expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return userID, expiresAt, nil
}
