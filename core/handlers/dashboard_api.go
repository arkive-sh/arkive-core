package handlers

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	filessvc "arkive/core/services/files"
	settingssvc "arkive/core/services/settings"
	"arkive/pkg/apierror"
	appcontext "arkive/pkg/context"
	"arkive/pkg/errs"
)

type dashboardRecentFile struct {
	ID                string    `json:"id"`
	FolderID          *string   `json:"folderId"`
	VaultID           string    `json:"vaultId"`
	EncryptedMetadata string    `json:"encryptedMetadata"`
	EncryptedFileKey  string    `json:"encryptedFileKey"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type dashboardResponse struct {
	TotalFiles      int                   `json:"totalFiles"`
	UsedBytes       int64                 `json:"usedBytes"`
	MaxStorageBytes int64                 `json:"maxStorageBytes"`
	RecentFiles     []dashboardRecentFile `json:"recentFiles"`
}

func APIDashboard(filesService *filessvc.Service, settingsService *settingssvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := appcontext.UserFromContext(c)
		if !ok || user.ID == "" {
			apierror.Unauthorized(c)
			return
		}
		if err := filesService.TouchUserActivity(c.Request.Context(), user.ID); err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Dashboard data failed")
			return
		}

		list, err := filesService.ListCompletedUploads(c.Request.Context(), user.ID, 1, 4)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Dashboard data failed")
			return
		}
		storageSettings, err := settingsService.StorageSettings(c.Request.Context())
		if err != nil {
			storageSettings = models.StorageSettings{}
		}

		recentFiles := make([]dashboardRecentFile, 0, len(list.Files))
		for _, file := range list.Files {
			recentFiles = append(recentFiles, dashboardRecentFile{
				ID:                file.ID,
				FolderID:          file.FolderID,
				VaultID:           user.ID,
				EncryptedMetadata: base64.StdEncoding.EncodeToString(file.EncryptedMetadata),
				EncryptedFileKey:  base64.StdEncoding.EncodeToString(file.EncryptedFileKey),
				UpdatedAt:         file.UpdatedAt,
			})
		}

		c.JSON(http.StatusOK, dashboardResponse{
			TotalFiles:      list.TotalFiles,
			UsedBytes:       user.UsedBytes,
			MaxStorageBytes: storageSettings.MaxStorageBytes,
			RecentFiles:     recentFiles,
		})
	}
}
