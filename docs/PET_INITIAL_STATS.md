# Pet initial attributes

Fresh recruits use `catalog_pet_initial_stats` in assets.db (structured schema
v18). Each row has minimum and maximum STR, CON, INT, WIS and AGI, a desired
`TotalPoints`, and source provenance. Recruitment begins at the minima and
randomly assigns remaining points among attributes below their maxima until
the desired total is reached. SQL loading and administration reject impossible
totals or inverted ranges. These definitions do not alter NPC enemy stats or
level-up growth weights.

Without a usable table entry, recruitment uses half of each NPC-template
attribute, rounding each odd value down. Empty and all-zero wiki placeholders
are omitted. HP/SP and combat stats still use the shared player formulas; wiki
HP/SP/ATK/DEF/MAT/MDF/SPD columns do not override those formulas. Wild captures
retain their separate half-total rounding budget and growth rolls. Existing
owned and restored reserve pets retain their base attributes, growth, potential
and equipment.

## References and precedence

The owner selected classic WLO tables and ranges over conflicting Rhodes values.
Values before parentheses are initial stats; parenthesized rebirth values are
excluded. The primary saved page is
`var/reference-web/stats/NPC - Wonderland ONLINE Wiki_.htm`
([classic NPC tables](https://wikiwiki.jp/wlo/NPC)), SHA-256
`7a51ce91dc1d3bad9a558d1ac0c42c6c0526b72660a44251af7da4d6ce303c42`.
It provides 23 complete initial-attribute entries, including ranges.

The owner's original saved
`var/reference-web/NPC - 飄流幻境（Wonderland ONLINE）羅德島傳　日本語wiki Wiki_.htm`
([Rhodes NPC tables](https://wikiwiki.jp/wlo_ratoku/NPC)), SHA-256
`7d901009ea9f348a3a3d057476e5d46f4270880df34761a978a0d7e2a85c354d`,
provides seven additional entries absent from the classic tables. The resulting
authored seed is `internal/assetsql/pet_initial_stats_defaults.json` (30 entries).
Runtime reads the typed SQL projection, never these pages or a JSON file.

Additional supplied references were reviewed as corroboration:

| Saved page under `var/reference-web/stats` | SHA-256 | Treatment |
| --- | --- | --- |
| `NPC - 魅惑の音色 Wiki_.htm` | `f3376d3c9421dc9a388125ba061162ec7db7da7b8674e9135a6e1799f4166e49` | Broadly corroborates classic ranges; conflicting WIS/range values defer to the classic table. |
| `人性宠物初始属性大全飘流幻境——17173网络游戏专区.htm` | `3916d487169831be2f28865cc1bbcd2549e173ef2a58a663b433c93a4b462908` | 2006 example starting rolls; not used to replace complete ranges. |
| `漂流初期宠物讨论飘流幻境——17173网络游戏专区.htm` | `bd74632ee89014abe25d5348d9dd81c8bc1dcacac2c573a4b9dc8dcf2549908c` | 2006 examples include different regional values; classic-first precedence retained. |
| `【情報】各_人寵_素質，請安心食用!! @飄流幻境 Online 哈啦板 - 巴哈姆特.htm` | `01cc91b5e3886d0ba7fc04280017c1f29b68c820880d34b06eec9fa723cbbda1` | Individual examples and incomplete values; no override of complete classic ranges. |
| `クエストNPC - 夜空に願いを Wiki_.htm` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` | Empty file; supplies no evidence. |

## Desired totals

Robinson (12178; NPC alias 12032) uses STR 7, CON 10–11, INT 2, WIS 4–5,
AGI 6–7 and the owner-selected total of **30**. One point is randomly assigned
to CON, WIS or AGI above their minima.

Other variable definitions use half the existing template's total, rounded
down, when it fits the reference bounds; otherwise their total is the rounded-
down midpoint between the minimum and maximum sums. Fixed tables use their
exact sum. These totals are authored seed values and remain administrator-
editable in SQL; they are not recalculated from NPC stats at runtime.

## Migration and validation

The preserving v17 → v18 assets migration adds the initial-stat table and its
presence marker. Existing tables, administrator edits and intentionally empty
initial-stat definitions remain intact. Follow [ASSET_DATABASE.md](ASSET_DATABASE.md)
when deploying the server.

Tests independently transcribe all 30 rows and desired totals, cover bounded
rolls, fallback/capture behavior, fresh recruitment and reserve restoration,
and verify preserving SQL migration. Installed-data recruitment tests use
`WONDERLAND_TEST_ASSETS_DB` pointing at the upgraded assets database.

## Authored starting ranges and totals

| ID | Pet | STR | CON | INT | WIS | AGI | Total |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 12068 | Magellan | 7–9 | 6–7 | 4–6 | 5–6 | 3 | 27 |
| 12095 | Frederico | 15 | 19 | 34 | 33 | 17 | 118 |
| 12147 | Louis | 11–12 | 10–11 | 7 | 8–9 | 8–9 | 45 |
| 12148 | Athena | 19 | 27 | 35 | 31 | 25 | 137 |
| 12149 | Venus | 16 | 24 | 34 | 33 | 26 | 133 |
| 12178 | Robinson | 7 | 10–11 | 2 | 4–5 | 6–7 | 30 |
| 12179 | R.Soldier | 9 | 9 | 5 | 5 | 8 | 36 |
| 12180 | Kana / Kama | 21 | 30 | 43 | 36 | 5 | 135 |
| 12181 | Yang Jian | 44 | 40 | 20 | 25 | 6 | 135 |
| 12217 | Shadow Frederico | 19 | 23 | 38 | 37 | 21 | 138 |
| 12219 | Saint Angela | 11 | 19 | 24 | 25 | 24 | 103 |
| 12222 | Night Eva | 12 | 20 | 27 | 26 | 26 | 111 |
| 14081 | Niss | 1 | 1 | 5–6 | 5–6 | 2 | 15 |
| 14156 | Xaolan | 1–2 | 1–2 | 2–4 | 2–4 | 4–6 | 13 |
| 14162 | Roca | 4–5 | 5–6 | 2–3 | 2–3 | 4–5 | 18 |
| 14175 | Clive | 7–8 | 6–7 | 2 | 1 | 4–5 | 21 |
| 14230 | Elin | 3–4 | 10–11 | 5–6 | 4–6 | 7–8 | 30 |
| 14242 | Eva | 4 | 5–6 | 13–15 | 12–14 | 12–14 | 48 |
| 14243 | Angela | 7 | 15–16 | 19–21 | 19–22 | 21–23 | 84 |
| 14274 | Sam | 21–22 | 16–17 | 7–8 | 4 | 14–15 | 63 |
| 14548 | Kanako | 12–13 | 16–17 | 11–12 | 14–15 | 12 | 66 |
| 14609 | Shasha | 7–8 | 6 | 15–16 | 15–16 | 8–9 | 52 |
| 14615 | Qlaya | 3 | 5 | 14–15 | 10–11 | 6–7 | 39 |
| 14637 | Shizune | 12 | 10 | 12 | 11 | 13 | 58 |
| 15657 | Victoria | 18–20 | 18–20 | 6 | 11–12 | 12–13 | 67 |
| 15689 | Hayate | 14–15 | 10 | 11 | 13–14 | 12 | 61 |
| 17162 | Monkey | 1 | 2 | 1 | 1 | 2 | 7 |
| 17454 | Arez | 16–17 | 10–11 | 2–3 | 1 | 3–4 | 33 |
| 17914 | Charlotte | 2 | 2 | 7 | 10 | 15 | 36 |
| 35253 | Joan | 21–22 | 20–21 | 11 | 5 | 14 | 72 |
