package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	filessvc "arkive/core/services/files"
	folderssvc "arkive/core/services/folders"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
	"arkive/pkg/pagination"
)

type createFolderRequest struct {
	// The client assigns this before encryption so it can authenticate the folder ID.
	ID                string                          `json:"id"`
	ParentFolderID    *string                         `json:"parentFolderId"`
	EncryptedName     string                          `json:"encryptedName"`
	EncryptedMetadata string                          `json:"encryptedMetadata"`
	SearchTokens      []searchTokenRequest            `json:"searchTokens"`
	ShareEntries      []createFolderShareEntryRequest `json:"shareEntries"`
}

type createFolderShareEntryRequest struct {
	ShareID           string `json:"shareId"`
	EncryptedName     string `json:"encryptedName"`
	EncryptedMetadata string `json:"encryptedMetadata"`
}

type moveEntriesRequest struct {
	TargetFolderID *string             `json:"targetFolderId"`
	Files          []moveFileRequest   `json:"files"`
	Folders        []moveFolderRequest `json:"folders"`
}

type moveFileRequest struct {
	ID               string `json:"id"`
	EncryptedFileKey string `json:"encryptedFileKey"`
}

type moveFolderRequest struct {
	ID                string `json:"id"`
	EncryptedName     string `json:"encryptedName"`
	EncryptedMetadata string `json:"encryptedMetadata"`
}

type deleteEntriesRequest struct {
	FileIDs   []string `json:"fileIds"`
	FolderIDs []string `json:"folderIds"`
}

type renameEntryRequest struct {
	Type              string                          `json:"type"`
	ID                string                          `json:"id"`
	EncryptedName     string                          `json:"encryptedName"`
	EncryptedMetadata string                          `json:"encryptedMetadata"`
	SearchTokens      []searchTokenRequest            `json:"searchTokens"`
	ShareEntries      []createFolderShareEntryRequest `json:"shareEntries"`
}

type createFolderResponse struct {
	ID                string  `json:"id"`
	ParentFolderID    *string `json:"parentFolderId"`
	EncryptedName     string  `json:"encryptedName"`
	EncryptedMetadata string  `json:"encryptedMetadata,omitempty"`
}

type deleteEntriesResponse struct {
	DeletedFiles   int `json:"deletedFiles"`
	DeletedFolders int `json:"deletedFolders"`
}

type folderEntrySummary struct {
	ID                string
	VaultID           string
	ParentFolderID    *string
	EncryptedName     []byte
	EncryptedMetadata []byte
	SizeBytes         *int64 `json:"SizeBytes,omitempty"`
	UpdatedAt         time.Time
}

type fileEntrySummary struct {
	ID                string
	FolderID          *string
	EncryptedMetadata []byte
	EncryptedFileKey  []byte
	UpdatedAt         time.Time
	IsShared          bool
}

// Cursor pages avoid a total count and return only fields used by the file browser.
type cursorFolderEntriesResponse struct {
	FolderID   *string
	Path       []folderEntrySummary
	Folders    []folderEntrySummary
	Files      []fileEntrySummary
	NextCursor *string
	HasMore    bool
}

func APICreateFolder(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		var req createFolderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		encryptedName, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedName))
		if err != nil || len(encryptedName) == 0 {
			apierror.InvalidPayload(c)
			return
		}

		var encryptedMetadata []byte
		if strings.TrimSpace(req.EncryptedMetadata) != "" {
			encryptedMetadata, err = base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedMetadata))
			if err != nil {
				apierror.InvalidPayload(c)
				return
			}
		}
		searchTokens, err := decodeSearchTokens(req.SearchTokens)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		shareEntries, err := decodeFolderShareEntries(req.ShareEntries)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}

		folder, err := folderService.CreateFolder(c.Request.Context(), folderssvc.CreateFolderInput{
			ID:                req.ID,
			UserID:            userID.(string),
			VaultID:           userID.(string),
			ParentFolderID:    req.ParentFolderID,
			EncryptedName:     encryptedName,
			EncryptedMetadata: encryptedMetadata,
			SearchTokens:      searchTokens,
			ShareEntries:      shareEntries,
		})
		if err != nil {
			switch err {
			case folderssvc.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case folderssvc.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "parent_folder_not_found", "Parent folder not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Create folder failed")
			}
			return
		}

		resp := createFolderResponse{
			ID:             folder.ID,
			ParentFolderID: folder.ParentFolderID,
			EncryptedName:  base64.StdEncoding.EncodeToString(folder.EncryptedName),
		}
		if len(folder.EncryptedMetadata) > 0 {
			resp.EncryptedMetadata = base64.StdEncoding.EncodeToString(folder.EncryptedMetadata)
		}
		c.JSON(http.StatusCreated, gin.H{"folder": resp})
	}
}

