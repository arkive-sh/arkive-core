package usersrepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"

	"github.com/jackc/pgx/v5"
)

type Repository struct{}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CountUsers(ctx context.Context, db database.PgExecutor) (int64, error) {
	query := `SELECT COUNT(*) FROM users`
	var count int64
	if err := db.QueryRow(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) GetStorageUsage(ctx context.Context, db database.PgExecutor, userID string) (int64, int64, error) {
	query := `SELECT used_bytes, reserved_bytes FROM users WHERE id = $1`
	var usedBytes int64
	var reservedBytes int64
	if err := db.QueryRow(ctx, query, userID).Scan(&usedBytes, &reservedBytes); err != nil {
		return 0, 0, err
	}
	return usedBytes, reservedBytes, nil
}

func (r *Repository) UpdateLoginActivity(ctx context.Context, db database.PgExecutor, userID string, loginAt time.Time) error {
	query := `UPDATE
		users
	SET
		last_login_at = $2
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, userID, loginAt)
	return err
}

func (r *Repository) SetRecoverySetupTokenHash(ctx context.Context, db database.PgExecutor, userID string, tokenHash []byte, expiresAt time.Time) error {
	query := `UPDATE
		users
	SET
		recovery_setup_token_hash = $2,
		recovery_setup_token_expires_at = $3
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) HasValidRecoverySetupTokenHash(ctx context.Context, db database.PgExecutor, tokenHash []byte, now time.Time) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1
		FROM users
		WHERE recovery_setup_token_hash = $1
			AND recovery_setup_token_expires_at IS NOT NULL
			AND recovery_setup_token_expires_at > $2
	)`
	var ok bool
	if err := db.QueryRow(ctx, query, tokenHash, now).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (r *Repository) ClearRecoverySetupTokenHash(ctx context.Context, db database.PgExecutor, tokenHash []byte) error {
	query := `UPDATE
		users
	SET
		recovery_setup_token_hash = NULL,
		recovery_setup_token_expires_at = NULL
	WHERE
		recovery_setup_token_hash = $1`
	_, err := db.Exec(ctx, query, tokenHash)
	return err
}

func (r *Repository) SetRecoveryWrappedMasterKey(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	encryptedMasterKeyRecovery []byte,
	recoveryProofPublicKey []byte,
) error {
	query := `UPDATE
		users
	SET
		encrypted_master_key_recovery = $2,
		recovery_proof_public_key = $3,
		recovery_setup_token_hash = NULL,
		recovery_setup_token_expires_at = NULL
	WHERE
		id = $1`
	_, err := tx.Exec(ctx, query, userID, encryptedMasterKeyRecovery, recoveryProofPublicKey)
	return err
}

func (r *Repository) GetByRecoverySetupTokenHash(ctx context.Context, db database.PgExecutor, tokenHash []byte, now time.Time) (models.User, error) {
	var user models.User
	query := `SELECT
		id, encrypted_master_key
	FROM
		users
	WHERE
		recovery_setup_token_hash = $1
		AND recovery_setup_token_expires_at IS NOT NULL
		AND recovery_setup_token_expires_at > $2`
	if err := db.QueryRow(ctx, query, tokenHash, now).Scan(&user.ID, &user.EncryptedMasterKey); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (r *Repository) SetPasswordResetToken(
	ctx context.Context,
	db database.PgPool,
	email string,
	tokenHash string,
	expiresAt time.Time,
) error {
	query := `UPDATE
		users
	SET
		password_reset_token_hash = $2,
		password_reset_token_expires_at = $3
	WHERE
		email = $1`
	_, err := db.Exec(ctx, query, email, tokenHash, expiresAt)
	return err
}

func (r *Repository) FindByPasswordResetTokenHash(
	ctx context.Context,
	db database.PgExecutor,
	tokenHash string,
) (*models.User, error) {
	var user models.User
	query := `SELECT
		id, username, email, vault_salt, encrypted_master_key_recovery,
		recovery_proof_public_key,
		password_reset_token_expires_at
	FROM
		users
	WHERE
		password_reset_token_hash = $1
		AND password_reset_token_expires_at IS NOT NULL
		AND password_reset_token_expires_at > now()`
	if err := db.QueryRow(ctx, query, tokenHash).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.VaultSalt,
		&user.EncryptedMasterKeyRecovery,
		&user.RecoveryProofPublicKey,
		&user.PasswordResetTokenExpiresAt,
	); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) CompletePasswordRecovery(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	resetTokenHash string,
	newOpaqueRegistrationRecord []byte,
	newVaultSalt []byte,
	newEncryptedMasterKey []byte,
) error {
	query := `UPDATE
		users
	SET
		opaque_registration_record = $2,
		vault_salt = $3,
		encrypted_master_key = $4,
		password_reset_token_hash = NULL,
		password_reset_token_expires_at = NULL
	WHERE
		id = $1
		AND password_reset_token_hash = $5
		AND password_reset_token_expires_at IS NOT NULL
		AND password_reset_token_expires_at > now()`
	cmd, err := tx.Exec(ctx, query, userID, newOpaqueRegistrationRecord, newVaultSalt, newEncryptedMasterKey, resetTokenHash)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}
