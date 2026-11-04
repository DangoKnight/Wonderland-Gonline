# Short-name audit

The NPC research follow-up is complete: [enemy name review](ENEMY_NAME_REVIEW.md)
records 129 additional corrections and the evidence for retained names. The
counts and reports below describe the original audit baseline. Item candidates
have not been changed by the enemy review.

The existing automatic restorations consider ten-byte NPC names and item names
of at least thirteen bytes. Explicit owner choices can bypass the NPC threshold.
Pineapple Monster exposed the consequence: genuinely abbreviated shorter names
are skipped as well. This audit reports them without changing either compiled
patch or the assets database.

Against the supplied native exports and saved reference tables:

| Catalog | Short names with longer prefix candidates | Unique metadata-matched extensions |
| --- | ---: | ---: |
| Monsters/NPCs | 387 | 62 |
| Items | 307 | 9 |

These are candidate counts, not counts of proven incorrect names. Monster
metadata matches require level, element and all five attributes. Item matches
require icon/rank, with descriptions used for disambiguation. Already-complete
item names are identified separately. The six explicit owner choices and the
approved Pineapple Monster correction are excluded from the NPC audit.

## Likely missed restorations

Examples with a single matching reference name:

| Native ID | Current name | Reference candidate |
| --- | --- | --- |
| 17004 | Fragrance | Fragrance Monster |
| 17005 | Delicate | Delicate Monster |
| 17410 | Delicate | Delicate Monster |
| 17411 | Floating | Floating Monster |
| 17019 | Poisonous | Poisonous Ant |
| 17050 | Poisonous | Poisonous Mushroom |
| 17079 | Poisonous | Poisonous Frog |
| 34432 | 20% EXP Caps | 20% EXP Capsule |
| 34433 | 25% EXP Caps | 25% EXP Capsule |
| 34439 | 2% EXP Caps | 2% EXP Capsule |
| 34632 | Crab RB | Crab RB Pill |

## Reasons to review the shorter names

Several results are potential variant or reference-data problems:

- `UFO` (48031) matches `UFO Capsule`; vehicle versus capsule meaning needs checking.
- `Pizza` (32162) matches `Pizza A`, and `Steel Hammer` (25203) matches
  `Steel Hammer 9`. A unique icon/rank result alone does not prove the variant.
- `Orangutan` matches the reference spelling `Orangutang`; `Elliot` matches
  `Elliott`. These are spelling differences rather than obvious truncation.
- `Tower`, `Torch`, `Tsiji` and `Barricade` gain `(Trojan War)` annotations;
  `Troll` gains `(North to South Island Subway)`. These can be database context
  rather than intended in-game names.
- Other NPCs named `Pineapple` have different metadata from the reference's
  level-one monster. They are reported separately; only the explicitly approved
  NPC 17001 has been renamed so far.
- Broad prefixes such as `Poisonous`, `Defensive` and `Pink Snow` refer to
  multiple different creatures. Their IDs must remain distinct during review.

The reports include every candidate, exact metadata matches, source IDs and
classification hints:

- [Monster/NPC short names](../tools/data_export/reference/name-edge-cases/monster_short_names.csv)
- [Item short names](../tools/data_export/reference/name-edge-cases/item_short_names.csv)
- [Input checksums and counts](../tools/data_export/reference/name-edge-cases/sources.json)

Classification hints identify obvious annotation, spelling or variant suffixes;
they are not a comprehensive semantic safety check. Other source spellings may
also need correction. Approval of one candidate should be authored by native ID,
as with the earlier owner choices, instead of replacing every matching prefix.

## Reproduce

```powershell
python tools/data_export/audit_short_names.py --npc-export /path/to/npc_data.json --item-export /path/to/item_data.json --output-dir /path/to/output
```

The compiled NPC patch and authored reference CSVs default to repository paths;
`--npc-patch`, `--monster-reference` and `--item-reference` select alternatives.
The audit does not read or write a gameplay database. Supplied inputs reproduce
both reports and the checksum manifest byte-for-byte.
