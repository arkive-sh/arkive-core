package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
	"arkive/core/models"
	"arkive/core/opaque"
	authrepo "arkive/core/repositories/auth"
	sessionrepo "arkive/core/repositories/session"
	usersrepo "arkive/core/repositories/users"
	"arkive/pkg/opaqueffi"
	"arkive/pkg/tokens"
	"arkive/pkg/validation"
)

const maxEncryptedOpaqueLoginStateBytes = 8 << 10

type Service struct {
	db          database.PgPool
	authRepo    *authrepo.Repository
	sessionRepo *sessionrepo.Repository
	userRepo    *usersrepo.Repository

	sessionTTL  time.Duration
	resetTTL    time.Duration
	resetURL    string
	opaqueSetup string
}

type Config struct {
	SessionTTL            time.Duration
	PasswordResetTokenTTL time.Duration
	BaseURL               string
	OpaqueServerSetup     string
}

type LoginUnlockResult struct {
	SessionToken       string
	ExpiresAt          time.Time
	VaultSalt          []byte
	EncryptedMasterKey []byte
}

type OpaqueLoginStartResult struct {
	AttemptID     string
	LoginResponse string
}

type PasswordResetRequestResult struct {
	ResetToken string
	ResetURL   string
	ExpiresAt  time.Time
}

type PasswordRecoveryVault struct {
	UserID                     string
	VaultSalt                  []byte
	EncryptedMasterKeyRecovery []byte
}

func NewService(
	db database.PgPool,
	authRepo *authrepo.Repository,
	sessionRepo *sessionrepo.Repository,
	userRepo *usersrepo.Repository,
	cfg Config,
) *Service {
	return &Service{
		db:          db,
		authRepo:    authRepo,
		sessionRepo: sessionRepo,
		userRepo:    userRepo,
		sessionTTL:  cfg.SessionTTL,
		resetTTL:    cfg.PasswordResetTokenTTL,
		resetURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		opaqueSetup: cfg.OpaqueServerSetup,
	}
}

func (s *Service) StartOpaqueLogin(ctx context.Context, email string, loginRequest []byte) (OpaqueLoginStartResult, error) {
	email = strings.TrimSpace(email)
	userIdentifier := strings.ToLower(email)
	var userID *string
	var registrationRecord []byte
	user, err := s.authRepo.GetUserByEmail(ctx, s.db, email)
	if err == nil {
		userIdentifier = strings.ToLower(user.Username)
		userID = &user.ID
		registrationRecord = user.OpaqueRegistrationRecord
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return OpaqueLoginStartResult{}, err
	}
	result, err := opaque.StartLogin(s.opaqueSetup, userIdentifier, registrationRecord, loginRequest)
	if err != nil {
		return OpaqueLoginStartResult{}, ErrInvalidProtocolMessage
	}
	defer result.Response.Close()
	defer result.State.Close()
	attemptID := uuid.NewV4().String()
	expiresAt := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	// Encrypt before storage: a database reader could replay the expected client MAC to finish login.
	state, err := s.sealOpaqueLoginState(attemptID, userID, expiresAt, result.State.Bytes())
	if err != nil {
		return OpaqueLoginStartResult{}, err
	}
	if err := s.authRepo.CreateOpaqueLoginAttempt(ctx, s.db, attemptID, userID, state, expiresAt); err != nil {
		return OpaqueLoginStartResult{}, err
	}
	return OpaqueLoginStartResult{
		AttemptID:     attemptID,
		LoginResponse: base64.RawURLEncoding.EncodeToString(result.Response.Bytes()),
	}, nil
}

func (s *Service) FinishOpaqueLogin(ctx context.Context, attemptID string, finalization []byte, lastIP string) (LoginUnlockResult, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return LoginUnlockResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attempt, err := s.authRepo.ConsumeOpaqueLoginAttempt(ctx, tx, strings.TrimSpace(attemptID))
	if err != nil {
		// Missing or expired attempts are credential failures; database errors must remain visible.
		if errors.Is(err, pgx.ErrNoRows) {
			return LoginUnlockResult{}, ErrInvalidCredentials
		}
		return LoginUnlockResult{}, err
	}
	state, stateErr := s.openOpaqueLoginState(strings.TrimSpace(attemptID), attempt.UserID, attempt.ExpiresAt, attempt.State)
	if stateErr != nil || attempt.UserID == nil || opaque.FinishLogin(state, finalization) != nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return LoginUnlockResult{}, commitErr
		}
		return LoginUnlockResult{}, ErrInvalidCredentials
	}
	user, err := s.authRepo.GetAuthUserByID(ctx, tx, *attempt.UserID)
	if err != nil {
		return LoginUnlockResult{}, err
	}
	if len(user.VaultSalt) == 0 || len(user.EncryptedMasterKey) == 0 {
		return LoginUnlockResult{}, ErrVaultNotConfigured
	}
	sessionToken, expiresAt, err := s.createSession(ctx, tx, user.ID, lastIP)
	if err != nil {
		return LoginUnlockResult{}, err
	}
	if err := s.userRepo.UpdateLoginActivity(ctx, tx, user.ID, time.Now()); err != nil {
		return LoginUnlockResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LoginUnlockResult{}, err
	}
	return LoginUnlockResult{SessionToken: sessionToken, ExpiresAt: expiresAt, VaultSalt: user.VaultSalt, EncryptedMasterKey: user.EncryptedMasterKey}, nil
}

