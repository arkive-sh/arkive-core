package shares

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"uuid"

	"arkive/core/database"
	"arkive/core/models"
	"arkive/core/opaque"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	"arkive/pkg/tokens"
	"arkive/pkg/validation"

	"github.com/jackc/pgx/v5"
)

const (
	ShareStatusActive                = "active"
	ShareStatusRevoked               = "revoked"
	shareOpaqueLoginTTL              = 2 * time.Minute
	maxEncryptedShareLoginStateBytes = 8 << 10
)

type Service struct {
	db          database.PgPool
	fileRepo    *filerepo.Repository
	folderRepo  *folderrepo.Repository
	shareRepo   *sharerepo.Repository
	opaqueSetup string
}

type CreateInput struct {
	FileID                   string
	FolderID                 string
	OwnerUserID              string
	Token                    string
	RequirePassword          bool
	OpaqueRegistrationUpload []byte
	ExpiresAt                *time.Time
	AllowPreview             bool
	AllowDownload            bool
	BurnAfterRead            bool
	EncryptedShareKey        string
	EncryptedFileKeyForShare string
	EncryptedFolderName      string
	EncryptedFolderMetadata  string
}

func NewService(db database.PgPool, fileRepo *filerepo.Repository, folderRepo *folderrepo.Repository, shareRepo *sharerepo.Repository, opaqueSetup string) *Service {
	return &Service{
		db:          db,
		fileRepo:    fileRepo,
		folderRepo:  folderRepo,
		shareRepo:   shareRepo,
		opaqueSetup: opaqueSetup,
	}
}

