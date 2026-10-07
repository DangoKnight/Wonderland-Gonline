# World and combat reference parity

## Shared scripted props

EVE actor prop-state operations without quest mark conditions now persist their
shared frame and reset deadline in `wonderland.db.map_props`. Eligible viewers
receive AC22:1 after the SQL update. New map entrants and event resynchronization
replay the active frame, including props with linked scripts. After 60 seconds,
the world tick clears the record and publishes intact frame zero. Cooldowns
survive restart; expired rows are cleared once. Quest-conditioned props remain
per character. Private tents and hidden/foreign-map actors do not receive shared
frames. This records presentation and recovery; it does not infer new reward
probabilities or override authored event conditions. Existing transactional
resource-node grants retain their configured cooldowns.

Gameplay schema v15 adds `map_props.frame`, preserving old implicit broken frames
with default one. Asset schema remains v9. Use the preserving database-copy
procedure in [ASSET_DATABASE.md](ASSET_DATABASE.md); installation data is not reset.

## Generic NPC greeting

After visible/in-range actors fail to open an authored event, service or resource
interaction, the source generic fallback shows its native Kelan welcome dialogue
(Talk ID `0x21284c`) and the SQL Talk text, or “Hello!” for the accompanying system
line if absent. AC20:6 releases the callback and movement lock. Hidden actors and
active/disabled or condition-blocked authored events retain their existing gates;
the fallback cannot execute their rewards or bypass quest conditions.

## Clinics

Free-rest offers consider missing HP/SP on the character and every accompanying
pet. Confirmation refills all accompanying pets and the character, saves once,
and replays owner/pet progression packets before the native rest confirmation.
Reserve and hotel pets are excluded. Battle and changed-map confirmations are
ignored. Travel clears pending offers; failed saves retain the offer for retry.

## Starter packs

Configured starter items are assigned during character creation and saved with
the new character in one transaction. Login never refills the pack, even if all
starter items have been consumed or discarded. This deliberately replaces the
legacy level-one login recovery rule. Changes to the configured pack apply to
future character creations; existing inventories are preserved.

Admin mass online delivery remains an explicit SQL grant path, with an audit row for
each manual grant, recipient results, busy/full-bag isolation and pending walking
preservation. See [administration](ADMINISTRATION.md#starter-pack-delivery).

## Battle synchronization and retained rules

Battle entry replays the owner's battle-pet progression before constructing the
native battle UI and refreshes participating party rosters/vitals. Existing
owner formation and fighter HP/SP packets remain unchanged. AC53:5 submission
ACKs are broadcast to active battle participants on both sides, without reaching
map spectators. Duplicate submissions receive no second ACK. AC50:6 animation
turn notifications already broadcast through the battle steps.

PvP keeps the owner's explicit no-reward rule: no EXP, gold, loot or captures.
Private Server's attacking-victory 150 EXP/100 gold grant is deliberately omitted.
Native animation/HUD and multiplayer/load acceptance remain separate work.

## Laura exit fallback remains pending

The reference adds a gift on map 10001 → 10000 using IDs 32000, 32075 and quest
10035. The extracted WLRI catalog has no item 32000, calls 32075 Chocolate and
32001 White Rice; the actual Tent is 36002. Inspection of typed event operations
on maps 10000/10001 found no matching positive item reward. These facts do not
identify a verified replacement cutscene, remote or completion mark. No duplicate
hardcoded fallback is added. Authored EVE rewards elsewhere retain their normal
transactional execution. Verify native content/captures before implementing this
separate fallback.

## Starter Beach monkey cinematic

Map 11016, actor 1 (S.Monkey, template 17162), links to event 1. Its authored
rescue branch 7 plays `12000.sty` (the monkey falling and rescue, ten lines),
then `11011.sty` (six concluding lines). Branch 10 contains the same movies
with a full-party response. The previous explicit controller replaced the
movies with sixteen dialogue packets, so neither native nor Go clients could
play the cinematic.

The rescue controller now reads its movie operations from the assets database
and sends the native AC20:1 kind-5 frames in order, waiting for AC20:6 after
each movie. It then commits the companion and marks 12002/12003 together.
Full-party attempts explain the capacity limit without completing the quest;
cancelled movies and failed commits do not grant or hide the companion.
Definitions without movie operations retain the older dialogue sequence.
Already-completed rescues retain their short response and do not replay rewards.

For a live check, use a character without S.Monkey or completed mark 12002,
leave a free companion slot, enter Starter Beach and click the monkey near
(733, 388). Confirm both movies play, then check the recruited pet. Repeat
with a full party and cancel during the movie to verify the retry paths.
Reference video supplied by the user: https://www.youtube.com/watch?v=XpG62F146k0.
The event and movie IDs above are verified against the local WLRI exports;
the video could not be fetched by the investigation tools.

The Go movie renderer applies the authored NPC `Extra` depth offset when sorting
actors (`FUN_0033de44`): S.Monkey's +100 places it in front of the tree during
movie 12000. Movie 11011 has the illustration background `JRPJRole_5_3`, rather
than a map. `JRP` is resolved to the exported JPEG picture; the 832×640 artwork
is drawn opaque at the authored camera origin and cropped to the 800×600 game
viewport. It does not reuse the current map or its objects. Owned pet speakers
prefer their roster nickname. Focused tests cover both movies' completion,
the authored depth offset, visible illustration pixels and compiled GPU parity.