func (s *Service) LogoutSession(ctx context.Context, sessionToken string) error {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil
	}
	sessionHash := hashSessionToken(sessionToken)
	if sessionHash == nil {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	if err := s.sessionRepo.DeleteSession(ctx, tx, sessionHash); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}

	return tx.Commit(ctx)
}

func (s *Service) ValidateSession(ctx context.Context, sessionToken string) (string, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return "", ErrSessionNotFound
	}
	sessionHash := hashSessionToken(sessionToken)
	if sessionHash == nil {
		return "", ErrSessionNotFound
	}

	userID, _, err := s.sessionRepo.GetSessionByTokenHash(ctx, s.db, sessionHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSessionNotFound
		}
		return "", err
	}
	return userID, nil
}

func (s *Service) GetUserByID(ctx context.Context, userID string) (models.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return models.User{}, ErrInvalidInput
	}
	return s.authRepo.GetUserByID(ctx, s.db, userID)
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) (PasswordResetRequestResult, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return PasswordResetRequestResult{}, nil
	}

	user, err := s.authRepo.GetPasswordRecoveryInfoByEmail(ctx, s.db, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PasswordResetRequestResult{}, nil
		}
		return PasswordResetRequestResult{}, err
	}
	if len(user.EncryptedMasterKeyRecovery) == 0 || len(user.RecoveryProofPublicKey) != ed25519.PublicKeySize {
		return PasswordResetRequestResult{}, nil
	}

	token, rawHash, err := tokens.Generate()
	if err != nil {
		return PasswordResetRequestResult{}, err
	}
	expiresAt := time.Now().Add(s.resetTTL)
	if err := s.userRepo.SetPasswordResetToken(ctx, s.db, email, hex.EncodeToString(rawHash), expiresAt); err != nil {
		return PasswordResetRequestResult{}, err
	}

	return PasswordResetRequestResult{
		ResetToken: token,
		ResetURL:   s.buildPasswordResetURL(token),
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Service) LoadPasswordRecoveryVault(ctx context.Context, resetToken string) (PasswordRecoveryVault, error) {
	user, err := s.lookupPasswordResetUser(ctx, s.db, resetToken)
	if err != nil {
		return PasswordRecoveryVault{}, err
	}
	if len(user.VaultSalt) == 0 || len(user.EncryptedMasterKeyRecovery) == 0 {
		return PasswordRecoveryVault{}, ErrVaultNotConfigured
	}
	return PasswordRecoveryVault{
		UserID:                     user.ID,
		VaultSalt:                  user.VaultSalt,
		EncryptedMasterKeyRecovery: user.EncryptedMasterKeyRecovery,
	}, nil
}

func (s *Service) StartPasswordResetRegistration(ctx context.Context, resetToken string, registrationRequest []byte) (*opaqueffi.Buffer, error) {
	user, err := s.lookupPasswordResetUser(ctx, s.db, resetToken)
	if err != nil {
		return nil, err
	}
	return opaque.RegistrationResponse(s.opaqueSetup, strings.ToLower(user.Username), registrationRequest)
}

