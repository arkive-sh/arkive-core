package handlers

import (
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	filessvc "arkive/core/services/files"
	folderssvc "arkive/core/services/folders"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/validation"
)

type searchResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Meta     string `json:"meta,omitempty"`
	Status   string `json:"status,omitempty"`
	URL      string `json:"url"`
	Category string `json:"category"`
}

type encryptedSearchFileResult struct {
	ID                string  `json:"id"`
	Kind              string  `json:"kind"`
	FolderID          *string `json:"folderId"`
	VaultID           string  `json:"vaultId"`
	EncryptedMetadata string  `json:"encryptedMetadata"`
	EncryptedFileKey  string  `json:"encryptedFileKey"`
	Score             int64   `json:"score"`
	IsTrashed         bool    `json:"isTrashed"`
	URL               string  `json:"url"`
}

type encryptedSearchFolderResult struct {
	ID                string  `json:"id"`
	Kind              string  `json:"kind"`
	ParentFolderID    *string `json:"parentFolderId"`
	VaultID           string  `json:"vaultId"`
	EncryptedName     string  `json:"encryptedName"`
	EncryptedMetadata string  `json:"encryptedMetadata"`
	Score             int64   `json:"score"`
	IsTrashed         bool    `json:"isTrashed"`
	IsTrashContainer  bool    `json:"isTrashContainer"`
	URL               string  `json:"url"`
}

type searchRequest struct {
	Tokens   []string `json:"tokens"`
	Limit    int      `json:"limit"`
	Scopes   []string `json:"scopes"`
	FolderID *string  `json:"folderId"`
}

const maxSearchCandidates = 40

func APISearch(filesService *filessvc.Service, folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}

		var req searchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		if len(req.Tokens) == 0 {
			c.JSON(http.StatusOK, gin.H{"results": gin.H{}})
			return
		}
		if len(req.Scopes) == 0 {
			req.Scopes = []string{"allFiles"}
		}
		if len(req.Scopes) > 3 {
			apierror.InvalidPayload(c)
			return
		}
		scopes := make(map[string]bool, len(req.Scopes))
		for _, scope := range req.Scopes {
			switch scope {
			case "allFiles", "thisFolder", "trash":
				scopes[scope] = true
			default:
				apierror.InvalidPayload(c)
				return
			}
		}
		if scopes["allFiles"] && scopes["thisFolder"] {
			apierror.InvalidPayload(c)
			return
		}
		if scopes["thisFolder"] {
			if req.FolderID == nil {
				apierror.InvalidPayload(c)
				return
			}
			folderID, ok := validation.NormalizeUUID(*req.FolderID)
			if !ok {
				apierror.InvalidPayload(c)
				return
			}
			req.FolderID = &folderID
		} else if req.FolderID != nil {
			apierror.InvalidPayload(c)
			return
		}
		if len(req.Tokens) > filessvc.MaxSearchQueryTokens {
			apierror.InvalidPayload(c)
			return
		}

		tokenHashes := make([][]byte, 0, len(req.Tokens))
		seenTokens := make(map[string]struct{}, len(req.Tokens))
		for _, token := range req.Tokens {
			decoded, err := filessvc.DecodeSearchTokenString(token)
			if err != nil {
				apierror.InvalidPayload(c)
				return
			}
			if _, exists := seenTokens[string(decoded)]; exists {
				continue
			}
			seenTokens[string(decoded)] = struct{}{}
			tokenHashes = append(tokenHashes, decoded)
		}
		if len(tokenHashes) == 0 {
			c.JSON(http.StatusOK, gin.H{"results": gin.H{}})
			return
		}

		limit := req.Limit
		if limit <= 0 {
			limit = 20
		} else if limit > maxSearchCandidates {
			limit = maxSearchCandidates
		}
		// Require every blind query token; the browser verifies contiguous text after decryption.
		minimumMatches := len(tokenHashes)

		files := make([]models.File, 0, limit)
		folders := make([]models.Folder, 0, limit)
		searchScope := func(folderID *string, inTrash bool) error {
			matchedFiles, err := filesService.SearchFilesByTokens(c.Request.Context(), user.ID, user.ID, tokenHashes, minimumMatches, limit, folderID, inTrash)
			if err != nil {
				return err
			}
			matchedFolders, err := folderService.SearchFoldersByTokens(c.Request.Context(), user.ID, user.ID, tokenHashes, minimumMatches, limit, folderID, inTrash)
			if err != nil {
				return err
			}
			files = append(files, matchedFiles...)
			folders = append(folders, matchedFolders...)
			return nil
		}
		for _, scope := range []string{"allFiles", "thisFolder", "trash"} {
			if !scopes[scope] {
				continue
			}
			var folderID *string
			inTrash := scope == "trash"
			if scope == "thisFolder" {
				folderID = req.FolderID
			}
			if err := searchScope(folderID, inTrash); err != nil {
				if err == filessvc.ErrInvalidInput || err == folderssvc.ErrInvalidInput {
					apierror.InvalidPayload(c)
					return
				}
				apierror.Internal(c, "Search failed")
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"results": gin.H{
				"folders": mapEncryptedFolderResults(folders),
				"files":   mapEncryptedFileResults(files),
				"shares":  []searchResult{},
			},
		})
	}
}

func mapEncryptedFolderResults(folders []models.Folder) []encryptedSearchFolderResult {
	results := make([]encryptedSearchFolderResult, 0, len(folders))
	for _, folder := range folders {
		results = append(results, encryptedSearchFolderResult{
			ID:                folder.ID,
			Kind:              "folder",
			ParentFolderID:    folder.ParentFolderID,
			VaultID:           folder.VaultID,
			EncryptedName:     base64.StdEncoding.EncodeToString(folder.EncryptedName),
			EncryptedMetadata: base64.StdEncoding.EncodeToString(folder.EncryptedMetadata),
			Score:             folder.SearchScore,
			IsTrashed:         folder.DeletedAt != nil,
			IsTrashContainer:  folder.IsTrashContainer,
			URL:               "/folders/" + folder.ID,
		})
	}
	return results
}

func mapEncryptedFileResults(files []models.File) []encryptedSearchFileResult {
	results := make([]encryptedSearchFileResult, 0, len(files))
	for _, file := range files {
		results = append(results, encryptedSearchFileResult{
			ID:                file.ID,
			Kind:              "file",
			FolderID:          file.FolderID,
			VaultID:           file.UserID,
			EncryptedMetadata: base64.StdEncoding.EncodeToString(file.EncryptedMetadata),
			EncryptedFileKey:  base64.StdEncoding.EncodeToString(file.EncryptedFileKey),
			Score:             file.SearchScore,
			IsTrashed:         file.DeletedAt != nil,
			URL:               "/files/" + file.ID + "/view",
		})
	}
	return results
}
