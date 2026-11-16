package opaque_test

import (
	"arkive/core/internal/arktest"
	. "arkive/core/opaque"
	"errors"
	"strings"
	"testing"
)

func TestFFIErrorCodesAndRustOwnedOutput(t *testing.T) {
	arktest.InstallOpaqueLibrary(t)
	setupBuffer, err := GenerateSetup()
	if err != nil {
		t.Fatal(err)
	}
	defer setupBuffer.Close()
	setup := string(setupBuffer.Bytes())
	if err := ValidateSetup(setup); err != nil {
		t.Fatalf("generated setup did not validate: %v", err)
	}

	for input, want := range map[string]ErrorCode{
		"%%%":                     ErrorInvalidMessage,
		"\x00":                    ErrorInvalidMessage,
		strings.Repeat("x", 1025): ErrorInvalidInput,
	} {
		err := ValidateSetup(input)
		var ffiErr *Error
		if !errors.As(err, &ffiErr) || ffiErr.Code != want {
			t.Fatalf("ValidateSetup error = %v, want code %s", err, want)
		}
	}

	request := []byte{1, 2, 3}
	_, err = RegistrationResponse(setup, "test-user", request)
	var ffiErr *Error
	if !errors.As(err, &ffiErr) || ffiErr.Code != ErrorInvalidMessage {
		t.Fatalf("registration request error = %v, want code %s", err, ErrorInvalidMessage)
	}
	for _, value := range request {
		if value != 0 {
			t.Fatal("registration request was not zeroized by Rust")
		}
	}
	_, err = StartLogin(setup, "test-user", nil, []byte{1, 2, 3})
	if !errors.As(err, &ffiErr) || ffiErr.Code != ErrorInvalidMessage {
		t.Fatalf("login request error = %v, want code %s", err, ErrorInvalidMessage)
	}

	_, err = FinishRegistration(make([]byte, 3073))
	if !errors.As(err, &ffiErr) || ffiErr.Code != ErrorInvalidInput {
		t.Fatalf("oversized registration upload error = %v, want code %s", err, ErrorInvalidInput)
	}
}
