package filerepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct{}

type UploadDetails struct {
	ID            string
	ChunkSize     int64
	ChunkCount    int
	EncryptedSize int64
	UploadStatus  string
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateEncryptedFile(ctx context.Context, db database.PgExecutor, file models.File) (models.File, error) {
	var created models.File
	var folderID *string
	query := `INSERT INTO files
		(user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count, actual_encrypted_size, encrypted_hash, upload_status, expires_at)
	VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	RETURNING
		id, user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
		actual_encrypted_size, encrypted_hash, upload_status,
		thumbnail_size_bytes,
		created_at, updated_at, expires_at`
	if err := db.QueryRow(ctx, query,
		file.UserID,
		file.FolderID,
		file.EncryptedMetadata,
		file.EncryptedFileKey,
		file.EncryptedManifest,
		file.EncryptionVersion,
		file.ChunkSize,
		file.ChunkCount,
		file.ActualEncryptedSize,
		file.EncryptedHash,
		file.UploadStatus,
		file.ExpiresAt,
	).Scan(
		&created.ID,
		&created.UserID,
		&folderID,
		&created.EncryptedMetadata,
		&created.EncryptedFileKey,
		&created.EncryptedManifest,
		&created.EncryptionVersion,
		&created.ChunkSize,
		&created.ChunkCount,
		&created.ActualEncryptedSize,
		&created.EncryptedHash,
		&created.UploadStatus,
		&created.ThumbnailSizeBytes,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.ExpiresAt,
	); err != nil {
		return models.File{}, err
	}
	created.FolderID = folderID
	return created, nil
}

func (r *Repository) GetUploadDetailsForSession(ctx context.Context, db database.PgExecutor, uploadSessionID, userID string) (UploadDetails, error) {
	var details UploadDetails
	err := db.QueryRow(ctx, `SELECT
		files.id, files.chunk_size, files.chunk_count, upload_sessions.encrypted_size, files.upload_status
	FROM upload_sessions
	JOIN files ON files.id = upload_sessions.file_id
	WHERE upload_sessions.id = $1 AND files.user_id = $2
		AND files.deleted_at IS NULL AND upload_sessions.status = 'active'`, uploadSessionID, userID).Scan(
		&details.ID,
		&details.ChunkSize,
		&details.ChunkCount,
		&details.EncryptedSize,
		&details.UploadStatus,
	)
	return details, err
}

func (r *Repository) UpdateEncryptedFileEnvelope(ctx context.Context, db database.PgExecutor, fileID string, encryptedMetadata, encryptedFileKey, encryptedManifest, encryptedHash []byte) error {
	query := `UPDATE
		files
	SET
		encrypted_metadata = $2,
		encrypted_file_key = $3,
		encrypted_manifest = $4,
		encrypted_hash = $5,
		updated_at = now()
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, fileID, encryptedMetadata, encryptedFileKey, encryptedManifest, encryptedHash)
	return err
}

func (r *Repository) GetEncryptedFileRecordForUser(ctx context.Context, db database.PgExecutor, fileID, userID string) (models.File, error) {
	var file models.File
	var folderID *string
	query := `SELECT
		id, user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version,
		chunk_size, chunk_count, actual_encrypted_size, encrypted_hash, upload_status,
		thumbnail_size_bytes
	FROM
		files
	WHERE
		id = $1 AND user_id = $2
		AND deleted_at IS NULL`
	if err := db.QueryRow(ctx, query, fileID, userID).Scan(
		&file.ID,
		&file.UserID,
		&folderID,
		&file.EncryptedMetadata,
		&file.EncryptedFileKey,
		&file.EncryptedManifest,
		&file.EncryptionVersion,
		&file.ChunkSize,
		&file.ChunkCount,
		&file.ActualEncryptedSize,
		&file.EncryptedHash,
		&file.UploadStatus,
		&file.ThumbnailSizeBytes,
	); err != nil {
		return models.File{}, err
	}
	file.FolderID = folderID
	return file, nil
}

func (r *Repository) MarkEncryptedFileComplete(ctx context.Context, db database.PgExecutor, fileID string, actualEncryptedSize int64, encryptedHash []byte) error {
	query := `UPDATE
		files
	SET
		actual_encrypted_size = $2,
		encrypted_hash = $3,
		upload_status = 'complete',
		expires_at = NULL,
		updated_at = now()
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, fileID, actualEncryptedSize, encryptedHash)
	return err
}

