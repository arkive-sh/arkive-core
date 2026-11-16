package arktest

import (
	"context"

	"arkive/core/database"
	"arkive/core/models"
	authrepo "arkive/core/repositories/auth"
)

func CreateVerifiedUser(ctx context.Context, db database.PgExecutor, name, username, email string, registrationRecord, vaultSalt, encryptedMasterKey []byte) (models.User, error) {
	return authrepo.New().CreateVerifiedUser(ctx, db, name, username, email, registrationRecord, vaultSalt, encryptedMasterKey)
}