func (s *Service) CreateShare(ctx context.Context, input CreateInput) (models.Share, validation.Errors, error) {
	input.FileID = strings.TrimSpace(input.FileID)
	input.FolderID = strings.TrimSpace(input.FolderID)
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.Token = strings.TrimSpace(input.Token)
	input.EncryptedShareKey = strings.TrimSpace(input.EncryptedShareKey)
	input.EncryptedFileKeyForShare = strings.TrimSpace(input.EncryptedFileKeyForShare)
	input.EncryptedFolderName = strings.TrimSpace(input.EncryptedFolderName)
	input.EncryptedFolderMetadata = strings.TrimSpace(input.EncryptedFolderMetadata)

	validationErrors := validation.New()
	if input.OwnerUserID == "" {
		return models.Share{}, nil, ErrUnauthorized
	}
	isFolder := input.FolderID != ""
	if (input.FileID == "") != isFolder {
		validationErrors.Add("fileId", ErrFileIDRequired.Error())
	}
	if input.Token != "" && !isTokenValid(input.Token) {
		validationErrors.Add("token", ErrTokenInvalid.Error())
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		validationErrors.Add("expiresAt", ErrExpiryInvalid.Error())
	}
	if !input.AllowPreview && !input.AllowDownload {
		validationErrors.Add("permissions", "preview or download must be enabled")
	}
	if isFolder && input.BurnAfterRead {
		validationErrors.Add("burnAfterRead", "one-time links are only supported for file shares")
	}
	if input.RequirePassword != (len(input.OpaqueRegistrationUpload) > 0) {
		validationErrors.Add("opaqueRegistrationRecord", "password registration is missing or unexpected")
	}
	if input.EncryptedShareKey == "" {
		validationErrors.Add("encryptedShareKey", "encrypted share key is required")
	}
	if !isFolder && input.EncryptedFileKeyForShare == "" {
		validationErrors.Add("encryptedFileKeyForShare", "encrypted file key for share is required")
	}
	if isFolder && (input.EncryptedFolderName == "" || input.EncryptedFolderMetadata == "") {
		validationErrors.Add("encryptedFolder", "encrypted folder name and metadata are required")
	}
	if validationErrors.HasAny() {
		return models.Share{}, validationErrors, nil
	}

	encryptedShareKey, err := base64.StdEncoding.DecodeString(input.EncryptedShareKey)
	if err != nil || len(encryptedShareKey) == 0 {
		validationErrors.Add("encryptedShareKey", "encrypted share key must be base64")
	}
	var encryptedFileKeyForShare, encryptedFolderName, encryptedFolderMetadata []byte
	if !isFolder {
		encryptedFileKeyForShare, err = base64.StdEncoding.DecodeString(input.EncryptedFileKeyForShare)
		if err != nil || len(encryptedFileKeyForShare) == 0 {
			validationErrors.Add("encryptedFileKeyForShare", "encrypted file key for share must be base64")
		}
	} else {
		encryptedFolderName, err = base64.StdEncoding.DecodeString(input.EncryptedFolderName)
		if err != nil || len(encryptedFolderName) == 0 {
			validationErrors.Add("encryptedFolderName", "encrypted folder name must be base64")
		}
		encryptedFolderMetadata, err = base64.StdEncoding.DecodeString(input.EncryptedFolderMetadata)
		if err != nil || len(encryptedFolderMetadata) == 0 {
			validationErrors.Add("encryptedFolderMetadata", "encrypted folder metadata must be base64")
		}
	}
	if validationErrors.HasAny() {
		return models.Share{}, validationErrors, nil
	}

	itemType := "file"
	if isFolder {
		itemType = "folder"
		if _, err := s.folderRepo.GetForUser(ctx, s.db, input.OwnerUserID, input.FolderID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Share{}, nil, ErrNotFound
			}
			return models.Share{}, nil, err
		}
		if _, err := s.shareRepo.GetShareForFolderForUser(ctx, s.db, input.FolderID, input.OwnerUserID); err == nil {
			return models.Share{}, nil, ErrShareExists
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, nil, err
		}
	} else {
		file, err := s.fileRepo.GetFileForUser(ctx, s.db, input.FileID, input.OwnerUserID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Share{}, nil, ErrNotFound
			}
			return models.Share{}, nil, err
		}
		if file.UploadStatus != FileStatusComplete {
			return models.Share{}, nil, ErrInvalidInput
		}
		if _, err := s.shareRepo.GetShareForFile(ctx, s.db, input.FileID); err == nil {
			return models.Share{}, nil, ErrShareExists
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, nil, err
		}
	}

	token := input.Token
	if token == "" {
		generated, _, err := tokens.Generate()
		if err != nil {
			return models.Share{}, nil, err
		}
		token = generated
	}
	if !isTokenValid(token) {
		validationErrors.Add("token", ErrTokenInvalid.Error())
		return models.Share{}, validationErrors, nil
	}
	var opaqueRegistrationRecord []byte
	if input.RequirePassword {
		serverRecord, err := opaque.FinishRegistration(input.OpaqueRegistrationUpload)
		if err != nil {
			validationErrors.Add("opaqueRegistrationRecord", "password registration is invalid")
			return models.Share{}, validationErrors, nil
		}
		defer serverRecord.Close()
		opaqueRegistrationRecord = serverRecord.Bytes()
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.Share{}, nil, err
	}
	status := ShareStatusActive
	if isFolder {
		status = "provisioning"
	}
	shareLinkID, err := s.shareRepo.CreateShareLink(ctx, tx, sharerepo.CreateShareLinkInput{
		OwnerUserID:              input.OwnerUserID,
		Token:                    token,
		EncryptedShareKey:        encryptedShareKey,
		AllowPreview:             input.AllowPreview,
		AllowDownload:            input.AllowDownload,
		OpaqueRegistrationRecord: opaqueRegistrationRecord,
		ExpiresAt:                input.ExpiresAt,
		BurnAfterRead:            input.BurnAfterRead,
		MaxAccessCount:           shareMaxAccessCount(input.BurnAfterRead),
		Status:                   status,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		return models.Share{}, nil, err
	}
	shareItemID, err := s.shareRepo.CreateShareItem(ctx, tx, sharerepo.CreateShareItemInput{
		ShareLinkID:  shareLinkID,
		ItemType:     itemType,
		FileID:       input.FileID,
		FolderID:     input.FolderID,
		DisplayOrder: 0,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		return models.Share{}, nil, err
	}
	if isFolder {
		if err := s.shareRepo.CreateFolderShareEntry(ctx, tx, shareLinkID, sharerepo.FolderShareFolderEntry{
			ID: input.FolderID, EncryptedName: encryptedFolderName, EncryptedMetadata: encryptedFolderMetadata,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return models.Share{}, nil, err
		}
	} else {
		if err := s.shareRepo.CreateShareSnapshotFile(ctx, tx, sharerepo.CreateShareSnapshotFileInput{
			ShareItemID:              shareItemID,
			FileID:                   input.FileID,
			EncryptedFileKeyForShare: encryptedFileKeyForShare,
			DisplayOrder:             0,
		}); err != nil {
			_ = tx.Rollback(ctx)
			return models.Share{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Share{}, nil, err
	}
	created, err := s.shareRepo.GetShareForUser(ctx, s.db, shareLinkID, input.OwnerUserID)
	if err != nil {
		return models.Share{}, nil, err
	}
	return created, nil, nil
}

func (s *Service) GetShareByToken(ctx context.Context, token string) (models.Share, error) {
	return s.shareRepo.GetShareByToken(ctx, s.db, token)
}

type OpaqueLoginStartResult struct {
	AttemptID     string
	LoginResponse string
}

func (s *Service) StartPasswordRegistration(ctx context.Context, shareID, ownerUserID string, request []byte) (string, error) {
	var valid bool
	shareID, valid = validation.NormalizeUUID(shareID)
	if !valid {
		return "", ErrInvalidInput
	}
	share, err := s.GetShareForUser(ctx, shareID, ownerUserID)
	if err != nil {
		return "", err
	}
	if share.Status != ShareStatusActive && share.Status != "provisioning" {
		return "", ErrNotFound
	}
	return s.passwordRegistrationResponse(share.Token, request)
}

func (s *Service) StartNewSharePasswordRegistration(token string, request []byte) (string, error) {
	token = strings.TrimSpace(token)
	if !isTokenValid(token) {
		return "", ErrInvalidInput
	}
	return s.passwordRegistrationResponse(token, request)
}

func (s *Service) passwordRegistrationResponse(identifier string, request []byte) (string, error) {
	response, err := opaque.RegistrationResponse(s.opaqueSetup, identifier, request)
	if err != nil {
		return "", ErrInvalidInput
	}
	defer response.Close()
	return base64.RawURLEncoding.EncodeToString(response.Bytes()), nil
}

func (s *Service) FinishPasswordRegistration(ctx context.Context, shareID, ownerUserID string, upload []byte) (models.Share, error) {
	var valid bool
	shareID, valid = validation.NormalizeUUID(shareID)
	if !valid {
		return models.Share{}, ErrInvalidInput
	}
	share, err := s.GetShareForUser(ctx, shareID, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	if share.Status != ShareStatusActive && share.Status != "provisioning" {
		return models.Share{}, ErrNotFound
	}
	serverRecord, err := opaque.FinishRegistration(upload)
	if err != nil {
		return models.Share{}, ErrInvalidInput
	}
	defer serverRecord.Close()

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.Share{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.shareRepo.DeleteOpaqueLoginAttempts(ctx, tx, share.ID); err != nil {
		return models.Share{}, err
	}
	updated, err := s.shareRepo.SetOpaqueRegistrationRecord(ctx, tx, share.ID, ownerUserID, serverRecord.Bytes())
	if err != nil {
		return models.Share{}, err
	}
	if !updated {
		return models.Share{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Share{}, err
	}
	return s.GetShareForUser(ctx, share.ID, ownerUserID)
}

func (s *Service) RemovePassword(ctx context.Context, shareID, ownerUserID string) (models.Share, error) {
	var valid bool
	shareID, valid = validation.NormalizeUUID(shareID)
	if !valid {
		return models.Share{}, ErrInvalidInput
	}
	share, err := s.GetShareForUser(ctx, shareID, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.Share{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.shareRepo.DeleteOpaqueLoginAttempts(ctx, tx, share.ID); err != nil {
		return models.Share{}, err
	}
	updated, err := s.shareRepo.ClearOpaqueRegistrationRecord(ctx, tx, share.ID, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	if !updated {
		return models.Share{}, ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Share{}, err
	}
	return s.GetShareForUser(ctx, share.ID, ownerUserID)
}

func (s *Service) StartPasswordLogin(ctx context.Context, token string, request []byte) (OpaqueLoginStartResult, error) {
	token = strings.TrimSpace(token)
	if !isTokenValid(token) {
		return OpaqueLoginStartResult{}, ErrNotFound
	}
	share, err := s.shareRepo.GetShareByToken(ctx, s.db, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OpaqueLoginStartResult{}, ErrNotFound
		}
		return OpaqueLoginStartResult{}, err
	}
	if share.Status != ShareStatusActive || (share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now())) {
		return OpaqueLoginStartResult{}, ErrNotFound
	}
	if len(share.OpaqueRegistrationRecord) == 0 {
		return OpaqueLoginStartResult{}, ErrPasswordNotConfigured
	}
	registrationFingerprint := shareOpaqueRegistrationFingerprint(share.OpaqueRegistrationRecord)
	result, err := opaque.StartLogin(s.opaqueSetup, share.Token, share.OpaqueRegistrationRecord, request)
	if err != nil {
		return OpaqueLoginStartResult{}, ErrInvalidInput
	}
	defer result.Response.Close()
	defer result.State.Close()

	attemptID := uuid.NewV4().String()
	expiresAt := time.Now().UTC().Add(shareOpaqueLoginTTL).Truncate(time.Second)
	state, err := s.sealPasswordLoginState(
		attemptID,
		share.ID,
		registrationFingerprint,
		expiresAt,
		result.State.Bytes(),
	)
	if err != nil {
		return OpaqueLoginStartResult{}, err
	}
	if err := s.shareRepo.CreateOpaqueLoginAttempt(ctx, s.db, attemptID, share.ID, state, expiresAt); err != nil {
		return OpaqueLoginStartResult{}, err
	}
	return OpaqueLoginStartResult{
		AttemptID:     attemptID,
		LoginResponse: base64.RawURLEncoding.EncodeToString(result.Response.Bytes()),
	}, nil
}

func (s *Service) FinishPasswordLogin(ctx context.Context, token, attemptID string, finalization []byte) (models.Share, error) {
	token = strings.TrimSpace(token)
	if !isTokenValid(token) {
		return models.Share{}, ErrNotFound
	}
	var valid bool
	attemptID, valid = validation.NormalizeUUID(attemptID)
	if !valid {
		return models.Share{}, ErrInvalidCredentials
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.Share{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	share, err := s.shareRepo.GetShareByToken(ctx, tx, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, ErrNotFound
		}
		return models.Share{}, err
	}
	if share.Status != ShareStatusActive || (share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now())) {
		return models.Share{}, ErrNotFound
	}
	if len(share.OpaqueRegistrationRecord) == 0 {
		return models.Share{}, ErrPasswordNotConfigured
	}
	attemptID = strings.TrimSpace(attemptID)
	attempt, err := s.shareRepo.ConsumeOpaqueLoginAttempt(ctx, tx, attemptID, share.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, ErrInvalidCredentials
		}
		return models.Share{}, err
	}
	state, stateErr := s.openPasswordLoginState(
		attemptID,
		share.ID,
		shareOpaqueRegistrationFingerprint(share.OpaqueRegistrationRecord),
		attempt.ExpiresAt,
		attempt.State,
	)
	var finishErr error
	if stateErr == nil {
		finishErr = opaque.FinishLogin(state, finalization)
	}
	if stateErr != nil || finishErr != nil {
		if err := tx.Commit(ctx); err != nil {
			return models.Share{}, err
		}
		return models.Share{}, ErrInvalidCredentials
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Share{}, err
	}
	return share, nil
}

func (s *Service) sealPasswordLoginState(attemptID, shareID, registrationFingerprint string, expiresAt time.Time, state []byte) ([]byte, error) {
	aead, err := s.passwordLoginStateAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := append([]byte(nil), nonce...)
	return aead.Seal(sealed, nonce, state, passwordLoginStateAAD(attemptID, shareID, registrationFingerprint, expiresAt)), nil
}

func (s *Service) openPasswordLoginState(attemptID, shareID, registrationFingerprint string, expiresAt time.Time, state []byte) ([]byte, error) {
	aead, err := s.passwordLoginStateAEAD()
	if err != nil {
		return nil, err
	}
	if len(state) < aead.NonceSize()+aead.Overhead() || len(state) > maxEncryptedShareLoginStateBytes {
		return nil, errors.New("encrypted share OPAQUE state has invalid size")
	}
	return aead.Open(nil, state[:aead.NonceSize()], state[aead.NonceSize():], passwordLoginStateAAD(attemptID, shareID, registrationFingerprint, expiresAt))
}

func (s *Service) passwordLoginStateAEAD() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("arkive:opaque-share-login-state:v1\x00" + s.opaqueSetup))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func passwordLoginStateAAD(attemptID, shareID, registrationFingerprint string, expiresAt time.Time) []byte {
	// Bind the proof to its verifier so a racing password change invalidates it.
	return []byte(fmt.Sprintf("arkive:opaque-share-login-attempt:v1\n%s\n%s\n%s\n%d", attemptID, shareID, registrationFingerprint, expiresAt.Unix()))
}

func shareOpaqueRegistrationFingerprint(record []byte) string {
	fingerprint := sha256.Sum256(record)
	return base64.RawURLEncoding.EncodeToString(fingerprint[:])
}

func (s *Service) GetPublicShareRecord(ctx context.Context, token string) (models.PublicShareRecord, error) {
	return s.shareRepo.GetPublicShareRecord(ctx, s.db, token)
}

func (s *Service) GetPublicFolderShareRecord(ctx context.Context, token string) (models.PublicShareRecord, error) {
	return s.shareRepo.GetPublicFolderShareRecord(ctx, s.db, token)
}

func (s *Service) GetPublicFolderShareFileRecord(ctx context.Context, token, fileID string) (models.PublicShareRecord, error) {
	fileID, validFileID := validation.NormalizeUUID(fileID)
	if !isTokenValid(token) || !validFileID {
		return models.PublicShareRecord{}, ErrInvalidInput
	}
	return s.shareRepo.GetPublicFolderShareFileRecord(ctx, s.db, token, fileID)
}

func (s *Service) ListPublicFolderShareEntries(ctx context.Context, token, folderID, cursor string, pageSize int) ([]models.PublicFolderShareEntry, string, error) {
	folderID, validFolderID := validation.NormalizeUUID(folderID)
	if !isTokenValid(token) || !validFolderID || len(cursor) > 64 {
		return nil, "", ErrInvalidInput
	}
	if cursor != "" {
		var validCursor bool
		cursor, validCursor = validation.NormalizeUUID(cursor)
		if !validCursor {
			return nil, "", ErrInvalidInput
		}
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.shareRepo.ListFolderShareEntries(ctx, s.db, token, folderID, pageSize, cursor)
}

func (s *Service) AddFolderShareEntries(ctx context.Context, shareID, ownerUserID string, files []sharerepo.FolderShareFileEntry, folders []sharerepo.FolderShareFolderEntry) error {
	shareID, validShareID := validation.NormalizeUUID(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if !validShareID || ownerUserID == "" || len(files)+len(folders) == 0 || len(files)+len(folders) > 250 {
		return ErrInvalidInput
	}
	share, err := s.shareRepo.GetShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if share.ItemType != "folder" || (share.Status != "active" && share.Status != "provisioning" && share.Status != ShareStatusRevoked) {
		return ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(files)+len(folders))
	for index := range files {
		id, validID := validation.NormalizeUUID(files[index].ID)
		if !validID || len(files[index].EncryptedFileKeyForShare) == 0 {
			return ErrInvalidInput
		}
		files[index].ID = id
		if _, ok := seen["file:"+id]; ok {
			return ErrInvalidInput
		}
		seen["file:"+id] = struct{}{}
	}
	for index := range folders {
		id, validID := validation.NormalizeUUID(folders[index].ID)
		if !validID || len(folders[index].EncryptedName) == 0 || len(folders[index].EncryptedMetadata) == 0 {
			return ErrInvalidInput
		}
		folders[index].ID = id
		if _, ok := seen["folder:"+id]; ok {
			return ErrInvalidInput
		}
		seen["folder:"+id] = struct{}{}
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	count, err := s.shareRepo.AddFolderShareEntries(ctx, tx, shareID, files, folders)
	if err != nil {
		return err
	}
	if count != len(files)+len(folders) {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *Service) CompleteFolderShareSetup(ctx context.Context, shareID, ownerUserID string) error {
	shareID, validShareID := validation.NormalizeUUID(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if !validShareID || ownerUserID == "" {
		return ErrInvalidInput
	}
	share, err := s.shareRepo.GetShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if share.ItemType != "folder" || share.Status != "provisioning" {
		return ErrInvalidInput
	}
	activated, err := s.shareRepo.ActivateFolderShare(ctx, s.db, share.ID, share.OwnerUserID)
	if err != nil {
		return err
	}
	if !activated {
		return ErrSetupIncomplete
	}
	return nil
}

func (s *Service) ListFolderShareScopes(ctx context.Context, ownerUserID, folderID string) ([]models.Share, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrUnauthorized
	}
	var validFolderID bool
	folderID, validFolderID = validation.NormalizeUUID(folderID)
	if !validFolderID {
		return nil, ErrInvalidInput
	}
	if _, err := s.folderRepo.GetForUser(ctx, s.db, ownerUserID, folderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.shareRepo.ListFolderShareScopes(ctx, s.db, ownerUserID, folderID)
}

func (s *Service) GetShareForFileForUser(ctx context.Context, fileID, ownerUserID string) (models.Share, error) {
	fileID = strings.TrimSpace(fileID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return models.Share{}, ErrUnauthorized
	}
	if fileID == "" {
		return models.Share{}, ErrInvalidInput
	}
	return s.shareRepo.GetShareForFileForUser(ctx, s.db, fileID, ownerUserID)
}

func (s *Service) GetShareForFolderForUser(ctx context.Context, folderID, ownerUserID string) (models.Share, error) {
	folderID = strings.TrimSpace(folderID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return models.Share{}, ErrUnauthorized
	}
	if folderID == "" {
		return models.Share{}, ErrInvalidInput
	}
	return s.shareRepo.GetShareForFolderForUser(ctx, s.db, folderID, ownerUserID)
}

func (s *Service) GetShareForUser(ctx context.Context, shareID, ownerUserID string) (models.Share, error) {
	shareID = strings.TrimSpace(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return models.Share{}, ErrUnauthorized
	}
	if shareID == "" {
		return models.Share{}, ErrInvalidInput
	}
	share, err := s.shareRepo.GetShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, ErrNotFound
		}
		return models.Share{}, err
	}
	return share, nil
}

func (s *Service) UpdateShareForUser(ctx context.Context, shareID, ownerUserID string, expiresAt *time.Time, burnAfterRead *bool, allowPreview, allowDownload *bool) (models.Share, validation.Errors, error) {
	shareID = strings.TrimSpace(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)

	validationErrors := validation.New()
	if ownerUserID == "" {
		return models.Share{}, nil, ErrUnauthorized
	}
	if shareID == "" {
		return models.Share{}, nil, ErrInvalidInput
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		validationErrors.Add("expiresAt", ErrExpiryInvalid.Error())
	}
	existing, err := s.shareRepo.GetShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, nil, ErrNotFound
		}
		return models.Share{}, nil, err
	}
	if allowPreview == nil {
		value := existing.AllowPreview
		allowPreview = &value
	}
	if allowDownload == nil {
		value := existing.AllowDownload
		allowDownload = &value
	}
	if burnAfterRead == nil {
		value := existing.BurnAfterRead
		burnAfterRead = &value
	}
	if !*allowPreview && !*allowDownload {
		validationErrors.Add("permissions", "preview or download must be enabled")
	}

	if validationErrors.HasAny() {
		return models.Share{}, validationErrors, nil
	}
	updated, err := s.shareRepo.UpdateShareForUser(ctx, s.db, shareID, ownerUserID, expiresAt, *burnAfterRead, shareMaxAccessCount(*burnAfterRead), *allowPreview, *allowDownload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, nil, ErrNotFound
		}
		return models.Share{}, nil, err
	}
	return updated, nil, nil
}

func (s *Service) DeleteShareForUser(ctx context.Context, shareID, ownerUserID string) (bool, error) {
	shareID = strings.TrimSpace(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return false, ErrUnauthorized
	}
	if shareID == "" {
		return false, ErrInvalidInput
	}
	return s.shareRepo.DeleteShareForUser(ctx, s.db, shareID, ownerUserID)
}

func (s *Service) RevokeShareForUser(ctx context.Context, shareID, ownerUserID string) (models.Share, error) {
	shareID = strings.TrimSpace(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return models.Share{}, ErrUnauthorized
	}
	if shareID == "" {
		return models.Share{}, ErrInvalidInput
	}
	share, err := s.shareRepo.RevokeShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, ErrNotFound
		}
		return models.Share{}, err
	}
	return share, nil
}

func (s *Service) ActivateShareForUser(ctx context.Context, shareID, ownerUserID string) (models.Share, error) {
	shareID = strings.TrimSpace(shareID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return models.Share{}, ErrUnauthorized
	}
	if shareID == "" {
		return models.Share{}, ErrInvalidInput
	}
	share, err := s.shareRepo.ActivateShareForUser(ctx, s.db, shareID, ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, ErrNotFound
		}
		return models.Share{}, err
	}
	return share, nil
}

func (s *Service) ConsumeShareByToken(ctx context.Context, token string) (models.Share, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return models.Share{}, false, ErrInvalidInput
	}

	share, err := s.shareRepo.GetShareByToken(ctx, s.db, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Share{}, false, ErrNotFound
		}
		return models.Share{}, false, err
	}
	if share.Status != ShareStatusActive {
		if share.Status == "burned" {
			return models.Share{}, false, ErrAlreadyConsumed
		}
		return models.Share{}, false, ErrNotFound
	}
	if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now()) {
		return models.Share{}, false, ErrNotFound
	}
	if !share.BurnAfterRead || share.MaxAccessCount == nil {
		return share, false, nil
	}

	consumed, changed, err := s.shareRepo.ConsumeShareByToken(ctx, s.db, token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			latest, latestErr := s.shareRepo.GetShareByToken(ctx, s.db, token)
			if latestErr == nil && latest.Status == "burned" {
				return models.Share{}, false, ErrAlreadyConsumed
			}
			if latestErr != nil && errors.Is(latestErr, pgx.ErrNoRows) {
				return models.Share{}, false, ErrNotFound
			}
			if latestErr != nil {
				return models.Share{}, false, latestErr
			}
			return models.Share{}, false, ErrNotFound
		}
		return models.Share{}, false, err
	}
	return consumed, changed, nil
}

func (s *Service) ListSharesForUser(ctx context.Context, ownerUserID string) ([]models.ShareWithFile, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrUnauthorized
	}
	return s.shareRepo.ListSharesForUser(ctx, s.db, ownerUserID)
}

func isTokenValid(token string) bool {
	if len(token) < TokenMinLength || len(token) > TokenMaxLength {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func shareMaxAccessCount(burnAfterRead bool) *int {
	if !burnAfterRead {
		return nil
	}
	value := 1
	return &value
}
