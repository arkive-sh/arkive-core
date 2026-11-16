-- Includes former migrations 000001-000004; continue future versions at 000005
-- because existing development databases may already record versions 1-4.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  username CITEXT NOT NULL,
  email CITEXT NOT NULL,
  opaque_registration_record BYTEA NOT NULL,
  vault_salt BYTEA,
  encrypted_master_key BYTEA,
  used_bytes BIGINT NOT NULL DEFAULT 0,
  reserved_bytes BIGINT NOT NULL DEFAULT 0,
  last_login_at TIMESTAMPTZ,
  recovery_setup_token_hash BYTEA UNIQUE CHECK (
    recovery_setup_token_hash IS NULL OR octet_length(recovery_setup_token_hash) = 32
  ),
  recovery_setup_token_expires_at TIMESTAMPTZ,
  encrypted_master_key_recovery BYTEA,
  recovery_proof_public_key BYTEA,
  password_reset_token_hash TEXT UNIQUE,
  password_reset_token_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_username_key UNIQUE (username),
  CONSTRAINT users_email_key UNIQUE (email),
  CONSTRAINT users_username_format_chk
    CHECK (
      char_length(username) BETWEEN 3 AND 20
      AND username::text !~ '[^A-Za-z0-9]'
    ),
  CONSTRAINT users_name_min_length_chk
    CHECK (char_length(btrim(name)) >= 3),
  CONSTRAINT users_bytes_nonnegative_chk
    CHECK (used_bytes >= 0 AND reserved_bytes >= 0)
);

CREATE FUNCTION prevent_username_change() RETURNS trigger AS $$
BEGIN
  IF NEW.username IS DISTINCT FROM OLD.username THEN
    RAISE EXCEPTION 'username cannot be changed';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER users_username_immutable
  BEFORE UPDATE OF username ON users
  FOR EACH ROW EXECUTE FUNCTION prevent_username_change();


CREATE TABLE sessions (
  -- The cookie is a bearer secret; only its SHA-256 digest belongs in the database.
  token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE opaque_login_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  -- Application code stores this OPAQUE state encrypted and authenticated.
  server_state BYTEA NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX opaque_login_attempts_expires_at_idx ON opaque_login_attempts (expires_at);

CREATE TABLE instance_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE folders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  vault_id UUID NOT NULL,
  parent_folder_id UUID REFERENCES folders(id) ON DELETE SET NULL,
  encrypted_name BYTEA NOT NULL,
  encrypted_metadata BYTEA,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX idx_folders_user_parent_created
  ON folders (user_id, parent_folder_id, created_at DESC)
  WHERE deleted_at IS NULL;

CREATE TABLE files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  folder_id UUID REFERENCES folders(id) ON DELETE SET NULL,
  encrypted_metadata BYTEA NOT NULL,
  encrypted_file_key BYTEA NOT NULL,
  encrypted_manifest BYTEA NOT NULL,
  encryption_version SMALLINT NOT NULL DEFAULT 1,
  chunk_size BIGINT NOT NULL,
  chunk_count INTEGER NOT NULL,
  actual_encrypted_size BIGINT NOT NULL DEFAULT 0,
  encrypted_hash BYTEA,
  upload_status TEXT NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMPTZ,
  thumbnail_size_bytes BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT files_upload_status_chk
    CHECK (upload_status IN ('pending', 'uploading', 'complete', 'failed', 'aborted')),
  CONSTRAINT files_thumbnail_bytes_nonnegative_chk
    CHECK (thumbnail_size_bytes >= 0)
);

CREATE INDEX files_user_id_idx ON files (user_id);
CREATE INDEX files_expires_at_idx
  ON files (expires_at)
  WHERE expires_at IS NOT NULL;
CREATE INDEX idx_files_user_folder_complete_created
  ON files (user_id, folder_id, created_at DESC)
  WHERE upload_status = 'complete'
    AND expires_at IS NULL;

CREATE TABLE file_search_tokens (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  vault_id UUID NOT NULL,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  token_hash BYTEA NOT NULL,
  field TEXT NOT NULL,
  weight INT NOT NULL DEFAULT 1,
  PRIMARY KEY (user_id, vault_id, token_hash, file_id, field)
);

CREATE INDEX idx_file_search_tokens_lookup
  ON file_search_tokens (user_id, vault_id, token_hash, weight DESC);