func decodeFolderShareEntries(input []createFolderShareEntryRequest) ([]folderssvc.CreateFolderShareEntry, error) {
	if len(input) > 100 {
		return nil, folderssvc.ErrInvalidInput
	}
	entries := make([]folderssvc.CreateFolderShareEntry, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		shareID := strings.TrimSpace(item.ShareID)
		name, nameErr := base64.StdEncoding.DecodeString(strings.TrimSpace(item.EncryptedName))
		metadata, metadataErr := base64.StdEncoding.DecodeString(strings.TrimSpace(item.EncryptedMetadata))
		if shareID == "" || nameErr != nil || metadataErr != nil || len(name) == 0 || len(metadata) == 0 {
			return nil, folderssvc.ErrInvalidInput
		}
		if _, exists := seen[shareID]; exists {
			return nil, folderssvc.ErrInvalidInput
		}
		seen[shareID] = struct{}{}
		entries = append(entries, folderssvc.CreateFolderShareEntry{
			ShareID: shareID, EncryptedName: name, EncryptedMetadata: metadata,
		})
	}
	return entries, nil
}

func APIListRootFolderEntries(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiListFolderEntries(c, folderService, nil)
	}
}

func APIListFolderEntries(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		folderID := strings.TrimSpace(c.Param("id"))
		if folderID == "" {
			apierror.Write(c, http.StatusNotFound, "folder_not_found", "Folder not found", nil)
			return
		}
		apiListFolderEntries(c, folderService, &folderID)
	}
}

func apiListFolderEntries(c *gin.Context, folderService *folderssvc.Service, folderID *string) {
	userID, ok := c.Get("user_id")
	if !ok {
		apierror.Unauthorized(c)
		return
	}

	cursorValue := strings.TrimSpace(c.Query("cursor"))
	if c.Query("page") != "" {
		apierror.InvalidPayload(c)
		return
	}
	result, err := folderService.ListEntriesCursor(c.Request.Context(), folderssvc.ListEntriesCursorInput{
		UserID:      userID.(string),
		FolderID:    folderID,
		PageSize:    pagination.ParsePageSizeParam(c.DefaultQuery("pageSize", "50")),
		Cursor:      cursorValue,
		SeekEntryID: strings.TrimSpace(c.Query("seekEntryId")),
	})
	if err != nil {
		switch err {
		case folderssvc.ErrInvalidInput:
			apierror.InvalidPayload(c)
		case folderssvc.ErrNotFound:
			apierror.Write(c, http.StatusNotFound, "folder_not_found", "Folder not found", nil)
		case folderssvc.ErrEntryNotFound:
			apierror.Write(c, http.StatusNotFound, "entry_not_found", "That entry is no longer in this folder.", nil)
		default:
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "List entries failed")
		}
		return
	}

	c.JSON(http.StatusOK, cursorFolderEntriesResponse{
		FolderID:   result.FolderID,
		Path:       folderEntrySummaries(result.Path),
		Folders:    folderEntrySummaries(result.Folders),
		Files:      fileEntrySummaries(result.Files),
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	})
}

func folderEntrySummaries(folders []models.Folder) []folderEntrySummary {
	entries := make([]folderEntrySummary, len(folders))
	for i, folder := range folders {
		entries[i] = folderEntrySummary{
			ID:                folder.ID,
			VaultID:           folder.VaultID,
			ParentFolderID:    folder.ParentFolderID,
			EncryptedName:     folder.EncryptedName,
			EncryptedMetadata: folder.EncryptedMetadata,
			SizeBytes:         folder.SizeBytes,
			UpdatedAt:         folder.UpdatedAt,
		}
	}
	return entries
}

func fileEntrySummaries(files []models.File) []fileEntrySummary {
	entries := make([]fileEntrySummary, len(files))
	for i, file := range files {
		entries[i] = fileEntrySummary{
			ID:                file.ID,
			FolderID:          file.FolderID,
			EncryptedMetadata: file.EncryptedMetadata,
			EncryptedFileKey:  file.EncryptedFileKey,
			UpdatedAt:         file.UpdatedAt,
			IsShared:          file.IsShared,
		}
	}
	return entries
}

