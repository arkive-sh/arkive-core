package trashrepo

import (
	"context"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

type Repository struct{}

type Cursor struct {
	DeletedAt time.Time
	Type      string
	ID        string
}

func New() *Repository {
	return &Repository{}
}

func (r *Repository) ListEntriesPage(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, cursor *Cursor, limit int) ([]models.TrashEntry, error) {
	var cursorTime any
	var cursorTypeOrder any
	var cursorID any
	if cursor != nil {
		cursorTime = cursor.DeletedAt
		cursorID = cursor.ID
		if cursor.Type == "folder" {
			cursorTypeOrder = 0
		} else {
			cursorTypeOrder = 1
		}
	}

	rows, err := db.Query(ctx, `WITH entries AS (
		SELECT
			0 AS entry_type_order,
			'folder' AS entry_type,
			f.id,
			f.parent_folder_id,
			f.encrypted_name,
			NULL::bytea AS encrypted_metadata,
			f.deleted_at
		FROM folders f
		WHERE f.user_id = $1
			AND f.deleted_at IS NOT NULL
			AND f.deleted_at > $2
			AND f.purged_at IS NULL
			AND f.purge_requested_at IS NULL
			AND (f.parent_folder_id IS NULL OR NOT EXISTS (
				SELECT 1 FROM folders parent
				WHERE parent.id = f.parent_folder_id
					AND parent.user_id = f.user_id
					AND parent.deleted_at IS NOT NULL
			))

		UNION ALL

		SELECT
			1 AS entry_type_order,
			'file' AS entry_type,
			file.id,
			file.folder_id AS parent_folder_id,
			NULL::bytea AS encrypted_name,
			file.encrypted_metadata,
			file.deleted_at
		FROM files file
		WHERE file.user_id = $1
			AND file.deleted_at IS NOT NULL
			AND file.deleted_at > $2
			AND file.purged_at IS NULL
			AND file.purge_requested_at IS NULL
			AND (file.folder_id IS NULL OR NOT EXISTS (
				SELECT 1 FROM folders parent
				WHERE parent.id = file.folder_id
					AND parent.user_id = file.user_id
					AND parent.deleted_at IS NOT NULL
			))
	)
	SELECT entry_type, id::text, parent_folder_id::text, encrypted_name, encrypted_metadata, deleted_at
	FROM entries
	WHERE $3::timestamptz IS NULL
		OR deleted_at < $3
		OR (deleted_at = $3 AND entry_type_order > $4)
		OR (deleted_at = $3 AND entry_type_order = $4 AND id > $5::uuid)
	ORDER BY deleted_at DESC, entry_type_order ASC, id ASC
	LIMIT $6`, userID, cutoff, cursorTime, cursorTypeOrder, cursorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]models.TrashEntry, 0, limit)
	for rows.Next() {
		var entry models.TrashEntry
		if err := rows.Scan(
			&entry.Type,
			&entry.ID,
			&entry.ParentFolderID,
			&entry.EncryptedName,
			&entry.EncryptedMetadata,
			&entry.DeletedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *Repository) CountRestorableRoots(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs, folderIDs []string) (int, int, error) {
	var fileCount, folderCount int
	err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM files file
		 WHERE file.user_id = $1
			AND file.id = ANY($3::uuid[])
			AND file.deleted_at IS NOT NULL
			AND file.deleted_at > $2
			AND file.purged_at IS NULL
			AND file.purge_requested_at IS NULL
			AND (file.folder_id IS NULL OR NOT EXISTS (
				SELECT 1 FROM folders parent
				WHERE parent.id = file.folder_id AND parent.user_id = file.user_id AND parent.deleted_at IS NOT NULL
			))),
		(SELECT count(*) FROM folders folder
		 WHERE folder.user_id = $1
			AND folder.id = ANY($4::uuid[])
			AND folder.deleted_at IS NOT NULL
			AND folder.deleted_at > $2
			AND folder.purged_at IS NULL
			AND folder.purge_requested_at IS NULL
			AND (folder.parent_folder_id IS NULL OR NOT EXISTS (
				SELECT 1 FROM folders parent
				WHERE parent.id = folder.parent_folder_id AND parent.user_id = folder.user_id AND parent.deleted_at IS NOT NULL
			)))`, userID, cutoff, fileIDs, folderIDs).Scan(&fileCount, &folderCount)
	return fileCount, folderCount, err
}

func (r *Repository) LockTrashFolders(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, folderIDs []string) (int, error) {
	rows, err := db.Query(ctx, `SELECT id FROM folders
	WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at > $3 AND purged_at IS NULL AND purge_requested_at IS NULL
	ORDER BY id
	FOR UPDATE`, userID, folderIDs, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		count++
	}
	return count, rows.Err()
}

func (r *Repository) LockRestorableFolderFiles(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) (bool, error) {
	rows, err := db.Query(ctx, `WITH RECURSIVE tree(id, deleted_at) AS (
		SELECT id, deleted_at FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NOT NULL AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id, child.deleted_at
		FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id AND child.deleted_at = parent.deleted_at
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	)
	SELECT file.purged_at, file.purge_requested_at
	FROM files file
	JOIN tree ON tree.id = file.folder_id AND tree.deleted_at = file.deleted_at
	WHERE file.user_id = $1
	FOR UPDATE OF file`, userID, folderIDs)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	blocked := false
	for rows.Next() {
		var purgedAt, purgeRequestedAt *time.Time
		if err := rows.Scan(&purgedAt, &purgeRequestedAt); err != nil {
			return false, err
		}
		blocked = blocked || purgedAt != nil || purgeRequestedAt != nil
	}
	return blocked, rows.Err()
}

func (r *Repository) RestoreFolderTrees(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, folderIDs []string) (int64, int64, error) {
	// Matching delete timestamps keeps older, separately trashed descendants in Trash.
	files, err := db.Exec(ctx, `WITH RECURSIVE tree(id, deleted_at) AS (
		SELECT id, deleted_at FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at > $3 AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id, child.deleted_at
		FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id AND child.deleted_at = parent.deleted_at
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	)
	UPDATE files file
	SET deleted_at = NULL, updated_at = now()
	WHERE file.user_id = $1
		AND file.purged_at IS NULL
		AND file.purge_requested_at IS NULL
		AND file.deleted_at > $3
		AND EXISTS (
			SELECT 1 FROM tree
			WHERE tree.id = file.folder_id AND tree.deleted_at = file.deleted_at
		)`, userID, folderIDs, cutoff)
	if err != nil {
		return 0, 0, err
	}

	folders, err := db.Exec(ctx, `WITH RECURSIVE tree(id, deleted_at) AS (
		SELECT id, deleted_at FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at > $3 AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id, child.deleted_at
		FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id AND child.deleted_at = parent.deleted_at
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	)
	UPDATE folders folder
	SET deleted_at = NULL, updated_at = now()
	WHERE folder.user_id = $1 AND folder.id IN (SELECT id FROM tree)`, userID, folderIDs, cutoff)
	if err != nil {
		return 0, 0, err
	}
	return files.RowsAffected(), folders.RowsAffected(), nil
}

func (r *Repository) RestoreFiles(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs []string) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE files file
	SET deleted_at = NULL, updated_at = now()
	WHERE file.user_id = $1
		AND file.id = ANY($2::uuid[])
		AND file.deleted_at IS NOT NULL
		AND file.deleted_at > $3
		AND file.purged_at IS NULL
		AND file.purge_requested_at IS NULL
		AND (file.folder_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM folders parent
			WHERE parent.id = file.folder_id AND parent.user_id = file.user_id AND parent.deleted_at IS NOT NULL
		))`, userID, fileIDs, cutoff)
	return tag.RowsAffected(), err
}