func (r *Repository) UpdateThumbnailSize(ctx context.Context, db database.PgExecutor, fileID string, sizeBytes int64) error {
	query := `UPDATE
		files
	SET
		thumbnail_size_bytes = $2,
		updated_at = now()
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, fileID, sizeBytes)
	return err
}

func (r *Repository) UpdateEncryptedFileStatusIf(ctx context.Context, db database.PgExecutor, fileID, status string, allowed []string) (bool, error) {
	query := `UPDATE
		files
	SET
		upload_status = $2,
		updated_at = now()
	WHERE
		id = $1 AND upload_status = ANY($3)`
	tag, err := db.Exec(ctx, query, fileID, status, allowed)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) CountInFlightForUser(ctx context.Context, db database.PgExecutor, userID string) (int64, error) {
	var total int64
	query := `SELECT
		COUNT(DISTINCT files.id)
	FROM
		files
	INNER JOIN
		upload_sessions ON upload_sessions.file_id = files.id
	WHERE
		files.user_id = $1
		AND files.upload_status IN ('pending', 'uploading')
		AND files.deleted_at IS NULL
		AND upload_sessions.status = 'active'
		AND upload_sessions.expires_at > now()`
	if err := db.QueryRow(ctx, query, userID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *Repository) CountArchivedFilesForUser(ctx context.Context, db database.PgExecutor, userID string) (int64, error) {
	var total int64
	query := `SELECT
		COUNT(*)
	FROM
		files
	WHERE
		user_id = $1
		AND upload_status = 'complete'
		AND expires_at IS NOT NULL
		AND deleted_at IS NULL`
	if err := db.QueryRow(ctx, query, userID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *Repository) GetFileByID(ctx context.Context, db database.PgExecutor, fileID string) (models.File, error) {
	var file models.File
	var folderID *string
	query := `SELECT
		id, user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
		actual_encrypted_size, encrypted_hash, upload_status,
		thumbnail_size_bytes,
		created_at, updated_at, expires_at
	FROM
		files
	WHERE
		id = $1
		AND deleted_at IS NULL`
	if err := db.QueryRow(ctx, query, fileID).Scan(
		&file.ID,
		&file.UserID,
		&folderID,
		&file.EncryptedMetadata,
		&file.EncryptedFileKey,
		&file.EncryptedManifest,
		&file.EncryptionVersion,
		&file.ChunkSize,
		&file.ChunkCount,
		&file.ActualEncryptedSize,
		&file.EncryptedHash,
		&file.UploadStatus,
		&file.ThumbnailSizeBytes,
		&file.CreatedAt,
		&file.UpdatedAt,
		&file.ExpiresAt,
	); err != nil {
		return models.File{}, err
	}
	file.FolderID = folderID
	return file, nil
}

func (r *Repository) GetFileForUser(ctx context.Context, db database.PgExecutor, fileID, userID string) (models.File, error) {
	var file models.File
	var folderID *string
	query := `SELECT
		id, user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
		actual_encrypted_size, encrypted_hash, upload_status,
		thumbnail_size_bytes,
		created_at, updated_at, expires_at
	FROM
		files
	WHERE
		id = $1 AND user_id = $2
		AND deleted_at IS NULL`
	if err := db.QueryRow(ctx, query, fileID, userID).Scan(
		&file.ID,
		&file.UserID,
		&folderID,
		&file.EncryptedMetadata,
		&file.EncryptedFileKey,
		&file.EncryptedManifest,
		&file.EncryptionVersion,
		&file.ChunkSize,
		&file.ChunkCount,
		&file.ActualEncryptedSize,
		&file.EncryptedHash,
		&file.UploadStatus,
		&file.ThumbnailSizeBytes,
		&file.CreatedAt,
		&file.UpdatedAt,
		&file.ExpiresAt,
	); err != nil {
		return models.File{}, err
	}
	file.FolderID = folderID
	return file, nil
}

func (r *Repository) CountByFolder(ctx context.Context, db database.PgExecutor, userID string, folderID *string) (int, error) {
	query := `SELECT
		COUNT(1)
	FROM
		files
	WHERE
		user_id = $1
		AND upload_status = 'complete'
		AND expires_at IS NULL
		AND deleted_at IS NULL
		AND (
			($2::uuid IS NULL AND folder_id IS NULL)
			OR folder_id = $2
		)`
	var count int
	if err := db.QueryRow(ctx, query, userID, folderID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) ListByFolder(ctx context.Context, db database.PgExecutor, userID string, folderID *string, limit, offset int) ([]models.File, error) {
	query := `SELECT
		id, user_id, folder_id, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
		actual_encrypted_size, encrypted_hash, upload_status, created_at, updated_at, expires_at
	FROM
		files
	WHERE
		user_id = $1
		AND upload_status = 'complete'
		AND expires_at IS NULL
		AND deleted_at IS NULL
		AND (
			($2::uuid IS NULL AND folder_id IS NULL)
			OR folder_id = $2
		)
	ORDER BY created_at DESC
	LIMIT $3 OFFSET $4`
	rows, err := db.Query(ctx, query, userID, folderID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []models.File{}
	for rows.Next() {
		var file models.File
		var scannedFolderID *string
		if err := rows.Scan(
			&file.ID,
			&file.UserID,
			&scannedFolderID,
			&file.EncryptedMetadata,
			&file.EncryptedFileKey,
			&file.EncryptedManifest,
			&file.EncryptionVersion,
			&file.ChunkSize,
			&file.ChunkCount,
			&file.ActualEncryptedSize,
			&file.EncryptedHash,
			&file.UploadStatus,
			&file.CreatedAt,
			&file.UpdatedAt,
			&file.ExpiresAt,
		); err != nil {
			return nil, err
		}
		file.FolderID = scannedFolderID
		files = append(files, file)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return files, nil
}

func (r *Repository) ListByFolderAfter(ctx context.Context, db database.PgExecutor, userID string, folderID *string, limit int, afterCreatedAt *time.Time, afterID *string, inclusive bool) ([]models.File, error) {
	query := `SELECT
		file.id, file.folder_id, file.encrypted_metadata, file.encrypted_file_key, file.created_at, file.updated_at,
		EXISTS (
			SELECT 1
			FROM share_snapshot_files snapshot
			JOIN share_items item ON item.id = snapshot.share_item_id
			JOIN share_links link ON link.id = item.share_link_id
			WHERE snapshot.file_id = file.id
				AND link.owner_user_id = file.user_id
				AND link.status = 'active'
				AND link.revoked_at IS NULL
				AND link.consumed_at IS NULL
				AND (link.expires_at IS NULL OR link.expires_at > now())
				AND (link.max_access_count IS NULL OR link.access_count < link.max_access_count)
		) AS is_shared
	FROM files file
	WHERE
		file.user_id = $1
		AND file.upload_status = 'complete'
		AND file.expires_at IS NULL
		AND file.deleted_at IS NULL
		AND (
			($2::uuid IS NULL AND file.folder_id IS NULL)
			OR file.folder_id = $2
		)`
	args := []any{userID, folderID, limit}
	if afterCreatedAt != nil {
		operator := "<"
		if inclusive {
			operator = "<="
		}
		query += ` AND (created_at, id) ` + operator + ` ($4, $5::uuid)`
		args = append(args, afterCreatedAt, afterID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $3`
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []models.File{}
	for rows.Next() {
		var file models.File
		var scannedFolderID *string
		if err := rows.Scan(
			&file.ID,
			&scannedFolderID,
			&file.EncryptedMetadata,
			&file.EncryptedFileKey,
			&file.CreatedAt,
			&file.UpdatedAt,
			&file.IsShared,
		); err != nil {
			return nil, err
		}
		file.FolderID = scannedFolderID
		files = append(files, file)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return files, nil
}

func (r *Repository) GetListedFileCreatedAt(ctx context.Context, db database.PgExecutor, userID string, folderID *string, fileID string) (time.Time, error) {
	var createdAt time.Time
	err := db.QueryRow(ctx, `SELECT created_at
		FROM files
		WHERE id = $1
			AND user_id = $2
			AND upload_status = 'complete'
			AND expires_at IS NULL
			AND deleted_at IS NULL
			AND (($3::uuid IS NULL AND folder_id IS NULL) OR folder_id = $3)`,
		fileID, userID, folderID).Scan(&createdAt)
	return createdAt, err
}

func (r *Repository) MoveFilesToFolder(ctx context.Context, db database.PgExecutor, userID string, fileIDs []string, encryptedFileKeys [][]byte, targetFolderID *string) (int64, error) {
	query := `UPDATE files AS file
	SET
		folder_id = $4,
		encrypted_file_key = moved.encrypted_file_key,
		updated_at = now()
	FROM UNNEST($2::uuid[], $3::bytea[]) AS moved(id, encrypted_file_key)
	WHERE
		file.user_id = $1
		AND file.id = moved.id
		AND file.upload_status = 'complete'
		AND file.expires_at IS NULL
		AND file.deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, userID, fileIDs, encryptedFileKeys, targetFolderID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) RenameFileForUser(ctx context.Context, db database.PgExecutor, fileID, userID string, encryptedMetadata []byte) (bool, error) {
	query := `UPDATE files
	SET
		encrypted_metadata = $3,
		updated_at = now()
	WHERE
		id = $1
		AND user_id = $2
		AND upload_status = 'complete'
		AND expires_at IS NULL
		AND deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, fileID, userID, encryptedMetadata)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) DeleteFileForUser(ctx context.Context, db database.PgExecutor, fileID, userID string) (bool, error) {
	query := `UPDATE files
	SET
		deleted_at = now(),
		updated_at = now()
	WHERE
		id = $1
		AND user_id = $2
		AND deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, fileID, userID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) ListDeletedPendingPurge(ctx context.Context, db database.PgExecutor, cutoff time.Time, limit int) ([]models.File, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(ctx, `WITH RECURSIVE expired_folders(id, user_id) AS (
		SELECT id, user_id
		FROM folders
		WHERE deleted_at IS NOT NULL
			AND purged_at IS NULL
			AND (deleted_at <= $1 OR purge_requested_at IS NOT NULL)
		UNION
		SELECT child.id, child.user_id
		FROM folders child
		JOIN expired_folders parent ON parent.id = child.parent_folder_id AND parent.user_id = child.user_id
		WHERE child.deleted_at IS NOT NULL AND child.purged_at IS NULL
	), eligible_files(id) AS (
		SELECT id FROM files
		WHERE upload_status = 'complete'
			AND deleted_at IS NOT NULL
			AND purged_at IS NULL
			AND (deleted_at <= $1 OR purge_requested_at IS NOT NULL)
		UNION
		SELECT file.id
		FROM files file
		JOIN expired_folders folder ON folder.id = file.folder_id AND folder.user_id = file.user_id
		WHERE file.upload_status = 'complete' AND file.deleted_at IS NOT NULL AND file.purged_at IS NULL
	)
	SELECT file.id, file.user_id, file.actual_encrypted_size, file.thumbnail_size_bytes
	FROM files file
	JOIN eligible_files eligible ON eligible.id = file.id
	ORDER BY file.deleted_at ASC, file.id ASC
	LIMIT $2
	FOR UPDATE OF file SKIP LOCKED`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]models.File, 0, limit)
	for rows.Next() {
		var file models.File
		if err := rows.Scan(
			&file.ID,
			&file.UserID,
			&file.ActualEncryptedSize,
			&file.ThumbnailSizeBytes,
		); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return files, nil
}

func (r *Repository) MarkFilesPurged(ctx context.Context, db database.PgExecutor, fileIDs []string) ([]models.File, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}
	rows, err := db.Query(ctx, `UPDATE files
	SET
		purged_at = now(),
		purge_requested_at = NULL,
		updated_at = now()
	WHERE
		id = ANY($1::uuid[])
		AND deleted_at IS NOT NULL
		AND purged_at IS NULL
	RETURNING id, user_id, actual_encrypted_size, thumbnail_size_bytes`, fileIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]models.File, 0, len(fileIDs))
	for rows.Next() {
		var file models.File
		if err := rows.Scan(&file.ID, &file.UserID, &file.ActualEncryptedSize, &file.ThumbnailSizeBytes); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}
