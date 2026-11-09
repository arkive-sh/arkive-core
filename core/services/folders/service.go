package folders

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"arkive/core/database"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	foldersrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	trashrepo "arkive/core/repositories/trash"
	filessvc "arkive/core/services/files"
	"arkive/pkg/validation"
)

const (
	defaultPageSize = 50
	maxPageSize     = 100
	maxMoveEntries  = 1000
	maxFolderDepth  = 100
)

type Service struct {
	db         database.PgPool
	folderRepo *foldersrepo.Repository
	fileRepo   *filerepo.Repository
	trashRepo  *trashrepo.Repository
	shareRepo  *sharerepo.Repository
	filesSvc   *filessvc.Service
}

func NewService(db database.PgPool, folderRepo *foldersrepo.Repository, fileRepo *filerepo.Repository, shareRepo *sharerepo.Repository, filesSvc *filessvc.Service) *Service {
	return &Service{
		db:         db,
		folderRepo: folderRepo,
		fileRepo:   fileRepo,
		trashRepo:  trashrepo.New(),
		shareRepo:  shareRepo,
		filesSvc:   filesSvc,
	}
}

func (s *Service) CreateFolder(ctx context.Context, input CreateFolderInput) (models.Folder, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" || len(input.EncryptedName) == 0 || len(input.EncryptedMetadata) == 0 {
		return models.Folder{}, ErrInvalidInput
	}
	folderID, err := validateUUIDValue(input.ID)
	if err != nil {
		return models.Folder{}, err
	}
	searchTokens, err := filessvc.NormalizeSearchTokens(input.SearchTokens, filessvc.MaxSearchTokensPerFile)
	if err != nil {
		return models.Folder{}, ErrInvalidInput
	}
	vaultID := strings.TrimSpace(input.VaultID)
	if vaultID == "" {
		vaultID = userID
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.Folder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if input.ParentFolderID != nil {
		parentID, err := validateUUIDValue(*input.ParentFolderID)
		if err != nil {
			return models.Folder{}, err
		}
		parent, err := s.folderRepo.GetForUser(ctx, tx, userID, parentID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.Folder{}, ErrNotFound
			}
			return models.Folder{}, err
		}
		vaultID = parent.VaultID
		input.ParentFolderID = &parentID
	}

	folder, err := s.folderRepo.Create(ctx, tx, models.Folder{
		ID:                folderID,
		UserID:            userID,
		VaultID:           vaultID,
		ParentFolderID:    input.ParentFolderID,
		EncryptedName:     input.EncryptedName,
		EncryptedMetadata: input.EncryptedMetadata,
	})
	if err != nil {
		return models.Folder{}, err
	}
	if err := s.folderRepo.InsertSearchTokensForFolder(ctx, tx, userID, folder.VaultID, folder.ID, searchTokens); err != nil {
		return models.Folder{}, err
	}
	if len(input.ShareEntries) > 100 {
		return models.Folder{}, ErrInvalidInput
	}
	seenShares := make(map[string]struct{}, len(input.ShareEntries))
	for _, entry := range input.ShareEntries {
		shareID := strings.TrimSpace(entry.ShareID)
		if shareID == "" || len(entry.EncryptedName) == 0 || len(entry.EncryptedMetadata) == 0 {
			return models.Folder{}, ErrInvalidInput
		}
		if _, exists := seenShares[shareID]; exists {
			return models.Folder{}, ErrInvalidInput
		}
		seenShares[shareID] = struct{}{}
		count, err := s.shareRepo.AddFolderShareEntries(ctx, tx, shareID, nil, []sharerepo.FolderShareFolderEntry{{
			ID: folder.ID, EncryptedName: entry.EncryptedName, EncryptedMetadata: entry.EncryptedMetadata,
		}})
		if err != nil {
			return models.Folder{}, err
		}
		if count != 1 {
			return models.Folder{}, ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Folder{}, err
	}
	return folder, nil
}

