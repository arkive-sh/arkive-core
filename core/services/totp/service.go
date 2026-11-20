package totpservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
	totprepo "arkive/core/repositories/totp"
)

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrInvalidCode      = errors.New("invalid verification code")
	ErrChallengeExpired = errors.New("two-factor sign-in challenge expired")
	ErrLocked           = errors.New("too many verification attempts; try again later")
	ErrAlreadyEnabled   = errors.New("two-factor authentication is already enabled")
	ErrNotEnabled       = errors.New("two-factor authentication is not enabled")
)

const TrustedDeviceTTL = 7 * 24 * time.Hour

type Service struct {
	db          database.PgPool
	repo        *totprepo.Repository
	opaqueSetup string
}

type SetupResult struct {
	Secret        string `json:"secret"`
	QRCodeDataURL string `json:"qrCodeDataURL"`
}

type StatusResult struct {
	Enabled                bool `json:"enabled"`
	RecoveryCodesRemaining int  `json:"recoveryCodesRemaining"`
}

type LoginChallenge struct {
	Token string
	Setup *SetupResult
}

type LoginVerification struct {
	UserID        string
	RecoveryCodes []string
}

type TrustedDevice struct {
	Token     string
	ExpiresAt time.Time
}

func NewService(db database.PgPool, repo *totprepo.Repository, opaqueSetup string) *Service {
	return &Service{db: db, repo: repo, opaqueSetup: opaqueSetup}
}

