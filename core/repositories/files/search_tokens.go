package filerepo

import (
	"context"

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

func (r *Repository) SearchCompletedForTokens(ctx context.Context, db database.PgExecutor, userID, vaultID string, tokenHashes [][]byte, minimumMatches, limit int) ([]models.File, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(ctx, `SELECT
		f.id,
		f.user_id,
		f.folder_id,
		f.encrypted_metadata,
		f.encrypted_file_key,
		SUM(fst.weight) AS score
	FROM
		file_search_tokens fst
	INNER JOIN files f ON f.id = fst.file_id
	WHERE
		fst.user_id = $1
		AND fst.vault_id = $2
		AND fst.token_hash = ANY($3::bytea[])
		AND f.upload_status = 'complete'
		AND f.expires_at IS NULL
		AND f.deleted_at IS NULL
	GROUP BY
		f.id
	HAVING
		COUNT(DISTINCT fst.token_hash) >= $4
	ORDER BY
		COUNT(DISTINCT fst.token_hash) DESC,
		score DESC,
		f.updated_at DESC,
		f.id
	LIMIT $5`, userID, vaultID, tokenHashes, minimumMatches, limit)
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