func (s *Service) CompletePasswordRecovery(
	ctx context.Context,
	resetToken string,
	registrationUpload []byte,
	newVaultSalt []byte,
	newEncryptedMasterKey []byte,
	recoveryProof []byte,
) (validation.Errors, error) {
	resetToken = strings.TrimSpace(resetToken)
	tokenHash := sha256.Sum256([]byte(resetToken))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	validationErrors := validation.New()
	if resetToken == "" {
		validationErrors.Add("token", ErrPasswordResetToken.Error())
	}
	if len(registrationUpload) == 0 {
		validationErrors.Add("registrationRecord", "password registration is missing")
	}
	if len(newVaultSalt) != 16 {
		validationErrors.Add("vaultSalt", "vault salt is missing or invalid")
	}
	if len(newEncryptedMasterKey) == 0 {
		validationErrors.Add("encryptedMasterKey", "encrypted master key is missing")
	}
	if len(recoveryProof) != ed25519.SignatureSize {
		validationErrors.Add("recoveryProof", "recovery key proof is missing or invalid")
	}
	if validationErrors.HasAny() {
		return validationErrors, nil
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	user, err := s.lookupPasswordResetUser(ctx, tx, resetToken)
	if err != nil {
		if errors.Is(err, ErrPasswordResetToken) {
			validationErrors := validation.New()
			validationErrors.Add("token", ErrPasswordResetToken.Error())
			return validationErrors, nil
		}
		return nil, err
	}
	if !verifyPasswordRecoveryProof(user.ID, resetToken, user.RecoveryProofPublicKey, recoveryProof) {
		validationErrors := validation.New()
		validationErrors.Add("recoveryProof", "recovery key proof is invalid")
		return validationErrors, nil
	}
	registrationRecord, err := opaque.FinishRegistration(registrationUpload)
	if err != nil {
		validationErrors := validation.New()
		validationErrors.Add("registrationRecord", "password registration is invalid")
		return validationErrors, nil
	}
	defer registrationRecord.Close()
	if err := s.userRepo.CompletePasswordRecovery(ctx, tx, user.ID, tokenHashHex, registrationRecord.Bytes(), newVaultSalt, newEncryptedMasterKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			validationErrors := validation.New()
			validationErrors.Add("token", ErrPasswordResetToken.Error())
			return validationErrors, nil
		}
		return nil, err
	}
	if err := s.sessionRepo.DeleteSessionsByUserID(ctx, tx, user.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return nil, nil
}

func verifyPasswordRecoveryProof(userID, resetToken string, publicKey, proof []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize || len(proof) != ed25519.SignatureSize {
		return false
	}
	message := []byte(fmt.Sprintf("arkive:recovery-proof:v1\npassword-reset\n%s\n%s", userID, resetToken))
	return ed25519.Verify(ed25519.PublicKey(publicKey), message, proof)
}

func (s *Service) createSession(ctx context.Context, db database.PgExecutor, userID, lastIP string) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.sessionTTL)
	if err := s.authRepo.UpdateLastLogin(ctx, db, userID, strings.TrimSpace(lastIP)); err != nil {
		return "", time.Time{}, err
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", time.Time{}, err
	}
	sessionToken := base64.RawURLEncoding.EncodeToString(rawToken)
	sessionHash := sha256.Sum256(rawToken)
	// The cookie is a bearer credential; persist only its hash so a database read cannot replay it.
	if err := s.sessionRepo.CreateSession(ctx, db, userID, sessionHash[:], expiresAt); err != nil {
		return "", time.Time{}, err
	}
	return sessionToken, expiresAt, nil
}

func (s *Service) sealOpaqueLoginState(attemptID string, userID *string, expiresAt time.Time, state []byte) ([]byte, error) {
	aead, err := s.opaqueLoginStateAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := append([]byte(nil), nonce...)
	return aead.Seal(sealed, nonce, state, opaqueLoginStateAAD(attemptID, userID, expiresAt)), nil
}

func (s *Service) openOpaqueLoginState(attemptID string, userID *string, expiresAt time.Time, state []byte) ([]byte, error) {
	aead, err := s.opaqueLoginStateAEAD()
	if err != nil {
		return nil, err
	}
	if len(state) < aead.NonceSize()+aead.Overhead() || len(state) > maxEncryptedOpaqueLoginStateBytes {
		return nil, errors.New("encrypted OPAQUE login state has invalid size")
	}
	nonce := state[:aead.NonceSize()]
	return aead.Open(nil, nonce, state[aead.NonceSize():], opaqueLoginStateAAD(attemptID, userID, expiresAt))
}

func (s *Service) opaqueLoginStateAEAD() (cipher.AEAD, error) {
	// Derive a purpose-specific key from the OPAQUE server secret, which is kept outside Postgres.
	key := sha256.Sum256([]byte("arkive:opaque-login-state:v1\x00" + s.opaqueSetup))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func opaqueLoginStateAAD(attemptID string, userID *string, expiresAt time.Time) []byte {
	// Row metadata is authenticated so ciphertext cannot be moved to another attempt or user.
	user := ""
	if userID != nil {
		user = *userID
	}
	return []byte(fmt.Sprintf("arkive:opaque-login-attempt:v1\n%s\n%s\n%d", attemptID, user, expiresAt.Unix()))
}

func hashSessionToken(token string) []byte {
	if len(token) != 43 {
		return nil
	}
	rawToken, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(rawToken) != 32 {
		return nil
	}
	hash := sha256.Sum256(rawToken)
	return hash[:]
}

func (s *Service) buildPasswordResetURL(token string) string {
	base := s.resetURL
	return fmt.Sprintf("%s/reset-password?token=%s", base, url.QueryEscape(token))
}

func (s *Service) lookupPasswordResetUser(ctx context.Context, db database.PgExecutor, resetToken string) (*models.User, error) {
	resetToken = strings.TrimSpace(resetToken)
	if resetToken == "" {
		return nil, ErrPasswordResetToken
	}
	tokenHash := sha256.Sum256([]byte(resetToken))
	user, err := s.userRepo.FindByPasswordResetTokenHash(ctx, db, hex.EncodeToString(tokenHash[:]))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPasswordResetToken
		}
		return nil, err
	}
	return user, nil
}
