package files_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"arkive/core/models"
	filesvc "arkive/core/services/files"
)

func TestNormalizeSearchTokensDeduplicatesAndCopiesHashes(t *testing.T) {
	const tokenHashBytes = 32
	hash := bytes.Repeat([]byte{7}, tokenHashBytes)
	tokens := []models.FileSearchToken{
		{TokenHash: hash, Field: "name", Weight: 4},
		{TokenHash: hash, Field: " name ", Weight: 9},
	}

	got, err := filesvc.NormalizeSearchTokens(tokens, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Weight != 4 {
		t.Fatalf("normalized duplicate tokens = %+v, want first token only", got)
	}
	hash[0] = 99
	if got[0].TokenHash[0] != 7 {
		t.Fatal("normalized token retained caller-owned hash bytes")
	}
}

func TestNormalizeSearchTokensRejectsInvalidFieldsHashesWeightsAndCount(t *testing.T) {
	const tokenHashBytes = 32
	validHash := bytes.Repeat([]byte{3}, tokenHashBytes)
	tests := []struct {
		name   string
		tokens []models.FileSearchToken
		limit  int
	}{
		{name: "unknown field", tokens: []models.FileSearchToken{{TokenHash: validHash, Field: "plaintext", Weight: 1}}},
		{name: "short hash", tokens: []models.FileSearchToken{{TokenHash: validHash[:31], Field: "name", Weight: 1}}},
		{name: "low weight", tokens: []models.FileSearchToken{{TokenHash: validHash, Field: "name", Weight: 0}}},
		{name: "high weight", tokens: []models.FileSearchToken{{TokenHash: validHash, Field: "name", Weight: 101}}},
		{name: "too many tokens", tokens: []models.FileSearchToken{{TokenHash: validHash, Field: "name", Weight: 1}, {TokenHash: bytes.Repeat([]byte{4}, tokenHashBytes), Field: "name", Weight: 1}}, limit: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := filesvc.NormalizeSearchTokens(test.tokens, test.limit); err != filesvc.ErrInvalidInput {
				t.Fatalf("NormalizeSearchTokens error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestDecodeSearchTokenStringAcceptsHexAndRawURLBase64(t *testing.T) {
	const tokenHashBytes = 32
	hash := bytes.Repeat([]byte{5}, tokenHashBytes)
	for name, encoded := range map[string]string{
		"hex":            hex.EncodeToString(hash),
		"raw URL base64": base64.RawURLEncoding.EncodeToString(hash),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := filesvc.DecodeSearchTokenString(encoded)
			if err != nil || !bytes.Equal(got, hash) {
				t.Fatalf("decoded hash = %x, error %v", got, err)
			}
		})
	}
	for _, invalid := range []string{"", "not-a-hash", "00"} {
		if _, err := filesvc.DecodeSearchTokenString(invalid); err != filesvc.ErrInvalidInput {
			t.Errorf("DecodeSearchTokenString(%q) error = %v, want ErrInvalidInput", invalid, err)
		}
	}
}
