//go:build integration

package folders_test

import (
	. "arkive/core/services/folders"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	storagerepo "arkive/core/repositories/storage"
	usersrepo "arkive/core/repositories/users"
	filesvc "arkive/core/services/files"
	sharesvc "arkive/core/services/shares"
	"arkive/pkg/storage"
)

func TestFileAndFolderMoveRenameTrashRestoreAndSharedFilePurgeThroughServices(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := arktest.CreateVerifiedUser(ctx, env.DB, "Trash User", "trashuser", "trashuser@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := arktest.CreateVerifiedUser(ctx, env.DB, "Other User", "otheruser", "otheruser@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID, otherID := user.ID, other.ID

	fileRepo := filerepo.New()
	folderRepo := folderrepo.New()
	shareRepo := sharerepo.New()
	deleteFailure := &deleteObjectsFailureStorage{Provider: env.Storage, err: errors.New("temporary object-store failure")}
	fileService := filesvc.NewService(env.DB, storagerepo.New(), fileRepo, shareRepo, deleteFailure, filesvc.Config{})
	folderService := NewService(env.DB, folderRepo, fileRepo, shareRepo, fileService)
	const encryptedSize int64 = 34
	file, err := fileRepo.CreateEncryptedFile(ctx, env.DB, models.File{
		UserID: userID, EncryptedMetadata: []byte("metadata-ciphertext"),
		EncryptedFileKey: []byte("file-key-ciphertext"), EncryptedManifest: []byte("manifest-ciphertext"),
		EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, ActualEncryptedSize: encryptedSize,
		UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	shareService := sharesvc.NewService(env.DB, fileRepo, folderRepo, shareRepo)
	share, validationErrors, err := shareService.CreateShare(ctx, sharesvc.CreateInput{
		FileID: file.ID, OwnerUserID: userID, Token: strings.Repeat("s", sharesvc.TokenMinLength),
		AllowPreview: true, EncryptedShareKey: "c2hhcmUta2V5", EncryptedFileKeyForShare: "c2hhcmUtZmlsZS1rZXk=",
	})
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("create file share before purge: validations=%v error=%v", validationErrors, err)
	}
	objectKey, err := storage.BuildObjectKey(userID, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	env.Storage.PutObject(objectKey, make([]byte, encryptedSize))
	if _, err := env.DB.Exec(ctx, `UPDATE users SET used_bytes = $2 WHERE id = $1`, userID, encryptedSize); err != nil {
		t.Fatal(err)
	}
	parent, err := folderService.CreateFolder(ctx, CreateFolderInput{
		ID: "33333333-3333-4333-8333-333333333333", UserID: userID, VaultID: userID,
		EncryptedName: []byte("parent-name-ciphertext"), EncryptedMetadata: []byte("parent-metadata-ciphertext"),
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := folderService.CreateFolder(ctx, CreateFolderInput{
		ID: "44444444-4444-4444-8444-444444444444", UserID: userID, VaultID: userID,
		EncryptedName: []byte("child-name-ciphertext"), EncryptedMetadata: []byte("child-metadata-ciphertext"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := folderService.MoveEntries(ctx, MoveEntriesInput{
		UserID: userID, TargetFolderID: &parent.ID,
		Folders: []MoveFolderInput{{ID: child.ID, EncryptedName: []byte("moved-child-name-ciphertext"), EncryptedMetadata: []byte("moved-child-metadata-ciphertext")}},
	}); err != nil {
		t.Fatal(err)
	}
	movedChild, err := folderRepo.GetForUser(ctx, env.DB, userID, child.ID)
	if err != nil || movedChild.ParentFolderID == nil || *movedChild.ParentFolderID != parent.ID || string(movedChild.EncryptedName) != "moved-child-name-ciphertext" {
		t.Fatalf("moved folder = %+v, error %v", movedChild, err)
	}
	if err := folderService.MoveEntries(ctx, MoveEntriesInput{
		UserID: userID, TargetFolderID: &child.ID,
		Folders: []MoveFolderInput{{ID: parent.ID, EncryptedName: []byte("cycle-name"), EncryptedMetadata: []byte("cycle-metadata")}},
	}); !errors.Is(err, ErrInvalidMove) {
		t.Fatalf("moving a folder into its descendant = %v, want ErrInvalidMove", err)
	}
	if err := folderService.MoveEntries(ctx, MoveEntriesInput{
		UserID: otherID, TargetFolderID: &child.ID,
		Files: []MoveFileInput{{ID: file.ID, EncryptedFileKey: []byte("unauthorized-key")}},
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moving another user's file = %v, want ErrNotFound", err)
	}
	if err := folderService.MoveEntries(ctx, MoveEntriesInput{
		UserID: userID, TargetFolderID: &child.ID,
		Files: []MoveFileInput{{ID: file.ID, EncryptedFileKey: []byte("moved-file-key-ciphertext")}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := fileService.RenameFile(ctx, filesvc.RenameFileInput{
		UserID: otherID, FileID: file.ID, EncryptedMetadata: []byte("unauthorized-rename"),
	}); !errors.Is(err, filesvc.ErrNotFound) {
		t.Fatalf("renaming another user's file = %v, want ErrNotFound", err)
	}
	if err := fileService.RenameFile(ctx, filesvc.RenameFileInput{
		UserID: userID, FileID: file.ID, EncryptedMetadata: []byte("renamed-metadata-ciphertext"),
	}); err != nil {
		t.Fatal(err)
	}
	movedFile, err := fileRepo.GetEncryptedFileRecordForUser(ctx, env.DB, file.ID, userID)
	if err != nil || movedFile.FolderID == nil || *movedFile.FolderID != child.ID || string(movedFile.EncryptedFileKey) != "moved-file-key-ciphertext" || string(movedFile.EncryptedMetadata) != "renamed-metadata-ciphertext" {
		t.Fatalf("renamed/moved file = %+v, error %v", movedFile, err)
	}
	entries, err := folderService.ListEntriesPage(ctx, ListEntriesPageInput{UserID: userID, FolderID: &child.ID, Page: 1, PageSize: 10})
	if err != nil || len(entries.Files) != 1 || entries.Files[0].ID != file.ID {
		t.Fatalf("listing moved file folder = %+v, error %v", entries, err)
	}
	downloadURL, err := fileService.PresignDownload(ctx, userID, file.ID)
	if err != nil || !strings.HasPrefix(downloadURL, "memory://download/") {
		t.Fatalf("presigned download URL = %q, error %v", downloadURL, err)
	}
	deletedTree, err := folderService.DeleteEntries(ctx, DeleteEntriesInput{UserID: userID, FolderIDs: []string{parent.ID}})
	if err != nil || deletedTree.DeletedFiles != 1 || deletedTree.DeletedFolders != 2 {
		t.Fatalf("trash folder tree = %+v, error %v; want one file and two folders", deletedTree, err)
	}
	treeTrash, err := folderService.ListTrash(ctx, ListTrashInput{UserID: userID, Limit: 10})
	if err != nil || len(treeTrash.Entries) != 1 || treeTrash.Entries[0].ID != parent.ID || treeTrash.Entries[0].Type != "folder" {
		t.Fatalf("trashed folder tree listing = %+v, error %v", treeTrash, err)
	}
	restoredTree, err := folderService.RestoreTrash(ctx, RestoreTrashInput{UserID: userID, FolderIDs: []string{parent.ID}})
	if err != nil || restoredTree.RestoredFiles != 1 || restoredTree.RestoredFolders != 2 {
		t.Fatalf("restore folder tree = %+v, error %v; want one file and two folders", restoredTree, err)
	}

	if _, err := folderService.DeleteEntries(ctx, DeleteEntriesInput{UserID: otherID, FileIDs: []string{file.ID}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete by another user = %v, want ErrNotFound", err)
	}
	deleted, err := folderService.DeleteEntries(ctx, DeleteEntriesInput{UserID: userID, FileIDs: []string{file.ID}})
	if err != nil || deleted.DeletedFiles != 1 {
		t.Fatalf("trash file result=%+v error=%v", deleted, err)
	}
	trash, err := folderService.ListTrash(ctx, ListTrashInput{UserID: userID, ParentFolderID: child.ID, Limit: 10})
	if err != nil || len(trash.Entries) != 1 || trash.Entries[0].ID != file.ID || trash.Entries[0].Type != "file" {
		t.Fatalf("trash listing=%+v error=%v, want the trashed file", trash, err)
	}
	if used, reserved, err := usersrepo.New().GetStorageUsage(ctx, env.DB, userID); err != nil || used != encryptedSize || reserved != 0 {
		t.Fatalf("usage in trash = %d/%d error=%v, want %d/0", used, reserved, err, encryptedSize)
	}
	if _, ok := env.Storage.GetObject(objectKey); !ok {
		t.Fatal("trash removed the stored object")
	}

	restored, err := folderService.RestoreTrash(ctx, RestoreTrashInput{UserID: userID, FileIDs: []string{file.ID}})
	if err != nil || restored.RestoredFiles != 1 {
		t.Fatalf("restore result=%+v error=%v", restored, err)
	}
	if _, err := fileRepo.GetEncryptedFileRecordForUser(ctx, env.DB, file.ID, userID); err != nil {
		t.Fatalf("restored file is not available to its owner: %v", err)
	}

	deleted, err = folderService.DeleteEntries(ctx, DeleteEntriesInput{UserID: userID, FileIDs: []string{file.ID}, Permanent: true})
	if err != nil || deleted.DeletedFiles != 1 {
		t.Fatalf("permanent delete request result=%+v error=%v", deleted, err)
	}
	if _, err := fileService.PurgeDeletedFiles(ctx); err == nil {
		t.Fatal("purge succeeded despite object-store failure")
	}
	var purgedAtSet bool
	if err := env.DB.QueryRow(ctx, `SELECT purged_at IS NOT NULL FROM files WHERE id = $1`, file.ID).Scan(&purgedAtSet); err != nil {
		t.Fatal(err)
	}
	if purgedAtSet {
		t.Fatal("file was marked purged before its encrypted object was deleted")
	}
	if used, _, err := usersrepo.New().GetStorageUsage(ctx, env.DB, userID); err != nil || used != encryptedSize {
		t.Fatalf("usage after failed purge = %d error=%v, want %d", used, err, encryptedSize)
	}
	if _, ok := env.Storage.GetObject(objectKey); !ok {
		t.Fatal("object disappeared after failed purge")
	}
	var shareCount int
	if err := env.DB.QueryRow(ctx, `SELECT count(*) FROM share_links WHERE id = $1`, share.ID).Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 1 {
		t.Fatalf("share link count after failed purge = %d, want 1", shareCount)
	}

	deleteFailure.err = nil
	purged, err := fileService.PurgeDeletedFiles(ctx)
	if err != nil || purged != 1 {
		t.Fatalf("purge retry = %d, error %v; want 1, nil", purged, err)
	}
	if used, reserved, err := usersrepo.New().GetStorageUsage(ctx, env.DB, userID); err != nil || used != 0 || reserved != 0 {
		t.Fatalf("usage after purge = %d/%d error=%v, want 0/0", used, reserved, err)
	}
	if _, ok := env.Storage.GetObject(objectKey); ok {
		t.Fatal("object remains after successful purge")
	}
	if err := env.DB.QueryRow(ctx, `SELECT count(*) FROM share_links WHERE id = $1`, share.ID).Scan(&shareCount); err != nil {
		t.Fatal(err)
	}
	if shareCount != 0 {
		t.Fatalf("share link count after successful purge = %d, want 0", shareCount)
	}
}

type deleteObjectsFailureStorage struct {
	storage.Provider
	err error
}

func (s *deleteObjectsFailureStorage) DeleteObjects(ctx context.Context, keys []string) (map[string]error, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.Provider.DeleteObjects(ctx, keys)
}

var _ storage.Provider = (*deleteObjectsFailureStorage)(nil)