// ListEntriesPage serves the legacy Go-rendered file pages.
func (s *Service) ListEntriesPage(ctx context.Context, input ListEntriesPageInput) (ListEntriesPageResult, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return ListEntriesPageResult{}, ErrInvalidInput
	}
	if input.Page < 1 {
		input.Page = 1
	}
	if input.PageSize <= 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageSize > maxPageSize {
		input.PageSize = maxPageSize
	}
	if input.FolderID != nil {
		folderID, err := validateUUIDValue(*input.FolderID)
		if err != nil {
			return ListEntriesPageResult{}, err
		}
		if _, err := s.folderRepo.GetForUser(ctx, s.db, userID, folderID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ListEntriesPageResult{}, ErrNotFound
			}
			return ListEntriesPageResult{}, err
		}
		input.FolderID = &folderID
	}

	folderCount, err := s.folderRepo.CountChildFolders(ctx, s.db, userID, input.FolderID)
	if err != nil {
		return ListEntriesPageResult{}, err
	}
	fileCount, err := s.fileRepo.CountByFolder(ctx, s.db, userID, input.FolderID)
	if err != nil {
		return ListEntriesPageResult{}, err
	}

	totalEntries := folderCount + fileCount
	totalPages := 0
	if totalEntries > 0 {
		totalPages = (totalEntries + input.PageSize - 1) / input.PageSize
	}
	offset := (input.Page - 1) * input.PageSize

	result := ListEntriesPageResult{
		FolderID:     input.FolderID,
		Page:         input.Page,
		PageSize:     input.PageSize,
		TotalEntries: totalEntries,
		TotalPages:   totalPages,
		Path:         []models.Folder{},
		Folders:      []models.Folder{},
		Files:        []models.File{},
	}
	if input.FolderID != nil {
		path, err := s.folderPath(ctx, s.db, userID, *input.FolderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ListEntriesPageResult{}, ErrNotFound
			}
			return ListEntriesPageResult{}, err
		}
		result.Path = path
	}
	if totalEntries == 0 || offset >= totalEntries {
		return result, nil
	}

	if offset < folderCount {
		folderLimit := input.PageSize
		if remainingFolders := folderCount - offset; remainingFolders < folderLimit {
			folderLimit = remainingFolders
		}
		folders, err := s.folderRepo.ListChildFolders(ctx, s.db, userID, input.FolderID, folderLimit, offset)
		if err != nil {
			return ListEntriesPageResult{}, err
		}
		result.Folders = folders

		remaining := input.PageSize - len(folders)
		if remaining > 0 {
			files, err := s.fileRepo.ListByFolder(ctx, s.db, userID, input.FolderID, remaining, 0)
			if err != nil {
				return ListEntriesPageResult{}, err
			}
			result.Files = files
		}
		return result, nil
	}

	fileOffset := offset - folderCount
	files, err := s.fileRepo.ListByFolder(ctx, s.db, userID, input.FolderID, input.PageSize, fileOffset)
	if err != nil {
		return ListEntriesPageResult{}, err
	}
	result.Files = files
	return result, nil
}

// ListEntriesCursor serves the file browser API without counting all entries.
func (s *Service) ListEntriesCursor(ctx context.Context, input ListEntriesCursorInput) (ListEntriesCursorResult, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return ListEntriesCursorResult{}, ErrInvalidInput
	}
	if input.PageSize <= 0 {
		input.PageSize = defaultPageSize
	}
	if input.PageSize > maxPageSize {
		input.PageSize = maxPageSize
	}
	if input.FolderID != nil {
		folderID, err := validateUUIDValue(*input.FolderID)
		if err != nil {
			return ListEntriesCursorResult{}, err
		}
		input.FolderID = &folderID
	}
	cursor := strings.TrimSpace(input.Cursor)
	seekEntryID := strings.TrimSpace(input.SeekEntryID)
	if seekEntryID != "" {
		if cursor != "" {
			return ListEntriesCursorResult{}, ErrInvalidInput
		}
		seekEntryID, err := validateUUIDValue(seekEntryID)
		if err != nil {
			return ListEntriesCursorResult{}, ErrInvalidInput
		}
		cursor, err = s.cursorForEntry(ctx, userID, input.FolderID, seekEntryID)
		if err != nil {
			return ListEntriesCursorResult{}, err
		}
	}
	return s.listEntriesByCursor(ctx, userID, input.FolderID, input.PageSize, cursor)
}