func (r *Repository) RequestPurge(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs, folderIDs []string) (int64, int64, error) {
	files, err := db.Exec(ctx, `UPDATE files file
	SET purge_requested_at = now(), updated_at = now()
	WHERE file.user_id = $1
		AND file.id = ANY($2::uuid[])
		AND file.deleted_at > $3
		AND file.purge_requested_at IS NULL
		AND file.purged_at IS NULL
		AND (file.folder_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM folders parent
			WHERE parent.id = file.folder_id AND parent.user_id = file.user_id AND parent.deleted_at IS NOT NULL
		))`, userID, fileIDs, cutoff)
	if err != nil {
		return 0, 0, err
	}
	folders, err := db.Exec(ctx, `UPDATE folders folder
	SET purge_requested_at = now(), updated_at = now()
	WHERE folder.user_id = $1
		AND folder.id = ANY($2::uuid[])
		AND folder.deleted_at > $3
		AND folder.purge_requested_at IS NULL
		AND folder.purged_at IS NULL
		AND (folder.parent_folder_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM folders parent
			WHERE parent.id = folder.parent_folder_id AND parent.user_id = folder.user_id AND parent.deleted_at IS NOT NULL
		))`, userID, folderIDs, cutoff)
	if err != nil {
		return 0, 0, err
	}
	return files.RowsAffected(), folders.RowsAffected(), nil
}

func (r *Repository) MarkExpiredFoldersPurged(ctx context.Context, db database.PgExecutor, cutoff time.Time) (int64, error) {
	// Keep folder tombstones until their retained file objects have all been purged.
	tag, err := db.Exec(ctx, `WITH RECURSIVE expired_tree(root_id, folder_id, user_id) AS (
		SELECT id, id, user_id FROM folders
		WHERE deleted_at IS NOT NULL AND purged_at IS NULL
			AND (deleted_at <= $1 OR purge_requested_at IS NOT NULL)
		UNION
		SELECT tree.root_id, child.id, tree.user_id
		FROM expired_tree tree
		JOIN folders child ON child.parent_folder_id = tree.folder_id AND child.user_id = tree.user_id
		WHERE child.deleted_at IS NOT NULL AND child.purged_at IS NULL
	), ready AS (
		SELECT root.id
		FROM folders root
		WHERE root.deleted_at IS NOT NULL AND root.purged_at IS NULL
			AND (root.deleted_at <= $1 OR root.purge_requested_at IS NOT NULL)
			AND NOT EXISTS (
				SELECT 1 FROM expired_tree tree
				JOIN files file ON file.folder_id = tree.folder_id AND file.user_id = tree.user_id
				WHERE tree.root_id = root.id AND file.deleted_at IS NOT NULL AND file.purged_at IS NULL
			)
	)
	UPDATE folders folder
	SET purged_at = now(), purge_requested_at = NULL, updated_at = now()
	WHERE folder.purged_at IS NULL AND folder.id IN (
		SELECT tree.folder_id FROM expired_tree tree JOIN ready root ON root.id = tree.root_id
	)`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
