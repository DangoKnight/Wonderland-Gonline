# World PK reference and capture findings

Status: UI presentation implemented; gameplay and network integration are not yet
enabled. This document separates observed packets, decompiled client behavior
and published rules. It does not turn unverified mobile rules into PC defaults.

## Evidence

The user completed the World PK sequence on the Legacy server on 2026-11-06:
window, leaderboard/page navigation, My Ranking, both offline permissions,
battle, victory and reward inspection/opening. The requested character was
Test0002. The active connection used local port 50450, process 44936, with
participating character ID 4617590. Port 61942 was the other connection. The
capture has no character-name/login packet, so that name-to-ID association is
based on the requested test and active connection, not a decoded login name.

Local artifacts (not distributable server definitions):

- `var/pet-switch-capture/world-pk-reference-20261106-01.pcapng`
- `var/pet-switch-capture/world-pk-reference-20261106-01.world-pk.json`
- `var/pet-switch-capture/world-pk-reference-20261106-01-helper.log`
- `var/pet-switch-capture/decode_world_pk.go`
- `var/decompiled/aLogin_ca19ee087b60.c`

Packet Monitor stopped normally and restored the initially empty filter set.
It reports 2,186 packet observations and 35 packet drops. After TCP segment
deduplication/reassembly, 626 native messages were decoded. Several outgoing
sequence ranges are absent, including the initial requests for leaderboard,
settings and matchmaking. Do not manufacture request goldens from their replies.
The captured `[183,9]` request and both combat requests are direct wire evidence.

Saved gameplay references under `var/world-pvp`:

1. `世界PK - 飄流幻境（Wonderland ONLINE）羅德島傳　日本語wiki Wiki_.htm`
   is the primary PC/Rhodes rules reference (page revision 2021-12-13).
2. `PK - 飄流幻境(WonderlandONLINE)M　手機版 Wiki_.htm`
   describes the mobile version (2023-07-02). It differs in important ways.
3. The 2022-10-06 double-ten article describes a temporary PC reward promotion.
4. `《飄流幻境M》復活節活動網頁.htm` describes a temporary mobile promotion
   in April 2025; its table also identifies mobile chest probabilities.
5. The other official mobile page describes unrelated promotional rewards.

## Observed protocol

All examples below are decoded decimal payload bytes, including the action and
subcode. U16/U32 fields are little-endian. Times are relative to the first
decoded message, not the beginning of Packet Monitor recording.

| Time | Direction | Packet | Interpretation / evidence limit |
| --- | --- | --- | --- |
| 30.429 s | Server → client | AC183:5, 16 bytes | Own wins/rank summary, initially zero |
| 38.695 s | Server → client | AC183:7, 2,784 + 5,479 bytes | Full Top 300 in two chunks, 100 and 200 records |
| 50.153 s | Server → client | `[183,17,2]` | Offline permission confirmation: Disallow |
| 54.895 s | Server → client | `[183,17,1]` | Offline permission confirmation: Allow |
| 71.697 s | Server → client | `[11,10,13]` | World PK battle kind 13 |
| 71.697 s | Server → client | AC11:250 and AC11:5 | Player/pet battle formation |
| 71.697 s | Server → client | `[183,12,1,9]` | Activity display becomes nine |
| 119.553 s | Server → client | `[11,12,1]`, then AC11:0 | Victory for side 1; battle closes |
| 119.887 s | Server → client | AC183:5 | Wins becomes one, rank becomes 28,808 |
| 122.033 s | Client → server | `[183,9]` | Own-summary refresh; also called by native UI show/refresh |
| 127.373 s | Server → client | AC183:11 plus two AC23:6 packets | Chest result/status and actual inventory deliveries |

The primary connection's AC183:5 after victory is:

```text
[183,5,118,117,70,0,1,0,0,0,1,0,136,112,0,0]
```

`FUN_00155e2c` reads character U32, wins U32, an additional U16, and rank U32.
The U16 changed from zero to one here; preserve it separately until its meaning
is resolved. Do not conflate it with the rank or activity counter.

