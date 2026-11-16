//go:build integration

package files_test

import (
	. "arkive/core/services/files"
	"bytes"
	"context"
	"testing"
	"time"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	storagerepo "arkive/core/repositories/storage"
)

func TestFileSearchRespectsOwnerFolderScopeMatchThresholdAndTrashScope(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const (
		rootID     = "22222222-2222-4222-8222-222222222222"
		childID    = "33333333-3333-4333-8333-333333333333"
		wrongVault = "55555555-5555-4555-8555-555555555555"
	)
	owner, err := arktest.CreateVerifiedUser(ctx, env.DB, "Search User", "searchowner", "searchowner@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := arktest.CreateVerifiedUser(ctx, env.DB, "Search User", "searchother", "searchother@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID, otherID := owner.ID, other.ID

	fileRepo := filerepo.New()
	folderRepo := folderrepo.New()
	root, err := folderRepo.Create(ctx, env.DB, models.Folder{
		ID: rootID, UserID: userID, VaultID: userID,
		EncryptedName: []byte("root-name"), EncryptedMetadata: []byte("root-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := folderRepo.Create(ctx, env.DB, models.Folder{
		ID: childID, UserID: userID, VaultID: userID, ParentFolderID: &root.ID,
		EncryptedName: []byte("child-name"), EncryptedMetadata: []byte("child-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	createFile := func(ownerID string, folderID *string) models.File {
		t.Helper()
		file, err := fileRepo.CreateEncryptedFile(ctx, env.DB, models.File{
			UserID: ownerID, FolderID: folderID, EncryptedMetadata: []byte("encrypted-name"),
			EncryptedFileKey: []byte("wrapped-key"), EncryptedManifest: []byte("manifest"),
			EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, UploadStatus: "complete",
		})
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	insideFile := createFile(userID, &child.ID)
	outsideFile := createFile(userID, nil)
	trashedFile := createFile(userID, &child.ID)
	otherUserFile := createFile(otherID, nil)
	if _, err := env.DB.Exec(ctx, `UPDATE files SET deleted_at = now() WHERE id = $1`, trashedFile.ID); err != nil {
		t.Fatal(err)
	}

	tokenHashes := [][]byte{bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)}
	indexedTokens := []models.FileSearchToken{
		{TokenHash: tokenHashes[0], Field: "name", Weight: 3},
		{TokenHash: tokenHashes[1], Field: "name", Weight: 1},
	}
	for _, file := range []models.File{insideFile, outsideFile, trashedFile, otherUserFile} {
		if err := fileRepo.InsertSearchTokensForFile(ctx, env.DB, file.UserID, userID, file.ID, indexedTokens); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(env.DB, storagerepo.New(), fileRepo, sharerepo.New(), env.Storage, Config{})
	search := func(folderID *string, includeTrash bool, vaultID string) []models.File {
		t.Helper()
		files, err := service.SearchFilesByTokens(ctx, userID, vaultID, tokenHashes, 2, 20, folderID, includeTrash)
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	ids := func(files []models.File) map[string]bool {
		result := make(map[string]bool, len(files))
		for _, file := range files {
			result[file.ID] = true
		}
		return result
	}

	global := ids(search(nil, false, userID))
	if len(global) != 2 || !global[insideFile.ID] || !global[outsideFile.ID] {
		t.Fatalf("global search results = %v; want only active owner's two matching files", global)
	}
	folderScoped := ids(search(&root.ID, false, userID))
	if len(folderScoped) != 1 || !folderScoped[insideFile.ID] {
		t.Fatalf("recursive folder search results = %v; want nested file only", folderScoped)
	}
	withTrash := ids(search(nil, true, userID))
	if len(withTrash) != 1 || !withTrash[trashedFile.ID] {
		t.Fatalf("Trash search results = %v; want the trashed file only", withTrash)
	}
	wrongVaultResults := search(nil, false, wrongVault)
	if len(wrongVaultResults) != 0 {
		t.Fatalf("search in another vault returned %d files, want none", len(wrongVaultResults))
	}
	if _, err := service.SearchFilesByTokens(ctx, userID, userID, tokenHashes, 2, 20, &root.ID, true); err != ErrInvalidInput {
		t.Fatalf("folder search including Trash = %v, want ErrInvalidInput", err)
	}
}
