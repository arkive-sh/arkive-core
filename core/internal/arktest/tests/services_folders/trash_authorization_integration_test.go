//go:build integration

package folders_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	storagerepo "arkive/core/repositories/storage"
	filesvc "arkive/core/services/files"
	foldersvc "arkive/core/services/folders"
)

func TestTrashRestoreAndPermanentDeleteRequireTheOwningUser(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	owner, err := arktest.CreateVerifiedUser(ctx, env.DB, "Trash Owner", "trashowner", "trashowner@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := arktest.CreateVerifiedUser(ctx, env.DB, "Other User", "trashother", "trashother@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	fileRepo := filerepo.New()
	folderRepo := folderrepo.New()
	shareRepo := sharerepo.New()
	fileService := filesvc.NewService(env.DB, storagerepo.New(), fileRepo, shareRepo, env.Storage, filesvc.Config{})
	folderService := foldersvc.NewService(env.DB, folderRepo, fileRepo, shareRepo, fileService)

	root, err := folderService.CreateFolder(ctx, foldersvc.CreateFolderInput{
		ID: "33333333-3333-4333-8333-333333333333", UserID: owner.ID, VaultID: owner.ID,
		EncryptedName: []byte("root-name"), EncryptedMetadata: []byte("root-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := folderService.CreateFolder(ctx, foldersvc.CreateFolderInput{
		ID: "44444444-4444-4444-8444-444444444444", UserID: owner.ID, VaultID: owner.ID,
		ParentFolderID: &root.ID, EncryptedName: []byte("child-name"), EncryptedMetadata: []byte("child-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = fileRepo.CreateEncryptedFile(ctx, env.DB, models.File{
		UserID: owner.ID, FolderID: &root.ID, EncryptedMetadata: []byte("file-metadata"),
		EncryptedFileKey: []byte("file-key"), EncryptedManifest: []byte("manifest"),
		EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, ActualEncryptedSize: 34, UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := folderService.RenameFolder(ctx, foldersvc.RenameFolderInput{
		UserID: other.ID, FolderID: root.ID, EncryptedName: []byte("stolen-name"), EncryptedMetadata: []byte("stolen-metadata"),
	}); !errors.Is(err, foldersvc.ErrNotFound) {
		t.Fatalf("rename another user's folder = %v, want ErrNotFound", err)
	}
	if err := folderService.MoveEntries(ctx, foldersvc.MoveEntriesInput{
		UserID: other.ID, Folders: []foldersvc.MoveFolderInput{{ID: root.ID, EncryptedName: []byte("stolen-name"), EncryptedMetadata: []byte("stolen-metadata")}},
	}); !errors.Is(err, foldersvc.ErrNotFound) {
		t.Fatalf("move another user's folder = %v, want ErrNotFound", err)
	}
	if _, err := folderService.DeleteEntries(ctx, foldersvc.DeleteEntriesInput{UserID: other.ID, FolderIDs: []string{root.ID}}); !errors.Is(err, foldersvc.ErrNotFound) {
		t.Fatalf("trash another user's folder = %v, want ErrNotFound", err)
	}

	deleted, err := folderService.DeleteEntries(ctx, foldersvc.DeleteEntriesInput{UserID: owner.ID, FolderIDs: []string{root.ID}})
	if err != nil || deleted.DeletedFiles != 1 || deleted.DeletedFolders != 2 {
		t.Fatalf("trash owned folder tree = %+v, error %v", deleted, err)
	}
	if _, err := folderService.RestoreTrash(ctx, foldersvc.RestoreTrashInput{UserID: other.ID, FolderIDs: []string{root.ID}}); !errors.Is(err, foldersvc.ErrNotFound) {
		t.Fatalf("restore another user's folder = %v, want ErrNotFound", err)
	}
	if _, err := folderService.PermanentlyDeleteTrash(ctx, foldersvc.PermanentlyDeleteTrashInput{UserID: other.ID, FolderIDs: []string{root.ID}}); !errors.Is(err, foldersvc.ErrNotFound) {
		t.Fatalf("permanently delete another user's folder = %v, want ErrNotFound", err)
	}
	if entries, err := folderService.ListTrash(ctx, foldersvc.ListTrashInput{UserID: other.ID, Limit: 10}); err != nil || len(entries.Entries) != 0 {
		t.Fatalf("other user's trash = %+v, error %v; want no visible entries", entries, err)
	}

	restored, err := folderService.RestoreTrash(ctx, foldersvc.RestoreTrashInput{UserID: owner.ID, FolderIDs: []string{root.ID}})
	if err != nil || restored.RestoredFiles != 1 || restored.RestoredFolders != 2 {
		t.Fatalf("owner restore after denied requests = %+v, error %v", restored, err)
	}
	entries, err := folderService.ListEntriesPage(ctx, foldersvc.ListEntriesPageInput{UserID: owner.ID, FolderID: &root.ID, Page: 1, PageSize: 10})
	if err != nil || len(entries.Files) != 1 || len(entries.Folders) != 1 || entries.Folders[0].ID != child.ID {
		t.Fatalf("owner's restored folder tree = %+v, error %v", entries, err)
	}
}
