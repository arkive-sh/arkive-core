package foldersrepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct{}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) Create(ctx context.Context, db database.PgExecutor, folder models.Folder) (models.Folder, error) {
	var created models.Folder
	var parentFolderID *string
	query := `INSERT INTO folders
		(id, user_id, vault_id, parent_folder_id, encrypted_name, encrypted_metadata)
	VALUES
		($1, $2, $3, $4, $5, $6)
	RETURNING
		id, user_id, vault_id, parent_folder_id, encrypted_name, encrypted_metadata, created_at, updated_at, deleted_at`
	if err := db.QueryRow(
		ctx,
		query,
		folder.ID,
		folder.UserID,
		folder.VaultID,
		folder.ParentFolderID,
		folder.EncryptedName,
		folder.EncryptedMetadata,
	).Scan(
		&created.ID,
		&created.UserID,
		&created.VaultID,
		&parentFolderID,
		&created.EncryptedName,
		&created.EncryptedMetadata,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.DeletedAt,
	); err != nil {
		return models.Folder{}, err
	}
	created.ParentFolderID = parentFolderID
	return created, nil
}

func (r *Repository) GetForUser(ctx context.Context, db database.PgExecutor, userID, folderID string) (models.Folder, error) {
	var folder models.Folder
	var parentFolderID *string
	query := `SELECT
		id, user_id, vault_id, parent_folder_id, encrypted_name, encrypted_metadata, created_at, updated_at, deleted_at
	FROM
		folders
	WHERE
		id = $1
		AND user_id = $2
		AND deleted_at IS NULL`
	if err := db.QueryRow(ctx, query, folderID, userID).Scan(
		&folder.ID,
		&folder.UserID,
		&folder.VaultID,
		&parentFolderID,
		&folder.EncryptedName,
		&folder.EncryptedMetadata,
		&folder.CreatedAt,
		&folder.UpdatedAt,
		&folder.DeletedAt,
	); err != nil {
		return models.Folder{}, err
	}
	folder.ParentFolderID = parentFolderID
	return folder, nil
}