CREATE TABLE folder_search_tokens (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  vault_id UUID NOT NULL,
  folder_id UUID NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  token_hash BYTEA NOT NULL,
  field TEXT NOT NULL,
  weight INT NOT NULL DEFAULT 1,
  PRIMARY KEY (user_id, vault_id, token_hash, folder_id, field)
);

CREATE INDEX idx_folder_search_tokens_lookup
  ON folder_search_tokens (user_id, vault_id, token_hash, weight DESC);

CREATE TABLE upload_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  provider_upload_id TEXT,
  encrypted_size BIGINT NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ NOT NULL,
  upload_part_count INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT upload_sessions_status_chk
    CHECK (status IN ('active', 'completed', 'aborted', 'expired', 'failed')),
  CONSTRAINT upload_sessions_encrypted_size_nonnegative_chk
    CHECK (encrypted_size >= 0)
);

CREATE INDEX upload_sessions_file_id_idx ON upload_sessions (file_id);
CREATE INDEX upload_sessions_expires_at_idx ON upload_sessions (expires_at);

CREATE TABLE upload_parts (
  upload_session_id UUID NOT NULL REFERENCES upload_sessions(id) ON DELETE CASCADE,
  part_number INTEGER NOT NULL,
  etag TEXT,
  encrypted_hash BYTEA NOT NULL,
  PRIMARY KEY (upload_session_id, part_number)
);

CREATE TABLE share_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active',
  encrypted_share_key BYTEA NOT NULL,
  password_hash TEXT,
  access_count INTEGER NOT NULL DEFAULT 0,
  max_access_count INTEGER,
  consumed_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  allow_preview BOOLEAN NOT NULL DEFAULT true,
  allow_download BOOLEAN NOT NULL DEFAULT true,
  burn_after_read BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT share_links_status_chk
    CHECK (status IN ('active', 'expired', 'revoked', 'burned', 'disabled'))
);

CREATE INDEX share_links_owner_user_id_idx ON share_links (owner_user_id);

CREATE TABLE share_items (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  share_link_id UUID NOT NULL REFERENCES share_links(id) ON DELETE CASCADE,
  item_type TEXT NOT NULL,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  display_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT share_items_item_type_chk
    CHECK (item_type = 'file')
);

CREATE INDEX share_items_share_link_id_idx ON share_items (share_link_id);
CREATE INDEX share_items_file_id_idx ON share_items (file_id);

CREATE TABLE share_snapshot_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  share_item_id UUID NOT NULL REFERENCES share_items(id) ON DELETE CASCADE,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  encrypted_relative_path BYTEA,
  encrypted_file_key_for_share BYTEA NOT NULL,
  display_order INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX share_snapshot_files_share_item_id_idx ON share_snapshot_files (share_item_id);
CREATE INDEX share_snapshot_files_file_id_idx ON share_snapshot_files (file_id);

ALTER TABLE files
ADD COLUMN IF NOT EXISTS deleted_at timestamptz NULL,
ADD COLUMN IF NOT EXISTS purged_at timestamptz NULL,
ADD COLUMN IF NOT EXISTS purge_requested_at timestamptz NULL,
ADD COLUMN IF NOT EXISTS tombstone_purged_at timestamptz NULL;

ALTER TABLE folders
ADD COLUMN IF NOT EXISTS purged_at timestamptz NULL,
ADD COLUMN IF NOT EXISTS purge_requested_at timestamptz NULL;

CREATE INDEX IF NOT EXISTS idx_files_deleted_cleanup
ON files(deleted_at)
WHERE deleted_at IS NOT NULL
  AND purged_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_files_tombstone_cleanup
ON files(deleted_at)
WHERE deleted_at IS NOT NULL
  AND purged_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_folders_deleted_cleanup
ON folders(deleted_at)
WHERE deleted_at IS NOT NULL
  AND purged_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_files_purge_requested
ON files(purge_requested_at)
WHERE purge_requested_at IS NOT NULL
  AND purged_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_folders_purge_requested
ON folders(purge_requested_at)
WHERE purge_requested_at IS NOT NULL
  AND purged_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_files_sync_listing
ON files(user_id, folder_id, updated_at DESC, id)
WHERE upload_status = 'complete';

CREATE INDEX IF NOT EXISTS idx_folders_sync_listing
ON folders(user_id, parent_folder_id, updated_at DESC, id);

ALTER TABLE upload_sessions
  ADD COLUMN upload_part_size BIGINT NOT NULL DEFAULT 0;

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
