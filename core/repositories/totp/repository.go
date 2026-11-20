package totprepo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

type UserState struct {
	EncryptedSecret []byte
	LastStep        *int64
	SetupRequired   bool
	LockedUntil     *time.Time
}

type Challenge struct {
	UserID         string
	FailedAttempts int
}

func (r *Repository) GetUserState(ctx context.Context, db database.PgExecutor, userID string) (UserState, error) {
	var state UserState
	err := db.QueryRow(ctx, `SELECT encrypted_totp_secret, totp_last_step, totp_setup_required, totp_locked_until
		FROM users WHERE id = $1`, userID).Scan(
		&state.EncryptedSecret, &state.LastStep, &state.SetupRequired, &state.LockedUntil,
	)
	return state, err
}

func (r *Repository) IsTrustedDevice(ctx context.Context, db database.PgExecutor, userID string, tokenHash []byte) (bool, error) {
	var trusted bool
	err := db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM totp_trusted_devices
		WHERE user_id = $1 AND token_hash = $2 AND expires_at > now()
	)`, userID, tokenHash).Scan(&trusted)
	return trusted, err
}

func (r *Repository) CreateTrustedDevice(ctx context.Context, tx pgx.Tx, userID string, tokenHash []byte, expiresAt time.Time) error {
	if _, err := tx.Exec(ctx, `DELETE FROM totp_trusted_devices WHERE expires_at <= now()`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO totp_trusted_devices (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`, userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) DeleteTrustedDevices(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM totp_trusted_devices WHERE user_id = $1`, userID)
	return err
}

func (r *Repository) CreateChallenge(ctx context.Context, db database.PgExecutor, userID string, tokenHash []byte, expiresAt time.Time) error {
	if _, err := db.Exec(ctx, `DELETE FROM auth_mfa_challenges WHERE expires_at <= now()`); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `INSERT INTO auth_mfa_challenges (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`, tokenHash, userID, expiresAt)
	return err
}

func (r *Repository) GetChallengeForUpdate(ctx context.Context, tx pgx.Tx, tokenHash []byte) (Challenge, error) {
	var challenge Challenge
	err := tx.QueryRow(ctx, `SELECT user_id, failed_attempts FROM auth_mfa_challenges
		WHERE token_hash = $1 AND expires_at > now() FOR UPDATE`, tokenHash).Scan(
		&challenge.UserID, &challenge.FailedAttempts,
	)
	return challenge, err
}

func (r *Repository) IncrementChallengeFailure(ctx context.Context, tx pgx.Tx, tokenHash []byte) error {
	_, err := tx.Exec(ctx, `UPDATE auth_mfa_challenges SET failed_attempts = failed_attempts + 1
		WHERE token_hash = $1 AND failed_attempts < 5`, tokenHash)
	return err
}

func (r *Repository) DeleteChallenge(ctx context.Context, tx pgx.Tx, tokenHash []byte) error {
	_, err := tx.Exec(ctx, `DELETE FROM auth_mfa_challenges WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *Repository) SetPendingSecret(ctx context.Context, db database.PgExecutor, userID string, encryptedSecret []byte) error {
	tag, err := db.Exec(ctx, `UPDATE users SET pending_totp_secret = $2,
		pending_totp_expires_at = now() + interval '10 minutes'
		WHERE id = $1 AND encrypted_totp_secret IS NULL`, userID, encryptedSecret)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) GetPendingSecret(ctx context.Context, db database.PgExecutor, userID string) ([]byte, error) {
	var secret []byte
	err := db.QueryRow(ctx, `SELECT pending_totp_secret FROM users
		WHERE id = $1 AND pending_totp_secret IS NOT NULL AND pending_totp_expires_at > now()`, userID).Scan(&secret)
	return secret, err
}

func (r *Repository) Activate(ctx context.Context, tx pgx.Tx, userID string, lastStep int64, recoveryHashes [][]byte) error {
	tag, err := tx.Exec(ctx, `UPDATE users SET encrypted_totp_secret = pending_totp_secret,
		pending_totp_secret = NULL, pending_totp_expires_at = NULL, totp_last_step = $2,
		totp_setup_required = FALSE, totp_failed_attempts = 0,
		totp_failure_window_started_at = NULL, totp_locked_until = NULL
		WHERE id = $1 AND pending_totp_secret IS NOT NULL AND pending_totp_expires_at > now()
		AND encrypted_totp_secret IS NULL`, userID, lastStep)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, codeHash := range recoveryHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO totp_recovery_codes (user_id, code_hash) VALUES ($1, $2)`, userID, codeHash); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) UseRecoveryCode(ctx context.Context, tx pgx.Tx, userID string, codeHash []byte) (bool, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = $1 AND code_hash = $2`, userID, codeHash)
	return tag.RowsAffected() == 1, err
}

