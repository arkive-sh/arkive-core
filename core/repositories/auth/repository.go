package authrepo

import (
	"context"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct {
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateVerifiedUser(ctx context.Context, db database.PgExecutor, name, username, email, passwordHash string, vaultSalt, encryptedMasterKey []byte) (models.User, error) {
	var user models.User
	query := `INSERT INTO users
		(name, username, email, password_hash, vault_salt, encrypted_master_key)
	VALUES
		($1, $2, $3, $4, $5, $6)
	RETURNING
		id, name, username, email, vault_salt, encrypted_master_key`
	if err := db.QueryRow(ctx, query, name, username, email, passwordHash, vaultSalt, encryptedMasterKey).Scan(
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
		id, name, username, email, vault_salt, encrypted_master_key, encrypted_master_key_recovery
	FROM
		users
	WHERE
		email = $1`
	if err := db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Username,
		&user.Email,
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

func (r *Repository) GetPasswordHashByEmail(ctx context.Context, db database.PgExecutor, email string) (*string, error) {
	var hash *string
	query := `SELECT password_hash FROM users WHERE email = $1`
	if err := db.QueryRow(ctx, query, email).Scan(&hash); err != nil {
		return nil, err
	}
	return hash, nil
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
