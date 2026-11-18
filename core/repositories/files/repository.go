package filerepo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

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
		id, user_id, folder_id, COALESCE(storage_object_id, id)::text, version_number, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
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
		&created.StorageObjectID,
		&created.VersionNumber,
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
		files.id, upload_sessions.file_chunk_size, upload_sessions.total_chunks, upload_sessions.encrypted_size, files.upload_status
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

func (r *Repository) SnapshotCurrentVersion(ctx context.Context, db database.PgExecutor, fileID string) error {
	_, err := db.Exec(ctx, `INSERT INTO file_versions
		(file_id, version_number, folder_id, storage_object_id, encrypted_metadata, encrypted_file_key,
		 encrypted_manifest, encryption_version, chunk_size, chunk_count, actual_encrypted_size, encrypted_hash,
		 thumbnail_size_bytes, created_at)
	SELECT id, version_number, folder_id, COALESCE(storage_object_id, id), encrypted_metadata, encrypted_file_key,
		 encrypted_manifest, encryption_version, chunk_size, chunk_count, actual_encrypted_size, encrypted_hash,
		 thumbnail_size_bytes, updated_at
	FROM files
	WHERE id = $1 AND upload_status = 'complete' AND deleted_at IS NULL
	ON CONFLICT (file_id, version_number) DO NOTHING`, fileID)
	return err
}

func (r *Repository) LockFileForVersioning(ctx context.Context, db database.PgExecutor, fileID, userID string) (models.File, error) {
	var file models.File
	var folderID *string
	err := db.QueryRow(ctx, `SELECT id, user_id, folder_id, COALESCE(storage_object_id, id)::text, version_number,
		encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
		actual_encrypted_size, encrypted_hash, upload_status, thumbnail_size_bytes, expires_at
	FROM files
	WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND purged_at IS NULL
	FOR UPDATE`, fileID, userID).Scan(
		&file.ID, &file.UserID, &folderID, &file.StorageObjectID, &file.VersionNumber,
		&file.EncryptedMetadata, &file.EncryptedFileKey, &file.EncryptedManifest, &file.EncryptionVersion,
		&file.ChunkSize, &file.ChunkCount, &file.ActualEncryptedSize, &file.EncryptedHash,
		&file.UploadStatus, &file.ThumbnailSizeBytes, &file.ExpiresAt)
	file.FolderID = folderID
	return file, err
}

func (r *Repository) ApplyVersionUpload(ctx context.Context, db database.PgExecutor, fileID, storageObjectID string, encryptedMetadata, encryptedFileKey, encryptedManifest, encryptedHash []byte, encryptionVersion int16, chunkSize int64, chunkCount int, encryptedSize, thumbnailSize int64) error {
	tag, err := db.Exec(ctx, `UPDATE files
	SET storage_object_id = $2,
		encrypted_metadata = $3,
		encrypted_file_key = $4,
		encrypted_manifest = $5,
		encrypted_hash = $6,
		encryption_version = $7,
		chunk_size = $8,
		chunk_count = $9,
		actual_encrypted_size = $10,
		thumbnail_size_bytes = $11,
		version_number = version_number + 1,
		updated_at = now()
	WHERE id = $1 AND upload_status = 'complete' AND deleted_at IS NULL`,
		fileID, storageObjectID, encryptedMetadata, encryptedFileKey, encryptedManifest, encryptedHash,
		encryptionVersion, chunkSize, chunkCount, encryptedSize, thumbnailSize)
	if err == nil && tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}