### Leaderboard records

`FUN_00155fd0` and continuation `FUN_001563e8` describe AC183:7. The initial
chunk contains subtype 1, requesting character U32, own rank U32 and total U16.
The continuation has subtype 2 and character U32 before its records. This trace
contains exactly 100 + 200 complete records, with no trailing bytes.

Each record contains:

| Field | Encoding |
| --- | --- |
| Character ID | U32 |
| Wins | U32 |
| Body / head | U8 / U8 |
| Appearance colors | U32 / U32 |
| Level | U8 |
| Name | U8 byte length, then Big5 bytes |
| Additional badge/class marker | U8; retain raw until named from native lookup |

The first record has character ID 15837, wins 26135 and level 190, matching the
provided leaderboard's leading win count. The second chunk begins with ID
25487 and wins 3129. Four displayed rows per page yields 75 pages for 300 rows.
Page arrows are local list navigation; the capture does not establish a separate
network request per page. Rank refresh/personal lookup must follow the native
request handlers rather than assuming that every button sends a page number.

### Battle compatibility

World PK uses the familiar AC50 action, AC51 stat, AC52 ready and AC53 submitted
messages, with extra World PK state around them. It is not a separate damage
wire format. This test includes four human combatants and their four pets.
Some participants can be server-controlled.

The formation starts `[11,250,14,0,22,2,...]`: retain the initial participant
mode 22 and battle kind 13 as explicit native compatibility values. Several pet
AC11:5 messages are 36 bytes, while other participant messages are 34 bytes.
AC11:13 messages also occur. Existing strict Gonline parsers must be extended
using the native layouts; simply adding AC183 to the toolbar is insufficient.

### Chest receipts

At 127.373 s the server sends:

```text
[183,11,9,1,104,1]
[183,11,2,1,2]
[183,11,6,1,2,64,137,1,83,133,1]
```

The accompanying AC23:6 packets deliver one item 35136 and one item 34131.
The repository's English catalog names them Mini SP Pack and 3 Holy EXP Potion.
The six-byte status includes U16 value 360; its precise timer interpretation
still needs the chest handler cross-check. Other status records include
`[183,11,9,2]`, `[183,12,3,120]` and `[183,12,3,119]`; keep these raw rather
than assigning guessed units or meanings. Chest state changes must not themselves
duplicate inventory delivery already handled through AC23.

## Decompiled request evidence

These calls exist in the native client, but their corresponding outbound frames
were not all captured. Verify full writer layouts before adding wire goldens.

| Native function | Call / role |
| --- | --- |
| `FUN_00158dcc` | AC183:1, begin opponent search; rejects an active stall |
| `FUN_00158e74` | AC183:2, cancel/end search path |
| `FUN_001596ac` | AC183:7, ranking request with an in-flight guard |
| `FUN_00159844` | AC183:8, additional ranking/personal request path |
| `FUN_0015927c` | AC183:9, summary refresh; directly observed on wire |
| `FUN_00158788` | AC183:17, offline participation permission change |
| `FUN_00159018` | Incoming AC183 dispatch |
| `FUN_00156b80` | Incoming permission acknowledgement and UI refresh |

`FUN_001592bc` offers immediate chest clearing for five points; a separate
native dialog offers activity restoration for fifty points. These are distinct
transactions, not the same operation or the same price. The native UI also has
search cancellation, reward slots, countdown/respawn text and personal-ranking
controls beyond the initial presentation implementation.

## Gameplay rules and source priority

Use the PC/Rhodes reference first, corroborated by live traffic and native code:

- Matching usually uses server-controlled copies of real players; live opponents
  can also occur. Parties may be uneven. The reference describes grouping by
  wins/ranking proximity; its example rank bands are illustrative, not a complete
  reproducible matchmaking algorithm.
- Offline Allow/Disallow controls whether a stored copy can be used as opponent
  or teammate. Outcomes for that copy do not change its owner's score. The UI
  additionally says three days offline resets permission to Allow.
