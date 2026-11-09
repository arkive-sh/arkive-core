package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"arkive/core/models"
	filessvc "arkive/core/services/files"
	"arkive/core/services/shares"
	"arkive/pkg/apierror"
	"arkive/pkg/cookies"
	"arkive/pkg/errs"
)

const (
	shareAccessCookieName = "arkive_share_access"
	shareAccessTTL        = 15 * time.Minute
)

func APIPublicShareUnlock(shareService *shares.Service, cookieSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Param("token"))
		if token == "" {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}

		var input struct {
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		share, err := shareService.GetShareByToken(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share lookup failed")
			return
		}
		if share.Status == "burned" {
			apierror.Write(c, http.StatusGone, "share_consumed", "Share has already been used", nil)
			return
		}
		if share.Status != shares.ShareStatusActive || (share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now())) {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		if !share.AllowPreview && !share.AllowDownload {
			apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
			return
		}
		if share.PasswordHash == nil || hasShareAccess(c, share, cookieSecret) {
			c.Status(http.StatusNoContent)
			return
		}

		if input.Password == "" || bcrypt.CompareHashAndPassword([]byte(*share.PasswordHash), []byte(input.Password)) != nil {
			apierror.Write(c, http.StatusUnauthorized, "invalid_share_password", "Password is incorrect.", nil)
			return
		}

		setShareAccess(c, share, cookieSecret)
		c.Status(http.StatusNoContent)
	}
}

func APIPublicShareRecord(shareService *shares.Service, filesService *filessvc.Service, cookieSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Param("token"))
		if token == "" {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}

		share, err := shareService.GetShareByToken(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share lookup failed")
			return
		}
		if share.Status != shares.ShareStatusActive {
			if share.Status == "burned" {
				apierror.Write(c, http.StatusGone, "share_consumed", "Share has already been used", nil)
				return
			}
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		if share.PasswordHash != nil && !hasShareAccess(c, share, cookieSecret) {
			apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
			return
		}
		if !share.AllowPreview && !share.AllowDownload {
			apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
			return
		}
		if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now()) {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		if share.ItemType == "folder" {
			record, err := shareService.GetPublicFolderShareRecord(c.Request.Context(), token)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
					return
				}
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share record failed")
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"itemType": "folder", "shareId": record.ShareID, "token": record.Token,
				"folderId": record.FolderID, "vaultId": record.VaultID,
				"encryptedFolderName":     base64.StdEncoding.EncodeToString(record.EncryptedFolderName),
				"encryptedFolderMetadata": base64.StdEncoding.EncodeToString(record.EncryptedFolderMetadata),
				"allowPreview":            share.AllowPreview, "allowDownload": share.AllowDownload,
			})
			return
		}

		file, err := filesService.GetFileForShare(c.Request.Context(), share.FileID)
		if err != nil {
			switch err {
			case filessvc.ErrNotFound, filessvc.ErrUploadCancelled:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			case filessvc.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share lookup failed")
			}
			return
		}

		sourceURL, err := filesService.PresignShareSourceForFile(c.Request.Context(), file)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share source failed")
			return
		}

		record, err := shareService.GetPublicShareRecord(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share record failed")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"itemType":                 "file",
			"shareId":                  record.ShareID,
			"token":                    record.Token,
			"fileId":                   record.FileID,
			"vaultId":                  record.VaultID,
			"encryptionVersion":        record.EncryptionVersion,
			"chunkSize":                record.ChunkSize,
			"totalChunks":              record.TotalChunks,
			"encryptedHash":            base64.StdEncoding.EncodeToString(record.EncryptedHash),
			"encryptedMetadata":        base64.StdEncoding.EncodeToString(record.EncryptedMetadata),
			"encryptedManifest":        base64.StdEncoding.EncodeToString(record.EncryptedManifest),
			"encryptedFileKeyForShare": base64.StdEncoding.EncodeToString(record.EncryptedFileKeyForShare),
			"sourceUrl":                sourceURL,
			"allowPreview":             share.AllowPreview,
			"allowDownload":            share.AllowDownload,
			"burnAfterRead":            share.BurnAfterRead,
		})
	}
}