type listEntriesCursor struct {
	FolderID  string `json:"folderId"`
	Stage     string `json:"stage"`
	CreatedAt string `json:"createdAt,omitempty"`
	ID        string `json:"id,omitempty"`
	Inclusive bool   `json:"inclusive,omitempty"`
}

type listEntriesCursorState struct {
	stage          string
	afterCreatedAt *time.Time
	afterID        *string
	inclusive      bool
}

func decodeListEntriesCursor(value, folderID string) (listEntriesCursorState, error) {
	if value == "" {
		return listEntriesCursorState{stage: "folders"}, nil
	}
	if len(value) > 2048 {
		return listEntriesCursorState{}, ErrInvalidInput
	}

	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	var cursor listEntriesCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.FolderID != folderID {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	if cursor.Stage != "folders" && cursor.Stage != "files" {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	if (cursor.CreatedAt == "") != (cursor.ID == "") {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	if cursor.Inclusive && cursor.CreatedAt == "" {
		return listEntriesCursorState{}, ErrInvalidInput
	}

	state := listEntriesCursorState{stage: cursor.Stage, inclusive: cursor.Inclusive}
	if cursor.CreatedAt == "" {
		if cursor.Stage == "folders" {
			return listEntriesCursorState{}, ErrInvalidInput
		}
		return state, nil
	}

	createdAt, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	if err != nil {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	id, err := validateUUIDValue(cursor.ID)
	if err != nil {
		return listEntriesCursorState{}, ErrInvalidInput
	}
	state.afterCreatedAt = &createdAt
	state.afterID = &id
	return state, nil
}

func encodeListEntriesCursor(folderID, stage string, createdAt *time.Time, id string, inclusive bool) string {
	cursor := listEntriesCursor{FolderID: folderID, Stage: stage, Inclusive: inclusive}
	if createdAt != nil {
		cursor.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		cursor.ID = id
	}
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func (s *Service) cursorForEntry(ctx context.Context, userID string, folderID *string, entryID string) (string, error) {
	createdAt, err := s.fileRepo.GetListedFileCreatedAt(ctx, s.db, userID, folderID, entryID)
	stage := "files"
	if errors.Is(err, pgx.ErrNoRows) {
		createdAt, err = s.folderRepo.GetChildFolderCreatedAt(ctx, s.db, userID, folderID, entryID)
		stage = "folders"
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEntryNotFound
	}
	if err != nil {
		return "", err
	}
	folderScope := ""
	if folderID != nil {
		folderScope = *folderID
	}
	return encodeListEntriesCursor(folderScope, stage, &createdAt, entryID, true), nil
}

func (s *Service) listEntriesByCursor(ctx context.Context, userID string, folderID *string, pageSize int, value string) (ListEntriesCursorResult, error) {
	folderScope := ""
	if folderID != nil {
		folderScope = *folderID
	}
	cursor, err := decodeListEntriesCursor(value, folderScope)
	if err != nil {
		return ListEntriesCursorResult{}, err
	}

	result := ListEntriesCursorResult{
		FolderID: folderID,
		Path:     []models.Folder{},
		Folders:  []models.Folder{},
		Files:    []models.File{},
	}
	if folderID != nil {
		path, err := s.folderPath(ctx, s.db, userID, *folderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ListEntriesCursorResult{}, ErrNotFound
			}
			return ListEntriesCursorResult{}, err
		}
		result.Path = path
	}

	if cursor.stage == "folders" {
		folders, err := s.folderRepo.ListChildFoldersAfter(ctx, s.db, userID, folderID, pageSize+1, cursor.afterCreatedAt, cursor.afterID, cursor.inclusive)
		if err != nil {
			return ListEntriesCursorResult{}, err
		}
		if len(folders) > pageSize {
			folders = folders[:pageSize]
			result.Folders = folders
			last := folders[len(folders)-1]
			next := encodeListEntriesCursor(folderScope, "folders", &last.CreatedAt, last.ID, false)
			result.NextCursor = &next
			result.HasMore = true
			return result, nil
		}
		result.Folders = folders
	}

	remaining := pageSize - len(result.Folders)
	fileLimit := remaining + 1
	if fileLimit < 1 {
		fileLimit = 1
	}
	var afterCreatedAt *time.Time
	var afterID *string
	if cursor.stage == "files" {
		afterCreatedAt = cursor.afterCreatedAt
		afterID = cursor.afterID
	}
	files, err := s.fileRepo.ListByFolderAfter(ctx, s.db, userID, folderID, fileLimit, afterCreatedAt, afterID, cursor.inclusive)
	if err != nil {
		return ListEntriesCursorResult{}, err
	}
	if len(files) > remaining {
		result.HasMore = true
		if remaining == 0 {
			next := encodeListEntriesCursor(folderScope, "files", nil, "", false)
			result.NextCursor = &next
			return result, nil
		}
		files = files[:remaining]
		last := files[len(files)-1]
		next := encodeListEntriesCursor(folderScope, "files", &last.CreatedAt, last.ID, false)
		result.NextCursor = &next
	}
	result.Files = files
	return result, nil
}

func (s *Service) ValidateFolderAccess(ctx context.Context, userID, folderID string) (models.Folder, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return models.Folder{}, ErrInvalidInput
	}
	var err error
	folderID, err = validateUUIDValue(folderID)
	if err != nil {
		return models.Folder{}, err
	}

	folder, err := s.folderRepo.GetForUser(ctx, s.db, userID, folderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Folder{}, ErrNotFound
		}
		return models.Folder{}, err
	}
	return folder, nil
}

func (s *Service) RenameFolder(ctx context.Context, input RenameFolderInput) error {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" || len(input.EncryptedName) == 0 || len(input.EncryptedMetadata) == 0 {
		return ErrInvalidInput
	}
	searchTokens, err := filessvc.NormalizeSearchTokens(input.SearchTokens, filessvc.MaxSearchTokensPerFile)
	if err != nil {
		return ErrInvalidInput
	}

	folderID, err := validateUUIDValue(input.FolderID)
	if err != nil {
		return err
	}

	folder, err := s.folderRepo.GetForUser(ctx, s.db, userID, folderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	renamed, err := s.folderRepo.RenameFolderForUser(ctx, tx, userID, folderID, input.EncryptedName, input.EncryptedMetadata)
	if err != nil {
		return err
	}
	if !renamed {
		return ErrNotFound
	}
	if err := s.folderRepo.ReplaceSearchTokensForFolder(ctx, tx, userID, folder.VaultID, folderID, searchTokens); err != nil {
		return err
	}
	if len(input.ShareEntries) > 100 {
		return ErrInvalidInput
	}
	seenShares := make(map[string]struct{}, len(input.ShareEntries))
	for _, entry := range input.ShareEntries {
		shareID := strings.TrimSpace(entry.ShareID)
		if shareID == "" || len(entry.EncryptedName) == 0 || len(entry.EncryptedMetadata) == 0 {
			return ErrInvalidInput
		}
		if _, exists := seenShares[shareID]; exists {
			return ErrInvalidInput
		}
		seenShares[shareID] = struct{}{}
		count, err := s.shareRepo.AddFolderShareEntries(ctx, tx, shareID, nil, []sharerepo.FolderShareFolderEntry{{
			ID: folderID, EncryptedName: entry.EncryptedName, EncryptedMetadata: entry.EncryptedMetadata,
		}})
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) SearchFoldersByTokens(ctx context.Context, userID, vaultID string, tokenHashes [][]byte, minimumMatches, limit int) ([]models.Folder, error) {
	userID = strings.TrimSpace(userID)
	vaultID = strings.TrimSpace(vaultID)
	if userID == "" || vaultID == "" {
		return nil, ErrInvalidInput
	}
	if len(tokenHashes) == 0 {
		return []models.Folder{}, nil
	}
	if len(tokenHashes) > filessvc.MaxSearchQueryTokens {
		return nil, ErrInvalidInput
	}
	if minimumMatches < 1 || minimumMatches > len(tokenHashes) {
		return nil, ErrInvalidInput
	}
	return s.folderRepo.SearchFoldersForTokens(ctx, s.db, userID, vaultID, tokenHashes, minimumMatches, limit)
}

func (s *Service) folderPath(ctx context.Context, db database.PgExecutor, userID, folderID string) ([]models.Folder, error) {
	path := []models.Folder{}
	currentID := folderID
	for depth := 0; strings.TrimSpace(currentID) != "" && depth < maxFolderDepth; depth++ {
		folder, err := s.folderRepo.GetForUser(ctx, db, userID, currentID)
		if err != nil {
			return nil, err
		}
		path = append(path, folder)
		if folder.ParentFolderID == nil {
			break
		}
		currentID = strings.TrimSpace(*folder.ParentFolderID)
	}
	if len(path) >= maxFolderDepth && currentID != "" {
		return nil, ErrInvalidInput
	}

	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, nil
}

func (s *Service) MoveEntries(ctx context.Context, input MoveEntriesInput) error {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return ErrInvalidInput
	}
	if len(input.Files) == 0 && len(input.Folders) == 0 {
		return ErrInvalidInput
	}
	if len(input.Files)+len(input.Folders) > maxMoveEntries {
		return ErrInvalidInput
	}
	fileIDs := make([]string, len(input.Files))
	encryptedFileKeys := make([][]byte, len(input.Files))
	seenFiles := make(map[string]struct{}, len(input.Files))
	for i, file := range input.Files {
		id, err := validateUUIDValue(file.ID)
		if err != nil || len(file.EncryptedFileKey) == 0 {
			return ErrInvalidInput
		}
		if _, exists := seenFiles[id]; exists {
			return ErrInvalidInput
		}
		seenFiles[id] = struct{}{}
		fileIDs[i], encryptedFileKeys[i] = id, file.EncryptedFileKey
	}
	folderIDs := make([]string, len(input.Folders))
	encryptedNames := make([][]byte, len(input.Folders))
	encryptedMetadata := make([][]byte, len(input.Folders))
	seenFolders := make(map[string]struct{}, len(input.Folders))
	for i, folder := range input.Folders {
		id, err := validateUUIDValue(folder.ID)
		if err != nil || len(folder.EncryptedName) == 0 || len(folder.EncryptedMetadata) == 0 {
			return ErrInvalidInput
		}
		if _, exists := seenFolders[id]; exists {
			return ErrInvalidInput
		}
		seenFolders[id] = struct{}{}
		folderIDs[i] = id
		encryptedNames[i], encryptedMetadata[i] = folder.EncryptedName, folder.EncryptedMetadata
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var targetFolderID *string
	if input.TargetFolderID != nil {
		targetID, err := validateUUIDValue(*input.TargetFolderID)
		if err != nil {
			return err
		}
		if _, err := s.folderRepo.GetForUser(ctx, tx, userID, targetID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		targetFolderID = &targetID
	}

	if len(folderIDs) > 0 && targetFolderID != nil {
		for _, folderID := range folderIDs {
			if folderID == *targetFolderID {
				return ErrInvalidMove
			}
		}
		isDescendant, err := s.folderRepo.TargetIsDescendant(ctx, tx, userID, folderIDs, *targetFolderID)
		if err != nil {
			return err
		}
		if isDescendant {
			return ErrInvalidMove
		}
	}

	if len(folderIDs) > 0 {
		// Re-encrypt moved folder context because its authenticated parent ID changes.
		moved, err := s.folderRepo.MoveFolders(ctx, tx, userID, folderIDs, encryptedNames, encryptedMetadata, targetFolderID)
		if err != nil {
			return err
		}
		if moved != int64(len(folderIDs)) {
			return ErrNotFound
		}
	}

	if len(fileIDs) > 0 {
		// A file key is authenticated against its direct parent, so moves must replace its wrapped key.
		moved, err := s.fileRepo.MoveFilesToFolder(ctx, tx, userID, fileIDs, encryptedFileKeys, targetFolderID)
		if err != nil {
			return err
		}
		if moved != int64(len(fileIDs)) {
			return ErrNotFound
		}
	}

	return tx.Commit(ctx)
}

func (s *Service) resolveDeleteScope(ctx context.Context, db database.PgExecutor, input ResolveDeleteScopeInput) (DeleteScope, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return DeleteScope{}, ErrInvalidInput
	}

	fileIDs := uniqueNonEmpty(input.FileIDs)
	folderIDs := uniqueNonEmpty(input.FolderIDs)
	if len(fileIDs) == 0 && len(folderIDs) == 0 {
		return DeleteScope{}, ErrInvalidInput
	}

	var err error
	fileIDs, err = validateUUIDList(fileIDs)
	if err != nil {
		return DeleteScope{}, err
	}
	folderIDs, err = validateUUIDList(folderIDs)
	if err != nil {
		return DeleteScope{}, err
	}

	explicitFiles := make([]string, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		file, getErr := s.fileRepo.GetFileForUser(ctx, db, fileID, userID)
		if getErr != nil {
			if errors.Is(getErr, pgx.ErrNoRows) {
				return DeleteScope{}, ErrNotFound
			}
			return DeleteScope{}, getErr
		}
		if file.UploadStatus != "complete" || file.ExpiresAt != nil {
			return DeleteScope{}, ErrNotFound
		}
		explicitFiles = append(explicitFiles, file.ID)
	}

	resolvedFolderIDs := []string{}
	folderFileIDs := []string{}
	if len(folderIDs) > 0 {
		resolvedFolderIDs, err = s.folderRepo.DescendantFolderIDs(ctx, db, userID, folderIDs)
		if err != nil {
			return DeleteScope{}, err
		}
		if len(resolvedFolderIDs) == 0 {
			return DeleteScope{}, ErrNotFound
		}

		resolvedSet := make(map[string]struct{}, len(resolvedFolderIDs))
		for _, folderID := range resolvedFolderIDs {
			resolvedSet[folderID] = struct{}{}
		}
		for _, folderID := range folderIDs {
			if _, ok := resolvedSet[folderID]; !ok {
				return DeleteScope{}, ErrNotFound
			}
		}

		folderFileIDs, err = s.folderRepo.FileIDsInFolders(ctx, db, userID, resolvedFolderIDs)
		if err != nil {
			return DeleteScope{}, err
		}
	}

	return DeleteScope{
		FolderIDs: uniqueNonEmpty(resolvedFolderIDs),
		FileIDs:   uniqueNonEmpty(append(explicitFiles, folderFileIDs...)),
	}, nil
}

func (s *Service) DeleteEntries(ctx context.Context, input DeleteEntriesInput) (DeleteEntriesResult, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return DeleteEntriesResult{}, ErrInvalidInput
	}
	if s.filesSvc == nil {
		return DeleteEntriesResult{}, ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DeleteEntriesResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scope, err := s.resolveDeleteScope(ctx, tx, ResolveDeleteScopeInput{
		UserID:    userID,
		FileIDs:   input.FileIDs,
		FolderIDs: input.FolderIDs,
	})
	if err != nil {
		return DeleteEntriesResult{}, err
	}

	deletedFiles := make([]models.File, 0)
	if len(scope.FileIDs) > 0 {
		deletedFiles, err = s.filesSvc.DeleteFilesWithinTx(ctx, tx, userID, scope.FileIDs)
		if err != nil {
			return DeleteEntriesResult{}, err
		}
	}

	deletedFolders := 0
	if len(scope.FolderIDs) > 0 {
		affected, err := s.folderRepo.SoftDeleteFolders(ctx, tx, userID, scope.FolderIDs)
		if err != nil {
			return DeleteEntriesResult{}, err
		}
		if affected != int64(len(scope.FolderIDs)) {
			return DeleteEntriesResult{}, ErrNotFound
		}
		deletedFolders = int(affected)
	}

	if err := tx.Commit(ctx); err != nil {
		return DeleteEntriesResult{}, err
	}

	return DeleteEntriesResult{
		DeletedFiles:   len(deletedFiles),
		DeletedFolders: deletedFolders,
	}, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func validateUUIDList(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		validated, err := validateUUIDValue(value)
		if err != nil {
			return nil, err
		}
		out = append(out, validated)
	}
	return out, nil
}

func validateUUIDValue(value string) (string, error) {
	normalized, ok := validation.NormalizeUUID(value)
	if !ok {
		return "", ErrInvalidInput
	}
	return normalized, nil
}