func (s *Service) CreateLoginChallenge(ctx context.Context, tx pgx.Tx, userID, username, trustedDeviceToken string) (*LoginChallenge, error) {
	state, err := s.repo.GetUserState(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if !state.SetupRequired && len(state.EncryptedSecret) == 0 {
		return nil, nil
	}
	if !state.SetupRequired && len(trustedDeviceToken) == 43 {
		rawToken, err := base64.RawURLEncoding.DecodeString(trustedDeviceToken)
		if err == nil && len(rawToken) == 32 {
			tokenHash := sha256.Sum256(rawToken)
			clear(rawToken)
			trusted, err := s.repo.IsTrustedDevice(ctx, tx, userID, tokenHash[:])
			if err != nil {
				return nil, err
			}
			if trusted {
				return nil, nil
			}
		} else if rawToken != nil {
			clear(rawToken)
		}
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256(rawToken)
	clear(rawToken)
	if err := s.repo.CreateChallenge(ctx, tx, userID, tokenHash[:], time.Now().Add(5*time.Minute)); err != nil {
		return nil, err
	}
	challenge := &LoginChallenge{Token: token}
	if state.SetupRequired {
		_, setup, err := s.pendingSecret(ctx, tx, userID, username)
		if err != nil {
			return nil, err
		}
		challenge.Setup = &setup
	}
	return challenge, nil
}

func (s *Service) RememberDevice(ctx context.Context, tx pgx.Tx, userID string) (TrustedDevice, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return TrustedDevice{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256(rawToken)
	clear(rawToken)
	expiresAt := time.Now().Add(TrustedDeviceTTL)
	if err := s.repo.CreateTrustedDevice(ctx, tx, userID, tokenHash[:], expiresAt); err != nil {
		return TrustedDevice{}, err
	}
	return TrustedDevice{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) VerifyLogin(ctx context.Context, tx pgx.Tx, challengeToken, code, recoveryCode string) (LoginVerification, error) {
	code = strings.TrimSpace(code)
	recoveryCode = strings.TrimSpace(recoveryCode)
	if (code == "") == (recoveryCode == "") {
		return LoginVerification{}, ErrInvalidCode
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(challengeToken))
	if err != nil || len(rawToken) != 32 {
		return LoginVerification{}, ErrInvalidCode
	}
	tokenHash := sha256.Sum256(rawToken)
	clear(rawToken)
	challenge, err := s.repo.GetChallengeForUpdate(ctx, tx, tokenHash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginVerification{}, ErrChallengeExpired
	}
	if err != nil {
		return LoginVerification{}, err
	}
	if challenge.FailedAttempts >= 5 {
		return LoginVerification{}, ErrChallengeExpired
	}
	state, err := s.repo.GetUserState(ctx, tx, challenge.UserID)
	if err != nil {
		return LoginVerification{}, err
	}
	if state.LockedUntil != nil && state.LockedUntil.After(time.Now()) {
		return LoginVerification{}, ErrLocked
	}
	var recoveryCodes []string
	switch {
	case state.SetupRequired:
		if recoveryCode != "" {
			return LoginVerification{}, s.recordFailure(ctx, tx, tokenHash[:], challenge.UserID)
		}
		sealed, err := s.repo.GetPendingSecret(ctx, tx, challenge.UserID)
		if err != nil {
			return LoginVerification{}, ErrInvalidCode
		}
		secret, err := s.openSecret(challenge.UserID, sealed)
		if err != nil {
			return LoginVerification{}, err
		}
		defer clear(secret)
		step, valid := verifyTOTP(secret, code, time.Now())
		if !valid {
			return LoginVerification{}, s.recordFailure(ctx, tx, tokenHash[:], challenge.UserID)
		}
		var hashes [][]byte
		recoveryCodes, hashes, err = newRecoveryCodes()
		if err != nil {
			return LoginVerification{}, err
		}
		if err := s.repo.Activate(ctx, tx, challenge.UserID, step, hashes); err != nil {
			return LoginVerification{}, err
		}
	case len(state.EncryptedSecret) > 0:
		if code != "" {
			secret, err := s.openSecret(challenge.UserID, state.EncryptedSecret)
			if err != nil {
				return LoginVerification{}, err
			}
			defer clear(secret)
			step, valid := verifyTOTP(secret, code, time.Now())
			if !valid || (state.LastStep != nil && step <= *state.LastStep) {
				return LoginVerification{}, s.recordFailure(ctx, tx, tokenHash[:], challenge.UserID)
			}
			updated, err := s.repo.MarkUsed(ctx, tx, challenge.UserID, step)
			if err != nil {
				return LoginVerification{}, err
			}
			if !updated {
				return LoginVerification{}, s.recordFailure(ctx, tx, tokenHash[:], challenge.UserID)
			}
		} else {
			used, err := s.repo.UseRecoveryCode(ctx, tx, challenge.UserID, hashRecoveryCode(recoveryCode))
			if err != nil {
				return LoginVerification{}, err
			}
			if !used {
				return LoginVerification{}, s.recordFailure(ctx, tx, tokenHash[:], challenge.UserID)
			}
		}
	default:
		return LoginVerification{}, ErrInvalidCode
	}
	if err := s.repo.DeleteChallenge(ctx, tx, tokenHash[:]); err != nil {
		return LoginVerification{}, err
	}
	if err := s.repo.ResetFailures(ctx, tx, challenge.UserID); err != nil {
		return LoginVerification{}, err
	}
	return LoginVerification{UserID: challenge.UserID, RecoveryCodes: recoveryCodes}, nil
}

func (s *Service) Status(ctx context.Context, userID string) (StatusResult, error) {
	if strings.TrimSpace(userID) == "" {
		return StatusResult{}, ErrInvalidInput
	}
	enabled, remaining, err := s.repo.Status(ctx, s.db, userID)
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{Enabled: enabled, RecoveryCodesRemaining: remaining}, nil
}

func (s *Service) BeginEnrollment(ctx context.Context, userID, username string) (SetupResult, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(username) == "" {
		return SetupResult{}, ErrInvalidInput
	}
	enabled, _, err := s.repo.Status(ctx, s.db, userID)
	if err != nil {
		return SetupResult{}, err
	}
	if enabled {
		return SetupResult{}, ErrAlreadyEnabled
	}
	secret, setup, err := newSetup(username)
	if err != nil {
		return SetupResult{}, err
	}
	defer clear(secret)
	sealed, err := s.sealSecret(userID, secret)
	if err != nil {
		return SetupResult{}, err
	}
	if err := s.repo.SetPendingSecret(ctx, s.db, userID, sealed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SetupResult{}, ErrAlreadyEnabled
		}
		return SetupResult{}, err
	}
	return setup, nil
}

func (s *Service) ConfirmEnrollment(ctx context.Context, userID, code string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := s.repo.GetUserState(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if state.LockedUntil != nil && state.LockedUntil.After(time.Now()) {
		return nil, ErrLocked
	}
	sealed, err := s.repo.GetPendingSecret(ctx, tx, userID)
	if err != nil {
		return nil, ErrInvalidCode
	}
	secret, err := s.openSecret(userID, sealed)
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	step, valid := verifyTOTP(secret, code, time.Now())
	if !valid {
		return nil, s.recordFailure(ctx, tx, nil, userID)
	}
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.repo.Activate(ctx, tx, userID, step, hashes); err != nil {
		return nil, err
	}
	if err := s.repo.ResetFailures(ctx, tx, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) Disable(ctx context.Context, userID, code, recoveryCode string) error {
	code = strings.TrimSpace(code)
	recoveryCode = strings.TrimSpace(recoveryCode)
	if strings.TrimSpace(userID) == "" || (code == "") == (recoveryCode == "") {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := s.repo.GetUserState(ctx, tx, userID)
	if err != nil {
		return err
	}
	if len(state.EncryptedSecret) == 0 {
		return ErrNotEnabled
	}
	if state.LockedUntil != nil && state.LockedUntil.After(time.Now()) {
		return ErrLocked
	}
	if code != "" {
		secret, err := s.openSecret(userID, state.EncryptedSecret)
		if err != nil {
			return err
		}
		defer clear(secret)
		step, valid := verifyTOTP(secret, code, time.Now())
		if !valid || (state.LastStep != nil && step <= *state.LastStep) {
			return s.recordFailure(ctx, tx, nil, userID)
		}
		updated, err := s.repo.MarkUsed(ctx, tx, userID, step)
		if err != nil {
			return err
		}
		if !updated {
			return s.recordFailure(ctx, tx, nil, userID)
		}
	} else {
		used, err := s.repo.UseRecoveryCode(ctx, tx, userID, hashRecoveryCode(recoveryCode))
		if err != nil {
			return err
		}
		if !used {
			return s.recordFailure(ctx, tx, nil, userID)
		}
	}
	if err := s.repo.Disable(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ResetForPasswordRecovery(ctx context.Context, tx pgx.Tx, userID string) error {
	return s.repo.ResetForPasswordRecovery(ctx, tx, userID)
}

func (s *Service) pendingSecret(ctx context.Context, db database.PgExecutor, userID, username string) ([]byte, SetupResult, error) {
	sealed, err := s.repo.GetPendingSecret(ctx, db, userID)
	if err == nil {
		secret, err := s.openSecret(userID, sealed)
		if err != nil {
			return nil, SetupResult{}, err
		}
		setup, err := renderSetup(username, secret)
		if err != nil {
			clear(secret)
			return nil, SetupResult{}, err
		}
		return secret, setup, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, SetupResult{}, err
	}
	secret, setup, err := newSetup(username)
	if err != nil {
		return nil, SetupResult{}, err
	}
	sealed, err = s.sealSecret(userID, secret)
	if err != nil {
		clear(secret)
		return nil, SetupResult{}, err
	}
	if err := s.repo.SetPendingSecret(ctx, db, userID, sealed); err != nil {
		clear(secret)
		return nil, SetupResult{}, err
	}
	return secret, setup, nil
}

func (s *Service) recordFailure(ctx context.Context, tx pgx.Tx, challengeHash []byte, userID string) error {
	if len(challengeHash) > 0 {
		if err := s.repo.IncrementChallengeFailure(ctx, tx, challengeHash); err != nil {
			return err
		}
	}
	locked, err := s.repo.RecordFailure(ctx, tx, userID)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if locked {
		return ErrLocked
	}
	return ErrInvalidCode
}