func APIPublicShareFolderEntries(shareService *shares.Service, cookieSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		share, ok := loadAuthorizedPublicShare(c, shareService, cookieSecret)
		if !ok {
			return
		}
		if share.ItemType != "folder" {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
		entries, next, err := shareService.ListPublicFolderShareEntries(
			c.Request.Context(), share.Token, c.Param("folderID"), c.Query("cursor"), pageSize)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, shares.ErrNotFound) {
				apierror.Write(c, http.StatusNotFound, "folder_not_found", "Folder not found", nil)
				return
			}
			if errors.Is(err, shares.ErrInvalidInput) {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Folder listing failed")
			return
		}
		folders := make([]gin.H, 0, len(entries))
		files := make([]gin.H, 0, len(entries))
		for _, entry := range entries {
			if entry.ItemType == "folder" {
				folders = append(folders, gin.H{
					"id": entry.ID, "parentFolderId": entry.ParentFolderID, "vaultId": entry.VaultID,
					"encryptedName":     base64.StdEncoding.EncodeToString(entry.EncryptedName),
					"encryptedMetadata": base64.StdEncoding.EncodeToString(entry.EncryptedMetadata),
					"updatedAt":         entry.UpdatedAt,
				})
			} else {
				files = append(files, gin.H{
					"id": entry.ID, "folderId": entry.ParentFolderID, "vaultId": entry.VaultID,
					"encryptedMetadata":        base64.StdEncoding.EncodeToString(entry.EncryptedFileMetadata),
					"encryptedFileKeyForShare": base64.StdEncoding.EncodeToString(entry.EncryptedFileKeyForShare),
					"updatedAt":                entry.UpdatedAt,
				})
			}
		}
		c.JSON(http.StatusOK, gin.H{"folders": folders, "files": files, "nextCursor": next})
	}
}

func APIPublicShareFolderFile(shareService *shares.Service, filesService *filessvc.Service, cookieSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		share, ok := loadAuthorizedPublicShare(c, shareService, cookieSecret)
		if !ok {
			return
		}
		if share.ItemType != "folder" {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		record, err := shareService.GetPublicFolderShareFileRecord(c.Request.Context(), share.Token, c.Param("fileID"))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "file_not_found", "File not found", nil)
				return
			}
			if errors.Is(err, shares.ErrInvalidInput) {
				apierror.InvalidPayload(c)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share file lookup failed")
			return
		}
		file, err := filesService.GetFileForShare(c.Request.Context(), record.FileID)
		if err != nil {
			apierror.Write(c, http.StatusNotFound, "file_not_found", "File not found", nil)
			return
		}
		sourceURL, err := filesService.PresignShareSourceForFile(c.Request.Context(), file)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share source failed")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"itemType": "file", "shareId": record.ShareID, "token": record.Token,
			"fileId": record.FileID, "vaultId": record.VaultID,
			"encryptionVersion": record.EncryptionVersion, "chunkSize": record.ChunkSize,
			"totalChunks":              record.TotalChunks,
			"encryptedHash":            base64.StdEncoding.EncodeToString(record.EncryptedHash),
			"encryptedMetadata":        base64.StdEncoding.EncodeToString(record.EncryptedMetadata),
			"encryptedManifest":        base64.StdEncoding.EncodeToString(record.EncryptedManifest),
			"encryptedFileKeyForShare": base64.StdEncoding.EncodeToString(record.EncryptedFileKeyForShare),
			"sourceUrl":                sourceURL, "allowPreview": share.AllowPreview,
			"allowDownload": share.AllowDownload,
		})
	}
}

func loadAuthorizedPublicShare(c *gin.Context, shareService *shares.Service, cookieSecret string) (models.Share, bool) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
		return models.Share{}, false
	}
	share, err := shareService.GetShareByToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return models.Share{}, false
		}
		_ = c.Error(errs.WithStack(err))
		apierror.Internal(c, "Share lookup failed")
		return models.Share{}, false
	}
	if share.Status == "burned" {
		apierror.Write(c, http.StatusGone, "share_consumed", "Share has already been used", nil)
		return models.Share{}, false
	}
	if share.Status != shares.ShareStatusActive || (share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now())) {
		apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
		return models.Share{}, false
	}
	if share.PasswordHash != nil && !hasShareAccess(c, share, cookieSecret) {
		apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
		return models.Share{}, false
	}
	if !share.AllowPreview && !share.AllowDownload {
		apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
		return models.Share{}, false
	}
	return share, true
}

