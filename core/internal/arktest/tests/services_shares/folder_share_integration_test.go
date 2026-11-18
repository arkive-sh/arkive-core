//go:build integration

package shares_test

import (
	. "arkive/core/services/shares"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"arkive/core/internal/arktest"
	"arkive/core/models"
	filerepo "arkive/core/repositories/files"
	folderrepo "arkive/core/repositories/folders"
	sharerepo "arkive/core/repositories/shares"
	"github.com/jackc/pgx/v5"
)

func TestFileShareReturnsEncryptedRecordAndRevocationBlocksPublicAccess(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := arktest.CreateVerifiedUser(ctx, env.DB, "Share User", "fileshareowner", "fileshareowner@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := arktest.CreateVerifiedUser(ctx, env.DB, "Share User", "fileshareother", "fileshareother@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID, otherID := owner.ID, other.ID

	files := filerepo.New()
	file, err := files.CreateEncryptedFile(ctx, env.DB, models.File{
		UserID: userID, EncryptedMetadata: []byte("encrypted-file-metadata"),
		EncryptedFileKey: []byte("owner-wrapped-file-key"), EncryptedManifest: []byte("encrypted-manifest"),
		EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, ActualEncryptedSize: 34,
		UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(env.DB, files, folderrepo.New(), sharerepo.New(), "")
	input := CreateInput{
		FileID: file.ID, OwnerUserID: userID, Token: strings.Repeat("c", TokenMinLength),
		AllowPreview:             true,
		EncryptedShareKey:        base64.StdEncoding.EncodeToString([]byte("encrypted-share-key")),
		EncryptedFileKeyForShare: base64.StdEncoding.EncodeToString([]byte("share-wrapped-file-key")),
	}
	unauthorized := input
	unauthorized.OwnerUserID = otherID
	unauthorized.Token = strings.Repeat("d", TokenMinLength)
	if _, _, err := service.CreateShare(ctx, unauthorized); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sharing another user's file = %v, want ErrNotFound", err)
	}

	created, validationErrors, err := service.CreateShare(ctx, input)
	if err != nil || validationErrors.HasAny() {
		t.Fatalf("create valid file share: validation=%v error=%v", validationErrors, err)
	}
	if created.Status != ShareStatusActive || created.FileID != file.ID {
		t.Fatalf("created file share = %+v, want active share for file %s", created, file.ID)
	}
	public, err := service.GetPublicShareRecord(ctx, input.Token)
	if err != nil {
		t.Fatalf("open public file share: %v", err)
	}
	if public.FileID != file.ID || string(public.EncryptedFileKeyForShare) != "share-wrapped-file-key" || string(public.EncryptedMetadata) != "encrypted-file-metadata" || string(public.EncryptedManifest) != "encrypted-manifest" {
		t.Fatalf("public file share returned unexpected encrypted record: %+v", public)
	}
	if _, err := env.DB.Exec(ctx, `UPDATE share_links SET expires_at = now() - interval '1 minute' WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPublicShareRecord(ctx, input.Token); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public record after expiry = %v, want pgx.ErrNoRows", err)
	}
	if _, err := env.DB.Exec(ctx, `UPDATE share_links SET expires_at = NULL WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeShareForUser(ctx, created.ID, otherID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke another user's share = %v, want ErrNotFound", err)
	}
	if _, err := service.RevokeShareForUser(ctx, created.ID, userID); err != nil {
		t.Fatalf("revoke file share: %v", err)
	}
	if _, err := service.GetPublicShareRecord(ctx, input.Token); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public access after revocation = %v, want pgx.ErrNoRows", err)
	}
}

func TestFolderShareRequiresOwnerAndCanBeActivatedAndListed(t *testing.T) {
	env := arktest.NewEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const folderID = "33333333-3333-4333-8333-333333333333"
	owner, err := arktest.CreateVerifiedUser(ctx, env.DB, "Test User", "sharetest", "sharetest@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := arktest.CreateVerifiedUser(ctx, env.DB, "Other User", "othershare", "othershare@example.test", []byte("registration"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	userID, otherID, vaultID := owner.ID, other.ID, owner.ID
	if _, err := env.DB.Exec(ctx, `INSERT INTO folders (id, user_id, vault_id, encrypted_name, encrypted_metadata)
		VALUES ($1, $2, $3, $4, $5)`, folderID, userID, vaultID, []byte("folder-name"), []byte("folder-metadata")); err != nil {
		t.Fatal(err)
	}

	files := filerepo.New()
	folders := folderrepo.New()
	service := NewService(env.DB, files, folders, sharerepo.New(), "")
	input := CreateInput{
		FolderID:                folderID,
		OwnerUserID:             userID,
		Token:                   strings.Repeat("a", TokenMinLength),
		AllowPreview:            true,
		EncryptedShareKey:       "c2hhcmUta2V5",
		EncryptedFolderName:     "Zm9sZGVyLW5hbWU=",
		EncryptedFolderMetadata: "Zm9sZGVyLW1ldGFkYXRh",
	}
	unauthorizedInput := input
	unauthorizedInput.OwnerUserID = otherID
	unauthorizedInput.Token = strings.Repeat("b", TokenMinLength)
	if _, _, err := service.CreateShare(ctx, unauthorizedInput); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-owner folder share error = %v, want ErrNotFound", err)
	}

	created, validationErrors, err := service.CreateShare(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if validationErrors.HasAny() {
		t.Fatalf("valid folder-share input failed validation: %v", validationErrors)
	}
	if created.Status != "provisioning" || created.FolderID != folderID || created.FileID != "" {
		t.Fatalf("unexpected folder share after creation: %+v", created)
	}

	if err := service.CompleteFolderShareSetup(ctx, created.ID, userID); err != nil {
		t.Fatalf("complete folder-share setup: %v", err)
	}
	shares, err := service.ListSharesForUser(ctx, userID)
	if err != nil {
		t.Fatalf("list folder share: %v", err)
	}
	if len(shares) != 1 {
		t.Fatalf("listed shares = %d, want 1", len(shares))
	}
	listed := shares[0]
	if listed.ItemType != "folder" || listed.FolderID != folderID || listed.FileID != "" || listed.Status != ShareStatusActive {
		t.Fatalf("unexpected listed folder share: %+v", listed)
	}
	if listed.FolderVaultID != vaultID || string(listed.EncryptedFolderName) != "folder-name" {
		t.Fatalf("folder metadata was not loaded from the share: %+v", listed)
	}

	parentFolderID := folderID
	childFolder, err := folders.Create(ctx, env.DB, models.Folder{
		ID: "55555555-5555-4555-8555-555555555555", UserID: userID, VaultID: vaultID,
		ParentFolderID: &parentFolderID, EncryptedName: []byte("child-folder-name"), EncryptedMetadata: []byte("child-folder-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := files.CreateEncryptedFile(ctx, env.DB, models.File{
		UserID: userID, FolderID: &parentFolderID, EncryptedMetadata: []byte("file-metadata"),
		EncryptedFileKey: []byte("owner-file-key"), EncryptedManifest: []byte("file-manifest"),
		EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, ActualEncryptedSize: 34,
		UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	unsharedFile, err := files.CreateEncryptedFile(ctx, env.DB, models.File{
		UserID: userID, FolderID: &parentFolderID, EncryptedMetadata: []byte("private-file-metadata"),
		EncryptedFileKey: []byte("private-file-key"), EncryptedManifest: []byte("private-manifest"),
		EncryptionVersion: 1, ChunkSize: 1024, ChunkCount: 1, ActualEncryptedSize: 34,
		UploadStatus: "complete",
	})
	if err != nil {
		t.Fatal(err)
	}
	outsideFolder, err := folders.Create(ctx, env.DB, models.Folder{
		ID: "66666666-6666-4666-8666-666666666666", UserID: userID, VaultID: vaultID,
		EncryptedName: []byte("outside-folder-name"), EncryptedMetadata: []byte("outside-folder-metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, nextCursor, err := service.ListPublicFolderShareEntries(ctx, input.Token, folderID, "", 10)
	if err != nil || len(entries) != 0 || nextCursor != "" {
		t.Fatalf("unadded folder contents = %+v cursor=%q error=%v; want empty", entries, nextCursor, err)
	}
	if err := service.AddFolderShareEntries(ctx, created.ID, otherID,
		[]sharerepo.FolderShareFileEntry{{ID: file.ID, EncryptedFileKeyForShare: []byte("other-user-key")}}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("adding contents as another user = %v, want ErrNotFound", err)
	}
	if err := service.AddFolderShareEntries(ctx, created.ID, userID,
		[]sharerepo.FolderShareFileEntry{{ID: file.ID, EncryptedFileKeyForShare: []byte("share-file-key")}},
		[]sharerepo.FolderShareFolderEntry{{ID: childFolder.ID, EncryptedName: []byte("shared-child-name"), EncryptedMetadata: []byte("shared-child-metadata")}}); err != nil {
		t.Fatalf("add owner-selected folder contents to active share: %v", err)
	}
	entries, nextCursor, err = service.ListPublicFolderShareEntries(ctx, input.Token, folderID, "", 1)
	if err != nil || len(entries) != 1 || nextCursor == "" {
		t.Fatalf("first public folder page = %+v cursor=%q error=%v; want one entry and next cursor", entries, nextCursor, err)
	}
	first := entries[0]
	entries, finalCursor, err := service.ListPublicFolderShareEntries(ctx, input.Token, folderID, nextCursor, 1)
	if err != nil || len(entries) != 1 || finalCursor != "" || entries[0].ID == first.ID {
		t.Fatalf("second public folder page = %+v cursor=%q error=%v; want the other entry and no next cursor", entries, finalCursor, err)
	}
	if _, err := service.GetPublicFolderShareFileRecord(ctx, input.Token, unsharedFile.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public access to a file omitted from the share = %v, want pgx.ErrNoRows", err)
	}
	outsideEntries, outsideCursor, err := service.ListPublicFolderShareEntries(ctx, input.Token, outsideFolder.ID, "", 10)
	if err != nil || len(outsideEntries) != 0 || outsideCursor != "" {
		t.Fatalf("public listing outside the shared folder = %+v cursor=%q error=%v; want empty", outsideEntries, outsideCursor, err)
	}
	byID := map[string]models.PublicFolderShareEntry{first.ID: first, entries[0].ID: entries[0]}
	sharedChild := byID[childFolder.ID]
	if sharedChild.ItemType != "folder" || sharedChild.ParentFolderID == nil || *sharedChild.ParentFolderID != folderID || string(sharedChild.EncryptedName) != "shared-child-name" {
		t.Fatalf("public folder listing has wrong encrypted child folder: %+v", sharedChild)
	}
	sharedFile := byID[file.ID]
	if sharedFile.ItemType != "file" || sharedFile.ParentFolderID == nil || *sharedFile.ParentFolderID != folderID || string(sharedFile.EncryptedFileMetadata) != "file-metadata" || string(sharedFile.EncryptedFileKeyForShare) != "share-file-key" {
		t.Fatalf("public folder listing has wrong encrypted file entry: %+v", sharedFile)
	}
	publicFile, err := service.GetPublicFolderShareFileRecord(ctx, input.Token, file.ID)
	if err != nil || publicFile.FileID != file.ID || string(publicFile.EncryptedFileKeyForShare) != "share-file-key" {
		t.Fatalf("public file record = %+v, error %v", publicFile, err)
	}
	if _, err := env.DB.Exec(ctx, `UPDATE share_links SET expires_at = now() - interval '1 minute' WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPublicFolderShareRecord(ctx, input.Token); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public folder record after expiry = %v, want pgx.ErrNoRows", err)
	}
	if _, err := service.GetPublicFolderShareFileRecord(ctx, input.Token, file.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public folder file record after expiry = %v, want pgx.ErrNoRows", err)
	}
	entries, nextCursor, err = service.ListPublicFolderShareEntries(ctx, input.Token, folderID, "", 10)
	if err != nil || len(entries) != 0 || nextCursor != "" {
		t.Fatalf("public folder entries after expiry = %+v cursor=%q error=%v; want empty", entries, nextCursor, err)
	}
	if _, err := env.DB.Exec(ctx, `UPDATE share_links SET expires_at = NULL WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeShareForUser(ctx, created.ID, userID); err != nil {
		t.Fatalf("revoke folder share: %v", err)
	}
	if _, err := service.GetPublicFolderShareRecord(ctx, input.Token); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("public folder record after revocation = %v, want pgx.ErrNoRows", err)
	}
}
