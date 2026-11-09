package handlers

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"arkive/core/models"
	sharerepo "arkive/core/repositories/shares"
	"arkive/core/services/shares"
	"arkive/pkg/apierror"
	"arkive/pkg/errs"
	"arkive/pkg/validation"
)

type shareCreateRequest struct {
	Token                    string `json:"token"`
	ExpiresAt                string `json:"expiresAt"`
	Password                 string `json:"password"`
	AllowPreview             bool   `json:"allowPreview"`
	AllowDownload            bool   `json:"allowDownload"`
	BurnAfterRead            bool   `json:"burnAfterRead"`
	EncryptedShareKey        string `json:"encryptedShareKey"`
	EncryptedFileKeyForShare string `json:"encryptedFileKeyForShare"`
	EncryptedFolderName      string `json:"encryptedFolderName"`
	EncryptedFolderMetadata  string `json:"encryptedFolderMetadata"`
}

type shareUpdateRequest struct {
	ExpiresAt       string `json:"expiresAt"`
	Password        string `json:"password"`
	RequirePassword bool   `json:"requirePassword"`
	AllowPreview    *bool  `json:"allowPreview"`
	AllowDownload   *bool  `json:"allowDownload"`
	BurnAfterRead   *bool  `json:"burnAfterRead"`
}

func APICreateShare(svc *shares.Service) gin.HandlerFunc {
	return apiCreateShare(svc, false)
}

func APICreateFolderShare(svc *shares.Service) gin.HandlerFunc {
	return apiCreateShare(svc, true)
}

func apiCreateShare(svc *shares.Service, folderShare bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		entryID := strings.TrimSpace(c.Param("id"))
		if entryID == "" {
			apierror.InvalidPayload(c)
			return
		}

		var req shareCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		var expiresAt *time.Time
		if strings.TrimSpace(req.ExpiresAt) != "" {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
			if err != nil {
				errors := validation.New()
				errors.Add("expiresAt", "expiry must be RFC3339")
				apierror.Validation(c, errors)
				return
			}
			expiresAt = &parsed
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		if strings.TrimSpace(req.EncryptedShareKey) != "" {
			if _, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedShareKey)); err != nil {
				apierror.InvalidPayload(c)
				return
			}
		}
		if strings.TrimSpace(req.EncryptedFileKeyForShare) != "" {
			if _, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.EncryptedFileKeyForShare)); err != nil {
				apierror.InvalidPayload(c)
				return
			}
		}
		fileID, folderID := entryID, ""
		if folderShare {
			fileID, folderID = "", entryID
		}

		share, validationErrors, err := svc.CreateShare(c.Request.Context(), shares.CreateInput{
			FileID:                   fileID,
			FolderID:                 folderID,
			OwnerUserID:              userID.(string),
			Token:                    req.Token,
			Password:                 req.Password,
			ExpiresAt:                expiresAt,
			AllowPreview:             req.AllowPreview,
			AllowDownload:            req.AllowDownload,
			BurnAfterRead:            req.BurnAfterRead,
			EncryptedShareKey:        req.EncryptedShareKey,
			EncryptedFileKeyForShare: req.EncryptedFileKeyForShare,
			EncryptedFolderName:      req.EncryptedFolderName,
			EncryptedFolderMetadata:  req.EncryptedFolderMetadata,
		})
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrNotFound:
				if folderShare {
					apierror.Write(c, http.StatusNotFound, "folder_not_found", "Folder not found", nil)
				} else {
					apierror.Write(c, http.StatusNotFound, "file_not_found", "File not found", nil)
				}
			case shares.ErrShareExists:
				apierror.Write(c, http.StatusConflict, "share_already_exists", "Share already exists", nil)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case shares.ErrPasswordHashFailed:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Password hashing failed")
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share create failed")
			}
			return
		}
		if validationErrors != nil && validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}

		status, expired := shareStatus(share)
		c.JSON(http.StatusCreated, gin.H{
			"id":                share.ID,
			"itemType":          share.ItemType,
			"fileId":            share.FileID,
			"folderId":          share.FolderID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"accessCount":       share.AccessCount,
			"maxAccessCount":    share.MaxAccessCount,
			"consumedAt":        share.ConsumedAt,
			"expiresAt":         share.ExpiresAt,
			"hasPassword":       share.PasswordHash != nil,
			"status":            status,
			"isExpired":         expired,
			"createdAt":         share.CreatedAt,
		})
	}
}