func (r *Repository) CountChildFolders(ctx context.Context, db database.PgExecutor, userID string, parentID *string) (int, error) {
	var count int
	query := `SELECT
		COUNT(1)
	FROM
		folders
	WHERE
		user_id = $1
		AND deleted_at IS NULL
		AND (
			($2::uuid IS NULL AND parent_folder_id IS NULL)
			OR parent_folder_id = $2
		)`
	if err := db.QueryRow(ctx, query, userID, parentID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) ListChildFolders(ctx context.Context, db database.PgExecutor, userID string, parentID *string, limit, offset int) ([]models.Folder, error) {
	query := `SELECT
		id, user_id, vault_id, parent_folder_id, encrypted_name, encrypted_metadata, created_at, updated_at, deleted_at
	FROM
		folders
	WHERE
		user_id = $1
		AND deleted_at IS NULL
		AND (
			($2::uuid IS NULL AND parent_folder_id IS NULL)
			OR parent_folder_id = $2
		)
	ORDER BY created_at DESC
	LIMIT $3 OFFSET $4`
	rows, err := db.Query(ctx, query, userID, parentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []models.Folder{}
	for rows.Next() {
		var folder models.Folder
		var parentFolderID *string
		if err := rows.Scan(
			&folder.ID,
			&folder.UserID,
			&folder.VaultID,
			&parentFolderID,
			&folder.EncryptedName,
			&folder.EncryptedMetadata,
			&folder.CreatedAt,
			&folder.UpdatedAt,
			&folder.DeletedAt,
		); err != nil {
			return nil, err
		}
		folder.ParentFolderID = parentFolderID
		folders = append(folders, folder)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return folders, nil
}

func (r *Repository) ListChildFoldersAfter(ctx context.Context, db database.PgExecutor, userID string, parentID *string, limit int, afterCreatedAt *time.Time, afterID *string, inclusive bool) ([]models.Folder, error) {
	query := `WITH RECURSIVE folder_page AS MATERIALIZED (
		SELECT
			id, vault_id, parent_folder_id, encrypted_name, encrypted_metadata, created_at, updated_at
		FROM folders
		WHERE
			user_id = $1
			AND deleted_at IS NULL
			AND (
				($2::uuid IS NULL AND parent_folder_id IS NULL)
				OR parent_folder_id = $2
			)`
	args := []any{userID, parentID, limit, limit - 1}
	if afterCreatedAt != nil {
		operator := "<"
		if inclusive {
			operator = "<="
		}
		query += ` AND (created_at, id) ` + operator + ` ($5, $6::uuid)`
		args = append(args, afterCreatedAt, afterID)
	}
	query += `
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	), folder_size_roots AS (
		-- Sum ciphertext sizes only for folders on this page; avoid persisted folder totals that need updates on every mutation.
		SELECT id
		FROM folder_page
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	), folder_tree(root_id, folder_id) AS (
		SELECT id, id FROM folder_size_roots
		UNION
		SELECT tree.root_id, child.id
		FROM folder_tree tree
		INNER JOIN folders child ON child.parent_folder_id = tree.folder_id
		WHERE child.user_id = $1 AND child.deleted_at IS NULL
	), folder_sizes AS (
		SELECT tree.root_id, COALESCE(SUM(file.actual_encrypted_size), 0)::bigint AS size_bytes
		FROM folder_tree tree
		INNER JOIN files file ON file.folder_id = tree.folder_id
		WHERE
			file.user_id = $1
			AND file.upload_status = 'complete'
			AND file.expires_at IS NULL
			AND file.deleted_at IS NULL
		GROUP BY tree.root_id
	)
	SELECT
		folder_page.id,
		folder_page.vault_id,
		folder_page.parent_folder_id,
		folder_page.encrypted_name,
		folder_page.encrypted_metadata,
		folder_page.created_at,
		folder_page.updated_at,
		COALESCE(folder_sizes.size_bytes, 0)::bigint
	FROM folder_page
	LEFT JOIN folder_sizes ON folder_sizes.root_id = folder_page.id
	ORDER BY folder_page.created_at DESC, folder_page.id DESC`
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := []models.Folder{}
	for rows.Next() {
		var folder models.Folder
		var parentFolderID *string
		var sizeBytes int64
		if err := rows.Scan(
			&folder.ID,
			&folder.VaultID,
			&parentFolderID,
			&folder.EncryptedName,
			&folder.EncryptedMetadata,
			&folder.CreatedAt,
			&folder.UpdatedAt,
			&sizeBytes,
		); err != nil {
			return nil, err
		}
		folder.ParentFolderID = parentFolderID
		folder.SizeBytes = &sizeBytes
		folders = append(folders, folder)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return folders, nil
}

func (r *Repository) GetChildFolderCreatedAt(ctx context.Context, db database.PgExecutor, userID string, parentID *string, folderID string) (time.Time, error) {
	var createdAt time.Time
	err := db.QueryRow(ctx, `SELECT created_at
		FROM folders
		WHERE id = $1
			AND user_id = $2
			AND deleted_at IS NULL
			AND (($3::uuid IS NULL AND parent_folder_id IS NULL) OR parent_folder_id = $3)`,
		folderID, userID, parentID).Scan(&createdAt)
	return createdAt, err
}

func (r *Repository) MoveFolders(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string, encryptedNames, encryptedMetadata [][]byte, targetFolderID *string) (int64, error) {
	query := `UPDATE folders AS folder
	SET
		parent_folder_id = $5,
		encrypted_name = moved.encrypted_name,
		encrypted_metadata = moved.encrypted_metadata,
		updated_at = now()
	FROM UNNEST($2::uuid[], $3::bytea[], $4::bytea[]) AS moved(id, encrypted_name, encrypted_metadata)
	WHERE
		folder.user_id = $1
		AND folder.id = moved.id
		AND folder.deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, userID, folderIDs, encryptedNames, encryptedMetadata, targetFolderID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) RenameFolderForUser(ctx context.Context, db database.PgExecutor, userID, folderID string, encryptedName, encryptedMetadata []byte) (bool, error) {
	query := `UPDATE folders
	SET
		encrypted_name = $3,
		encrypted_metadata = $4,
		updated_at = now()
	WHERE
		id = $1
		AND user_id = $2
		AND deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, folderID, userID, encryptedName, encryptedMetadata)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) TargetIsDescendant(ctx context.Context, db database.PgExecutor, userID string, movingFolderIDs []string, targetFolderID string) (bool, error) {
	var exists bool
	query := `WITH RECURSIVE descendants AS (
		SELECT id
		FROM folders
		WHERE user_id = $1
			AND deleted_at IS NULL
			AND id = ANY($2::uuid[])
		UNION ALL
		SELECT f.id
		FROM folders f
		INNER JOIN descendants d ON f.parent_folder_id = d.id
		WHERE f.user_id = $1
			AND f.deleted_at IS NULL
	)
	SELECT EXISTS(
		SELECT 1
		FROM descendants
		WHERE id = $3::uuid
	)`
	if err := db.QueryRow(ctx, query, userID, movingFolderIDs, targetFolderID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *Repository) DescendantFolderIDs(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) ([]string, error) {
	query := `WITH RECURSIVE descendants AS (
		SELECT id
		FROM folders
		WHERE user_id = $1
			AND id = ANY($2::uuid[])
			AND deleted_at IS NULL
		UNION ALL
		SELECT f.id
		FROM folders f
		INNER JOIN descendants d ON f.parent_folder_id = d.id
		WHERE f.user_id = $1
			AND f.deleted_at IS NULL
	)
	SELECT id::text
	FROM descendants`
	rows, err := db.Query(ctx, query, userID, folderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resolved := make([]string, 0, len(folderIDs))
	for rows.Next() {
		var folderID string
		if err := rows.Scan(&folderID); err != nil {
			return nil, err
		}
		resolved = append(resolved, folderID)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return resolved, nil
}

func (r *Repository) FileIDsInFolders(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) ([]string, error) {
	query := `SELECT id::text
	FROM files
	WHERE user_id = $1
		AND upload_status = 'complete'
		AND expires_at IS NULL
		AND deleted_at IS NULL
		AND folder_id = ANY($2::uuid[])`
	rows, err := db.Query(ctx, query, userID, folderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fileIDs := make([]string, 0)
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return fileIDs, nil
}

func (r *Repository) SoftDeleteFolders(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) (int64, error) {
	query := `UPDATE folders
	SET
		deleted_at = now(),
		updated_at = now()
	WHERE
		user_id = $1
		AND id = ANY($2::uuid[])
		AND deleted_at IS NULL`
	tag, err := db.Exec(ctx, query, userID, folderIDs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
