package authrepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
	"github.com/jackc/pgx/v5"
)

type Repository struct {
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateVerifiedUser(ctx context.Context, db database.PgExecutor, name, username, email string, opaqueRegistrationRecord, vaultSalt, encryptedMasterKey []byte) (models.User, error) {
	var user models.User
	query := `INSERT INTO users
		(name, username, email, opaque_registration_record, vault_salt, encrypted_master_key)
	VALUES
		($1, $2, $3, $4, $5, $6)
	RETURNING
		id, name, username, email, vault_salt, encrypted_master_key`
	if err := db.QueryRow(ctx, query, name, username, email, opaqueRegistrationRecord, vaultSalt, encryptedMasterKey).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.Email,
		&user.VaultSalt,
		&user.EncryptedMasterKey,
	); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, db database.PgExecutor, email string) (models.User, error) {
	var user models.User
	query := `SELECT
		id, name, username, email, opaque_registration_record, vault_salt, encrypted_master_key, encrypted_master_key_recovery
	FROM
		users
	WHERE
		email = $1`
	if err := db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.Email,
		&user.OpaqueRegistrationRecord,
		&user.VaultSalt,
		&user.EncryptedMasterKey,
		&user.EncryptedMasterKeyRecovery,
	); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *Repository) GetPasswordRecoveryInfoByEmail(ctx context.Context, db database.PgExecutor, email string) (models.User, error) {
	var user models.User
	query := `SELECT encrypted_master_key_recovery, recovery_proof_public_key
		FROM users
		WHERE email = $1`
	if err := db.QueryRow(ctx, query, email).Scan(&user.EncryptedMasterKeyRecovery, &user.RecoveryProofPublicKey); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, db database.PgExecutor, userID string) (models.User, error) {
	var user models.User
	query := `SELECT
		id, name, username, email, used_bytes, last_login_at, created_at
	FROM
		users
	WHERE
		id = $1`
	if err := db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.Email,
		&user.UsedBytes,
		&user.LastLoginAt,
		&user.CreatedAt,
	); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *Repository) GetAuthUserByID(ctx context.Context, db database.PgExecutor, userID string) (models.User, error) {
	var user models.User
	query := `SELECT id, username, email, opaque_registration_record, vault_salt, encrypted_master_key
		FROM users WHERE id = $1`
	if err := db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.OpaqueRegistrationRecord,
		&user.VaultSalt,
		&user.EncryptedMasterKey,
	); err != nil {
		return models.User{}, err
	}
	return user, nil
}

type OpaqueLoginAttempt struct {
	UserID    *string
	State     []byte
	ExpiresAt time.Time
}

func (r *Repository) CreateOpaqueLoginAttempt(ctx context.Context, db database.PgExecutor, id string, userID *string, state []byte, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `DELETE FROM opaque_login_attempts WHERE expires_at <= now()`)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO opaque_login_attempts (id, user_id, server_state, expires_at)
		VALUES ($1, $2, $3, $4)`, id, userID, state, expiresAt)
	return err
}

func (r *Repository) ConsumeOpaqueLoginAttempt(ctx context.Context, tx pgx.Tx, id string) (OpaqueLoginAttempt, error) {
	var attempt OpaqueLoginAttempt
	if err := tx.QueryRow(ctx, `SELECT user_id, server_state, expires_at FROM opaque_login_attempts
		WHERE id = $1 AND expires_at > now() FOR UPDATE`, id).Scan(&attempt.UserID, &attempt.State, &attempt.ExpiresAt); err != nil {
		return OpaqueLoginAttempt{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM opaque_login_attempts WHERE id = $1`, id); err != nil {
		return OpaqueLoginAttempt{}, err
	}
	return attempt, nil
}

func (r *Repository) UpdateLastLogin(ctx context.Context, db database.PgExecutor, userID, lastIP string) error {
	query := `UPDATE
		users
	SET
		last_login_at = now()
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, userID)
	return err
}
