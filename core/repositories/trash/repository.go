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

func (r *Repository) ListEntriesPage(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, parentFolderID string, cursor *Cursor, limit int) ([]models.TrashEntry, error) {
	var parentID any
	if parentFolderID != "" {
		parentID = parentFolderID
	}

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

	rows, err := db.Query(ctx, `WITH RECURSIVE trash_entries AS (
		SELECT 'folder' AS entry_type, f.id, f.parent_folder_id, f.deleted_at, f.user_id
		FROM folders f
		WHERE f.user_id = $1 AND f.deleted_at > $2 AND f.purged_at IS NULL AND f.purge_requested_at IS NULL

		UNION ALL

		SELECT 'file', file.id, file.folder_id, file.deleted_at, file.user_id
		FROM files file
		WHERE file.user_id = $1 AND file.deleted_at > $2 AND file.purged_at IS NULL AND file.purge_requested_at IS NULL
	), trash_ancestors(folder_id, user_id, deleted_at) AS (
		SELECT parent_folder_id, user_id, deleted_at
		FROM trash_entries
		WHERE parent_folder_id IS NOT NULL
		UNION
		SELECT current.parent_folder_id, ancestor.user_id, ancestor.deleted_at
		FROM trash_ancestors ancestor
		JOIN folders current ON current.id = ancestor.folder_id AND current.user_id = ancestor.user_id
		WHERE current.parent_folder_id IS NOT NULL
	), containers AS (
		SELECT folder.id, folder.parent_folder_id, max(ancestor.deleted_at) AS deleted_at
		FROM trash_ancestors ancestor
		JOIN folders folder ON folder.id = ancestor.folder_id AND folder.user_id = ancestor.user_id
		WHERE folder.deleted_at IS NULL
		GROUP BY folder.id, folder.parent_folder_id
	), entries AS (
		SELECT 0 AS entry_type_order, trash.entry_type, trash.id, trash.parent_folder_id,
			folder.encrypted_name, folder.encrypted_metadata, NULL::bytea AS encrypted_file_key,
			trash.deleted_at, false AS is_container
		FROM trash_entries trash
		JOIN folders folder ON folder.id = trash.id AND folder.user_id = trash.user_id
		WHERE trash.entry_type = 'folder'
			AND (($3::uuid IS NULL AND trash.parent_folder_id IS NULL) OR trash.parent_folder_id = $3::uuid)

		UNION ALL

		SELECT 1, trash.entry_type, trash.id, trash.parent_folder_id, NULL::bytea,
			file.encrypted_metadata, file.encrypted_file_key, trash.deleted_at, false
		FROM trash_entries trash
		JOIN files file ON file.id = trash.id AND file.user_id = trash.user_id
		WHERE trash.entry_type = 'file'
			AND (($3::uuid IS NULL AND trash.parent_folder_id IS NULL) OR trash.parent_folder_id = $3::uuid)

		UNION ALL

		SELECT 0, 'folder', container.id, container.parent_folder_id, folder.encrypted_name,
			folder.encrypted_metadata, NULL::bytea, container.deleted_at, true
		FROM containers container
		JOIN folders folder ON folder.id = container.id AND folder.user_id = $1
		WHERE ($3::uuid IS NULL AND container.parent_folder_id IS NULL) OR container.parent_folder_id = $3::uuid
	)
	SELECT entry_type, id::text, parent_folder_id::text, encrypted_name, encrypted_metadata,
		encrypted_file_key, deleted_at, is_container
	FROM entries
	WHERE ($3::uuid IS NULL OR EXISTS (
		SELECT 1 FROM trash_entries parent
		WHERE parent.entry_type = 'folder' AND parent.id = $3::uuid
	) OR EXISTS (SELECT 1 FROM containers WHERE id = $3::uuid))
	AND ($4::timestamptz IS NULL
		OR deleted_at < $4
		OR (deleted_at = $4 AND entry_type_order > $5)
		OR (deleted_at = $4 AND entry_type_order = $5 AND id > $6::uuid))
	ORDER BY deleted_at DESC, entry_type_order ASC, id ASC
	LIMIT $7`, userID, cutoff, parentID, cursorTime, cursorTypeOrder, cursorID, limit)
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
			&entry.EncryptedFileKey,
			&entry.DeletedAt,
			&entry.IsContainer,
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

func (r *Repository) LockTrashFiles(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs []string) (int, error) {
	rows, err := db.Query(ctx, `SELECT id FROM files
	WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at > $3 AND purged_at IS NULL AND purge_requested_at IS NULL
	ORDER BY id
	FOR UPDATE`, userID, fileIDs, cutoff)
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

// LockTrashAncestors returns deleted ancestors needed to make a restored entry reachable.
func (r *Repository) LockTrashAncestors(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs, folderIDs []string) ([]string, bool, error) {
	rows, err := db.Query(ctx, `WITH RECURSIVE ancestors(id) AS (
		SELECT id FROM (
			SELECT folder_id AS id FROM files WHERE user_id = $1 AND id = ANY($2::uuid[]) AND folder_id IS NOT NULL
			UNION
			SELECT parent_folder_id AS id FROM folders WHERE user_id = $1 AND id = ANY($3::uuid[]) AND parent_folder_id IS NOT NULL
		) seeds
		UNION
		SELECT current.parent_folder_id
		FROM ancestors ancestor
		JOIN folders current ON current.id = ancestor.id AND current.user_id = $1
		WHERE current.parent_folder_id IS NOT NULL
	)
	SELECT folder.id::text, folder.deleted_at, folder.purged_at, folder.purge_requested_at
	FROM folders folder
	JOIN ancestors ON ancestors.id = folder.id
	WHERE folder.user_id = $1
	ORDER BY folder.id
	FOR UPDATE OF folder`, userID, fileIDs, folderIDs)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	deletedAncestors := make([]string, 0)
	blocked := false
	for rows.Next() {
		var id string
		var deletedAt, purgedAt, purgeRequestedAt *time.Time
		if err := rows.Scan(&id, &deletedAt, &purgedAt, &purgeRequestedAt); err != nil {
			return nil, false, err
		}
		if purgedAt != nil || purgeRequestedAt != nil {
			blocked = true
			continue
		}
		if deletedAt == nil {
			continue
		}
		if !deletedAt.After(cutoff) {
			blocked = true
			continue
		}
		deletedAncestors = append(deletedAncestors, id)
	}
	return deletedAncestors, blocked, rows.Err()
}

func (r *Repository) RestoreTrashAncestors(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) (int64, error) {
	if len(folderIDs) == 0 {
		return 0, nil
	}
	tag, err := db.Exec(ctx, `UPDATE folders SET deleted_at = NULL, updated_at = now()
	WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NOT NULL AND purged_at IS NULL AND purge_requested_at IS NULL`, userID, folderIDs)
	return tag.RowsAffected(), err
}

func (r *Repository) LockActiveTrashContainers(ctx context.Context, db database.PgExecutor, userID string, folderIDs []string) (int, error) {
	rows, err := db.Query(ctx, `SELECT id FROM folders
	WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL
		AND purged_at IS NULL AND purge_requested_at IS NULL
	ORDER BY id
	FOR UPDATE`, userID, folderIDs)
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

func (r *Repository) RestoreRemainingTrashContents(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, folderIDs []string) (int64, int64, error) {
	// Active ancestor folders stay navigable in Trash after restoring a deep child; restore their remaining deleted descendants together.
	var files, folders int64
	err := db.QueryRow(ctx, `WITH RECURSIVE tree(id) AS (
		SELECT id FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL
			AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	), restored_files AS (
		UPDATE files file
		SET deleted_at = NULL, updated_at = now()
		WHERE file.user_id = $1 AND file.folder_id IN (SELECT id FROM tree)
			AND file.deleted_at > $3 AND file.purged_at IS NULL AND file.purge_requested_at IS NULL
		RETURNING file.id
	), restored_folders AS (
		UPDATE folders folder
		SET deleted_at = NULL, updated_at = now()
		WHERE folder.user_id = $1 AND folder.id IN (SELECT id FROM tree)
			AND folder.id <> ALL($2::uuid[]) AND folder.deleted_at > $3
			AND folder.purged_at IS NULL AND folder.purge_requested_at IS NULL
		RETURNING folder.id
	)
	SELECT (SELECT count(*) FROM restored_files), (SELECT count(*) FROM restored_folders)`,
		userID, folderIDs, cutoff).Scan(&files, &folders)
	if err != nil {
		return 0, 0, err
	}
	return files, folders, nil
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
	var files, folders int64
	err := db.QueryRow(ctx, `WITH RECURSIVE tree(id, deleted_at) AS (
		SELECT id, deleted_at FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at > $3 AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id, child.deleted_at
		FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id AND child.deleted_at = parent.deleted_at
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	), restored_files AS (
		UPDATE files file
		SET deleted_at = NULL, updated_at = now()
		WHERE file.user_id = $1
			AND file.purged_at IS NULL
			AND file.purge_requested_at IS NULL
			AND file.deleted_at > $3
			AND EXISTS (
				SELECT 1 FROM tree
				WHERE tree.id = file.folder_id AND tree.deleted_at = file.deleted_at
			)
		RETURNING file.id
	), restored_folders AS (
		UPDATE folders folder
		SET deleted_at = NULL, updated_at = now()
		WHERE folder.user_id = $1 AND folder.id IN (SELECT id FROM tree)
		RETURNING folder.id
	)
	SELECT (SELECT count(*) FROM restored_files), (SELECT count(*) FROM restored_folders)`,
		userID, folderIDs, cutoff).Scan(&files, &folders)
	if err != nil {
		return 0, 0, err
	}
	return files, folders, nil
}

func (r *Repository) RestoreFiles(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs []string) (int64, error) {
	tag, err := db.Exec(ctx, `UPDATE files file
	SET deleted_at = NULL, updated_at = now()
	WHERE file.user_id = $1
		AND file.id = ANY($2::uuid[])
		AND file.deleted_at IS NOT NULL
		AND file.deleted_at > $3
		AND file.purged_at IS NULL
		AND file.purge_requested_at IS NULL`, userID, fileIDs, cutoff)
	return tag.RowsAffected(), err
}

func (r *Repository) QueuePermanentDelete(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, fileIDs, folderIDs []string) (int64, int64, error) {
	var files, folders int64
	err := db.QueryRow(ctx, `WITH queued_files AS (
		UPDATE files file
		SET purge_requested_at = now(), updated_at = now()
		WHERE file.user_id = $1 AND file.id = ANY($2::uuid[])
			AND file.deleted_at > $3 AND file.purge_requested_at IS NULL AND file.purged_at IS NULL
		RETURNING file.id
	), queued_folders AS (
		UPDATE folders folder
		SET purge_requested_at = now(), updated_at = now()
		WHERE folder.user_id = $1 AND folder.id = ANY($4::uuid[])
			AND folder.deleted_at > $3 AND folder.purge_requested_at IS NULL AND folder.purged_at IS NULL
		RETURNING folder.id
	)
	SELECT (SELECT count(*) FROM queued_files), (SELECT count(*) FROM queued_folders)`,
		userID, fileIDs, cutoff, folderIDs).Scan(&files, &folders)
	return files, folders, err
}

func (r *Repository) QueuePermanentDeleteRemainingTrashContents(ctx context.Context, db database.PgExecutor, userID string, cutoff time.Time, folderIDs []string) (int64, int64, error) {
	// Bulk deletion scopes to trashed descendants while leaving restored container folders intact.
	var files, folders int64
	err := db.QueryRow(ctx, `WITH RECURSIVE tree(id) AS (
		SELECT id FROM folders
		WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL
			AND purged_at IS NULL AND purge_requested_at IS NULL
		UNION
		SELECT child.id FROM folders child
		JOIN tree parent ON child.parent_folder_id = parent.id
		WHERE child.user_id = $1 AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
	), queued_files AS (
		UPDATE files file
		SET purge_requested_at = now(), updated_at = now()
		WHERE file.user_id = $1 AND file.folder_id IN (SELECT id FROM tree)
			AND file.deleted_at > $3 AND file.purged_at IS NULL AND file.purge_requested_at IS NULL
		RETURNING file.id
	), queued_folders AS (
		UPDATE folders folder
		SET purge_requested_at = now(), updated_at = now()
		WHERE folder.user_id = $1 AND folder.id IN (SELECT id FROM tree)
			AND folder.deleted_at > $3 AND folder.purged_at IS NULL AND folder.purge_requested_at IS NULL
		RETURNING folder.id
	)
	SELECT (SELECT count(*) FROM queued_files), (SELECT count(*) FROM queued_folders)`,
		userID, folderIDs, cutoff).Scan(&files, &folders)
	return files, folders, err
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
