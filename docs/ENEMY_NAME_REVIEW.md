# Remaining enemy-name review

The owner's request to investigate remaining ambiguous enemy names produced
**129 additional ID-specific corrections**, bringing the compiled patch from
769 to **898** names. Inputs are the saved 1,190-entry WLOHub reference and the
4,930-record native NPC export. No enemy stats or other gameplay rules changed.

The review includes all 387 entries in the earlier short-name audit and all
1,676 additional unpatched names occupying the native ten-byte name capacity.
Some records are NPCs or props rather than battle enemies; they remain in the
evidence report so that an object is not mistaken for a similarly named monster.

| Result | Records |
| --- | ---: |
| Name corrected | 129 |
| Exact existing full name confirmed | 115 |
| Existing name retained without reference context annotations | 5 |
| Valid existing spelling retained | 5 |
| Insufficient evidence for a replacement | 1,809 |
| Total reviewed | 2,063 |

The unresolved count does **not** mean 1,809 names are truncated. Many are valid
short names, NPCs absent from the monster reference, or capacity-length names
that may already be complete. They retain their existing display names.

## Evidence and decisions

The 129 authored resolutions break down as follows:

- 52 retain the native prefix and match level, element and all five attributes.
- 15 match that metadata and the creature identity despite missing leading or
  middle words: Snake becomes Poisonous Snake, Chameleon becomes Ancient
  Chameleon, and Snow God becomes Snow Door God for the specific verified IDs.
- 34 additionally match both HP and SP. These restore abbreviations and different
  word order, such as S.Dragonfl to Sky Dragonfly, Bird God R to Ruka the Bird God,
  and Green Mamm to Grass Mammoth.
- 28 are individually reviewed species variants. Native model/portrait data
  separates Poisonous Flower, Ant, Spider, Snake and Scopion. The five levelled
  Pineapple variants use the fruit-monster model; the human NPC and prop with
  similar names are excluded. Notes identify differences in reference metadata,
  including Snow Evil Spirit's level, Land Snow Monkey's CON and Supplying
  Ghost's element. Reference stats are never imported into gameplay definitions.

`enemy_name_resolutions.csv` records each selected native ID, original name,
source IDs, evidence method, native level/element/attributes/vitals and rationale.
Extraction rejects stale native identities, missing source records, inconsistent
source names and invalid exact-stat/vital claims. Researched corrections can
restore non-prefix words; they remain guarded by native ID and original name.

`enemy_name_review.csv` records every reviewed row's native type, level, element,
HP/SP, attributes, sprite, portrait, four palette values, prefix candidates,
exact metadata/vital matches and decision. These are factual matches, not an
automatic assertion that identically statted enemies have the same identity.
Both CSVs and input checksums are under
`tools/data_export/reference/wlohub`.

Important preserved cases include Bear versus Beard the Pirate, Monk versus
Monkey, Ares versus Aresh Cactus, and the Blueberry cake-shaped model versus
Blueberry Icedrop. Orangutan and Elliot retain their native spellings. Tower,
Torch, Tsiji, Barricade and Troll do not gain website location annotations.
Ancient Sc has the attributes of a reference Ancient Spider but a scorpion
model and different vitals, so that apparent match is rejected. Some reference
spellings, including Scopion and Scopian, remain verbatim in accepted full names;
this review does not invent separate spelling corrections.

## Reproduction and deployment

```powershell
python tools/data_export/extract_monster_names.py --pages var/reference-web/monsters --npc-export data/npc_data.json --accept-single-candidates --strip-quest --output-dir var/enemy-names
python tools/data_export/review_enemy_names.py --npc-export data/npc_data.json --output-dir var/enemy-names
```

Input paths can point to any saved reference/native export location. The compiled
CSV is shared by client display loading and offline server initialization; server
runtime reads names from SQL. Assets migration **v27** updates only matching
original/previous name fields, preserves custom names and missing records, and
does not rewrite player-owned pet names or native exports. Repeat upgrades are
idempotent. Existing installations use [ASSET_DATABASE.md](ASSET_DATABASE.md).

The local offline assets database was backed up at
`var/backups/pre-migrate-20261104-064512` and upgraded to v27. All 898 patched
names were read back; exactly 129 additional SQL names changed. A before/after
hash of every other NPC column across all 4,930 records was identical. Gameplay
data was not migrated. Both next server/client executables build successfully.