- A player's active combat pet and equipment come from the stored snapshot.
  Copies start at full HP/SP. Do not let a simulated copy mutate the real
  offline character's recoverable vitals, equipment or inventory.
- Win adds one victory point; defeat subtracts one. The score floor and ties
  remain unresolved. The captured test changes wins 0 → 1.
- No victory EXP, defeat EXP loss, pet amity loss, or overkill return-to-spawn.
  These exceptions must apply to this battle mode, not change normal combat.
- Recovery items are prohibited. Do not infer that recovery/revive skills are
  prohibited from that restriction; their exact native skill behavior needs care.
- Activity caps at ten and recovers one per two hours. At zero, PC/Rhodes permits
  continued fighting without rank/reward benefit, consistent with the native UI.
  Purchasing one activity point costs fifty paid points in the PC reference.
- Three chest slots. A win can award a chest; opened/used slots clear on the
  six-hour schedule. Precisely when expiry starts and how full slots affect
  activity consumption still need verification, rather than guessed timers.

Mobile sources are supplementary. They claim zero activity blocks participation,
activity restoration costs five green stones, matching is largely level-based,
and status seals last two turns. Those differ from or are not established by
the PC reference. Do not copy them into PC-compatible defaults.

The mobile Easter table gives wood/silver/gold selection probabilities 85/10/5.
It does not establish PC probabilities. The PC wiki lists differing reward-pool
sizes by chest tier and guaranteed recovery supplies, but does not provide a
complete current weighted table. The 2022 PC and 2025 mobile articles describe
dated bonus campaigns, not permanent rewards to enable today.

## Possible chest rewards

### Selected provisional Gonline model: mobile rewards

The administrator/user selected the mobile reward model provisionally, rather
than the PC/Rhodes pool below. Use this selection when implementing World PK
rewards; the current World PK module remains presentation-only.

| Chest | Chest-selection probability | Mobile reference contents |
| --- | --- | --- |
| Wood | 85% | 1–3 green stones, 500–1,000 EXP capsules, training dolls, and unspecified other rewards |
| Iron (middle/silver UI tier) | 10% | 3–5 green stones, 1–2 Aura/Potential pills, 1–2 Chaos Crystals, recovery bottles, and unspecified other rewards |
| Gold | 5% | 5–9 green stones, 2–3 Aura/Potential pills, 3–4 Chaos Crystals, recovery bottles, and unspecified other rewards |

The EXP range describes capsule denominations, not a roll of 500–1,000 capsule
items. The source does not establish whether every listed category is granted
together, how many rewards are selected, their individual weights, recovery
bottle quantities, or whether all integers within each range are equally likely.
Do not import the PC reward-entry counts or guaranteed-recovery rule into this
model. Do not treat the mobile reference's open-ended “etc.” as an authored pool.

For implementation, preserve these as configurable shared reward definitions in
assets SQL. Resolve green stones to an explicit currency or item before granting
them; a similarly named catalog stone is not sufficient evidence. Confirm exact
capsule/doll IDs and the Aura pill alias too. These unresolved delivery mappings
must not silently become unrelated PC rewards. Mobile reward selection does not
change the separately researched PC combat, activity or paid-operation rules.
Seasonal campaign bonuses remain excluded.

The reference list is now stored in the `WorldPKRewards` asset definition
(structured asset schema v58). The tracked initialization input is
[`world_pk_rewards_defaults.json`](../internal/assetsql/world_pk_rewards_defaults.json).
Its root, chest and reward tables are `catalog_world_p_k_rewards`,
`catalog_world_p_k_rewards_chests` and
`catalog_world_p_k_rewards_chests_rewards`. SQL retains source/model metadata,
chest selection weights 85/10/5, individual reward keys, item/currency identity,
quantity ranges, capsule value ranges and unresolved evidence. Individual reward
weight zero means unverified, not a configured drop; quantity zero likewise means
unknown. The listed capsule values do not specify every denomination in the
interval. These records are not yet executable chest grants. Runtime must not
load the JSON or fabricate missing values. Explicit migration seeds new tables
only, preserving existing edits and empty pools.

