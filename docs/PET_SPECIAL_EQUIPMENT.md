# Companion starting equipment

Freshly acquired companions receive their normal special item equipped directly
on the pet, without using player inventory space. The saved equipment and pet
are committed together. This covers recruitment, vouchers, NPC bazaar purchases,
GM creation and capture. Returning reserve/hotel pets and player transfers keep
their saved equipment; removed weapons are not regenerated.

The owner's saved WLOHub Pets page supplies 20 normal equipment definitions.
Robinson and Shasha have no listed normal item. Monkey's Banana Peel is a
consumable in the native item database, so it is not equipped. Reborn equipment
is retained in the reference report but is not granted on normal acquisition.
This change does not implement equipment replacement on rebirth.

Definitions belong to the SQL `Story.CompanionEquipment` collection, stored in
`catalog_story_companion_equipment` and its equipment child table. Native slots
1–6 become zero-based equipment indexes. The world projects these definitions
into pet templates; the fresh-pet factory validates native item slots and fills
HP/SP after applying equipment bonuses. Clients receive the standard AC15:8
roster because AC15:1 does not contain equipment IDs.

Assets migration v26 appends missing new definitions, preserves administrator
edits and intentional empty collections, and does not restore deliberately
removed historical Roca/Xaolan defaults. Existing owned pets are unchanged.
The JSON is an offline initialization seed; runtime reads SQL only.

Reproduce the import from an extracted native item catalog:

```powershell
python tools/data_export/extract_pet_equipment.py --pages var/reference-web/pets --item-export data/item_data.json --output-dir var/pet-equipment
```

The resulting `pet_special_equipment_defaults.json` seeds assets initialization;
`equipment.csv` and `sources.json` are archived under
`tools/data_export/reference/wlohub-pets`. That directory also contains authored
pet identity mappings and documented item choices. Name matches require icon
and rank agreement. Athena uses the ordinary ATK/MAT +50 spear, rather than the
modified seal-resistance version; translated item aliases are explicit choices.

The local assets database was upgraded to v26 and all 20 mappings verified
(21 SQL entries including both historical Roca aliases). The original is backed
up at `var/backups/pre-migrate-20261104-063031`. Gameplay data was not migrated.
