DROP INDEX IF EXISTS idx_folders_user_parent_created;
CREATE INDEX idx_folders_user_parent_created
  ON folders (user_id, parent_folder_id, created_at DESC, id DESC)
  WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_files_user_folder_complete_created;
CREATE INDEX idx_files_user_folder_complete_created
  ON files (user_id, folder_id, created_at DESC, id DESC)
  WHERE upload_status = 'complete'
    AND expires_at IS NULL
    AND deleted_at IS NULL;
