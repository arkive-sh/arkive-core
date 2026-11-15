package filerepo

import (
	"context"
	"fmt"
	"time"

	"arkive/core/database"
	"arkive/core/models"
)

func (r *Repository) InsertSearchTokensForFile(ctx context.Context, db database.PgExecutor, userID, vaultID, fileID string, tokens []models.FileSearchToken) error {
	if len(tokens) == 0 {
		return nil
	}
	query := `INSERT INTO file_search_tokens
		(user_id, vault_id, file_id, token_hash, field, weight)
	VALUES
		($1, $2, $3, $4, $5, $6)
	ON CONFLICT DO NOTHING`
	for _, token := range tokens {
		if _, err := db.Exec(ctx, query, userID, vaultID, fileID, token.TokenHash, token.Field, token.Weight); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ReplaceSearchTokensForFile(ctx context.Context, db database.PgExecutor, userID, vaultID, fileID string, tokens []models.FileSearchToken) error {
	if _, err := db.Exec(ctx, `DELETE FROM file_search_tokens WHERE user_id = $1 AND vault_id = $2 AND file_id = $3`, userID, vaultID, fileID); err != nil {
		return err
	}
	return r.InsertSearchTokensForFile(ctx, db, userID, vaultID, fileID, tokens)
}

func (r *Repository) SearchForTokens(ctx context.Context, db database.PgExecutor, userID, vaultID string, tokenHashes [][]byte, minimumMatches, limit int, folderID *string, includeTrash bool, trashCutoff time.Time) ([]models.File, error) {
	if limit <= 0 {
		limit = 20
	}
	queryPrefix := ""
	folderFilter := ""
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
		folderFilter = `AND f.folder_id IN (SELECT id FROM search_scope)`
		args = append(args, *folderID)
	}
	trashFilter := "AND f.deleted_at IS NULL AND f.expires_at IS NULL"
	if includeTrash {
		cutoffParam := len(args) + 1
		trashFilter = fmt.Sprintf(`AND f.deleted_at > $%d
			AND f.purged_at IS NULL AND f.purge_requested_at IS NULL`, cutoffParam)
		args = append(args, trashCutoff)
	}
	rows, err := db.Query(ctx, queryPrefix+`SELECT
		f.id,
		f.user_id,
		f.folder_id,
		f.encrypted_metadata,
		f.encrypted_file_key,
		SUM(fst.weight) AS score,
		f.deleted_at
	FROM
		file_search_tokens fst
	INNER JOIN files f ON f.id = fst.file_id AND f.user_id = fst.user_id
	WHERE
		fst.user_id = $1
		AND fst.vault_id = $2
		AND fst.token_hash = ANY($3::bytea[])
		AND f.upload_status = 'complete'
		`+trashFilter+`
		`+folderFilter+`
	GROUP BY
		f.id
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

	files := make([]models.File, 0, limit)
	for rows.Next() {
		var file models.File
		var folderID *string
		if err := rows.Scan(
			&file.ID,
			&file.UserID,
			&folderID,
			&file.EncryptedMetadata,
			&file.EncryptedFileKey,
			&file.SearchScore,
			&file.DeletedAt,
		); err != nil {
			return nil, err
		}
		file.FolderID = folderID
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}
