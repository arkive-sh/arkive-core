ALTER TABLE upload_sessions
  ADD COLUMN upload_part_size BIGINT NOT NULL DEFAULT 0;
