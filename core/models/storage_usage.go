package models

type StorageUsage struct {
	UsedBytes           int64 `json:"usedBytes"`
	ReservedBytes       int64 `json:"reservedBytes"`
	CurrentFileBytes    int64 `json:"currentFileBytes"`
	VersionHistoryBytes int64 `json:"versionHistoryBytes"`
	TrashBytes          int64 `json:"trashBytes"`
	MaxStorageBytes     int64 `json:"maxStorageBytes"`
}
