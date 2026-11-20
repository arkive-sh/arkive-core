DROP TABLE IF EXISTS share_folder_entries;
DROP TABLE IF EXISTS share_snapshot_files;
DROP TABLE IF EXISTS share_items;
DROP TABLE IF EXISTS share_opaque_login_attempts;
DROP TABLE IF EXISTS share_links;
DROP TABLE IF EXISTS upload_parts;
DROP TABLE IF EXISTS upload_sessions;
DROP TABLE IF EXISTS folder_search_tokens;
DROP TABLE IF EXISTS file_search_tokens;
DROP TABLE IF EXISTS file_versions;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS folders;
DROP TABLE IF EXISTS opaque_login_attempts;
DROP TABLE IF EXISTS instance_settings;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS totp_trusted_devices;
DROP TABLE IF EXISTS totp_recovery_codes;
DROP TABLE IF EXISTS auth_mfa_challenges;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS prevent_username_change();

DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
