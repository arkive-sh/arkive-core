package folders

import "arkive/core/models"

type EntryType string

const (
	EntryTypeFolder EntryType = "folder"
	EntryTypeFile   EntryType = "file"
)

type ListEntriesPageResult struct {
	FolderID     *string
	Page         int
	PageSize     int
	TotalEntries int
	TotalPages   int
	Path         []models.Folder
	Folders      []models.Folder
	Files        []models.File
}

type ListEntriesCursorResult struct {
	FolderID   *string
	Path       []models.Folder
	Folders    []models.Folder
	Files      []models.File
	NextCursor *string
	HasMore    bool
}

type CreateFolderInput struct {
	ID                string
	UserID            string
	VaultID           string
	ParentFolderID    *string
	EncryptedName     []byte
	EncryptedMetadata []byte
	SearchTokens      []models.FileSearchToken
}

type ListEntriesPageInput struct {
	UserID   string
	FolderID *string
	Page     int
	PageSize int
}

type ListEntriesCursorInput struct {
	UserID      string
	FolderID    *string
	PageSize    int
	Cursor      string
	SeekEntryID string
}

type MoveEntriesInput struct {
	UserID         string
	TargetFolderID *string
	Files          []MoveFileInput
	Folders        []MoveFolderInput
}

type MoveFileInput struct {
	ID               string
	EncryptedFileKey []byte
}

type MoveFolderInput struct {
	ID                string
	EncryptedName     []byte
	EncryptedMetadata []byte
}

type RenameFolderInput struct {
	UserID            string
	FolderID          string
	EncryptedName     []byte
	EncryptedMetadata []byte
	SearchTokens      []models.FileSearchToken
}

type ResolveDeleteScopeInput struct {
	UserID    string
	FileIDs   []string
	FolderIDs []string
}

type DeleteScope struct {
	FolderIDs []string
	FileIDs   []string
}

type DeleteEntriesInput struct {
	UserID    string
	FileIDs   []string
	FolderIDs []string
}

type DeleteEntriesResult struct {
	DeletedFiles   int
	DeletedFolders int
}

type ListTrashInput struct {
	UserID         string
	ParentFolderID string
	Cursor         string
	Limit          int
}

type ListTrashResult struct {
	Entries    []models.TrashEntry
	NextCursor *string
	HasMore    bool
}

type RestoreTrashInput struct {
	UserID             string
	FileIDs            []string
	FolderIDs          []string
	ContainerFolderIDs []string
}

type RestoreTrashResult struct {
	RestoredFiles   int `json:"restoredFiles"`
	RestoredFolders int `json:"restoredFolders"`
}

type PermanentlyDeleteTrashInput struct {
	UserID             string
	FileIDs            []string
	FolderIDs          []string
	ContainerFolderIDs []string
}

type PermanentlyDeleteTrashResult struct {
	QueuedFiles   int `json:"queuedFiles"`
	QueuedFolders int `json:"queuedFolders"`
}
