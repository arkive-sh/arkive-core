package settings_test

import (
	. "arkive/core/services/settings"
	"testing"

	"arkive/core/models"
	"arkive/pkg/validation"
)

func validStorageInput(storageGB string) StorageInput {
	return StorageInput{
		StorageGB:         storageGB,
		S3AccessKeyID:     "access-key",
		S3SecretAccessKey: "secret-key",
		S3Bucket:          "arkive-test",
		S3Endpoint:        "https://storage.example.test",
	}
}

func TestBuildStorageSettingsRequiresS3CompatibleConfiguration(t *testing.T) {
	settings, validationErrors := BuildStorageSettings(validStorageInput("2"))
	ValidateStorageSettings(settings, validationErrors)
	if validationErrors.HasAny() {
		t.Fatalf("valid S3 settings failed validation: %v", validationErrors)
	}
	if settings.Provider != "s3" {
		t.Fatalf("provider = %q, want s3", settings.Provider)
	}
	if settings.MaxStorageBytes != 2*1024*1024*1024 {
		t.Fatalf("max storage = %d, want 2 GiB", settings.MaxStorageBytes)
	}
	if settings.S3Region != "auto" {
		t.Fatalf("default region = %q, want auto", settings.S3Region)
	}
}

func TestBuildStorageSettingsTreatsZeroAsUnlimited(t *testing.T) {
	settings, validationErrors := BuildStorageSettings(validStorageInput("0"))
	ValidateStorageSettings(settings, validationErrors)
	if validationErrors.HasAny() {
		t.Fatalf("zero storage limit failed validation: %v", validationErrors)
	}
	if settings.MaxStorageBytes != 0 {
		t.Fatalf("max storage = %d, want 0 for unlimited", settings.MaxStorageBytes)
	}
}

func TestBuildStorageSettingsRejectsOverflowingStorageLimit(t *testing.T) {
	_, validationErrors := BuildStorageSettings(StorageInput{StorageGB: "9223372036854775807"})
	if validationErrors["storage_gb"] == "" {
		t.Fatal("overflowing storage limit passed validation")
	}
}

func TestValidateStorageSettingsRejectsLegacyLocalProvider(t *testing.T) {
	validationErrors := validation.New()
	ValidateStorageSettings(models.StorageSettings{Provider: "local"}, validationErrors)
	if validationErrors["storage_provider"] == "" {
		t.Fatal("local storage provider passed validation")
	}
}

func TestValidateStorageSettingsRequiresAllS3Fields(t *testing.T) {
	settings, validationErrors := BuildStorageSettings(StorageInput{})
	ValidateStorageSettings(settings, validationErrors)
	for _, field := range []string{"s3_access_key_id", "s3_secret_access_key", "s3_bucket", "s3_endpoint"} {
		if validationErrors[field] == "" {
			t.Errorf("missing required field %q was not reported", field)
		}
	}
}
