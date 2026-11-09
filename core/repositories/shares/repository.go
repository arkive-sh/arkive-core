package sharerepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct{}

type CreateShareLinkInput struct {
	OwnerUserID       string
	Token             string
	EncryptedShareKey []byte
	AllowPreview      bool
	AllowDownload     bool
	PasswordHash      *string
	ExpiresAt         *time.Time
	BurnAfterRead     bool
	MaxAccessCount    *int
	Status            string
}

type CreateShareItemInput struct {
	ShareLinkID  string
	ItemType     string
	FileID       string
	FolderID     string
	DisplayOrder int
}

type FolderShareFileEntry struct {
	ID                       string
	EncryptedFileKeyForShare []byte
}

type FolderShareFolderEntry struct {
	ID                string
	EncryptedName     []byte
	EncryptedMetadata []byte
}

type CreateShareSnapshotFileInput struct {
	ShareItemID              string
	FileID                   string
	EncryptedRelativePath    []byte
	EncryptedFileKeyForShare []byte
	DisplayOrder             int
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateShareLink(ctx context.Context, db database.PgExecutor, input CreateShareLinkInput) (string, error) {
	query := `INSERT INTO share_links
		(owner_user_id, token, encrypted_share_key, allow_preview, allow_download, password_hash, expires_at, burn_after_read, max_access_count, status)
	VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	RETURNING id`
	var id string
	if err := db.QueryRow(ctx, query,
		input.OwnerUserID,
		input.Token,
		input.EncryptedShareKey,
		input.AllowPreview,
		input.AllowDownload,
		input.PasswordHash,
		input.ExpiresAt,
		input.BurnAfterRead,
		input.MaxAccessCount,
		input.Status,
	).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) CreateShareItem(ctx context.Context, db database.PgExecutor, input CreateShareItemInput) (string, error) {
	var fileID, folderID any
	if input.FileID != "" {
		fileID = input.FileID
	}
	if input.FolderID != "" {
		folderID = input.FolderID
	}

	query := `INSERT INTO share_items
		(share_link_id, item_type, file_id, folder_id, display_order)
	VALUES
		($1, $2, $3, $4, $5)
	RETURNING id`
	var id string
	if err := db.QueryRow(ctx, query,
		input.ShareLinkID,
		input.ItemType,
		fileID,
		folderID,
		input.DisplayOrder,
	).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repository) CreateFolderShareEntry(ctx context.Context, db database.PgExecutor, shareID string, entry FolderShareFolderEntry) error {
	_, err := db.Exec(ctx, `INSERT INTO share_folder_entries
		(share_link_id, folder_id, encrypted_name, encrypted_metadata)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (share_link_id, folder_id) WHERE folder_id IS NOT NULL
	DO UPDATE SET encrypted_name = EXCLUDED.encrypted_name,
	              encrypted_metadata = EXCLUDED.encrypted_metadata`,
		shareID, entry.ID, entry.EncryptedName, entry.EncryptedMetadata)
	return err
}

func (r *Repository) AddFolderShareEntries(ctx context.Context, db database.PgExecutor, shareID string, files []FolderShareFileEntry, folders []FolderShareFolderEntry) (int, error) {
	count := 0
	if len(files) > 0 {
		ids := make([]string, len(files))
		keys := make([][]byte, len(files))
		for i, entry := range files {
			ids[i], keys[i] = entry.ID, entry.EncryptedFileKeyForShare
		}
		tag, err := db.Exec(ctx, `INSERT INTO share_folder_entries
			(share_link_id, file_id, encrypted_file_key_for_share)
		SELECT $1, file.id, incoming.encrypted_key
		FROM UNNEST($2::uuid[], $3::bytea[]) AS incoming(id, encrypted_key)
		JOIN files file ON file.id = incoming.id
		JOIN share_links link ON link.id = $1 AND link.owner_user_id = file.user_id AND link.status IN ('active', 'provisioning', 'revoked')
		JOIN share_items root ON root.share_link_id = link.id AND root.item_type = 'folder'
		WHERE file.purged_at IS NULL
		ON CONFLICT (share_link_id, file_id) WHERE file_id IS NOT NULL
		DO UPDATE SET encrypted_file_key_for_share = EXCLUDED.encrypted_file_key_for_share`, shareID, ids, keys)
		if err != nil {
			return 0, err
		}
		count += int(tag.RowsAffected())
	}
	if len(folders) > 0 {
		ids := make([]string, len(folders))
		names := make([][]byte, len(folders))
		metadata := make([][]byte, len(folders))
		for i, entry := range folders {
			ids[i], names[i], metadata[i] = entry.ID, entry.EncryptedName, entry.EncryptedMetadata
		}
		tag, err := db.Exec(ctx, `INSERT INTO share_folder_entries
			(share_link_id, folder_id, encrypted_name, encrypted_metadata)
		SELECT $1, folder.id, incoming.encrypted_name, incoming.encrypted_metadata
		FROM UNNEST($2::uuid[], $3::bytea[], $4::bytea[]) AS incoming(id, encrypted_name, encrypted_metadata)
		JOIN folders folder ON folder.id = incoming.id
		JOIN share_links link ON link.id = $1 AND link.owner_user_id = folder.user_id AND link.status IN ('active', 'provisioning', 'revoked')
		JOIN share_items root ON root.share_link_id = link.id AND root.item_type = 'folder'
		WHERE folder.purged_at IS NULL
		ON CONFLICT (share_link_id, folder_id) WHERE folder_id IS NOT NULL
		DO UPDATE SET encrypted_name = EXCLUDED.encrypted_name,
		              encrypted_metadata = EXCLUDED.encrypted_metadata`, shareID, ids, names, metadata)
		if err != nil {
			return 0, err
		}
		count += int(tag.RowsAffected())
	}
	return count, nil
}

func (r *Repository) ActivateFolderShare(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string) (bool, error) {
	result, err := db.Exec(ctx, `WITH RECURSIVE root AS (
		SELECT folder.id, folder.user_id
		FROM share_items item
		JOIN folders folder ON folder.id = item.folder_id
		WHERE item.share_link_id = $1 AND item.item_type = 'folder'
			AND folder.user_id = $2 AND folder.deleted_at IS NULL AND folder.purged_at IS NULL
	), tree AS (
		SELECT id, user_id FROM root
		UNION ALL
		SELECT child.id, child.user_id
		FROM folders child JOIN tree parent ON child.parent_folder_id = parent.id
		WHERE child.user_id = $2 AND child.deleted_at IS NULL AND child.purged_at IS NULL
	)
	UPDATE share_links link
	SET status = 'active', updated_at = now()
	WHERE link.id = $1 AND link.owner_user_id = $2 AND link.status = 'provisioning'
		AND EXISTS (SELECT 1 FROM root)
		AND EXISTS (SELECT 1 FROM share_folder_entries envelope
			WHERE envelope.share_link_id = link.id AND envelope.folder_id = (SELECT id FROM root))
		AND NOT EXISTS (
			SELECT 1 FROM tree folder
			LEFT JOIN share_folder_entries envelope
				ON envelope.share_link_id = link.id AND envelope.folder_id = folder.id
			WHERE envelope.folder_id IS NULL
		)
		AND NOT EXISTS (
			SELECT 1 FROM files file
			JOIN tree parent ON parent.id = file.folder_id
			LEFT JOIN share_folder_entries envelope
				ON envelope.share_link_id = link.id AND envelope.file_id = file.id
			WHERE file.user_id = $2 AND file.deleted_at IS NULL AND file.purged_at IS NULL
				AND file.upload_status = 'complete' AND envelope.file_id IS NULL
		)`, shareID, ownerUserID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() == 1, nil
}

func (r *Repository) ListFolderShareScopes(ctx context.Context, db database.PgExecutor, ownerUserID, folderID string) ([]models.Share, error) {
	// Retain envelopes for revoked and expired links so reactivation shows the current folder contents.
	rows, err := db.Query(ctx, `WITH RECURSIVE ancestors AS (
		SELECT id, parent_folder_id FROM folders
		WHERE id = $2 AND user_id = $1 AND deleted_at IS NULL AND purged_at IS NULL
		UNION ALL
		SELECT parent.id, parent.parent_folder_id
		FROM folders parent JOIN ancestors child ON child.parent_folder_id = parent.id
		WHERE parent.user_id = $1 AND parent.deleted_at IS NULL AND parent.purged_at IS NULL
	)
	SELECT link.id, link.token, link.encrypted_share_key
	FROM share_links link
	JOIN share_items item ON item.share_link_id = link.id AND item.item_type = 'folder'
	JOIN ancestors ON ancestors.id = item.folder_id
	WHERE link.owner_user_id = $1 AND link.status IN ('active', 'provisioning', 'revoked')
	ORDER BY link.created_at`, ownerUserID, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shares := []models.Share{}
	for rows.Next() {
		var share models.Share
		if err := rows.Scan(&share.ID, &share.Token, &share.EncryptedShareKey); err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

func (r *Repository) ListFolderShareEntries(ctx context.Context, db database.PgExecutor, token, folderID string, limit int, cursor string) ([]models.PublicFolderShareEntry, string, error) {
	rows, err := db.Query(ctx, `WITH RECURSIVE root AS (
		SELECT link.id AS share_id, link.owner_user_id, item.folder_id AS root_id
		FROM share_links link JOIN share_items item ON item.share_link_id = link.id
		WHERE link.token = $1 AND link.status = 'active' AND item.item_type = 'folder'
			AND (link.expires_at IS NULL OR link.expires_at > now())
	), path AS (
		SELECT folder.id, folder.parent_folder_id, root.share_id, root.owner_user_id, root.root_id
		FROM folders folder JOIN root ON folder.id = $2
		WHERE folder.user_id = root.owner_user_id AND folder.deleted_at IS NULL AND folder.purged_at IS NULL
		UNION ALL
		SELECT parent.id, parent.parent_folder_id, path.share_id, path.owner_user_id, path.root_id
		FROM folders parent JOIN path ON path.parent_folder_id = parent.id
		WHERE path.id <> path.root_id AND parent.user_id = path.owner_user_id
			AND parent.deleted_at IS NULL AND parent.purged_at IS NULL
	), requested AS (
		SELECT share_id, owner_user_id FROM path WHERE id = root_id
	), entries AS (
		SELECT folder.id, 'folder'::text AS item_type, folder.parent_folder_id, folder.vault_id,
			envelope.encrypted_name, envelope.encrypted_metadata, NULL::bytea AS encrypted_file_metadata,
			NULL::bytea AS encrypted_file_key, folder.updated_at, 0::smallint AS encryption_version,
			0::bigint AS chunk_size, 0::integer AS chunk_count, ''::bytea AS encrypted_hash
		FROM requested scope JOIN folders folder ON folder.parent_folder_id = $2 AND folder.user_id = scope.owner_user_id
		JOIN share_folder_entries envelope ON envelope.share_link_id = scope.share_id AND envelope.folder_id = folder.id
		WHERE folder.deleted_at IS NULL AND folder.purged_at IS NULL
		UNION ALL
		SELECT file.id, 'file'::text, file.folder_id, file.user_id,
			NULL::bytea, NULL::bytea, file.encrypted_metadata, envelope.encrypted_file_key_for_share,
			file.updated_at, file.encryption_version, file.chunk_size, file.chunk_count, file.encrypted_hash
		FROM requested scope JOIN files file ON file.folder_id = $2 AND file.user_id = scope.owner_user_id
		JOIN share_folder_entries envelope ON envelope.share_link_id = scope.share_id AND envelope.file_id = file.id
		WHERE file.deleted_at IS NULL
			AND file.purged_at IS NULL AND file.upload_status = 'complete'
	)
	SELECT id, item_type, parent_folder_id, vault_id, encrypted_name, encrypted_metadata,
		encrypted_file_metadata, encrypted_file_key, updated_at, encryption_version, chunk_size, chunk_count, encrypted_hash
	FROM entries
	WHERE id::text > $3
	ORDER BY id
	LIMIT $4`, token, folderID, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	entries := make([]models.PublicFolderShareEntry, 0, limit+1)
	for rows.Next() {
		var entry models.PublicFolderShareEntry
		if err := rows.Scan(&entry.ID, &entry.ItemType, &entry.ParentFolderID, &entry.VaultID,
			&entry.EncryptedName, &entry.EncryptedMetadata, &entry.EncryptedFileMetadata, &entry.EncryptedFileKeyForShare,
			&entry.UpdatedAt, &entry.EncryptionVersion, &entry.ChunkSize, &entry.TotalChunks,
			&entry.EncryptedHash); err != nil {
			return nil, "", err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(entries) > limit {
		entries = entries[:limit]
		next = entries[len(entries)-1].ID
	}
	return entries, next, nil
}

func (r *Repository) GetPublicFolderShareRecord(ctx context.Context, db database.PgExecutor, token string) (models.PublicShareRecord, error) {
	var record models.PublicShareRecord
	err := db.QueryRow(ctx, `SELECT link.id, link.token, item.folder_id, folder.vault_id,
		envelope.encrypted_name, envelope.encrypted_metadata
	FROM share_links link
	JOIN share_items item ON item.share_link_id = link.id AND item.item_type = 'folder'
	JOIN folders folder ON folder.id = item.folder_id
	JOIN share_folder_entries envelope ON envelope.share_link_id = link.id AND envelope.folder_id = folder.id
	WHERE link.token = $1 AND link.status = 'active'
		AND folder.user_id = link.owner_user_id AND folder.deleted_at IS NULL AND folder.purged_at IS NULL
		AND (link.expires_at IS NULL OR link.expires_at > now())`, token).Scan(
		&record.ShareID, &record.Token, &record.FolderID, &record.VaultID,
		&record.EncryptedFolderName, &record.EncryptedFolderMetadata)
	if err != nil {
		return models.PublicShareRecord{}, err
	}
	record.ItemType = "folder"
	return record, nil
}

func (r *Repository) GetPublicFolderShareFileRecord(ctx context.Context, db database.PgExecutor, token, fileID string) (models.PublicShareRecord, error) {
	var record models.PublicShareRecord
	err := db.QueryRow(ctx, `WITH RECURSIVE root AS (
		SELECT link.id AS share_id, link.owner_user_id, item.folder_id AS root_id
		FROM share_links link JOIN share_items item ON item.share_link_id = link.id
		WHERE link.token = $1 AND link.status = 'active' AND item.item_type = 'folder'
			AND (link.expires_at IS NULL OR link.expires_at > now())
	), target AS (
		SELECT root.share_id, root.owner_user_id, root.root_id, file.id AS file_id, file.folder_id
		FROM root JOIN files file ON file.id = $2 AND file.user_id = root.owner_user_id
		WHERE file.deleted_at IS NULL AND file.purged_at IS NULL AND file.upload_status = 'complete'
	), path AS (
		SELECT folder.id, folder.parent_folder_id, target.share_id, target.owner_user_id, target.root_id, target.file_id
		FROM target JOIN folders folder ON folder.id = target.folder_id
		WHERE folder.user_id = target.owner_user_id AND folder.deleted_at IS NULL AND folder.purged_at IS NULL
		UNION ALL
		SELECT parent.id, parent.parent_folder_id, path.share_id, path.owner_user_id, path.root_id, path.file_id
		FROM folders parent JOIN path ON path.parent_folder_id = parent.id
		WHERE path.id <> path.root_id AND parent.user_id = path.owner_user_id
			AND parent.deleted_at IS NULL AND parent.purged_at IS NULL
	), in_scope AS (
		SELECT share_id, owner_user_id, file_id FROM path WHERE id = root_id
	)
	SELECT link.id, link.token, file.id, file.user_id, envelope.encrypted_file_key_for_share,
		file.encrypted_metadata, file.encrypted_manifest, file.encrypted_hash,
		file.encryption_version, file.chunk_size, file.chunk_count
	FROM in_scope scope JOIN share_links link ON link.id = scope.share_id
	JOIN files file ON file.id = scope.file_id AND file.user_id = scope.owner_user_id
	JOIN share_folder_entries envelope ON envelope.share_link_id = scope.share_id AND envelope.file_id = file.id`, token, fileID).Scan(
		&record.ShareID, &record.Token, &record.FileID, &record.VaultID,
		&record.EncryptedFileKeyForShare, &record.EncryptedMetadata, &record.EncryptedManifest,
		&record.EncryptedHash, &record.EncryptionVersion, &record.ChunkSize, &record.TotalChunks)
	if err != nil {
		return models.PublicShareRecord{}, err
	}
	record.ItemType = "file"
	return record, nil
}

func (r *Repository) CreateShareSnapshotFile(ctx context.Context, db database.PgExecutor, input CreateShareSnapshotFileInput) error {
	query := `INSERT INTO share_snapshot_files
		(share_item_id, file_id, encrypted_relative_path, encrypted_file_key_for_share, display_order)
	VALUES
		($1, $2, $3, $4, $5)`
	_, err := db.Exec(ctx, query,
		input.ShareItemID,
		input.FileID,
		input.EncryptedRelativePath,
		input.EncryptedFileKeyForShare,
		input.DisplayOrder,
	)
	return err
}

func (r *Repository) DeleteLinksForFiles(ctx context.Context, db database.PgExecutor, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	_, err := db.Exec(ctx, `DELETE FROM share_links
		WHERE id IN (
			SELECT si.share_link_id
			FROM share_items si
			LEFT JOIN share_snapshot_files ssf ON ssf.share_item_id = si.id
			WHERE si.file_id = ANY($1::uuid[]) OR ssf.file_id = ANY($1::uuid[])
		)`, fileIDs)
	return err
}

func (r *Repository) DeleteLinksForPurgedFolders(ctx context.Context, db database.PgExecutor) error {
	_, err := db.Exec(ctx, `DELETE FROM share_links
		WHERE id IN (
			SELECT item.share_link_id
			FROM share_items item
			JOIN folders folder ON folder.id = item.folder_id
			WHERE item.item_type = 'folder' AND folder.purged_at IS NOT NULL
		)`)
	return err
}

func (r *Repository) GetShareByToken(ctx context.Context, db database.PgExecutor, token string) (models.Share, error) {
	var share models.Share
	query := `SELECT sl.id, si.item_type, COALESCE(ssf.file_id::text, ''), COALESCE(si.folder_id::text, ''),
		sl.owner_user_id, sl.token, sl.encrypted_share_key, sl.allow_preview, sl.allow_download, sl.burn_after_read,
		sl.access_count, sl.max_access_count, sl.password_hash, sl.expires_at, sl.status, sl.revoked_at,
		sl.consumed_at, sl.created_at, sl.updated_at
	FROM share_links sl
	JOIN share_items si ON si.share_link_id = sl.id
	LEFT JOIN share_snapshot_files ssf ON ssf.share_item_id = si.id
	LEFT JOIN files f ON f.id = ssf.file_id
	LEFT JOIN folders folder ON folder.id = si.folder_id
	WHERE sl.token = $1 AND (
		(si.item_type = 'file' AND f.deleted_at IS NULL AND f.purged_at IS NULL)
		OR (si.item_type = 'folder' AND folder.deleted_at IS NULL AND folder.purged_at IS NULL)
	)
	ORDER BY si.display_order ASC LIMIT 1`
	if err := db.QueryRow(ctx, query, token).Scan(
		&share.ID,
		&share.ItemType,
		&share.FileID,
		&share.FolderID,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	return share, nil
}

func (r *Repository) GetShareForFile(ctx context.Context, db database.PgExecutor, fileID string) (models.Share, error) {
	var share models.Share
	query := `SELECT
		sl.id, ssf.file_id, sl.owner_user_id, sl.token, sl.encrypted_share_key, sl.allow_preview, sl.allow_download, sl.burn_after_read,
		sl.access_count, sl.max_access_count, sl.password_hash, sl.expires_at, sl.status, sl.revoked_at, sl.consumed_at, sl.created_at, sl.updated_at
	FROM
		share_links sl
	JOIN
		share_items si ON si.share_link_id = sl.id
	JOIN
		share_snapshot_files ssf ON ssf.share_item_id = si.id
	JOIN
		files f ON f.id = ssf.file_id
	WHERE
		ssf.file_id = $1
		AND f.deleted_at IS NULL
	ORDER BY
		ssf.display_order ASC
	LIMIT 1`
	if err := db.QueryRow(ctx, query, fileID).Scan(
		&share.ID,
		&share.FileID,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	return share, nil
}

func (r *Repository) GetShareForFileForUser(ctx context.Context, db database.PgExecutor, fileID, ownerUserID string) (models.Share, error) {
	var share models.Share
	query := `SELECT
		sl.id, ssf.file_id, sl.owner_user_id, sl.token, sl.encrypted_share_key, sl.allow_preview, sl.allow_download, sl.burn_after_read,
		sl.access_count, sl.max_access_count, sl.password_hash, sl.expires_at, sl.status, sl.revoked_at, sl.consumed_at, sl.created_at, sl.updated_at
	FROM
		share_links sl
	JOIN
		share_items si ON si.share_link_id = sl.id
	JOIN
		share_snapshot_files ssf ON ssf.share_item_id = si.id
	JOIN
		files f ON f.id = ssf.file_id
	WHERE
		ssf.file_id = $1 AND sl.owner_user_id = $2
		AND f.deleted_at IS NULL
	ORDER BY
		ssf.display_order ASC
	LIMIT 1`
	if err := db.QueryRow(ctx, query, fileID, ownerUserID).Scan(
		&share.ID,
		&share.FileID,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	return share, nil
}

func (r *Repository) GetShareForFolderForUser(ctx context.Context, db database.PgExecutor, folderID, ownerUserID string) (models.Share, error) {
	var share models.Share
	err := db.QueryRow(ctx, `SELECT link.id, item.item_type, ''::text, item.folder_id::text,
		link.owner_user_id, link.token, link.encrypted_share_key, link.allow_preview, link.allow_download,
		link.burn_after_read, link.access_count, link.max_access_count, link.password_hash, link.expires_at,
		link.status, link.revoked_at, link.consumed_at, link.created_at, link.updated_at
	FROM share_links link
	JOIN share_items item ON item.share_link_id = link.id AND item.item_type = 'folder'
	JOIN folders folder ON folder.id = item.folder_id
	WHERE item.folder_id = $1 AND link.owner_user_id = $2 AND folder.deleted_at IS NULL AND folder.purged_at IS NULL
	ORDER BY item.display_order LIMIT 1`, folderID, ownerUserID).Scan(
		&share.ID, &share.ItemType, &share.FileID, &share.FolderID, &share.OwnerUserID, &share.Token,
		&share.EncryptedShareKey, &share.AllowPreview, &share.AllowDownload, &share.BurnAfterRead,
		&share.AccessCount, &share.MaxAccessCount, &share.PasswordHash, &share.ExpiresAt, &share.Status,
		&share.RevokedAt, &share.ConsumedAt, &share.CreatedAt, &share.UpdatedAt)
	return share, err
}

func (r *Repository) GetShareForUser(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string) (models.Share, error) {
	var share models.Share
	query := `SELECT sl.id, si.item_type, COALESCE(ssf.file_id::text, ''), COALESCE(si.folder_id::text, ''),
		sl.owner_user_id, sl.token, sl.encrypted_share_key, sl.allow_preview, sl.allow_download, sl.burn_after_read,
		sl.access_count, sl.max_access_count, sl.password_hash, sl.expires_at, sl.status, sl.revoked_at,
		sl.consumed_at, sl.created_at, sl.updated_at
	FROM share_links sl
	JOIN share_items si ON si.share_link_id = sl.id
	LEFT JOIN share_snapshot_files ssf ON ssf.share_item_id = si.id
	LEFT JOIN files f ON f.id = ssf.file_id
	LEFT JOIN folders folder ON folder.id = si.folder_id
	WHERE sl.id = $1 AND sl.owner_user_id = $2 AND (
		(si.item_type = 'file' AND f.purged_at IS NULL)
		OR (si.item_type = 'folder' AND folder.purged_at IS NULL)
	)
	ORDER BY si.display_order LIMIT 1`
	if err := db.QueryRow(ctx, query, shareID, ownerUserID).Scan(
		&share.ID,
		&share.ItemType,
		&share.FileID,
		&share.FolderID,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	return share, nil
}

func (r *Repository) UpdateShareForUser(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string, passwordHash *string, expiresAt *time.Time, burnAfterRead bool, maxAccessCount *int, allowPreview, allowDownload bool) (models.Share, error) {
	var share models.Share
	query := `UPDATE
		share_links
	SET
		password_hash = $3,
		expires_at = $4,
		burn_after_read = $5,
		max_access_count = $6,
		allow_preview = $7,
		allow_download = $8,
		updated_at = now()
	WHERE
		id = $1 AND owner_user_id = $2 AND status = 'active'
	RETURNING
		id, owner_user_id, token, encrypted_share_key, allow_preview, allow_download, burn_after_read,
		access_count, max_access_count, password_hash, expires_at, status, revoked_at, consumed_at, created_at, updated_at`
	var shareIDValue string
	if err := db.QueryRow(ctx, query, shareID, ownerUserID, passwordHash, expiresAt, burnAfterRead, maxAccessCount, allowPreview, allowDownload).Scan(
		&shareIDValue,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	share.ID = shareIDValue
	linked, err := r.GetShareForUser(ctx, db, shareIDValue, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	share.FileID = linked.FileID
	share.ItemType = linked.ItemType
	share.FolderID = linked.FolderID
	return share, nil
}

func (r *Repository) DeleteShareForUser(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string) (bool, error) {
	query := `DELETE FROM
		share_links
	WHERE
		id = $1 AND owner_user_id = $2`
	tag, err := db.Exec(ctx, query, shareID, ownerUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) RevokeShareForUser(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string) (models.Share, error) {
	var share models.Share
	query := `UPDATE
		share_links
	SET
		status = 'revoked',
		revoked_at = now(),
		updated_at = now()
	WHERE
		id = $1 AND owner_user_id = $2 AND status = 'active'
	RETURNING
		id, owner_user_id, token, encrypted_share_key, allow_preview, allow_download, burn_after_read,
		access_count, max_access_count, password_hash, expires_at, status, revoked_at, consumed_at, created_at, updated_at`
	var shareIDValue string
	if err := db.QueryRow(ctx, query, shareID, ownerUserID).Scan(
		&shareIDValue,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	share.ID = shareIDValue
	linked, err := r.GetShareForUser(ctx, db, shareIDValue, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	share.FileID = linked.FileID
	share.ItemType = linked.ItemType
	share.FolderID = linked.FolderID
	return share, nil
}

func (r *Repository) ActivateShareForUser(ctx context.Context, db database.PgExecutor, shareID, ownerUserID string) (models.Share, error) {
	var share models.Share
	query := `UPDATE
		share_links
	SET
		status = 'active',
		revoked_at = NULL,
		updated_at = now()
	WHERE
		id = $1 AND owner_user_id = $2 AND status = 'revoked'
	RETURNING
		id, owner_user_id, token, encrypted_share_key, allow_preview, allow_download, burn_after_read,
		access_count, max_access_count, password_hash, expires_at, status, revoked_at, consumed_at, created_at, updated_at`
	var shareIDValue string
	if err := db.QueryRow(ctx, query, shareID, ownerUserID).Scan(
		&shareIDValue,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	); err != nil {
		return models.Share{}, err
	}
	share.ID = shareIDValue
	linked, err := r.GetShareForUser(ctx, db, shareIDValue, ownerUserID)
	if err != nil {
		return models.Share{}, err
	}
	share.FileID = linked.FileID
	share.ItemType = linked.ItemType
	share.FolderID = linked.FolderID
	return share, nil
}

func (r *Repository) ConsumeShareByToken(ctx context.Context, db database.PgExecutor, token string) (models.Share, bool, error) {
	var share models.Share
	query := `UPDATE share_links
	SET
	  access_count = access_count + 1,
	  consumed_at = CASE
	    WHEN access_count + 1 >= max_access_count THEN now()
	    ELSE consumed_at
	  END,
	  status = CASE
	    WHEN access_count + 1 >= max_access_count THEN 'burned'
	    ELSE status
	  END,
	  updated_at = now()
	WHERE token = $1
	  AND status = 'active'
	  AND (expires_at IS NULL OR expires_at > now())
	  AND burn_after_read = true
	  AND max_access_count IS NOT NULL
	  AND access_count < max_access_count
	  AND EXISTS (
		SELECT 1
		FROM share_items si
		JOIN share_snapshot_files ssf ON ssf.share_item_id = si.id
		JOIN files f ON f.id = ssf.file_id
		WHERE si.share_link_id = share_links.id
		  AND f.deleted_at IS NULL
	  )
	RETURNING
	  id, owner_user_id, token, encrypted_share_key, allow_preview, allow_download, burn_after_read,
	  access_count, max_access_count, password_hash, expires_at, status, revoked_at, consumed_at, created_at, updated_at`
	err := db.QueryRow(ctx, query, token).Scan(
		&share.ID,
		&share.OwnerUserID,
		&share.Token,
		&share.EncryptedShareKey,
		&share.AllowPreview,
		&share.AllowDownload,
		&share.BurnAfterRead,
		&share.AccessCount,
		&share.MaxAccessCount,
		&share.PasswordHash,
		&share.ExpiresAt,
		&share.Status,
		&share.RevokedAt,
		&share.ConsumedAt,
		&share.CreatedAt,
		&share.UpdatedAt,
	)
	if err != nil {
		return models.Share{}, false, err
	}
	return share, true, nil
}

func (r *Repository) ListSharesForUser(ctx context.Context, db database.PgExecutor, ownerUserID string) ([]models.ShareWithFile, error) {
	rows, err := db.Query(ctx, `SELECT
		sl.id,
		si.item_type,
		COALESCE(ssf.file_id::text, ''),
		COALESCE(si.folder_id::text, ''),
		sl.owner_user_id,
		sl.token,
		sl.encrypted_share_key,
		sl.allow_preview,
		sl.allow_download,
		sl.burn_after_read,
		sl.access_count,
		sl.max_access_count,
		sl.password_hash,
		sl.expires_at,
		sl.status,
		sl.revoked_at,
		sl.consumed_at,
		sl.created_at,
		sl.updated_at,
		concat('file-', left(COALESCE(f.id, folder.id)::text, 8)),
		'application/octet-stream',
		COALESCE(f.actual_encrypted_size, 0),
		COALESCE(f.updated_at, folder.updated_at),
		COALESCE(f.user_id, folder.user_id),
		f.folder_id,
		f.encrypted_metadata,
		f.encrypted_file_key,
		f.deleted_at,
		folder.vault_id,
		folder.parent_folder_id,
		folder.encrypted_name,
		folder.encrypted_metadata,
		folder.deleted_at
	FROM
		share_links sl
	JOIN
		share_items si ON si.share_link_id = sl.id
	LEFT JOIN
		share_snapshot_files ssf ON ssf.share_item_id = si.id
	LEFT JOIN
		files f ON f.id = ssf.file_id
	LEFT JOIN
		folders folder ON folder.id = si.folder_id
	WHERE
		sl.owner_user_id = $1
		AND sl.status <> 'provisioning'
		AND ((si.item_type = 'file' AND f.purged_at IS NULL)
			OR (si.item_type = 'folder' AND folder.purged_at IS NULL))
	ORDER BY
		sl.created_at DESC`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	shares := []models.ShareWithFile{}
	for rows.Next() {
		var share models.ShareWithFile
		if err := rows.Scan(
			&share.ID,
			&share.ItemType,
			&share.FileID,
			&share.FolderID,
			&share.OwnerUserID,
			&share.Token,
			&share.EncryptedShareKey,
			&share.AllowPreview,
			&share.AllowDownload,
			&share.BurnAfterRead,
			&share.AccessCount,
			&share.MaxAccessCount,
			&share.PasswordHash,
			&share.ExpiresAt,
			&share.Status,
			&share.RevokedAt,
			&share.ConsumedAt,
			&share.CreatedAt,
			&share.UpdatedAt,
			&share.FileName,
			&share.FileContentType,
			&share.FileSizeBytes,
			&share.FileUpdatedAt,
			&share.FileVaultID,
			&share.FileFolderID,
			&share.EncryptedFileMetadata,
			&share.EncryptedFileKey,
			&share.FileDeletedAt,
			&share.FolderVaultID,
			&share.FolderParentID,
			&share.EncryptedFolderName,
			&share.EncryptedFolderMetadata,
			&share.FolderDeletedAt,
		); err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shares, nil
}

func (r *Repository) GetPublicShareRecord(ctx context.Context, db database.PgExecutor, token string) (models.PublicShareRecord, error) {
	var record models.PublicShareRecord
	query := `SELECT
		sl.id,
		sl.token,
		ssf.file_id,
		f.user_id,
		ssf.encrypted_file_key_for_share,
		f.encrypted_metadata,
		f.encrypted_manifest,
		f.encrypted_hash,
		f.encryption_version,
		f.chunk_size,
		f.chunk_count
	FROM
		share_links sl
	JOIN
		share_items si ON si.share_link_id = sl.id
	JOIN
		share_snapshot_files ssf ON ssf.share_item_id = si.id
	JOIN
		files f ON f.id = ssf.file_id
	WHERE
		sl.token = $1
		AND f.deleted_at IS NULL
	ORDER BY
		ssf.display_order ASC
	LIMIT 1`
	if err := db.QueryRow(ctx, query, token).Scan(
		&record.ShareID,
		&record.Token,
		&record.FileID,
		&record.VaultID,
		&record.EncryptedFileKeyForShare,
		&record.EncryptedMetadata,
		&record.EncryptedManifest,
		&record.EncryptedHash,
		&record.EncryptionVersion,
		&record.ChunkSize,
		&record.TotalChunks,
	); err != nil {
		return models.PublicShareRecord{}, err
	}
	return record, nil
}