### PC/Rhodes reference pool (not the selected default)

The saved PC/Rhodes `世界PK` page lists the following pools. These are reference
possibilities, not a verified current weighted reward configuration.

| Chest | Reward entries | Possible contents |
| --- | --- | --- |
| Wood | 2 | Small HP/SP recovery supplies, 2× EXP potion, Warriors Fragments, Demon Scroll, Principality Scroll |
| Silver | 2–3 | Wood pool plus one Aura pill, 2.5× EXP potion and Warriors Pieces |
| Gold | 3–4 | Silver pool plus Chaos Crystal, 3× and 4× EXP potions; Aura pill quantity is 1–3 |

The page says recovery supplies are guaranteed. It does not specify individual
item weights, the distribution of reward-entry counts, or whether duplicate
selections are possible. Its recovery-bottle wording does not resolve whether
the delivered item is a bottle or a pack containing bottles.

| Reference reward | Repository item ID | Catalog name / evidence |
| --- | --- | --- |
| Small HP recovery bottle | 34330 | Mini HP Potion; pack wrapper 35135 contains two |
| Small SP recovery bottle | 34331 | Mini SP Potion; pack wrapper 35136 contains two |
| 2× EXP potion | 34146 | 2 Holy EXP Potion |
| 2.5× EXP potion | 34147 | 2.5 Holy EXP Potion |
| 3× EXP potion | 34131 | 3 Holy EXP Potion; captured delivery ×1 |
| 4× EXP potion | 34143 | 4 Holy EXP Potion |
| Warriors fragments | 30553 | Warriors Fragm / 勇士徽章碎片 |
| Warriors pieces | 30552 | Warriors Piece / 勇士徽章拼圖 |
| Demon forging scroll | 34507 | Demon Scroll / 惡魔鍛煉卷 |
| Principality forging scroll | 34511 | Principality S / 權天使鍛煉卷 |
| Chaos Crystal | 61062 | Chaos Crystal / 混沌結晶 |
| Aura pill / オーラ薬 | Unresolved | Potential Pill 34269 is a candidate; confirm the alias before assigning an ID |

The live receipt confirms **Mini SP Pack 35136 ×1** and **3 Holy EXP Potion
34131 ×1**. The claimed chest's tier is not yet confidently decoded; do not infer
its tier or force every chest to award the wiki's minimum entry count from this
single receipt. Warriors Badge 30551 is assembled from the fragment/piece chain
and is not itself listed as a direct chest reward.

Keep seasonal rewards separate: the saved October 6–13, 2022 PC campaign added
Training Island trial tickets (wood: 10% ×1; silver: 50% ×2; gold: 100% ×3).
The saved mobile tables include green stones, EXP capsules and training dolls,
but neither those contents nor their 85/10/5 chest probabilities establish the
PC reward pool.

## Implementation work remaining

1. Add validated AC183 codecs/state and native combat compatibility, preserving
   raw packet goldens independently of encoder tests.
2. Wire the UI to acknowledged state: scores, activity, ranking chunks, settings,
   searching/cancel, chest status, reward previews and personal ranking.
3. Add a dedicated World PK battle policy for protected progression, offline
   copies, AI control, consumable restrictions and normal live round transport.
4. Own scores, snapshot versions, permissions, activity recovery, chest slots and
   paid operations in gameplay SQL. Reward/chest claims and paid purchases must
   debit/claim/deliver atomically with replay and concurrent-request protection.
   Shared reward definitions and configurable matching bands belong in assets
   SQL; queues, live battles and request guards remain in memory.
5. Resolve the missing request writers, extended participant fields, loss/escape
   score behavior, chest expiry/full-slot behavior and PC reward weights before
   claiming complete Legacy compatibility. A targeted follow-up capture can fill
   the outbound gaps; the completed battle/reward trace remains useful evidence.

No gameplay implementation or schema changes were made during this capture pass.
