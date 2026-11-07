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

Gameplay schema v15 introduced `map_props.frame`, preserving old implicit broken
frames with default one. Current schema versions are listed in the database guide.
Use the preserving in-place upgrade in [ASSET_DATABASE.md](ASSET_DATABASE.md);
installation data is not reset.

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

## Laura exit gift

Upstream 1ff5bba corrects the 10001 → 10000 exit reward to Tent 36002 and Remote
Control 34058, with talk 30126 and completion mark 10035. Asset v14 stores the
route and reward in `MapTransitions`. The exit resolves to (1892,1655) on map
10000. Both items, the mark and the destination commit together. A full bag
retains the old scene and items; a completed mark prevents repeat grants even
after the tent is consumed or sold. Existing old-tent ownership suppresses the
compatibility fallback. No player state from the source snapshot is imported.

## Starter Beach monkey cinematic

Map 11016, actor 1 (S.Monkey, template 17162), links to event 1. Its authored
rescue branch 7 plays `12000.sty` (the monkey falling and rescue, ten lines),
then `11011.sty` (six concluding lines). Branch 10 contains the same movies
with a full-party response. The previous explicit controller replaced the
movies with sixteen dialogue packets, so neither native nor Go clients could
play the cinematic.

The rescue controller now reads its movie operations from the assets database
and sends the native AC20:1 kind-5 frames in order, waiting for AC20:6 after
each movie in playback mode one. Full-party attempts play only the first
movie before their explanation. It then commits the companion and marks 12002/12003 together.
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
prefer their roster nickname. Verification is recorded in [TESTING.md](TESTING.md).

Noa's introduction (movie 26005) names `JRole_5_80` directly, without a `JRP`
prefix. The native movie loader distinguishes map backgrounds from registered
illustrations; both direct image names and prefixed names now use the same JPEG
lookup and empty world view. A rendering regression checks the 832×640 Noa
illustration against the displayed scene so a fallback to the current map fails.

Noa's Kelan puzzle (event 56) uses SQL-owned continuation policies to connect the
last answer to result evaluation, the failed-result reset to the quiz invitation,
and the successful marks to the recruitment movie and companion grant. These
handoffs retain the interaction lock for both clients. Declining the invitation
or cancelling still ends it. Native-script regressions cover a perfect attempt,
a failed attempt followed by a successful retry, a previously failed score,
and explicit decline/cancel, including mode-zero question acknowledgments.

### Native continuation ownership (Noa investigation)

The available aLogin decompilation does not choose the next server EVE branch
after a dialogue line or question. The relevant native paths are:

- `var/decompiled/aLogin_ca19ee087b60_full.c`, AC20 receive case at
  `0x002e3dc0`: subcommands 1–6 copy the server frame into the event manager
  and invoke `FUN_00304fd0`. Subcommand 7 marks a step complete; subcommand 8
  closes the dialogue, clears the active/waiting flags and releases movement.
- `FUN_00304fd0` (also `FUN_0031ee6c` in reference unit 213) interprets the
  supplied step. Question mode 0 sets both step-done and awaiting-answer;
  mode 1 sets only awaiting-answer. Neither chooses an EVE continuation.
- `FUN_00307300` (`FUN_0032119c` in reference unit 213) sends AC20:6 once the
  completed step has no outstanding dialogue, question, route or constellation
  wait. It does not end the interaction or send another actor click.
- `FUN_00307c4c`, `FUN_00307c78` and `FUN_00307ca4` send AC20:9 for Yes, No or
  a selected option and clear the awaiting-answer flag. Mode-zero questions
  subsequently also produce AC20:6. The server chooses which frame follows.

These observations establish the client/server boundary, not a general rule
for re-evaluating server EVE branches. The native Noa program has no explicit
question operation after its last-answer branches 15/16; result branches 17/19
and recruitment branch 21 are separate conditional entries. Its remaining raw
operation metadata does not supply a verified continuation rule in the traced
client paths. Do not infer an automatic loop over all eligible branches: that
can repeat rewards or reminders and prevent intentional conversation endings.

Consequently Noa's three SQL-owned continuation rules remain a scoped fallback,
not a claim that they reproduce a decoded generic EVE continuation flag. Native
operation bytes and conditions are unchanged. Replace the rules only when
server-side semantics or further reference evidence establish the general rule.

## Starter storm and beach rescue

The storm event frame precedes its AC186:12 movie trigger. AC186:9 arms completion;
premature AC20:6 requests are ignored. A run-scoped watchdog recovers a stalled
accepted movie. Beach arrival immediately stages Robinson as actor one and starts
the wake-up movie. The rescue releases posture/movement/scene gates without
starting quest 12040 or automatically opening dialogue. A manual Robinson click
starts first contact; event eight continues into its guide question within the
same interaction. Authored portal click IDs take priority over reverse geometry;
entry requests use the upstream two-cell margin. Scene-local prop and actor
state now applies to every map, with quest-defined visibility replay after combat.
