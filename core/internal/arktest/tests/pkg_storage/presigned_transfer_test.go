package s3client_test

import (
	. "arkive/pkg/storage/s3client"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestPresignedUploadDownloadAndPartURLsTargetStorageEndpoint(t *testing.T) {
	client, err := New(context.Background(), Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "arkive-test",
		Endpoint:        "https://objects.example.test",
		Region:          "auto",
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, makeURL := range map[string]func() (string, error){
		"upload": func() (string, error) {
			return client.PresignUpload(context.Background(), "user/file", "application/octet-stream", 123, time.Minute)
		},
		"download": func() (string, error) {
			return client.PresignDownload(context.Background(), "user/file", "file.bin", "attachment", time.Minute)
		},
		"multipart part": func() (string, error) {
			return client.PresignUploadPart(context.Background(), "user/file", "upload-id", 2, 123, time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rawURL, err := makeURL()
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Host != "objects.example.test" {
				t.Fatalf("presigned host = %q, want object storage endpoint", parsed.Host)
			}
			if parsed.Path != "/arkive-test/user/file" {
				t.Fatalf("presigned path = %q, want bucket and object key", parsed.Path)
			}
			query := parsed.Query()
			if query.Get("X-Amz-Signature") == "" || query.Get("X-Amz-Credential") == "" {
				t.Fatalf("URL is missing signature parameters: %q", parsed.RawQuery)
			}
			if name == "download" && query.Get("response-content-disposition") == "" {
				t.Fatal("download URL is missing its content disposition")
			}
			if name == "multipart part" && (query.Get("uploadId") != "upload-id" || query.Get("partNumber") != "2") {
				t.Fatalf("multipart URL has wrong upload parameters: %q", parsed.RawQuery)
			}
		})
	}
}

func TestPresignedUploadAndDownloadTransferDirectlyToStorage(t *testing.T) {
	var stored []byte
	var methods []string
	const endpoint = "https://objects.example.test"
	storageHTTP := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		if r.URL.Host != "objects.example.test" || r.URL.Path != "/arkive-test/user/file" {
			return nil, fmt.Errorf("request went to unexpected storage URL %q", r.URL)
		}
		if r.URL.Query().Get("X-Amz-Signature") == "" {
			return nil, fmt.Errorf("storage request is missing signature: %q", r.URL.RawQuery)
		}
		var body io.Reader = bytes.NewReader(nil)
		switch r.Method {
		case http.MethodPut:
			var err error
			stored, err = io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
		case http.MethodGet:
			body = bytes.NewReader(stored)
		default:
			return nil, fmt.Errorf("unexpected storage method %s", r.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(body),
			Request:    r,
		}, nil
	})}

	client, err := New(context.Background(), Config{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Bucket:          "arkive-test",
		Endpoint:        endpoint,
		Region:          "auto",
		UsePathStyle:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ciphertext := []byte("opaque encrypted bytes")
	uploadURL, err := client.PresignUpload(context.Background(), "user/file", "application/octet-stream", int64(len(ciphertext)), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	assertStorageURL(t, uploadURL, endpoint)
	uploadRequest, err := http.NewRequest(http.MethodPut, uploadURL, bytes.NewReader(ciphertext))
	if err != nil {
		t.Fatal(err)
	}
	uploadRequest.Header.Set("Content-Type", "application/octet-stream")
	uploadResponse, err := storageHTTP.Do(uploadRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusOK {
		t.Fatalf("direct storage PUT status = %d, want %d", uploadResponse.StatusCode, http.StatusOK)
	}

	downloadURL, err := client.PresignDownload(context.Background(), "user/file", "file.bin", "attachment", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	assertStorageURL(t, downloadURL, endpoint)
	downloadResponse, err := storageHTTP.Get(downloadURL)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, readErr := io.ReadAll(downloadResponse.Body)
	_ = downloadResponse.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if downloadResponse.StatusCode != http.StatusOK || !bytes.Equal(downloaded, ciphertext) {
		t.Fatalf("direct storage GET status/body = %d/%q, want %d/%q", downloadResponse.StatusCode, downloaded, http.StatusOK, ciphertext)
	}
	if len(methods) != 2 || methods[0] != http.MethodPut || methods[1] != http.MethodGet {
		t.Fatalf("storage endpoint methods = %v, want [PUT GET]", methods)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func assertStorageURL(t *testing.T, rawURL, endpoint string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	e, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != e.Host || u.Path != "/arkive-test/user/file" {
		t.Fatalf("presigned URL = %q, want direct storage endpoint %q and object path", rawURL, endpoint)
	}
	if u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("URL is missing signature parameters: %q", u.RawQuery)
	}
}
