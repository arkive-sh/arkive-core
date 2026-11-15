package foldersrepo

import (
	"context"
	"fmt"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

func (r *Repository) InsertSearchTokensForFolder(ctx context.Context, db database.PgExecutor, userID, vaultID, folderID string, tokens []models.FileSearchToken) error {
	if len(tokens) == 0 {
		return nil
	}
	query := `INSERT INTO folder_search_tokens
		(user_id, vault_id, folder_id, token_hash, field, weight)
	VALUES
		($1, $2, $3, $4, $5, $6)
	ON CONFLICT DO NOTHING`
	for _, token := range tokens {
		if _, err := db.Exec(ctx, query, userID, vaultID, folderID, token.TokenHash, token.Field, token.Weight); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ReplaceSearchTokensForFolder(ctx context.Context, db database.PgExecutor, userID, vaultID, folderID string, tokens []models.FileSearchToken) error {
	if _, err := db.Exec(ctx, `DELETE FROM folder_search_tokens WHERE user_id = $1 AND vault_id = $2 AND folder_id = $3`, userID, vaultID, folderID); err != nil {
		return err
	}
	return r.InsertSearchTokensForFolder(ctx, db, userID, vaultID, folderID, tokens)
}

func (r *Repository) SearchFoldersForTokens(ctx context.Context, db database.PgExecutor, userID, vaultID string, tokenHashes [][]byte, minimumMatches, limit int, folderID *string, includeTrash bool, trashCutoff time.Time) ([]models.Folder, error) {
	if limit <= 0 {
		limit = 20
	}
	queryPrefix := ""
	folderFilter := ""
	containerJoin := ""
	containerSelect := "false"
	groupBy := "f.id"
	args := []any{userID, vaultID, tokenHashes, minimumMatches, limit}
	if folderID != nil {
		queryPrefix = `WITH RECURSIVE search_scope(id) AS (
			SELECT id FROM folders
			WHERE id = $6::uuid AND user_id = $1 AND deleted_at IS NULL
				AND purged_at IS NULL AND purge_requested_at IS NULL
			UNION
			SELECT child.id FROM folders child
			JOIN search_scope parent ON child.parent_folder_id = parent.id
			WHERE child.user_id = $1 AND child.deleted_at IS NULL
				AND child.purged_at IS NULL AND child.purge_requested_at IS NULL
		)
		`
		folderFilter = `AND f.id IN (SELECT id FROM search_scope) AND f.id <> $6::uuid`
		args = append(args, *folderID)
	}
	trashFilter := "AND f.deleted_at IS NULL"
	if includeTrash {
		cutoffParam := len(args) + 1
		queryPrefix = fmt.Sprintf(`WITH RECURSIVE trash_ancestors(folder_id) AS (
			SELECT file.folder_id FROM files file
			WHERE file.user_id = $1 AND file.deleted_at > $%d
				AND file.purged_at IS NULL AND file.purge_requested_at IS NULL
				AND file.folder_id IS NOT NULL
			UNION
			SELECT folder.parent_folder_id FROM folders folder
			WHERE folder.user_id = $1 AND folder.deleted_at > $%d
				AND folder.purged_at IS NULL AND folder.purge_requested_at IS NULL
				AND folder.parent_folder_id IS NOT NULL
			UNION
			SELECT parent.parent_folder_id FROM folders parent
			JOIN trash_ancestors child ON parent.id = child.folder_id
			WHERE parent.user_id = $1 AND parent.parent_folder_id IS NOT NULL
		), containers AS (
			SELECT DISTINCT folder.id FROM folders folder
			JOIN trash_ancestors ancestor ON ancestor.folder_id = folder.id
			WHERE folder.user_id = $1 AND folder.deleted_at IS NULL
		)
		`, cutoffParam, cutoffParam)
		trashFilter = fmt.Sprintf(`AND ((f.deleted_at > $%d
			AND f.purged_at IS NULL AND f.purge_requested_at IS NULL)
			OR containers.id IS NOT NULL)`, cutoffParam)
		containerJoin = `LEFT JOIN containers ON containers.id = f.id`
		containerSelect = `containers.id IS NOT NULL`
		groupBy = `f.id, containers.id`
		args = append(args, trashCutoff)
	}
	rows, err := db.Query(ctx, queryPrefix+`SELECT
		f.id,
		f.vault_id,
		f.parent_folder_id,
		f.encrypted_name,
		f.encrypted_metadata,
		SUM(fst.weight) AS score,
		f.deleted_at,
		`+containerSelect+` AS is_trash_container
	FROM
		folder_search_tokens fst
	INNER JOIN folders f ON f.id = fst.folder_id AND f.user_id = fst.user_id
	`+containerJoin+`
	WHERE
		fst.user_id = $1
		AND fst.vault_id = $2
		AND fst.token_hash = ANY($3::bytea[])
		`+trashFilter+`
		`+folderFilter+`
	GROUP BY
		`+groupBy+`
	HAVING
		COUNT(DISTINCT fst.token_hash) >= $4
	ORDER BY
		COUNT(DISTINCT fst.token_hash) DESC,
		score DESC,
		f.updated_at DESC,
		f.id
		LIMIT $5`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	folders := make([]models.Folder, 0, limit)
	for rows.Next() {
		var folder models.Folder
		var parentFolderID *string
		if err := rows.Scan(
			&folder.ID,
			&folder.VaultID,
			&parentFolderID,
			&folder.EncryptedName,
			&folder.EncryptedMetadata,
			&folder.SearchScore,
			&folder.DeletedAt,
			&folder.IsTrashContainer,
		); err != nil {
			return nil, err
		}
		folder.ParentFolderID = parentFolderID
		folders = append(folders, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return folders, nil
}
