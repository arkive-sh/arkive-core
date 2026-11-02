package uploadrepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct{}

type StaleUpload struct {
	ID               string
	FileID           string
	UserID           string
	ProviderUploadID string
	EncryptedSize    int64
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) CreateUploadSession(ctx context.Context, db database.PgExecutor, upload models.UploadSession) (models.UploadSession, error) {
	var created models.UploadSession
	query := `INSERT INTO upload_sessions
		(file_id, provider_upload_id, encrypted_size, upload_part_size, upload_part_count, status, expires_at)
	VALUES
		($1, $2, $3, $4, $5, $6, $7)
	RETURNING
		id, file_id, provider_upload_id, encrypted_size, upload_part_size, upload_part_count, status, expires_at`
	if err := db.QueryRow(ctx, query,
		upload.FileID,
		upload.ProviderUploadID,
		upload.EncryptedSize,
		upload.UploadPartSize,
		upload.UploadPartCount,
		upload.Status,
		upload.ExpiresAt,
	).Scan(
		&created.ID,
		&created.FileID,
		&created.ProviderUploadID,
		&created.EncryptedSize,
		&created.UploadPartSize,
		&created.UploadPartCount,
		&created.Status,
		&created.ExpiresAt,
	); err != nil {
		return models.UploadSession{}, err
	}
	return created, nil
}

func (r *Repository) GetUploadSessionForUser(ctx context.Context, db database.PgExecutor, uploadSessionID, ownerID string) (models.UploadSession, error) {
	var upload models.UploadSession
	query := `SELECT
		upload_sessions.id, upload_sessions.file_id, upload_sessions.provider_upload_id,
		upload_sessions.upload_part_size,
		upload_sessions.upload_part_count,
		upload_sessions.status, upload_sessions.expires_at
	FROM
		upload_sessions
	INNER JOIN
		files ON files.id = upload_sessions.file_id
	WHERE
		upload_sessions.id = $1 AND files.user_id = $2`
	if err := db.QueryRow(ctx, query, uploadSessionID, ownerID).Scan(
		&upload.ID,
		&upload.FileID,
		&upload.ProviderUploadID,
		&upload.UploadPartSize,
		&upload.UploadPartCount,
		&upload.Status,
		&upload.ExpiresAt,
	); err != nil {
		return models.UploadSession{}, err
	}
	return upload, nil
}

func (r *Repository) UpdateUploadSessionStatus(ctx context.Context, db database.PgExecutor, uploadSessionID, status string) error {
	query := `UPDATE
		upload_sessions
	SET
		status = $2,
		encrypted_size = CASE WHEN $2 IN ('active', 'failed') THEN encrypted_size ELSE 0 END
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, uploadSessionID, status)
	return err
}

func (r *Repository) ListExpiredActiveUploads(ctx context.Context, db database.PgExecutor, limit int) ([]StaleUpload, error) {
	rows, err := db.Query(ctx, `SELECT
		upload_sessions.id, upload_sessions.file_id, files.user_id,
		COALESCE(upload_sessions.provider_upload_id, ''), upload_sessions.encrypted_size
	FROM upload_sessions
	JOIN files ON files.id = upload_sessions.file_id
	WHERE upload_sessions.status = 'active' AND upload_sessions.expires_at <= now()
	ORDER BY upload_sessions.expires_at
	LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []StaleUpload
	for rows.Next() {
		var upload StaleUpload
		if err := rows.Scan(&upload.ID, &upload.FileID, &upload.UserID,
			&upload.ProviderUploadID, &upload.EncryptedSize); err != nil {
			return nil, err
		}
		uploads = append(uploads, upload)
	}
	return uploads, rows.Err()
}

func (r *Repository) ExpireUploadSession(ctx context.Context, db database.PgExecutor, uploadSessionID string) (bool, error) {
	tag, err := db.Exec(ctx, `UPDATE upload_sessions
	SET status = 'expired', encrypted_size = 0
	WHERE id = $1 AND status = 'active'`, uploadSessionID)
	return tag.RowsAffected() > 0, err
}

func (r *Repository) TouchUploadSession(ctx context.Context, db database.PgExecutor, uploadSessionID, status string, expiresAt time.Time) error {
	query := `UPDATE
		upload_sessions
	SET
		status = $2,
		expires_at = $3
	WHERE
		id = $1`
	_, err := db.Exec(ctx, query, uploadSessionID, status, expiresAt)
	return err
}

func (r *Repository) UpsertUploadPart(ctx context.Context, db database.PgExecutor, part models.UploadPart) error {
	query := `INSERT INTO upload_parts
		(upload_session_id, part_number, etag, encrypted_hash)
	VALUES
		($1, $2, $3, $4)
	ON CONFLICT (upload_session_id, part_number)
	DO UPDATE SET
		etag = EXCLUDED.etag,
		encrypted_hash = EXCLUDED.encrypted_hash`
	_, err := db.Exec(ctx, query,
		part.UploadSessionID,
		part.PartNumber,
		part.ETag,
		part.EncryptedHash,
	)
	return err
}

func (r *Repository) ListUploadParts(ctx context.Context, db database.PgExecutor, uploadSessionID string) ([]models.UploadPart, error) {
	rows, err := db.Query(ctx, `SELECT
		upload_session_id, part_number, COALESCE(etag, ''), encrypted_hash
	FROM
		upload_parts
	WHERE
		upload_session_id = $1
	ORDER BY
		part_number ASC`, uploadSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parts []models.UploadPart
	for rows.Next() {
		var part models.UploadPart
		if err := rows.Scan(
			&part.UploadSessionID,
			&part.PartNumber,
			&part.ETag,
			&part.EncryptedHash,
		); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return parts, nil
}