type folderShareFileEntryRequest struct {
	ID                       string `json:"id"`
	EncryptedFileKeyForShare string `json:"encryptedFileKeyForShare"`
}

type folderShareFolderEntryRequest struct {
	ID                string `json:"id"`
	EncryptedName     string `json:"encryptedName"`
	EncryptedMetadata string `json:"encryptedMetadata"`
}

func APIAddFolderShareEntries(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Files   []folderShareFileEntryRequest   `json:"files"`
			Folders []folderShareFolderEntryRequest `json:"folders"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.Files)+len(req.Folders) == 0 || len(req.Files)+len(req.Folders) > 250 {
			apierror.InvalidPayload(c)
			return
		}
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		files := make([]sharerepo.FolderShareFileEntry, 0, len(req.Files))
		for _, item := range req.Files {
			key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item.EncryptedFileKeyForShare))
			if err != nil || len(key) == 0 || strings.TrimSpace(item.ID) == "" {
				apierror.InvalidPayload(c)
				return
			}
			files = append(files, sharerepo.FolderShareFileEntry{ID: strings.TrimSpace(item.ID), EncryptedFileKeyForShare: key})
		}
		folders := make([]sharerepo.FolderShareFolderEntry, 0, len(req.Folders))
		for _, item := range req.Folders {
			name, nameErr := base64.StdEncoding.DecodeString(strings.TrimSpace(item.EncryptedName))
			metadata, metadataErr := base64.StdEncoding.DecodeString(strings.TrimSpace(item.EncryptedMetadata))
			if nameErr != nil || metadataErr != nil || len(name) == 0 || len(metadata) == 0 || strings.TrimSpace(item.ID) == "" {
				apierror.InvalidPayload(c)
				return
			}
			folders = append(folders, sharerepo.FolderShareFolderEntry{
				ID: strings.TrimSpace(item.ID), EncryptedName: name, EncryptedMetadata: metadata,
			})
		}
		if err := svc.AddFolderShareEntries(c.Request.Context(), c.Param("id"), userID.(string), files, folders); err != nil {
			writeFolderShareError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func APICompleteFolderShareSetup(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		shareID := c.Param("id")
		if err := svc.CompleteFolderShareSetup(c.Request.Context(), shareID, userID.(string)); err != nil {
			writeFolderShareError(c, err)
			return
		}
		share, err := svc.GetShareForUser(c.Request.Context(), shareID, userID.(string))
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Folder share lookup failed")
			return
		}
		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id": share.ID, "itemType": share.ItemType, "folderId": share.FolderID,
			"token": share.Token, "encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview": share.AllowPreview, "allowDownload": share.AllowDownload,
			"burnAfterRead": share.BurnAfterRead, "accessCount": share.AccessCount,
			"maxAccessCount": share.MaxAccessCount, "consumedAt": share.ConsumedAt,
			"expiresAt": share.ExpiresAt, "hasPassword": share.PasswordHash != nil,
			"status": status, "isExpired": expired, "createdAt": share.CreatedAt,
		})
	}
}

func APIListFolderShareScopes(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		items, err := svc.ListFolderShareScopes(c.Request.Context(), userID.(string), c.Param("id"))
		if err != nil {
			writeFolderShareError(c, err)
			return
		}
		response := make([]gin.H, len(items))
		for index, item := range items {
			response[index] = gin.H{
				"id": item.ID, "token": item.Token,
				"encryptedShareKey": base64.StdEncoding.EncodeToString(item.EncryptedShareKey),
			}
		}
		c.JSON(http.StatusOK, gin.H{"shares": response})
	}
}

func writeFolderShareError(c *gin.Context, err error) {
	switch err {
	case shares.ErrUnauthorized:
		apierror.Unauthorized(c)
	case shares.ErrNotFound:
		apierror.Write(c, http.StatusNotFound, "share_not_found", "Share or folder not found", nil)
	case shares.ErrSetupIncomplete:
		apierror.Write(c, http.StatusConflict, "share_setup_incomplete", "Folder share setup is incomplete", nil)
	case shares.ErrInvalidInput:
		apierror.InvalidPayload(c)
	default:
		_ = c.Error(errs.WithStack(err))
		apierror.Internal(c, "Folder share operation failed")
	}
}

func APIUpdateShare(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := strings.TrimSpace(c.Param("id"))
		if shareID == "" {
			apierror.InvalidPayload(c)
			return
		}

		var req shareUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			apierror.InvalidPayload(c)
			return
		}

		var expiresAt *time.Time
		if strings.TrimSpace(req.ExpiresAt) != "" {
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
			if err != nil {
				errors := validation.New()
				errors.Add("expiresAt", "expiry must be RFC3339")
				apierror.Validation(c, errors)
				return
			}
			expiresAt = &parsed
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		share, validationErrors, err := svc.UpdateShareForUser(c.Request.Context(), shareID, userID.(string), expiresAt, req.Password, req.RequirePassword, req.BurnAfterRead, req.AllowPreview, req.AllowDownload)
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case shares.ErrPasswordHashFailed:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Password hashing failed")
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share update failed")
			}
			return
		}
		if validationErrors != nil && validationErrors.HasAny() {
			apierror.Validation(c, validationErrors)
			return
		}

		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id":                share.ID,
			"itemType":          share.ItemType,
			"fileId":            share.FileID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"accessCount":       share.AccessCount,
			"maxAccessCount":    share.MaxAccessCount,
			"consumedAt":        share.ConsumedAt,
			"expiresAt":         share.ExpiresAt,
			"hasPassword":       share.PasswordHash != nil,
			"status":            status,
			"isExpired":         expired,
			"createdAt":         share.CreatedAt,
		})
	}
}

func APIGetShareForFolder(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		folderID := strings.TrimSpace(c.Param("id"))
		userID, ok := c.Get("user_id")
		if folderID == "" {
			apierror.InvalidPayload(c)
			return
		}
		if !ok {
			apierror.Unauthorized(c)
			return
		}
		share, err := svc.GetShareForFolderForUser(c.Request.Context(), folderID, userID.(string))
		if err != nil {
			if err == shares.ErrUnauthorized {
				apierror.Unauthorized(c)
			} else {
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			}
			return
		}
		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id": share.ID, "itemType": share.ItemType, "folderId": share.FolderID,
			"token": share.Token, "encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview": share.AllowPreview, "allowDownload": share.AllowDownload,
			"burnAfterRead": share.BurnAfterRead, "accessCount": share.AccessCount,
			"maxAccessCount": share.MaxAccessCount, "consumedAt": share.ConsumedAt,
			"expiresAt": share.ExpiresAt, "hasPassword": share.PasswordHash != nil,
			"status": status, "isExpired": expired, "createdAt": share.CreatedAt,
		})
	}
}

func APIGetShareForFile(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		fileID := strings.TrimSpace(c.Param("id"))
		if fileID == "" {
			apierror.InvalidPayload(c)
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		share, err := svc.GetShareForFileForUser(c.Request.Context(), fileID, userID.(string))
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			}
			return
		}

		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id":                share.ID,
			"itemType":          share.ItemType,
			"fileId":            share.FileID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"accessCount":       share.AccessCount,
			"maxAccessCount":    share.MaxAccessCount,
			"consumedAt":        share.ConsumedAt,
			"expiresAt":         share.ExpiresAt,
			"hasPassword":       share.PasswordHash != nil,
			"status":            status,
			"isExpired":         expired,
			"createdAt":         share.CreatedAt,
		})
	}
}

func APIListShares(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		items, err := svc.ListSharesForUser(c.Request.Context(), userID.(string))
		if err != nil {
			_ = c.Error(errs.WithStack(err))
			apierror.Internal(c, "Share list failed")
			return
		}

		result := make([]gin.H, 0, len(items))
		for _, share := range items {
			status, expired := shareStatus(share.Share)
			vaultID := share.FileVaultID
			parentFolderID := share.FileFolderID
			isTrashed := share.FileDeletedAt != nil
			if share.ItemType == "folder" {
				vaultID = share.FolderVaultID
				parentFolderID = share.FolderParentID
				isTrashed = share.FolderDeletedAt != nil
			}
			result = append(result, gin.H{
				"id":                      share.ID,
				"itemType":                share.ItemType,
				"fileId":                  share.FileID,
				"folderId":                share.FolderID,
				"vaultId":                 vaultID,
				"parentFolderId":          parentFolderID,
				"encryptedMetadata":       base64.StdEncoding.EncodeToString(share.EncryptedFileMetadata),
				"encryptedFileKey":        base64.StdEncoding.EncodeToString(share.EncryptedFileKey),
				"encryptedFolderName":     base64.StdEncoding.EncodeToString(share.EncryptedFolderName),
				"encryptedFolderMetadata": base64.StdEncoding.EncodeToString(share.EncryptedFolderMetadata),
				"isTrashed":               isTrashed,
				"allowPreview":            share.AllowPreview,
				"allowDownload":           share.AllowDownload,
				"burnAfterRead":           share.BurnAfterRead,
				"accessCount":             share.AccessCount,
				"maxAccessCount":          share.MaxAccessCount,
				"expiresAt":               share.ExpiresAt,
				"hasPassword":             share.PasswordHash != nil,
				"status":                  status,
				"isExpired":               expired,
				"createdAt":               share.CreatedAt,
				"updatedAt":               share.UpdatedAt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"shares": result})
	}
}

func APIGetShareCryptoRecord(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := strings.TrimSpace(c.Param("id"))
		if shareID == "" {
			apierror.InvalidPayload(c)
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		share, err := svc.GetShareForUser(c.Request.Context(), shareID, userID.(string))
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share lookup failed")
			}
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"id":                share.ID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"hasPassword":       share.PasswordHash != nil,
		})
	}
}

func APIRevokeShare(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := strings.TrimSpace(c.Param("id"))
		if shareID == "" {
			apierror.InvalidPayload(c)
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		share, err := svc.RevokeShareForUser(c.Request.Context(), shareID, userID.(string))
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case shares.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share revoke failed")
			}
			return
		}

		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id":                share.ID,
			"fileId":            share.FileID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"accessCount":       share.AccessCount,
			"maxAccessCount":    share.MaxAccessCount,
			"consumedAt":        share.ConsumedAt,
			"expiresAt":         share.ExpiresAt,
			"hasPassword":       share.PasswordHash != nil,
			"status":            status,
			"isExpired":         expired,
			"revokedAt":         share.RevokedAt,
		})
	}
}

func APIActivateShare(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := strings.TrimSpace(c.Param("id"))
		if shareID == "" {
			apierror.InvalidPayload(c)
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		share, err := svc.ActivateShareForUser(c.Request.Context(), shareID, userID.(string))
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			case shares.ErrNotFound:
				apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share activate failed")
			}
			return
		}

		status, expired := shareStatus(share)
		c.JSON(http.StatusOK, gin.H{
			"id":                share.ID,
			"fileId":            share.FileID,
			"token":             share.Token,
			"encryptedShareKey": base64.StdEncoding.EncodeToString(share.EncryptedShareKey),
			"allowPreview":      share.AllowPreview,
			"allowDownload":     share.AllowDownload,
			"burnAfterRead":     share.BurnAfterRead,
			"accessCount":       share.AccessCount,
			"maxAccessCount":    share.MaxAccessCount,
			"consumedAt":        share.ConsumedAt,
			"expiresAt":         share.ExpiresAt,
			"hasPassword":       share.PasswordHash != nil,
			"status":            status,
			"isExpired":         expired,
			"revokedAt":         share.RevokedAt,
		})
	}
}

func APIDeleteShare(svc *shares.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		shareID := strings.TrimSpace(c.Param("id"))
		if shareID == "" {
			apierror.InvalidPayload(c)
			return
		}

		userID, ok := c.Get("user_id")
		if !ok {
			apierror.Unauthorized(c)
			return
		}

		deleted, err := svc.DeleteShareForUser(c.Request.Context(), shareID, userID.(string))
		if err != nil {
			switch err {
			case shares.ErrUnauthorized:
				apierror.Unauthorized(c)
			case shares.ErrInvalidInput:
				apierror.InvalidPayload(c)
			default:
				_ = c.Error(errs.WithStack(err))
				apierror.Internal(c, "Share delete failed")
			}
			return
		}
		if !deleted {
			apierror.Write(c, http.StatusNotFound, "share_not_found", "Share not found", nil)
			return
		}

		c.Status(http.StatusNoContent)
	}
}

func shareStatus(share models.Share) (string, bool) {
	if share.Status == "burned" {
		return "burned", false
	}
	if share.Status == shares.ShareStatusRevoked {
		return shares.ShareStatusRevoked, false
	}
	if share.ExpiresAt != nil && !share.ExpiresAt.After(time.Now()) {
		return "expired", true
	}
	return shares.ShareStatusActive, false
}