func APIPublicShareConsume(shareService *shares.Service, filesService *filessvc.Service, cookieSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := strings.TrimSpace(c.Param("token"))
		if token == "" {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}

		share, err := shareService.GetShareByToken(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share lookup failed")
			return
		}
		if share.PasswordHash != nil && !hasShareAccess(c, share, cookieSecret) {
			apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
			return
		}
		if share.Status == "burned" {
			apierror.Write(c, http.StatusGone, "share_consumed", "Share has already been used", nil)
			return
		}
		if share.Status != shares.ShareStatusActive {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now()) {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}
		if !share.AllowPreview && !share.AllowDownload {
			apierror.Write(c, http.StatusForbidden, "forbidden", "Share access denied", nil)
			return
		}

		file, err := filesService.GetFileForShare(c.Request.Context(), share.FileID)
		if err != nil {
			switch err {
			case filessvc.ErrNotFound, filessvc.ErrUploadCancelled:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			case filessvc.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share lookup failed")
			}
			return
		}

		sourceURL, err := filesService.PresignShareSourceForFile(c.Request.Context(), file)
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share source failed")
			return
		}

		record, err := shareService.GetPublicShareRecord(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
				return
			}
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share record failed")
			return
		}

		consumedShare, consumed, err := shareService.ConsumeShareByToken(c.Request.Context(), token)
		if err != nil {
			switch err {
			case shares.ErrAlreadyConsumed:
				apierror.Write(c, http.StatusGone, "share_consumed", "Share has already been used", nil)
			case shares.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share consume failed")
			}
			return
		}
		if consumed {
			share = consumedShare
		}

		c.JSON(http.StatusOK, gin.H{
			"shareId":                  record.ShareID,
			"token":                    record.Token,
			"fileId":                   record.FileID,
			"vaultId":                  record.VaultID,
			"encryptionVersion":        record.EncryptionVersion,
			"chunkSize":                record.ChunkSize,
			"totalChunks":              record.TotalChunks,
			"encryptedHash":            base64.StdEncoding.EncodeToString(record.EncryptedHash),
			"encryptedMetadata":        base64.StdEncoding.EncodeToString(record.EncryptedMetadata),
			"encryptedManifest":        base64.StdEncoding.EncodeToString(record.EncryptedManifest),
			"encryptedFileKeyForShare": base64.StdEncoding.EncodeToString(record.EncryptedFileKeyForShare),
			"sourceUrl":                sourceURL,
			"allowPreview":             share.AllowPreview,
			"allowDownload":            share.AllowDownload,
			"burnAfterRead":            share.BurnAfterRead,
			"accessCount":              share.AccessCount,
			"maxAccessCount":           share.MaxAccessCount,
			"consumedAt":               share.ConsumedAt,
			"status":                   share.Status,
		})
	}
}

func hasShareAccess(c *gin.Context, share models.Share, cookieSecret string) bool {
	if share.PasswordHash == nil {
		return true
	}
	cookie, err := c.Request.Cookie(shareAccessCookieName)
	if err != nil || cookie == nil {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload := string(payloadBytes)
	payloadParts := strings.Split(payload, ":")
	if len(payloadParts) != 2 {
		return false
	}
	if payloadParts[0] != share.Token {
		return false
	}
	expiresUnix, err := strconv.ParseInt(payloadParts[1], 10, 64)
	if err != nil || expiresUnix <= time.Now().Unix() {
		return false
	}
	expected := signShareAccessPayload(payload, cookieSecret)
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

func setShareAccess(c *gin.Context, share models.Share, cookieSecret string) {
	if share.PasswordHash == nil {
		return
	}
	expiresAt := time.Now().Add(shareAccessTTL)
	payload := share.Token + ":" + strconv.FormatInt(expiresAt.Unix(), 10)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signShareAccessPayload(payload, cookieSecret)
	cookies.SetCustom(c, shareAccessCookieName, value, expiresAt)
}

func signShareAccessPayload(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
