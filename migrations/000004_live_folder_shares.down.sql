DELETE FROM share_links
WHERE id IN (SELECT share_link_id FROM share_items WHERE item_type = 'folder');

DROP TABLE IF EXISTS share_folder_entries;
DROP INDEX IF EXISTS share_items_folder_id_idx;
ALTER TABLE share_items
  DROP CONSTRAINT share_items_item_type_chk,
  DROP COLUMN folder_id,
  ALTER COLUMN file_id SET NOT NULL,
  ADD CONSTRAINT share_items_item_type_chk CHECK (item_type = 'file');
ALTER TABLE share_links
  DROP CONSTRAINT share_links_status_chk,
  ADD CONSTRAINT share_links_status_chk
    CHECK (status IN ('active', 'expired', 'revoked', 'burned', 'disabled'));
