package models

import "time"

type User struct {
	ID                          string
	Name                        string
	Username                    string
	Email                       string
	OpaqueRegistrationRecord    []byte
	VaultSalt                   []byte
	EncryptedMasterKey          []byte
	EncryptedMasterKeyRecovery  []byte
	RecoveryProofPublicKey      []byte
	UsedBytes                   int64
	ReservedBytes               int64
	LastLoginAt                 *time.Time
	RecoverySetupTokenHash      *[]byte
	RecoverySetupTokenExpiresAt *time.Time
	PasswordResetTokenHash      *string
	PasswordResetTokenExpiresAt *time.Time
	CreatedAt                   time.Time
}
