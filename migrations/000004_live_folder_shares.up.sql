ALTER TABLE share_links
  DROP CONSTRAINT share_links_status_chk,
  ADD CONSTRAINT share_links_status_chk
    CHECK (status IN ('active', 'expired', 'revoked', 'burned', 'disabled', 'provisioning'));

ALTER TABLE share_items
  ALTER COLUMN file_id DROP NOT NULL,
  ADD COLUMN folder_id UUID REFERENCES folders(id) ON DELETE CASCADE,
  DROP CONSTRAINT share_items_item_type_chk,
  ADD CONSTRAINT share_items_item_type_chk
    CHECK (
      (item_type = 'file' AND file_id IS NOT NULL AND folder_id IS NULL)
      OR (item_type = 'folder' AND file_id IS NULL AND folder_id IS NOT NULL)
    );

CREATE INDEX share_items_folder_id_idx ON share_items (folder_id);

CREATE TABLE share_folder_entries (
  share_link_id UUID NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
  file_id UUID REFERENCES files(id) ON DELETE CASCADE,
  folder_id UUID REFERENCES folders(id) ON DELETE CASCADE,
  encrypted_file_key_for_share BYTEA,
  encrypted_name BYTEA,
  encrypted_metadata BYTEA,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT share_folder_entries_target_chk CHECK (
    (file_id IS NOT NULL AND folder_id IS NULL AND encrypted_file_key_for_share IS NOT NULL
      AND encrypted_name IS NULL AND encrypted_metadata IS NULL)
    OR (file_id IS NULL AND folder_id IS NOT NULL AND encrypted_file_key_for_share IS NULL
      AND encrypted_name IS NOT NULL AND encrypted_metadata IS NOT NULL)
  )
);

CREATE UNIQUE INDEX share_folder_entries_file_idx
  ON share_folder_entries (share_link_id, file_id) WHERE file_id IS NOT NULL;
CREATE UNIQUE INDEX share_folder_entries_folder_idx
  ON share_folder_entries (share_link_id, folder_id) WHERE folder_id IS NOT NULL;
CREATE INDEX share_folder_entries_folder_lookup_idx
  ON share_folder_entries (folder_id, share_link_id) WHERE folder_id IS NOT NULL;
CREATE INDEX share_folder_entries_file_lookup_idx
  ON share_folder_entries (file_id, share_link_id) WHERE file_id IS NOT NULL;