func (r *Repository) ListFileVersionsForUser(ctx context.Context, db database.PgExecutor, fileID, userID string) ([]models.FileVersion, error) {
	rows, err := db.Query(ctx, `SELECT version.id, version.file_id, version.version_number, version.folder_id,
		version.storage_object_id, version.encrypted_metadata, version.encrypted_file_key, version.encrypted_manifest,
		version.encryption_version, version.chunk_size, version.chunk_count, version.actual_encrypted_size,
		version.encrypted_hash, version.thumbnail_size_bytes, version.created_at
	FROM file_versions version
	JOIN files file ON file.id = version.file_id
	WHERE version.file_id = $1 AND file.user_id = $2 AND file.upload_status = 'complete'
		AND file.expires_at IS NULL AND file.deleted_at IS NULL AND file.purged_at IS NULL
	ORDER BY version.version_number DESC`, fileID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]models.FileVersion, 0)
	for rows.Next() {
		var version models.FileVersion
		if err := rows.Scan(&version.ID, &version.FileID, &version.VersionNumber, &version.FolderID,
			&version.StorageObjectID, &version.EncryptedMetadata, &version.EncryptedFileKey, &version.EncryptedManifest,
			&version.EncryptionVersion, &version.ChunkSize, &version.ChunkCount, &version.ActualEncryptedSize,
			&version.EncryptedHash, &version.ThumbnailSizeBytes, &version.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (r *Repository) GetFileVersionForUser(ctx context.Context, db database.PgExecutor, fileID, versionID, userID string) (models.FileVersion, error) {
	var version models.FileVersion
	err := db.QueryRow(ctx, `SELECT version.id, version.file_id, version.version_number, version.folder_id,
		version.storage_object_id, version.encrypted_metadata, version.encrypted_file_key, version.encrypted_manifest,
		version.encryption_version, version.chunk_size, version.chunk_count, version.actual_encrypted_size,
		version.encrypted_hash, version.thumbnail_size_bytes, version.created_at
	FROM file_versions version
	JOIN files file ON file.id = version.file_id
	WHERE version.file_id = $1 AND version.id = $2 AND file.user_id = $3
		AND file.upload_status = 'complete' AND file.expires_at IS NULL
		AND file.deleted_at IS NULL AND file.purged_at IS NULL`, fileID, versionID, userID).Scan(
		&version.ID, &version.FileID, &version.VersionNumber, &version.FolderID,
		&version.StorageObjectID, &version.EncryptedMetadata, &version.EncryptedFileKey, &version.EncryptedManifest,
		&version.EncryptionVersion, &version.ChunkSize, &version.ChunkCount, &version.ActualEncryptedSize,
		&version.EncryptedHash, &version.ThumbnailSizeBytes, &version.CreatedAt)
	return version, err
}

func (r *Repository) RestoreFileVersion(ctx context.Context, db database.PgExecutor, fileID, versionID string, encryptedMetadata, encryptedFileKey []byte) error {
	_, err := db.Exec(ctx, `UPDATE files current
	SET encrypted_metadata = $3,
		encrypted_file_key = $4,
		encrypted_manifest = version.encrypted_manifest,
		encryption_version = version.encryption_version,
		chunk_size = version.chunk_size,
		chunk_count = version.chunk_count,
		actual_encrypted_size = version.actual_encrypted_size,
		encrypted_hash = version.encrypted_hash,
		thumbnail_size_bytes = version.thumbnail_size_bytes,
		storage_object_id = version.storage_object_id,
		version_number = current.version_number + 1,
		updated_at = now()
	FROM file_versions version
	WHERE current.id = $1 AND version.id = $2 AND version.file_id = current.id
		AND current.upload_status = 'complete' AND current.deleted_at IS NULL`, fileID, versionID, encryptedMetadata, encryptedFileKey)
	return err
}

func (r *Repository) DeleteFileVersion(ctx context.Context, db database.PgExecutor, fileID, versionID, userID string) (models.FileVersion, bool, error) {
	var version models.FileVersion
	err := db.QueryRow(ctx, `DELETE FROM file_versions version
	USING files file
	WHERE version.id = $1 AND version.file_id = $2 AND file.id = version.file_id AND file.user_id = $3
		AND file.deleted_at IS NULL AND file.purged_at IS NULL
	RETURNING version.id, version.file_id, version.version_number, version.folder_id, version.storage_object_id,
		version.actual_encrypted_size, version.thumbnail_size_bytes`, versionID, fileID, userID).Scan(
		&version.ID, &version.FileID, &version.VersionNumber, &version.FolderID, &version.StorageObjectID,
		&version.ActualEncryptedSize, &version.ThumbnailSizeBytes)
	if err != nil {
		if err == pgx.ErrNoRows {
			return models.FileVersion{}, false, nil
		}
		return models.FileVersion{}, false, err
	}
	var referenced bool
	if err := db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM files WHERE id = $1 AND COALESCE(storage_object_id, id) = $2
		UNION ALL
		SELECT 1 FROM file_versions WHERE file_id = $1 AND storage_object_id = $2
	)`, fileID, version.StorageObjectID).Scan(&referenced); err != nil {
		return models.FileVersion{}, false, err
	}
	return version, !referenced, nil
}

func (r *Repository) GetEncryptedFileRecordForUser(ctx context.Context, db database.PgExecutor, fileID, userID string) (models.File, error) {
	var file models.File
	var folderID *string
	query := `SELECT
		id, user_id, folder_id, COALESCE(storage_object_id, id)::text, version_number, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version,
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
		&file.StorageObjectID,
		&file.VersionNumber,
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
		COUNT(DISTINCT upload_sessions.id)
	FROM
		files
	INNER JOIN
		upload_sessions ON upload_sessions.file_id = files.id
	WHERE
		files.user_id = $1
		AND (files.upload_status IN ('pending', 'uploading') OR upload_sessions.is_version)
		AND files.deleted_at IS NULL
		AND upload_sessions.status = 'active'
		AND upload_sessions.expires_at > now()`
	if err := db.QueryRow(ctx, query, userID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (r *Repository) GetFileByID(ctx context.Context, db database.PgExecutor, fileID string) (models.File, error) {
	var file models.File
	var folderID *string
	query := `SELECT
		id, user_id, folder_id, COALESCE(storage_object_id, id)::text, version_number, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
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
		&file.StorageObjectID,
		&file.VersionNumber,
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
		id, user_id, folder_id, COALESCE(storage_object_id, id)::text, version_number, encrypted_metadata, encrypted_file_key, encrypted_manifest, encryption_version, chunk_size, chunk_count,
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
		&file.StorageObjectID,
		&file.VersionNumber,
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
	return r.listByFolder(ctx, db, userID, folderID, limit, afterCreatedAt, afterID, inclusive, false)
}

func (r *Repository) ListByFolderBefore(ctx context.Context, db database.PgExecutor, userID string, folderID *string, limit int, beforeCreatedAt *time.Time, beforeID *string) ([]models.File, error) {
	return r.listByFolder(ctx, db, userID, folderID, limit, beforeCreatedAt, beforeID, false, true)
}

func (r *Repository) listByFolder(ctx context.Context, db database.PgExecutor, userID string, folderID *string, limit int, cursorCreatedAt *time.Time, cursorID *string, inclusive, before bool) ([]models.File, error) {
	query := `SELECT
		file.id, file.folder_id, file.encrypted_metadata, file.encrypted_file_key, file.created_at, file.updated_at,
		file.version_number,
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
	if cursorCreatedAt != nil {
		operator := "<"
		if before {
			operator = ">"
		}
		if inclusive {
			operator = "<="
		}
		query += ` AND (created_at, id) ` + operator + ` ($4, $5::uuid)`
		args = append(args, cursorCreatedAt, cursorID)
	}
	order := "DESC"
	if before {
		order = "ASC"
	}
	query += ` ORDER BY created_at ` + order + `, id ` + order + ` LIMIT $3`
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
			&file.VersionNumber,
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
	if before {
		for left, right := 0, len(files)-1; left < right; left, right = left+1, right-1 {
			files[left], files[right] = files[right], files[left]
		}
	}
	return files, nil
}

func (r *Repository) CountListedFilesBefore(ctx context.Context, db database.PgExecutor, userID string, folderID *string, createdAt time.Time, id string) (int, error) {
	var count int
	err := db.QueryRow(ctx, `SELECT COUNT(1)
		FROM files
		WHERE user_id = $1 AND upload_status = 'complete' AND expires_at IS NULL AND deleted_at IS NULL
			AND (($2::uuid IS NULL AND folder_id IS NULL) OR folder_id = $2)
			AND (created_at, id) > ($3, $4::uuid)`, userID, folderID, createdAt, id).Scan(&count)
	return count, err
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
	SELECT file.id, file.user_id, COALESCE(file.storage_object_id, file.id)::text,
		file.actual_encrypted_size, file.thumbnail_size_bytes,
		(SELECT COALESCE(SUM(objects.size_bytes), 0)
		 FROM (
			SELECT stored.storage_object_id, MAX(stored.size_bytes) AS size_bytes
			FROM (
				SELECT COALESCE(current.storage_object_id, current.id)::text AS storage_object_id,
					current.actual_encrypted_size + current.thumbnail_size_bytes AS size_bytes
				FROM files current WHERE current.id = file.id
				UNION ALL
				SELECT version.storage_object_id::text,
					version.actual_encrypted_size + version.thumbnail_size_bytes
				FROM file_versions version WHERE version.file_id = file.id
			) stored
			GROUP BY stored.storage_object_id
		) objects)
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
			&file.StorageObjectID,
			&file.ActualEncryptedSize,
			&file.ThumbnailSizeBytes,
			&file.TotalStoredBytes,
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

func (r *Repository) ListVersionStorageObjects(ctx context.Context, db database.PgExecutor, fileIDs []string) (map[string][]models.FileVersion, error) {
	objects := make(map[string][]models.FileVersion, len(fileIDs))
	if len(fileIDs) == 0 {
		return objects, nil
	}
	rows, err := db.Query(ctx, `SELECT file_id, storage_object_id, thumbnail_size_bytes
	FROM file_versions WHERE file_id = ANY($1::uuid[])`, fileIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var version models.FileVersion
		if err := rows.Scan(&version.FileID, &version.StorageObjectID, &version.ThumbnailSizeBytes); err != nil {
			return nil, err
		}
		objects[version.FileID] = append(objects[version.FileID], version)
	}
	return objects, rows.Err()
}

func (r *Repository) DeleteVersionsForFiles(ctx context.Context, db database.PgExecutor, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	_, err := db.Exec(ctx, `DELETE FROM file_versions WHERE file_id = ANY($1::uuid[])`, fileIDs)
	return err
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
