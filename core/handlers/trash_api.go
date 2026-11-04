package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	folderssvc "arkive/core/services/folders"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
)

type trashEntriesResponse struct {
	Entries    []models.TrashEntry `json:"entries"`
	NextCursor *string             `json:"nextCursor"`
	HasMore    bool                `json:"hasMore"`
}

type trashSelectionRequest struct {
	FileIDs   []string `json:"fileIds"`
	FolderIDs []string `json:"folderIds"`
}

func APIListTrash(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		limit := 0
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				apierror.InvalidPayload(c)
				return
			}
			limit = parsed
		}
		result, err := folderService.ListTrash(c.Request.Context(), folderssvc.ListTrashInput{
			UserID: userID.(string),
			Cursor: c.Query("cursor"),
			Limit:  limit,
		})
		if err != nil {
			if errors.Is(err, folderssvc.ErrInvalidInput) {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Trash listing failed")
			return
		}
		c.JSON(http.StatusOK, trashEntriesResponse{
			Entries:    result.Entries,
			NextCursor: result.NextCursor,
			HasMore:    result.HasMore,
		})
	}
}

func APIRestoreTrash(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		var req trashSelectionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		result, err := folderService.RestoreTrash(c.Request.Context(), folderssvc.RestoreTrashInput{
			UserID:    userID.(string),
			FileIDs:   req.FileIDs,
			FolderIDs: req.FolderIDs,
		})
		if err != nil {
			switch {
			case errors.Is(err, folderssvc.ErrInvalidInput):
				apierror.InvalidPayload(c)
			case errors.Is(err, folderssvc.ErrNotFound):
				apierror.Write(c, http.StatusNotFound, "trash_entry_not_found", "Trash entry not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Restore failed")
			}
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func APIPermanentlyDeleteTrash(folderService *folderssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		var req trashSelectionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		result, err := folderService.PermanentlyDeleteTrash(c.Request.Context(), folderssvc.PermanentlyDeleteTrashInput{
			UserID:    userID.(string),
			FileIDs:   req.FileIDs,
			FolderIDs: req.FolderIDs,
		})
		if err != nil {
			switch {
			case errors.Is(err, folderssvc.ErrInvalidInput):
				apierror.InvalidPayload(c)
			case errors.Is(err, folderssvc.ErrNotFound):
				apierror.Write(c, http.StatusNotFound, "trash_entry_not_found", "Trash entry not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Permanent delete failed")
			}
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
