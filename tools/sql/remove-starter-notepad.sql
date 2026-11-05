-- Content correction: Notepad (34038) is granted by the Astrology lady's
-- native event (map 12003, event 4), not the starter pack.
-- Run against assets.db with the server stopped, after taking a backup.
-- Preserve other configured grants and their order. Safe to apply repeatedly.
BEGIN IMMEDIATE;
CREATE TEMP TABLE retained_starter_grants AS
SELECT ROW_NUMBER() OVER (ORDER BY starter_items_ordinal) - 1 AS ordinal,
       value_id, value_count
FROM catalog_starter_items
WHERE value_id <> 34038;
DELETE FROM catalog_starter_items;
INSERT INTO catalog_starter_items (starter_items_ordinal, value_id, value_count)
SELECT ordinal, value_id, value_count FROM retained_starter_grants;
DROP TABLE retained_starter_grants;
COMMIT;
