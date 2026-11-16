package arktest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"arkive/pkg/storage"
)

var _ storage.Provider = (*MemoryStorage)(nil)

type MemoryStorage struct {
	mu         sync.RWMutex
	objects    map[string][]byte
	multipart  map[string]map[int32][]byte
	partSizes  map[int32]int64
	nextUpload uint64
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		objects:   make(map[string][]byte),
		multipart: make(map[string]map[int32][]byte),
		partSizes: make(map[int32]int64),
	}
}

func (m *MemoryStorage) PresignedPartSize(partNumber int32) (int64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	size, ok := m.partSizes[partNumber]
	return size, ok
}

func (m *MemoryStorage) PutObject(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), data...)
}

func (m *MemoryStorage) GetObject(key string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[key]
	return append([]byte(nil), data...), ok
}

func (m *MemoryStorage) PutPart(uploadID string, partNumber int32, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.multipart[uploadID] == nil {
		m.multipart[uploadID] = make(map[int32][]byte)
	}
	m.multipart[uploadID][partNumber] = append([]byte(nil), data...)
}

func (m *MemoryStorage) ActiveProvider(context.Context) (string, error) {
	return "s3", nil
}

func (m *MemoryStorage) PresignUpload(_ context.Context, key, _ string, _ int64, _ time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("key is required")
	}
	return "memory://upload/" + url.PathEscape(key), nil
}

func (m *MemoryStorage) PresignDownload(_ context.Context, key, _, _ string, _ time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("key is required")
	}
	return "memory://download/" + url.PathEscape(key), nil
}

func (m *MemoryStorage) ObjectSize(_ context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.objects[key]
	if !ok {
		return 0, os.ErrNotExist
	}
	return int64(len(data)), nil
}

func (m *MemoryStorage) CreateMultipartUpload(context.Context, string, string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextUpload++
	id := strconv.FormatUint(m.nextUpload, 10)
	m.multipart[id] = make(map[int32][]byte)
	return id, nil
}

func (m *MemoryStorage) PresignUploadPart(_ context.Context, _ string, uploadID string, partNumber int32, size int64, _ time.Duration) (string, error) {
	m.mu.Lock()
	m.partSizes[partNumber] = size
	m.mu.Unlock()
	return fmt.Sprintf("memory://multipart/%s/%d", url.PathEscape(uploadID), partNumber), nil
}

func (m *MemoryStorage) CompleteMultipartUpload(_ context.Context, key, uploadID string, parts []storage.CompletedPart) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	uploadParts, ok := m.multipart[uploadID]
	if !ok {
		return errors.New("multipart upload not found")
	}
	var object []byte
	for _, part := range parts {
		data, ok := uploadParts[part.PartNumber]
		if !ok {
			return fmt.Errorf("multipart part %d not found", part.PartNumber)
		}
		object = append(object, data...)
	}
	m.objects[key] = object
	delete(m.multipart, uploadID)
	return nil
}

func (m *MemoryStorage) AbortMultipartUpload(_ context.Context, _ string, uploadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.multipart, uploadID)
	return nil
}

func (m *MemoryStorage) DeleteObject(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *MemoryStorage) DeleteObjects(_ context.Context, keys []string) (map[string]error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.objects, key)
	}
	return map[string]error{}, nil
}
