# Fandom quest inventory and implementation

Research date: 2026-11-04. The requested [Quests by Notepad Page](https://wonderlandonline.fandom.com/wiki/Quests_by_Notepad_Page) index lists **345 quests on 35 pages**, describing the V5 journal. [QUEST_LIST.md](QUEST_LIST.md) lists every entry in source order, including unlinked labels. This is a census of that index, not a claim that it enumerates every quest ever added to Wonderland Online.

The public MediaWiki API supplied the index, all 368 main-namespace articles in [Category:Quests](https://wonderlandonline.fandom.com/wiki/Category:Quests), and the union of their linked quest titles. That produced 427 available canonical articles and 46 missing canonical article titles. Missing articles do not remove their journal entries from the list. Category membership also includes navigation/helper pages, so 368 is an article count rather than an additional quest count.

## Inventory and evidence

The factual reference tables are in [fandom-quests](../tools/data_export/reference/fandom-quests):

| File | Purpose |
| --- | --- |
| `notepad_quests.csv` | All 345 index positions, original labels, canonical article titles, revision IDs, native mark/event bindings and verification status |
| `category_pages.csv` | All 368 category article titles and source links |
| `notepad_aliases.csv` | 63 reviewed name/script bindings with identity guards and evidence |
| `completion_candidates.csv` | 350 reviewed native completion tails, their checkpoints and source positions |
| `reward_bundle_candidates.csv` | 25 mixed companion/skill completion tails, including four extensions of earlier contracts |
| `actor_candidates.csv` | 877 additional native animation/expression/turn contracts with operation identities |
| `constellation_actor_candidates.csv` | 16 additional scoped constellation cutscene operations |
| `constellation_rewards.csv` | Ten star IDs, native completion receipts and source positions |
| `battle_formations.csv` | All 287 formations referenced by the 551 mapped quest battle actions, retaining occupant queue IDs |
| `completion_policy_aliases.csv` | Stable identities for the first three completion contracts |
| `sources.json` | Source/cache checksums, catalog checksum and census |

All 345 notepad entries have bindings to existing native EVE scripts. Their union references 1,701 events. **A binding is not an end-to-end playthrough certification.** The CSV deliberately retains `native-script-unverified` for this distinction. Native event programs already contain much of this content; creating a second quest/reward implementation from wiki titles would duplicate existing state.

Full article wikitext and API responses remain in ignored `var/quest-research`, for offline inspection. The repository contains factual identifiers, titles, links and original implementation/evidence notes, not copied walkthrough prose. Wiki content is contributed by the Wonderland Online Wiki community; follow each source page's history for attribution and its footer for the applicable [CC BY-SA license](https://www.fandom.com/licensing).

Important identity corrections:

- Notepad **Apollo** means the Atum/Cairo **Sun God** rescue (15044/15045), not **Find Apollo** (14098) or **Apollo's Prophecy** (17000).
- **The Key of Babylon** uses 13214/13215. The native caption says “Secrets of Hanging Garden,” but its chest grants Key of Babylon 30229.
- **The Secrets of the Hanging Garden** uses 13216/13217. Its native caption says “Find the Lost Pass”; the gate, key prerequisite and Babylon Queen script identify the actual quest.
- NPC and item IDs occupy different namespaces. Temporary escort NPC 30178 is Murloc; item 30178 is Cocoa Nib.

## Implemented behavior

### Atomic native quest completion

Assets v29–v30 add 350 opt-in `quest_completion` contracts covering 210 distinct completion marks. Each identifies an exact map, event, branch, tail offset and first operation fingerprint. The executor validates the entire remaining native program before using the contract.

After the final dialogue acknowledgement, item consumption, item rewards, bounded gold changes, EXP/level effects and quest checkpoints commit in one owned-character SQL transaction. The transaction rechecks branch eligibility and affected checkpoints against fresh durable state. Invalid checkpoints, insufficient resources, cancellation or inventory overflow roll back the whole tail. Packet delivery happens after commit; a failed connection does not refund or repeat the reward.

Contracts only accept native item/gold/EXP and mark tails. They exclude callbacks and chest/timer programs. Existing exceptional authored rewards retain their own delivery contract. Unmatched programs retain the ordinary interpreter. These contracts strengthen native completion persistence; they do not replace 345 quests with 350 new quests.

Assets v36 adds 25 exact-scope `quest_reward_bundle` contracts for mixed companion/skill endings. They include 21 additional branches and four extensions that start before an existing resource-only tail. Recruitment or dismissal, learned skills, resources and completion receipts commit together. Recruitment retains normal initial stats, equipment, reserve restoration and Pet Hotel ownership checks. Relearning preserves an already trained skill. Roster and skill packets are published only after commit. Invalid receipts and unavailable companion slots roll back all effects; a failed socket does not repeat an already committed grant.

The four extensions are added only when their parent contract still equals its original default. An edited or removed parent is treated as an administrator decision, so migration does not silently extend its authority. Existing bundle edits and deliberately empty collections also survive. Native Collection and Xaolan rejoining endings have installed-catalog SQL regression fixtures; synthetic cases cover cancellation, ownership conflicts and failed post-commit delivery.

### Water Path Voice escort

Assets v31 adds 15 exact-scope `quest_companion` contracts for seven Water Path map events. Native companion actions 3/3 and 3/4 respectively join and remove the temporary Murloc. A whole-tail contract commits the roster change and associated quest advancement together, then sends ordinary legacy-compatible roster/checkpoint packets and actor visibility updates.

The native boat/material/free-party-slot gates remain authoritative. Reissue cannot create another escort while it is in the party, Pet Hotel or reserve. Unknown companion operands remain unsupported outside these reviewed contracts. Ordinary pets, battle selection and raft visibility rules retain their existing behavior. The C# reference also rejects these two escort operands; the implementation evidence is the native event programs and the [Water Path Voice](https://wonderlandonline.fandom.com/wiki/Water_Path_Voice) article.

### Secrete Fete Book

Assets v32 removes the obsolete `unsupported_companion_or_subject` disable reason from events 12765:13 and 12765:15. Their following-companion conditions are supported by the existing engine. Tests exercise the owned/following Fred alternatives at both quest stages and verify the battle formation. A different administrator-authored disable reason is preserved.

### Player equipment conditions

Native EVE condition 16 was missing, blocking the wedding-ring alternatives in [Fall in Love](https://wonderlandonline.fandom.com/wiki/Fall_in_Love). It now compares the item actually worn in the selected equipment slot, for equality or inequality. Inventory ownership alone does not satisfy it. Selector mapping is corroborated by all 45 native occurrences and their item equipment metadata, including veils, wedding clothes, boots, bouquets, rings and the flashlight. `FUN_0049c76c` in `var/decompiled/reference/units/275_unit.c` converts EVE 16 to native condition 22, retaining selector/item/comparison and zeroing the quantity operand.

This adds the missing equipment prerequisite; it does not certify the complete native marriage ceremony, reservations, invitation or deadline flow. Those need a separate native-client integration walkthrough; the server's existing chat marriage commands are not evidence that the ceremony works.

### Inventory capacity and the Sledge Wolf encounter

Native condition 15 now supports the paired can-receive/cannot-receive item checks as well as free-cell comparisons. The converter maps these to native inventory condition 21 with the item ID and selector retained. The authored positive branch grants the item; the negative branch displays talk 11087 or 50062, both explicitly inventory-space warnings. Capacity probes use a copy of the real inventory, respecting stack limits, locked slots and contiguous multi-cell footprints. Free-cell counts include occupied covered cells. A valid item-capacity warning is no longer suppressed merely because another unrelated cell is empty. Tests cover fragmentation, stack space, covered cells and event 12159:3's real 48008 reward branches.

Assets v37 adds one scoped `condition` policy for [Find the sledge wolf](https://wonderlandonline.fandom.com/wiki/Find_the_sledge_wolf). Event 12350:19 requires an unowned Sledge Wolf before starting the battle in which it can be captured. Its native 2/2/4 condition was unsupported. The policy permits that exact map/event/operand only while the wolf is absent from the party; the existing owned-wolf branch hides the actor, and the full-party branch retains its warning. This does not assign speculative semantics to 2/2/4 elsewhere. Installed-catalog tests cover all three alternatives and rejection outside the scope.

### The Princess Eloped disguise

Assets v33 adds 13 exact-scope `quest_disguise` contracts for the guard disguise and restoration in [The Princess Eloped!](https://wonderlandonline.fandom.com/wiki/The_Princess_Eloped!). These cosmetic actions use native AC5:5 owner/template records; zero restores the original appearance. `FUN_004560c0` (original `FUN_0043c46c`) in the reference role decompile saves the old appearance, loads the NPC template, and restores it on zero. `FUN_0049c76c` distinguishes temporary mode 2/restoration mode 3 from permanent conversion mode 1.

The server tracks active shape only in session memory and replays it in map snapshots to peers; native map-entry scripts provide the quest's reapplication/restoration points. The client supports batched records, NPC world/battle sprites and the status portrait without replacing permanent body/head, equipment, skills or stats. Permanent Breillat conversion remains a separate SQL transaction. Unknown disguise operands and unconfigured transformations remain closed. Tests render the actual disguise sprite, preserve the player's battle identity and restore the original appearance.

### Quest NPC reactions

Assets v34 adds 877 exact-scope actor-capability contracts, complementing 15 already covered native operations. They enable the 892 animation, expression and turn operations used by the mapped quests. Existing historical capabilities are excluded from the new batch, including when an administrator later removed one deliberately.

Native AC22:7 starts an NPC expression at code 100 plus its operand; AC22:8 starts an actor animation and selects the first available facing in the action family. The client now handles both packets, restarts repeated animations, releases an interrupted route and draws NPC expressions using the existing timed strip renderer. The decompiled handlers are `FUN_004437d0`/`FUN_00443868` (reference `FUN_0045d424`/`FUN_0045d4bc`); the latter does not use the compatibility byte following the action. Existing native event-kind-11 handling performs the authored turns. These additions leave gameplay rewards and quest checkpoints in their existing transaction paths.

### Constellation rewards and cutscenes

Assets v35 moves the old four-entry compiled constellation list into SQL-owned Story definitions and adds six native rewards: both Cave Bear Quest stars, Sasha, Death of Roca's Father, Secret Fete Book and Great Navigator. Receipt marks determine membership, duplicates are suppressed and the list is sent in native ID order. Sixteen additional scoped actor contracts enable their cutscenes while retaining previously reviewed ones. Operator edits and empty rule collections survive migration.

The client now accepts native AC15:19 collection snapshots and AC15:20 acquisitions. It shows the grey star image rising from its actor, flying across the native star-sky background, becoming colored, then fading back to the map. It plays WAV0149 and BGM0017 and restores the prior track. Crucially, AC20:6 remains held until the sequence finishes, so dialogue and rewards cannot overtake it. Cancellation and logout clear the presentation without a late acknowledgement.

The original executable SHA-256 is `ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`. Its image-name pointer table resolves `Star_<id>`; each image has grey/colored vertical halves. Disassembly verifies 10 pixels per 60 ms for ascent, an x87 speed of 0.2 pixels/ms for flight, five 300 ms hold intervals and 20 ms sky scrolling. The fade uses the client's established 30 ms frame clock. The decompile omits the x87 arithmetic, so `var/quest-research/star-motion-disassembly.txt` records the independently decoded instructions. Particle-trail and complete Star System panel parity remain separate visual acceptance work.

### Native quest reinforcement queues

Every mapped quest battle action has an installed formation and all referenced enemy definitions exist. The 551 actions reference 287 distinct formations. Ninety-five formations repeat an occupant slot among the first eight records; the old flat list could spawn those reinforcements immediately. The native three-byte member records now retain their occupant slot, and repeated occupants queue behind the same initial enemy. Replacements enter through the existing round-end reinforcement path, keeping the defeated occupant's EXP/loot history.

Screen positions deliberately retain the existing first-seen ordering. The native queue IDs are not assumed to be screen coordinates; their exact coordinate mapping still needs reference validation. Encounters with no EVE slot use the named sequential compatibility behavior. Independent raw-record tests and the full battle-package race tests cover this change.

### Player reactions

EVE player-expression mode 1 now sends the existing native AC32:1 expression packet, including to its owner, before the ordinary event frame. These temporary reactions do not replace a held pose or get replayed as persistent state. The mapped scripts use operands 1–17, corresponding to the existing E101–E117 expression strips; unresolved modes retain the ordinary event-frame behavior.

## Database upgrades and preservation

Use the [current offline migration](ASSET_DATABASE.md) with the configured asset
and gameplay databases. The former translation/quest branch split and its
v28–v38 deployment sequence are historical. They do not require intermediate
branch checkouts or restoring old development backups.

Versioned migrations preserve existing same-ID administrator policies, intentionally
empty collections and owned quest progress. Later batches add their own defaults
without replaying policies deliberately removed after an earlier batch. Native
EVE programs, source evidence and independent raw-byte golden tests remain intact.
Keep the complete backup printed by the migrator until acceptance finishes.

## Reproduce research and verification

From the repository root, with Python 3 and Go installed:

```powershell
python tools/data_export/fetch_wiki_quests.py --cache var/quest-research
go run ./tools/quest-catalog-export -assets var/assets.db -output var/quest-research/native-catalog-final.json
python tools/data_export/extract_wiki_quests.py --cache var/quest-research --native-catalog var/quest-research/native-catalog-final.json --output-dir var/quest-research/final-reference --notepad-document docs/QUEST_LIST.md
python -m unittest discover -s tools/data_export -p test_extract_wiki_quests.py
$env:WONDERLAND_TEST_ASSETS_DB = (Resolve-Path var/assets.db).Path
go test -race ./internal/server ./internal/assetsql ./internal/world -run 'TestInstalledFeteBook|TestInstalledNativePlayerEquipmentGates|TestInstalledWaterPathEscort|TestInstalledWikiQuest|TestQuestPlayerEquipment|TestQuestCompanion|TestWikiQuest|TestQuestCompletion|TestNativeQuestCompletion' -count=1 -timeout=3m
```

The fetcher uses paced public API requests and caches responses. The extractor emits candidate policies for review; it does not modify runtime SQL or automatically promote downloaded wiki prose to gameplay rules.

## Verification limits and remaining work

Additional Chinese-language source research is recorded in
[the Bahamut quest and mission review](GAMER_QUEST_RESEARCH.md), including source
inventories, native cost/reward comparisons and ceremony/skill-chain checklists.

All 350 resource completion tails, 25 mixed reward bundles and 15 escort tails were checked against both raw and prepared installed native programs. SQL-backed tests exercise Auntie's Soup, Dinner Incident, Squirrel Mission, Collection, Xaolan rejoining and Murloc join/reissue/return; synthetic tests exercise rollback and ownership failures. The expanded linked-event audit checks ordinary opcodes, scoped escort exceptions, skills, transformations, NPC presentation capabilities and battle formations. All 1,701 linked events pass that audit, and none retain an obsolete disabled reason.

These checks do not establish that every map route, team requirement, minigame, battle, client movie, dialogue translation or marriage ceremony is correct. The next verification work is full native-client walkthroughs of multi-map/team quests, reward/cost comparisons against detailed wiki articles, constellation particle/panel parity and native formation-coordinate mapping, with regression fixtures for any discrepancies. Native Fall in Love's journal ends at Cupid's ring reward; the article also describes the separate marriage ceremony, whose reservations/invitations/deadlines are not certified by this quest work. Keep unresolved operand semantics closed rather than granting rewards on guessed conditions. Track each new confirmed gap here and add a focused reproduction before expanding runtime capabilities.
