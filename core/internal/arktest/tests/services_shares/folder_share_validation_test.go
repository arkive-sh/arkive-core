package shares_test

import (
	. "arkive/core/services/shares"
	"encoding/base64"
	"strings"
	"testing"
)

func TestCreateFolderShareRejectsInvalidPermissionsAndEnvelopes(t *testing.T) {
	validInput := func() CreateInput {
		return CreateInput{
			FolderID:                "11111111-1111-4111-8111-111111111111",
			OwnerUserID:             "22222222-2222-4222-8222-222222222222",
			Token:                   strings.Repeat("a", TokenMinLength),
			AllowPreview:            true,
			EncryptedShareKey:       base64.StdEncoding.EncodeToString([]byte("share-key")),
			EncryptedFolderName:     base64.StdEncoding.EncodeToString([]byte("folder-name")),
			EncryptedFolderMetadata: base64.StdEncoding.EncodeToString([]byte("folder-metadata")),
		}
	}
	tests := []struct {
		name string
		edit func(*CreateInput)
		want string
	}{
		{
			name: "folder envelope required",
			edit: func(input *CreateInput) { input.EncryptedFolderName = "" },
			want: "encryptedFolder",
		},
		{
			name: "folder envelope must be base64",
			edit: func(input *CreateInput) { input.EncryptedFolderName = "not-base64" },
			want: "encryptedFolderName",
		},
		{
			name: "folder shares cannot burn after read",
			edit: func(input *CreateInput) { input.BurnAfterRead = true },
			want: "burnAfterRead",
		},
		{
			name: "at least one permission required",
			edit: func(input *CreateInput) { input.AllowPreview = false },
			want: "permissions",
		},
		{
			name: "encrypted share key must be base64",
			edit: func(input *CreateInput) { input.EncryptedShareKey = "not-base64" },
			want: "encryptedShareKey",
		},
	}
	service := &Service{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validInput()
			test.edit(&input)
			_, validationErrors, err := service.CreateShare(t.Context(), input)
			if err != nil {
				t.Fatalf("CreateShare error = %v, want validation error", err)
			}
			if validationErrors[test.want] == "" {
				t.Fatalf("validation errors %v do not contain %q", validationErrors, test.want)
			}
		})
	}
}
