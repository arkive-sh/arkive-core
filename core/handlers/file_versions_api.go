package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	filessvc "arkive/core/services/files"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
)

func writeFileVersionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, filessvc.ErrUnauthorized):
		apierror.Unauthorized(c)
	case errors.Is(err, filessvc.ErrInvalidInput):
		apierror.InvalidPayload(c)
	case errors.Is(err, filessvc.ErrNotFound):
		apierror.Write(c, http.StatusNotFound, "file_version_not_found", "File version not found", nil)
	case errors.Is(err, filessvc.ErrConflict):
		apierror.Write(c, http.StatusConflict, "file_changed", "File changed while restoring. Refresh and try again.", nil)
	default:
		_ = c.Error(errs.WithStack(err))
		apierror.Internal(c, "File version request failed")
	}
}

func APIListFileVersions(svc *filessvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		history, err := svc.ListFileVersions(c.Request.Context(), userID.(string), c.Param("id"))
		if err != nil {
			writeFileVersionError(c, err)
			return
		}
		versions := make([]gin.H, 0, len(history.Versions))
		for _, version := range history.Versions {
			versions = append(versions, gin.H{
				"id":                  version.ID,
				"versionNumber":       version.VersionNumber,
				"actualEncryptedSize": version.ActualEncryptedSize,
				"thumbnailSizeBytes":  version.ThumbnailSizeBytes,
				"createdAt":           version.CreatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"currentVersion": history.CurrentVersion, "versions": versions})
	}
}

func APIFileVersionRecord(svc *filessvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		record, err := svc.GetEncryptedFileVersionRecord(c.Request.Context(), userID.(string), c.Param("id"), c.Param("versionID"))
		if err != nil {
			writeFileVersionError(c, err)
			return
		}
		c.JSON(http.StatusOK, fileVersionRecordBody(record))
	}
}

func fileVersionRecordBody(record filessvc.EncryptedFileRecord) gin.H {
	return gin.H{
		"fileId":            record.FileID,
		"vaultId":           record.VaultID,
		"folderId":          record.FolderID,
		"encryptionVersion": record.EncryptionVersion,
		"chunkSize":         record.ChunkSize,
		"totalChunks":       record.TotalChunks,
		"encryptedHash":     record.EncryptedHash,
		"encryptedMetadata": record.EncryptedMetadata,
		"encryptedFileKey":  record.EncryptedFileKey,
		"encryptedManifest": record.EncryptedManifest,
		"sourceUrl":         record.SourceURL,
	}
}

type restoreFileVersionRequest struct {
	FolderID          *string              `json:"folderId"`
	EncryptedMetadata string               `json:"encryptedMetadata"`
	EncryptedFileKey  string               `json:"encryptedFileKey"`
	SearchTokens      []searchTokenRequest `json:"searchTokens"`
}

func APIRestoreFileVersion(svc *filessvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		var req restoreFileVersionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}
		encryptedMetadata, metadataErr := base64.StdEncoding.DecodeString(req.EncryptedMetadata)
		encryptedFileKey, keyErr := base64.StdEncoding.DecodeString(req.EncryptedFileKey)
		if metadataErr != nil || keyErr != nil || len(encryptedMetadata) == 0 || len(encryptedFileKey) == 0 {
			apierror.InvalidPayload(c)
			return
		}
		searchTokens, err := decodeSearchTokens(req.SearchTokens)
		if err != nil {
			apierror.InvalidPayload(c)
			return
		}
		if err := svc.RestoreFileVersion(c.Request.Context(), userID.(string), c.Param("id"), c.Param("versionID"), req.FolderID, encryptedMetadata, encryptedFileKey, searchTokens); err != nil {
			writeFileVersionError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func APIDeleteFileVersion(svc *filessvc.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		if err := svc.DeleteFileVersion(c.Request.Context(), userID.(string), c.Param("id"), c.Param("versionID")); err != nil {
			writeFileVersionError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
