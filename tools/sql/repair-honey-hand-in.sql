-- Repair the premature honey hand-in completion written by the old bee policy.
-- Run against the gameplay database with the server stopped, after a backup.
-- 13022 is the active request; 13023 must remain unset until handing in honey.
-- State 1 = InProgress, state 2 = Completed; item 30034 = Honeycomb.
-- Only repair unfinished requests with honey still in the player's bag.
-- Native hand-ins have step 1 and are preserved. Safe to repeat.
BEGIN IMMEDIATE;
DELETE FROM character_quests
WHERE quest_id = 13023 AND state = 2 AND step = 0
  AND EXISTS (
    SELECT 1 FROM character_quests AS request
    WHERE request.character_id = character_quests.character_id
      AND request.quest_id = 13022 AND request.state = 1 AND request.step = 1
  )
  AND EXISTS (
    SELECT 1 FROM character_items AS honey
    WHERE honey.character_id = character_quests.character_id
      AND honey.container = 'bag' AND honey.item_id = 30034 AND honey.count > 0
  );
COMMIT;
