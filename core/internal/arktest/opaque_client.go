package arktest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"arkive/core/opaque"
)

// Use the browser's OPAQUE client so integration tests exercise real protocol messages.
const opaqueClientScript = `
import * as opaque from '@serenity-kit/opaque';
import { readFileSync } from 'node:fs';

await opaque.ready;
const input = JSON.parse(readFileSync(0, 'utf8'));
let output;
switch (input.action) {
  case 'registration-start': {
    const result = opaque.client.startRegistration({ password: input.password });
    output = { state: result.clientRegistrationState, request: result.registrationRequest };
    break;
  }
  case 'registration-finish': {
    const result = opaque.client.finishRegistration({
      clientRegistrationState: input.state,
      registrationResponse: input.message,
      password: input.password,
    });
    output = { registrationRecord: result.registrationRecord };
    break;
  }
  case 'login-start': {
    const result = opaque.client.startLogin({ password: input.password });
    output = { state: result.clientLoginState, request: result.startLoginRequest };
    break;
  }
  case 'login-finish': {
    const result = opaque.client.finishLogin({
      clientLoginState: input.state,
      loginResponse: input.message,
      password: input.password,
    });
    output = result ? { request: result.finishLoginRequest, accepted: true } : { accepted: false };
    break;
  }
  default:
    throw new Error('unknown OPAQUE test action');
}
process.stdout.write(JSON.stringify(output));
`

type OpaqueClientResult struct {
	State              string `json:"state"`
	Request            string `json:"request"`
	RegistrationRecord string `json:"registrationRecord"`
	Accepted           bool   `json:"accepted"`
}

func InstallOpaqueLibrary(t testing.TB) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("find OPAQUE test library location")
	}
	repositoryRoot := filepath.Join(filepath.Dir(source), "../../..")
	libraryRoot, err := os.OpenRoot(filepath.Join(repositoryRoot, "rust/opaque-ffi/target/release"))
	if err != nil {
		t.Fatalf("open OPAQUE test library directory (run make build-opaque): %v", err)
	}
	t.Cleanup(func() {
		if err := libraryRoot.Close(); err != nil {
			t.Errorf("close OPAQUE test library directory: %v", err)
		}
	})
	name := "libarkive_opaque.so"
	switch runtime.GOOS {
	case "darwin":
		name = "libarkive_opaque.dylib"
	case "windows":
		name = "arkive_opaque.dll"
	}
	library, err := libraryRoot.ReadFile(name)
	if err != nil {
		t.Fatalf("read built OPAQUE test library (run make build-opaque): %v", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	targetRoot, err := os.OpenRoot(filepath.Dir(executable))
	if err != nil {
		t.Fatalf("open test binary directory: %v", err)
	}
	t.Cleanup(func() {
		if err := targetRoot.Close(); err != nil {
			t.Errorf("close test binary directory: %v", err)
		}
	})
	target, err := targetRoot.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return
	}
	if err != nil {
		t.Fatalf("create OPAQUE library beside test binary: %v", err)
	}
	if _, err := target.Write(library); err != nil {
		_ = target.Close()
		t.Fatalf("install OPAQUE library beside test binary: %v", err)
	}
	if err := target.Close(); err != nil {
		t.Fatalf("close OPAQUE library beside test binary: %v", err)
	}
}

func OpaqueClient(ctx context.Context, action, password, state, message string) (OpaqueClientResult, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return OpaqueClientResult{}, fmt.Errorf("find OPAQUE test client location")
	}
	webDir := filepath.Join(filepath.Dir(source), "../../../web")
	input, err := json.Marshal(map[string]string{
		"action": action, "password": password, "state": state, "message": message,
	})
	if err != nil {
		return OpaqueClientResult{}, err
	}
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", opaqueClientScript)
	cmd.Dir = webDir
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return OpaqueClientResult{}, fmt.Errorf("run OPAQUE test client: %w: %s", err, stderr.String())
	}
	var result OpaqueClientResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return OpaqueClientResult{}, fmt.Errorf("decode OPAQUE test client output: %w", err)
	}
	return result, nil
}

func OpaqueRegistrationUpload(ctx context.Context, serverSetup, username, password string) ([]byte, error) {
	start, err := OpaqueClient(ctx, "registration-start", password, "", "")
	if err != nil {
		return nil, err
	}
	request, err := base64.RawURLEncoding.DecodeString(start.Request)
	if err != nil {
		return nil, err
	}
	response, err := opaque.RegistrationResponse(serverSetup, username, request)
	if err != nil {
		return nil, err
	}
	responseMessage := base64.RawURLEncoding.EncodeToString(response.Bytes())
	response.Close()

	finish, err := OpaqueClient(ctx, "registration-finish", password, start.State, responseMessage)
	if err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(finish.RegistrationRecord)
}

func OpaqueLoginStart(ctx context.Context, password string) (string, []byte, error) {
	start, err := OpaqueClient(ctx, "login-start", password, "", "")
	if err != nil {
		return "", nil, err
	}
	request, err := base64.RawURLEncoding.DecodeString(start.Request)
	return start.State, request, err
}

func OpaqueLoginFinish(ctx context.Context, password, state string, response []byte) ([]byte, error) {
	finish, err := OpaqueClient(ctx, "login-finish", password, state, base64.RawURLEncoding.EncodeToString(response))
	if err != nil {
		return nil, err
	}
	if !finish.Accepted {
		return nil, fmt.Errorf("OPAQUE client rejected the server login response")
	}
	return base64.RawURLEncoding.DecodeString(finish.Request)
}