func (r *Repository) Disable(ctx context.Context, tx pgx.Tx, userID string) error {
	if err := r.DeleteTrustedDevices(ctx, tx, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET encrypted_totp_secret = NULL, pending_totp_secret = NULL,
		pending_totp_expires_at = NULL, totp_last_step = NULL, totp_setup_required = FALSE,
		totp_failed_attempts = 0, totp_failure_window_started_at = NULL, totp_locked_until = NULL
		WHERE id = $1`, userID)
	return err
}

func (r *Repository) MarkUsed(ctx context.Context, tx pgx.Tx, userID string, step int64) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE users SET totp_last_step = $2 WHERE id = $1
		AND (totp_last_step IS NULL OR totp_last_step < $2)`, userID, step)
	return tag.RowsAffected() == 1, err
}

func (r *Repository) RecordFailure(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var locked bool
	err := tx.QueryRow(ctx, `UPDATE users SET
		totp_failure_window_started_at = CASE
			WHEN totp_failure_window_started_at IS NULL OR totp_failure_window_started_at <= now() - interval '15 minutes' THEN now()
			ELSE totp_failure_window_started_at END,
		totp_failed_attempts = CASE
			WHEN totp_failure_window_started_at IS NULL OR totp_failure_window_started_at <= now() - interval '15 minutes' THEN 1
			ELSE totp_failed_attempts + 1 END,
		totp_locked_until = CASE
			WHEN (CASE
				WHEN totp_failure_window_started_at IS NULL OR totp_failure_window_started_at <= now() - interval '15 minutes' THEN 1
				ELSE totp_failed_attempts + 1 END) >= 10
			THEN now() + interval '15 minutes'
			ELSE totp_locked_until END
		WHERE id = $1
		RETURNING COALESCE(totp_locked_until > now(), FALSE)`, userID).Scan(&locked)
	return locked, err
}

func (r *Repository) ResetFailures(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE users SET totp_failed_attempts = 0,
		totp_failure_window_started_at = NULL, totp_locked_until = NULL WHERE id = $1`, userID)
	return err
}

func (r *Repository) Status(ctx context.Context, db database.PgExecutor, userID string) (bool, int, error) {
	var enabled bool
	var remaining int
	err := db.QueryRow(ctx, `SELECT users.encrypted_totp_secret IS NOT NULL,
		(SELECT count(*) FROM totp_recovery_codes WHERE user_id = users.id)
		FROM users WHERE id = $1`, userID).Scan(&enabled, &remaining)
	return enabled, remaining, err
}

func (r *Repository) ResetForPasswordRecovery(ctx context.Context, tx pgx.Tx, userID string) error {
	if err := r.DeleteTrustedDevices(ctx, tx, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM auth_mfa_challenges WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM totp_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET
		totp_setup_required = (encrypted_totp_secret IS NOT NULL OR totp_setup_required),
		encrypted_totp_secret = NULL, pending_totp_secret = NULL, pending_totp_expires_at = NULL,
		totp_last_step = NULL, totp_failed_attempts = 0,
		totp_failure_window_started_at = NULL, totp_locked_until = NULL
		WHERE id = $1`, userID)
	return err
}
