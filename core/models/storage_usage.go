package models

type StorageUsage struct {
	UsedBytes       int64 `json:"usedBytes"`
	ReservedBytes   int64 `json:"reservedBytes"`
	MaxStorageBytes int64 `json:"maxStorageBytes"`
}