func APIMoveEntries(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		var req moveEntriesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		moveFiles := make([]folderssvc.MoveFileInput, 0, len(req.Files))
		for _, file := range req.Files {
			encryptedFileKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(file.EncryptedFileKey))
			if err != nil || len(encryptedFileKey) == 0 {
				apierror.InvalidPayload(c)
				return
			}
			moveFiles = append(moveFiles, folderssvc.MoveFileInput{
				ID: file.ID, EncryptedFileKey: encryptedFileKey,
			})
		}
		moveFolders := make([]folderssvc.MoveFolderInput, 0, len(req.Folders))
		for _, folder := range req.Folders {
			encryptedName, nameErr := base64.StdEncoding.DecodeString(strings.TrimSpace(folder.EncryptedName))
			encryptedMetadata, metadataErr := base64.StdEncoding.DecodeString(strings.TrimSpace(folder.EncryptedMetadata))
			if nameErr != nil || metadataErr != nil || len(encryptedName) == 0 || len(encryptedMetadata) == 0 {
				apierror.InvalidPayload(c)
				return
			}
			moveFolders = append(moveFolders, folderssvc.MoveFolderInput{
				ID: folder.ID, EncryptedName: encryptedName, EncryptedMetadata: encryptedMetadata,
			})
		}

		if err := folderService.MoveEntries(c.Request.Context(), folderssvc.MoveEntriesInput{
			UserID:         userID.(string),
			TargetFolderID: req.TargetFolderID,
			Files:          moveFiles,
			Folders:        moveFolders,
		}); err != nil {
			switch err {
			case folderssvc.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case folderssvc.ErrInvalidMove:
				apierror.Write(c, http.StatusBadRequest, "invalid_move", "Invalid move", nil)
			case folderssvc.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "entry_not_found", "Entry not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Move failed")
			}
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func APIDeleteEntries(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		var req deleteEntriesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		result, err := folderService.DeleteEntries(c.Request.Context(), folderssvc.DeleteEntriesInput{
			UserID:    userID.(string),
			FileIDs:   req.FileIDs,
			FolderIDs: req.FolderIDs,
		})
		if err != nil {
			switch {
			case errors.Is(err, folderssvc.ErrInvalidInput):
				apierror.InvalidPayload(c)
			case errors.Is(err, folderssvc.ErrNotFound):
				apierror.Write(c, http.StatusNotFound, "entry_not_found", "Entry not found", nil)
			case errors.Is(err, filessvc.ErrUploadCancelled):
				apierror.Write(c, http.StatusConflict, "upload_in_progress", "Upload in progress", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Delete failed")
			}
			return
		}

		c.JSON(http.StatusOK, deleteEntriesResponse{
			DeletedFiles:   result.DeletedFiles,
			DeletedFolders: result.DeletedFolders,
		})
	}
}

func APIRenameEntry(folderService *folderssvc.Service, filesService *filessvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		var req renameEntryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		encryptedMetadata, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedMetadata))
		if err != nil || len(encryptedMetadata) == 0 {
			apierror.InvalidPayload(c)
			return
		}

		switch strings.TrimSpace(req.Type) {
		case "file":
			searchTokens, err := decodeSearchTokens(req.SearchTokens)
			if err != nil {
				apierror.InvalidPayload(c)
				return
			}
			if err := filesService.RenameFile(c.Request.Context(), filessvc.RenameFileInput{
				UserID:            userID.(string),
				FileID:            strings.TrimSpace(req.ID),
				EncryptedMetadata: encryptedMetadata,
				SearchTokens:      searchTokens,
			}); err != nil {
				switch {
				case errors.Is(err, filessvc.ErrInvalidInput):
					apierror.InvalidPayload(c)
				case errors.Is(err, filessvc.ErrNotFound):
					apierror.Write(c, http.StatusNotFound, "entry_not_found", "Entry not found", nil)
				case errors.Is(err, filessvc.ErrUploadCancelled):
					apierror.Write(c, http.StatusConflict, "upload_in_progress", "Upload in progress", nil)
				default:
					_ = c.Error(errs.WithStack(err))
					apierror.Internal(c, "Rename failed")
				}
				return
			}
		case "folder":
			searchTokens, decodeErr := decodeSearchTokens(req.SearchTokens)
			if decodeErr != nil {
				apierror.InvalidPayload(c)
				return
			}
			encryptedName, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedName))
			if decodeErr != nil || len(encryptedName) == 0 {
				apierror.InvalidPayload(c)
				return
			}
			shareEntries, decodeErr := decodeFolderShareEntries(req.ShareEntries)
			if decodeErr != nil {
				apierror.InvalidPayload(c)
				return
			}
			if err := folderService.RenameFolder(c.Request.Context(), folderssvc.RenameFolderInput{
				UserID:            userID.(string),
				FolderID:          strings.TrimSpace(req.ID),
				EncryptedName:     encryptedName,
				EncryptedMetadata: encryptedMetadata,
				SearchTokens:      searchTokens,
				ShareEntries:      shareEntries,
			}); err != nil {
				switch {
				case errors.Is(err, folderssvc.ErrInvalidInput):
					apierror.InvalidPayload(c)
				case errors.Is(err, folderssvc.ErrNotFound):
					apierror.Write(c, http.StatusNotFound, "entry_not_found", "Entry not found", nil)
				default:
					_ = c.Error(errs.WithStack(err))
					apierror.Internal(c, "Rename failed")
				}
				return
			}
		default:
			apierror.InvalidPayload(c)
			return
		}

		c.Status(http.StatusNoContent)
	}
}
