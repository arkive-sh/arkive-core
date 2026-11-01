package setup

import (
	"context"
	"crypto/ed25519"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"arkive/core/database"
	"arkive/core/models"
	"arkive/core/opaque"
	authrepo "arkive/core/repositories/auth"
	settingsrepo "arkive/core/repositories/settings"
	usersrepo "arkive/core/repositories/users"
	settingssvc "arkive/core/services/settings"
	"arkive/pkg/opaqueffi"
	"arkive/pkg/tokens"
	"arkive/pkg/validation"
)

var ErrAlreadyInitialized = errors.New("instance already initialized")
var ErrInvalidRecoverySetup = errors.New("invalid recovery key setup")
var ErrInvalidRegistrationInput = errors.New("invalid registration input")

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

type Service struct {
	db           database.PgPool
	authRepo     *authrepo.Repository
	userRepo     *usersrepo.Repository
	settingsRepo *settingsrepo.Repository
	opaqueSetup  string
}

type InitialAdminInput struct {
	Name                     string
	Username                 string
	Email                    string
	OpaqueRegistrationRecord []byte
	VaultSalt                []byte
	EncryptedMasterKey       []byte
	Storage                  models.StorageSettings
	LocalStorageGB           string
}

func NewService(db database.PgPool, authRepo *authrepo.Repository, userRepo *usersrepo.Repository, settingsRepo *settingsrepo.Repository, opaqueSetup string) *Service {
	return &Service{
		db:           db,
		authRepo:     authRepo,
		userRepo:     userRepo,
		settingsRepo: settingsRepo,
		opaqueSetup:  opaqueSetup,
	}
}

func (s *Service) StartOpaqueRegistration(ctx context.Context, username string, request []byte) (*opaqueffi.Buffer, error) {
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 20 || !usernamePattern.MatchString(username) {
		return nil, ErrInvalidRegistrationInput
	}
	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return nil, err
	}
	if initialized {
		return nil, ErrAlreadyInitialized
	}
	return opaque.RegistrationResponse(s.opaqueSetup, strings.ToLower(username), request)
}

func (s *Service) IsInitialized(ctx context.Context) (bool, error) {
	count, err := s.userRepo.CountUsers(ctx, s.db)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) CreateInitialAdmin(ctx context.Context, input InitialAdminInput) (models.User, validation.Errors, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.Storage = settingssvc.NormalizeStorageSettings(input.Storage)

	validationErrors := validateAdminInput(input.Name, input.Username, input.Email)
	if len(input.OpaqueRegistrationRecord) == 0 {
		validationErrors.Add(validation.GeneralKey, "password registration is missing")
	}
	if len(input.VaultSalt) != 16 {
		validationErrors.Add(validation.GeneralKey, "vault salt is missing or invalid")
	}
	if len(input.EncryptedMasterKey) == 0 {
		validationErrors.Add(validation.GeneralKey, "encrypted master key is missing")
	}
	settingssvc.ValidateStorageSettings(input.Storage, validationErrors)
	if validationErrors.HasAny() {
		return models.User{}, validationErrors, nil
	}
	registrationRecord, err := opaque.FinishRegistration(input.OpaqueRegistrationRecord)
	if err != nil {
		validationErrors.Add(validation.GeneralKey, "password registration is invalid")
		return models.User{}, validationErrors, nil
	}
	defer registrationRecord.Close()

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.User{}, nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	count, err := s.userRepo.CountUsers(ctx, tx)
	if err != nil {
		return models.User{}, nil, err
	}
	if count > 0 {
		return models.User{}, nil, ErrAlreadyInitialized
	}

	user, err := s.authRepo.CreateVerifiedUser(ctx, tx, input.Name, input.Username, input.Email, registrationRecord.Bytes(), input.VaultSalt, input.EncryptedMasterKey)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_username_key":
				validationErrors.Add("username", "username is already taken")
			case "users_email_key":
				validationErrors.Add("email", "email is already in use")
			}
			if validationErrors.HasAny() {
				return models.User{}, validationErrors, nil
			}
		}
		return models.User{}, nil, err
	}
	if err := settingssvc.SaveStorageSettingsTx(ctx, tx, s.settingsRepo, input.Storage); err != nil {
		return models.User{}, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.User{}, nil, err
	}
	return user, nil, nil
}

func (s *Service) IssueRecoverySetupToken(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	token, _, err := tokens.Generate()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(ttl)
	if err := s.userRepo.SetRecoverySetupToken(ctx, s.db, userID, token, expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) HasValidRecoverySetupToken(ctx context.Context, token string) (bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return false, nil
	}
	return s.userRepo.HasValidRecoverySetupToken(ctx, s.db, token, time.Now())
}

func (s *Service) ClearRecoverySetupToken(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.userRepo.ClearRecoverySetupToken(ctx, s.db, token)
}

func (s *Service) GetRecoverySetupUser(ctx context.Context, token string) (models.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return models.User{}, pgx.ErrNoRows
	}
	return s.userRepo.GetByRecoverySetupToken(ctx, s.db, token, time.Now())
}

func (s *Service) SaveRecoveryWrappedMasterKey(ctx context.Context, token string, encryptedMasterKeyRecovery, recoveryProofPublicKey []byte) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return pgx.ErrNoRows
	}
	if len(encryptedMasterKeyRecovery) == 0 || len(recoveryProofPublicKey) != ed25519.PublicKeySize {
		return ErrInvalidRecoverySetup
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	user, err := s.userRepo.GetByRecoverySetupToken(ctx, tx, token, time.Now())
	if err != nil {
		return err
	}
	if err := s.userRepo.SetRecoveryWrappedMasterKey(ctx, tx, user.ID, encryptedMasterKeyRecovery, recoveryProofPublicKey); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateAdminInput(name, username, email string) validation.Errors {
	validationErrors := validation.New()
	if name == "" {
		validationErrors.Add("name", "name is required")
	} else if utf8.RuneCountInString(name) < 3 {
		validationErrors.Add("name", "name must be at least 3 characters")
	}

	if username == "" {
		validationErrors.Add("username", "username is required")
	} else if utf8.RuneCountInString(username) < 3 {
		validationErrors.Add("username", "username must be at least 3 characters")
	} else if utf8.RuneCountInString(username) > 20 {
		validationErrors.Add("username", "username must be at most 20 characters")
	}
	if username != "" && !usernamePattern.MatchString(username) {
		validationErrors.Add("username", "username may only contain letters and numbers")
	}

	if email == "" {
		validationErrors.Add("email", "email is required")
	}

	return validationErrors
}
