# Testing and validation

## Full synchronized-branch pre-push validation — 2026-11-06

Validated native teams/leadership/map following, generalized roaming enemies,
Chicken Farm terrain grids, reconstructed scenery shadows, combat damage and
authored skill powers (asset schema v54), participation EXP and battle notices.
The root module ran `go test -json -race -count=1 -timeout=60m -p 1 ./...`;
the client module ran `go test -json -count=1 -timeout=60m -p 1 ./...` with
source assets, compiled packs, original-client references and GPU opt-ins enabled.
SQL integration used a separately migrated, verified assets snapshot; live
databases and account state were unchanged. Live account mutation opt-ins were
cleared. The pinned translated Item.dat census/hash uses its original reference
installation, rather than the newer installation's larger item catalog.

The initial commands exposed outdated integration fixtures. Corrections retain
literal protocol goldens while including the complete-map-snapshot offer bit,
preserve login capitalization, isolate dialogue markup from translated wording,
wait for map readiness/loading completion and asynchronous hover palettes, and
use independently calculated damage intervals for shield tests. Historical
story checks retain policy contents and precedence within competing families,
allowing unrelated later Noa/Xaolan continuations at their preserved migration
positions. No production behavior changed during these validation corrections.

The final audit combines the complete runs with explicitly recorded corrected
reruns: the entire native-assets and SQL-asset packages under `-race`, followed
by the corrected installed-story ordering test; socket login/handshake and
shield regressions under `-race`; and the affected client regressions (hover
repeated three times). Other tests retain their full-run results. All **40 root
packages** and **26 client packages** are accounted for: **2,435 passing server
test/subtest results, 2 optional skips**, and **934 passing client results,
39 optional skips**. Ten root and two client packages have no tests. There are
no unresolved failed tests, missing packages or race reports. This does not
establish new interactive live-server acceptance.

Also passed full server/client vet, server/client/migration builds, all three
10-second fuzz targets (`FuzzRead`, `FuzzEVE`, `FuzzNativeItems`), Python exporter
validation (69 tests, one optional skip), web formatting, HTML lint, Go formatting
and whitespace checks. Affected vet/format checks passed again after fixture
corrections. Asset packaging verified **74,841 files**, all reused from the
current cache (6,041.3 MiB source). The isolated SQL snapshot contains all
224 authored skill powers and preserves native coefficients.

Full logs, corrected rerun logs, package inventories, the audit script and final
`summary.json` remain under ignored `var/validation/prepush-all-20261106/`.

## Native team protocol — 2026-11-06

The two-Alogin Legacy capture and decompilation now back ID-only invitations,
Join Team targeting, formation-based membership, leader trails, public rosters,
native leave requests and the complete leadership confirmation exchange. Raw-byte
fixtures are authored independently of protocol builders. Tests cover joining
without an immediate full roster, missing/late appearances, map replacement,
unrelated teams, pet identity separation, disbanding without changing active pets,
transfer acceptance/refusal/expiry/cancellation and stale or malformed requests.

Focused server tests run under the race detector, including concurrent transfer
offer/reply/expiry, follower coordinate checkpoints, map snapshots and existing
party battle victory/flee/disconnect cases. A follower-initiated battle after
transfer keeps the new leader's captured formation cell while preserving the
original event/loot initiator. Focused client tests, affected-package vet checks,
compiled locale consistency checks and server/client builds pass. Validation
binaries are under ignored `var/validation/team-native-20261106/`.

This is development validation, not a full pre-push run. Repeat the two-client
live sequence for presentation/timing acceptance. Protocol evidence and exact
native serializer/callback references are in [TEAM_PROTOCOL.md](TEAM_PROTOCOL.md).

## Full quest-development pre-push validation — 2026-11-05

Validated native map backdrops/schema v50, client-local backdrop selection, the
connection-wide GLON hello, battle formation/weapon/AUTO fixes and area-title
presentation. The full root module ran with `go test -json -race -count=1
-timeout=60m -p 1 ./...`; the separate client module ran with `go test -json
-count=1 -timeout=60m -p 1 ./...`, including installed source assets, freshly
compiled packs and opt-in GPU checks. Used a consistent disposable copy of the
configured schema-v50 assets database; live gameplay state was not modified.
Live account creation/deletion opt-ins were cleared.

The final package/test audit covers all 38 server packages and 26 client
packages. It records 2,318 passing server test/subtest results with 2 skips,
and 827 passing client test/subtest results with 36 skips. No unresolved failed
tests, missing packages or race reports remain. Packages with no tests are
reported separately: 10 server and 2 client package skips.

The initial full run exposed two test issues. The socket-login wire golden
still expected the superseded GLCT offer; it now independently pins the combined
GLON hello and confirms a Legacy login can ignore it. The complete socket login
and connection/configuration regressions passed again under `-race`. The team
hover pixel test mixed real-time actor animations with a fixed scene clock;
both actor clocks now use the scene clock without weakening highlight or pixel
assertions. The entire client app package passed again with native/GPU options,
including that test in full package order. Other packages retain their full-run
results; the audit merges the explicitly recorded corrected reruns.

Also passed full server/client vet, server/client/migration builds, all three
10-second fuzz targets (`FuzzRead`, `FuzzEVE`, `FuzzNativeItems`), Python exporter
validation (48 tests, one optional skip), web formatting, HTML lint, Go formatting
and whitespace checks. Runtime asset packaging passed (72,855 files, 72,853
reused); an independent mounted-pack check verified 1,147 backdrop maps, 1,398
rectangles and the reference lakeside selector. No new original artwork extraction
was required. This automated run does not establish new interactive live-server
acceptance.

Logs, package inventories, final `summary.json`, the isolated assets snapshot
and build binaries remain under ignored
`var/validation/prepush-quest-20261105-connection/`.

## Native NPC services — 2026-11-04

Kelan Legacy captures and screenshots now back the client shop buy/sell,
Props Keeper storage, treatment and Pet Hotel windows. The existing spawn-point
save path remains in use. Native cart and selected-slot requests have independent
byte tests; captured incremental inventory/currency replies are replayed through
the client. Server regressions cover owned SQL state, stale inventory, pending
walking/recovery, service scope, native receipt order and hotel progression.
Hotel potential and amity fields were traced through FUN_003b377c/FUN_003b3cfc;
the earlier independent default hotel-record golden is retained.

Focused server/game tests pass with the race detector. Affected client inventory,
world, event/dialogue, trade, companion and lifecycle integrations pass. Affected
vet checks and server/client executable builds pass. Native-artwork CPU snapshots
were inspected for the service windows and treatment. Logs and validation binaries
are under ignored `var/validation/npc-services-*`; screenshots are under
`var/validation/npc-service-screens/`. This is development validation, with live
Legacy/Gonline interoperability acceptance still pending. No full pre-push battery
was run and no schema upgrade is required.

## Bank counter robustness - 2026-11-04

Built on PR #7 (`a4e294a`). Scripted bank opening belongs to its originating
event and character, and follows that event's movement release/resume in the same
completion flow. No extra resume acknowledgement is required. Trailing idle ACKs
are ignored at any latency; cancellation, replacement, teleport and character
changes cannot reopen an old bank window.

Both AC29 counter and AC45 ATM transfers now use one SQL-authoritative, owned
conditional update of only the two currency columns. Focused race tests cover
separate database connections competing for the same funds, foreign ownership,
stale session balances, injected SQL rollback, zero/insufficient/overflow amounts,
native failure completions, malformed packets and failed receipt delivery.
Buffered walking/recovery remains dirty until its normal checkpoint, which does
not revert the bank transfer. Existing independent balance goldens and database
reopen tests are retained.

Affected bank, clinic, dialogue, event, minigame, hotel, storage, and teleport
integration tests pass with the race detector. Affected-package vet and the
server executable build pass. The initial expanded trade-gate test was corrected
to allow its legitimate warning while forbidding state changes or bank receipts.
Logs and the validation binary remain outside the checkout under ignored
`var/validation/pr7-robustness-*.log` and `var/validation/pr7-bank-robustness.exe`.
This is focused development validation; no full pre-push battery or new live
client bank capture was run for these robustness changes. No schema upgrade is
required.

## Floor-item input ownership and hover — 2026-11-03

A floor-item click now cancels held-ground input, the old route/marker and any
pending NPC interaction before requesting pickup. When an active walk stops,
the client sends its current position with native AC6:1 before AC23:2; otherwise
the server could check pickup reach from the previous leg's destination. Distant
pickups own their approach and stop within reach. Arrival checks run during
simulation even when a background session is not rendered. New ground/keyboard
movement, map changes, or battle entry cancel pending pickup intent.

Hover and clicks share opaque source-pixel hit testing, selecting the frontmost
item by its native depth (and draw order on ties). Hover adds the native light
level 15 from `FUN_003d7b80` within the sorted sprite pass, preserving foliage
occlusion. A yellow outlined item name appears near the pointer as in the Legacy
reference; UI/dialogue/battle state suppresses floor hover. Transparent icon
margins continue to accept ordinary ground movement.

Regressions cover the actual left Beach Coconut interrupting a held walk,
independent raw stop/pickup packet order, background approach, cancellation,
transparent margins, overlapping items, additive highlight and yellow text.
Compiled-bundle GPU parity asserts that its hover fixture is selected and checks
the highlighted item and label, in addition to both trees and the pickup arc.

## Ground-item centre anchor — 2026-11-03

Legacy `FUN_003d78e4` stores half the item canvas dimensions at +0x110/+0x111;
`FUN_003d7b80` subtracts those halves on both axes for drawing. Gonline instead
subtracted a full image height, placing a normal 32-pixel icon 16 pixels too high.
Ground items and world acquisition flights now use the native centre anchor;
battle reward flights retain their separate presentation. Click bounds use the
same centred canvas, including its lower half. Tests pin the native 32-by-32
bounds and lower-half pickup request, and exercise the frame-loop leaf overlap
at the actual right Coconut coordinates (1454,871).

The real-server capture `coconut-reference-20261103-01.pcapng` independently
confirmed the complete native AC23:4 packet: right Coconut (1454,871), depth
+160; left Coconut (462,538), depth +170. These exactly match the corrected
server packets and installed asset definitions. The frame-loop regressions now
replay that full captured packet at both reference player positions and pin the
native centred 32-by-32 canvases. GPU parity covers both trees and the pickup
flight. No further server depth adjustment or per-item foliage mask was added.
The capture completed and restored the original packet filters. Its SHA-256 is
`BF20812887F999DD0DEDD1A759C00060DE792C25C03D17D431C3643E16E7321A`.

## Native ground-item draw depth — 2026-11-03

AC23:4 ends each 15-byte ground record with a signed 32-bit draw-depth offset.
The Legacy receive path `FUN_003dc558` passes it to `FUN_003d78e4`, which stores
it at +0x114; `FUN_0041d13c` sorts the item by Y + that offset. Gonline previously
sent zero and discarded the received field. The raw final EVE item words now
supply this offset without changing the asset table representation. Beach's
right Coconut is (1454,871), depth +160; its left Coconut is (462,538), depth
+170. Initial map packets and respawn packets preserve these values, while
player drops retain zero. This repairs the server packet for Legacy clients
and the Gonline client's rendering through the same native protocol.

These EVE words are no longer interpreted as respawn seconds. Native ground
items use the existing 120-second fallback. No asset rows or schema change.
Independent packet goldens cover the two coconuts and respawn; asset/client
tests cover signed offsets. The client regression renders the left Coconut at
the reference player position (562,635), while the zero-depth dropped-item test
still checks native foliage occlusion. GPU parity includes the native +170
Coconut and its pickup flight. Focused server pickup tests also retain inventory
capacity, persistence and source-removal checks.

## Coconut canopy and native harvesting presentation — 2026-11-03

The reported Coconut is a floor item on Beach (map 11016). Floor items used to
render after the entire world, bypassing the depth pass used by the player and
monkey. They now enter `World.DrawWithSprites` at their ground Y, before actors
on equal rows, with native scene objects covering them at their authored depth
lines. This applies to every floor item. The earlier composite-palm override
and its hand-authored mask metadata were removed; no source artwork or native
depth values are changed.

The regression uses the actual Beach foreground leaf picture 11016004 and a
native Coconut ground-spawn packet. Every overlapping opaque leaf pixel must
remain unchanged while the uncovered fruit stays visible. Generic scene tests
cover sprites behind, between, tied with and in front of native objects,
including a transparent opening. The bundled GPU fixture covers this same
Beach ground item and a mid-flight pickup. A successful own AC23:2 receipt now
starts the pickup arc; another player's removal does not. AC23:6 supplies the
acquisition notice independently.

Harvesting commits its existing transactional shared-node claim before sending
native slotless AC23:6, with its final actor-flight byte enabled. This allows the
Legacy client to present the reward too. Gonline displays the native acquisition
notice for 1,300 ms and flies the item icon along a 600 ms parabola from the last
clicked actor to the player. The flight survives event resume, expires normally,
and is cleared on map changes. Rejected inventory receipts do not present gains.
The arc duration/height are Gonline presentation choices; the native reference
is `FUN_003dbdb8` → `FUN_00308e70` in the local aLogin decompile.

Focused harvesting tests include an independent raw AC23:6 golden and verify
that only the harvester receives the acquisition. Client tests cover native ground-item layering,
inventory credit, notification, arc endpoints/apex, expiry and malformed grants.
Compiled-bundle CPU/GPU parity includes the beach canopy and a mid-flight pickup.

## Notification close continuity and persistent rescue presentation — 2026-11-03

Timed notices queued their closing image after the ordinary UI closing pass,
leaving one blank frame before it reappeared. Closing overlays now render in
the original foreground pass after notice updates, including the expiry frame.
Talk shares this ordering. Closing uses integer translation instead of scaling
the captured image, preserving authored checkerboard/dither pixels and text.
Tests check uninterrupted notification text across expiry and exact mask pixels
during closing. UI/HUD and affected app notice/dialogue/battle-result tests,
compiled-asset GPU parity and vet passed; the client candidate was rebuilt.

Arrival cinematics checked the native Robinson quest mark to avoid replay, but
finishing the introductory rescue intentionally does not create that quest.
Gameplay schema v19 now stores independent owned receipts in
`character_cinematics`, keyed by character and authored cinematic ID. Completion
commits before finish cues; a failed write retains the run for retry. Login
consults the receipt without modifying quests, inventory or buffered position.
Interrupted presentations can replay; explicit new shipwreck transitions still
play. Existing quest progress also continues to suppress the introduction.

Focused store/server tests passed under `-race`: preserving v18 upgrade,
ownership, idempotence, restart, canceled writes, rescue completion/reconnect
and save-failure retry. Related older migration checks passed. Server vet and
`bin/wonderland-server-next.exe` build passed. No running server was restarted
or live database changed; the preserving migration runs at the next startup.

## Player blink artwork and dialogue voice stalls — 2026-11-03

The user confirmed that art disappeared while the same line remained open.
Real player-art frames reproduced the cause: blink frame 1 contains only the
eyelid patch (about 1,500–4,500 opaque pixels), while frame 0 contains the full
character (about 23,000–45,000 pixels). Replacing frame 0 with frame 1 briefly
removed the body. Human face rendering now retains the base and overlays the
blink patch; direct explicit-frame drawing remains available for other uses.
Coverage checks all 15 installed player appearances, plus a timed nine-second
blink during the captured Breillat player line. Visually checked
`var/player-art-blink-fixed.png`. The CPU/GPU parity fixture also forces that
blink while keeping the same line open.

Cold decoding of the installed Breillat recordings 30640/30642/30643 measured
approximately 220/255/324 ms locally. `PlayVoice` performed this work on the UI
thread, and PCM decoding held the shared cache mutex throughout. Recorded
dialogue now uses one background worker per session and one pending latest-line
slot. Dismissal/replacement/stop invalidates stale playback; shutdown stops the
worker. The cache releases its mutex before file access/decoding, keeping cached
effects responsive. No protocol or authored dialogue delays were changed.

Deterministic blocked-decoder regressions verify nonblocking requests and cached
effects, latest-only replacement, stop cancellation while the session remains
open, and shutdown during decoding. Those audio/context tests passed under
`-race`. Full role/HUD suites, focused app dialogue/Breillat/session integrations,
compiled-asset CPU/GPU parity, vet and the candidate build passed. The temporary
decode-timing diagnostic was removed after measurement.

## Continuous map-edge clamping and brief complete-message closing — 2026-11-03

The camera used the unpadded map edge to decide when to clamp, then subtracted
the right-side 20-pixel margin only after crossing it. This let the view travel
20 pixels beyond its final position and reverse at the boundary. Camera axes
now clamp continuously to their final limits, including narrow maps; explicit
cinematic camera positions remain unchanged. Regression coverage walks one
pixel at a time through the boundaries on normal, near-screen and small maps,
checking monotonic movement, bounded steps and no overshoot.

Messages now close over 50 ms using a complete transparent image of panel and
contents, avoiding the former empty-border interlude. Logical dismissal, answers
and timeouts still happen immediately; closing images cannot receive input.
A following line/reopened form cancels its preceding image. The same path
covers Talk, timed notices and animated message forms, including battle results
and confirmations. Compatible render targets avoid GPU readbacks; images are
released on expiry, replacement, HideAll and session shutdown, with a bounded
queue. Logout and pet preview drawing follows the active render target so it
is included in the image.

The world/UI/HUD/Team/Inventory suites and affected app dialogue, battle-result,
logout, inventory, connection and pet-confirmation tests passed. Closing tests
verify both panel and text remain during the brief transition, complete removal
at 50 ms and cancellation by replacement. The compiled-asset CPU/GPU parity
fixture passed. Modified-package vet and the client build passed.

## Shared dialogue and notice presentation — 2026-11-03

Moved the easing, 100 ms opening duration and eight-pixel inset into
`seui/message.go`, shared with the Talk window. Applied the same presentation
to battle results, system notices, connection messages, settings choices,
logout, party/pet confirmations and inventory quantity/use prompts. Only the
decoration animates; text, child controls and logical hit boxes stay available
from the first frame. Message forms opt in explicitly, preserving ordinary
module-window behavior. Expiry/dismissal removes the whole message without an
empty closing panel. Existing timeout, scrolling and acknowledgement rules
remain unchanged; battle results still expire after six seconds and ignore clicks.

Coverage includes monotonic bounded geometry at irregular frame intervals,
stable layout, repeat-show/reopen behavior, immediately available battle/notice
content and no frame after expiry. Existing battle click/scroll/deadline,
dialogue, settings, connection, logout, party/pet and inventory tests exercise
the integrations. The UI/HUD/Team/Inventory suites passed. Visually checked
`var/battle-results-message-animation.png`, including the initial opening frame.
The affected app integration selection passed except two static-image checks
that compared an opening panel against settled artwork. Those checks now wait
for the shared opening duration before verifying the native quantity title and
fixed logout pose; their focused rerun passed alongside both new message
integrations. Vet for all modified packages, client build and diff checks passed.

## Empty panel after Breillat's player line — 2026-11-03

Replayed captured player talk 30641 ("Why do yo say that?") followed by
Breillat's talk 30642 ("Since you want to talk with me, I will tell you what
I think."). The trace has no empty speech step. The client itself created the
blank interval: `Hide` removed content immediately, while `Draw` still painted
the decorative panel through the 80 ms closing transition.

The regression reproduced the empty player panel before the fix. Dismissed
dialogue now removes panel and content together and stops hit testing. Hidden
closing geometry remains available for a smooth next-line takeover. The raw
packet test verifies one acknowledgement, no visible waiting panel and the
next Breillat line; HUD tests also assert that a dismissed panel never paints.
The HUD suite, focused dialogue/Breillat/movie integrations, vet and rebuilt
`bin/wonderland-client-next.exe` passed.

## Responsive Gonline dialogue transitions — 2026-11-03

At the user's request, dialogue presentation now improves on Legacy rather than
reproducing its stepped expansion and collapse. Text appears immediately; the
decorative panel opens over 100 ms and closes over 80 ms with easing and an
eight-pixel inset on each edge. Both dimensions move together, based on elapsed
time. The follow-up fix above hides the closing panel with its content rather
than painting an empty frame. Closing starts from the current size, repeated closes do not extend it,
and a following line can reverse a close without resetting to a collapsed box.
Panel visibility and hit testing catch up even when rendering was skipped.
Event acknowledgements, answer values, rewards and protocol timing are unchanged.

HUD tests cover monotonic bounded dimensions, the close deadline, repeated
server closes, interruption, skipped frames and immediate text for plain, face
and wide dialogues. The full HUD suite and focused Breillat, NPC/player dialogue,
questions, dialogue layering, conversation, pet notices and beach/monkey movie
tests passed. HUD/app vet and the rebuilt `bin/wonderland-client-next.exe` passed.

## Breillat visibility while turning to speak — 2026-11-03

Reproduced the intermittent disappearance with the installed Cabin Breillat
sprite (template 10000): talk while standing directly above, below, left or
right selects empty standing actions 8, 12, 10 or 14. Diagonal facings have
artwork, explaining why the symptom depends on approach position. Event end
restored the original action 11 and made her visible again.

Native `FUN_00307ae8` turns conversation actors through `FUN_00432638`.
`FUN_002fe570` resolves packed frame offsets by summing preceding action counts;
an empty slot consequently addresses the next stored artwork. Conversation
turning now selects the next available artwork within the standing group and
preserves the original pose if none follows. This is based on sprite availability,
without a Breillat-specific exception or a change to server-driven visibility.

The real-asset regression failed for all four cardinal approach directions
before the fix. It now checks nonempty bounds and rendered pixels in all eight
directions, plus facing restoration. It also passes against the compiled
`bin/client-assets.zip`. Unit coverage checks sparse facings and missing-artwork
fallbacks alongside the existing full-direction, prop and special-pose rules.
The world suite, related app dialogue/Breillat tests, world/app vet and updated
`bin/wonderland-client-next.exe` build passed.

## Breillat's immediate model switch — 2026-11-03

Captured the legacy client on Gonline through a TCP relay, including two
successful minigames, two voucher grants, the two-voucher exchange and the
accepted Breillat conversion. The final dialogue acknowledgement was followed
by `5 12 50 209 68 0 13`, an equipment snapshot, AC20:10, release and AC20:8.
There is no self AC3 reload or AC12 warp. The native handler `FUN_00451110`
uses `FUN_00484fcc` to map model 13 to body 4/head 3 and updates the player
and cached teammate appearance in place. The model code is not a body type.

The custom client had no model-transform handler, and its compatibility
skill-grade handler consumed every AC5:12 packet before general dispatch.
It now distinguishes the five-byte skill-grade layout from the seven-byte
native model layout, applies the native model table and immediately redresses
the player/portrait, inventory preview, peers and cached teammates. The
following equipment snapshot supplies the converted outfit without resetting
the map, vitals or event state. Unknown models and truncated packets do not
mutate appearance. No server exception or wire change was introduced.

The captured model packet failed the regression before the fix. Tests now
replay the raw model/equipment/end sequence, verify self and peer/cache updates,
preserve compact skill-grade updates, and compare rendered body/portrait pixels
with body 4/head 3. All native model mappings and invalid-byte values are checked
against an independent table. Focused model, skill UI and skill-state tests,
app/login vet and the candidate client build passed. The focused native-model
and app model-switch tests also passed under the race detector (43 seconds).
The original Windows
loopback capture contained no packets; only the verified relay trace is evidence
for this sequence. Both relay listeners were stopped after logout, and the
legacy server list was restored byte-for-byte from its backup.

## Breillat acknowledgement after an arcade result — 2026-11-03

The user confirmed that the failing interaction followed an arcade/minigame in
the same login. The fresh-login relay capture succeeded: one AC20:6 advanced
Breillat's line, followed by voucher removal, Chocolate Ice delivery and AC20:8.
That trace establishes the working exchange, not a capture of the failed case.
Both temporary relay listeners were stopped after logout; the original server
list was not changed.

A deterministic client regression reproduced the remaining failure. Sending a
minigame result queued its acknowledgement in `event.done`; an incoming event
end or next dialogue cleared it before the next frame. The server then consumed
Breillat's first acknowledgement as the outstanding minigame-result acknowledgement,
leaving the voucher exchange waiting. The result acknowledgement now lives outside
the replaceable event state and is sent before ordinary dialogue acknowledgements.
Disconnect and return-to-login clear it, preventing reuse in another session.

The independent raw-packet regression covers both immediate event-end and
next-dialogue replies, subsequent NPC advancement and absence of duplicate ACKs.
Validation uses the focused Breillat/arcade regressions and server minigame event
contract tests under the race detector, related app dialogue/minigame flows,
the minigame package, logout reset tests, app/login vet and a rebuilt
`bin/wonderland-client-next.exe`. An initial broader app race selection exceeded
its three-minute timeout while decoding graphics fixtures, without an assertion
failure; race coverage was narrowed to the affected handoff regressions, and the
broader affected flow selection was run without the race detector.
All final checks passed: focused app race regressions (104 seconds), server
minigame contract race tests (40 seconds), related client flows (23 seconds),
logout reset race test (30 seconds), minigame package, vet, build and diff checks.

## Player dialogue with native trailing flags — 2026-11-03

The installed used-arcade branch sends the player speech bytes
`20 1 0 0 0 1 1 7 0 0 1 0 0 0 0 156 119 1`. The native client reads talk ID
30620 as UInt16 (`FUN_00304fd0`, field +0x711c) for both text and voice lookup.
Gonline previously combined the trailing flag with the ID and looked up 96156,
rendering the portrait and empty window. The exact packet reproduced the empty
line before the fix. Native talk IDs now take precedence; extended export
references remain a fallback only when there is no native ID entry.

Focused HUD/app dialogue, native-ID precedence, extended-reference fallback,
question, conversation, event and recruitment tests passed. The used-arcade test
checks the brown-haired player variant, exact English line and rendered yellow
text pixels. The real compiled-bundle CPU/GPU parity fixture now includes this
speech packet and large portrait; all 69 stages passed. App/HUD vet and the client
candidate build passed. Visually checked ignored `var/player-dialogue-fixed.png`.
No server protocol or artwork/layout changes were required.

## Breillat dialogue acknowledgement after a pet notice — 2026-11-03

Replayed the installed Cabin voucher exchange to establish its native speech,
inventory debit/grant and event-close packets. A client regression reproduced
the reported symptom: after AC20:8 hid a pet-join notice, the retained local-notice
flag swallowed the next NPC dialogue's click, sending no AC20:6. The server
therefore waited before committing the exchange, leaving movement locked.

Server event frames now supersede local pet notices, and event completion clears
their display flag. The independent raw-packet regression verifies ordinary,
replaced and hidden-notice cases, one acknowledgement, voucher removal, reward
delivery and movement/event release. It passes against editable assets and the
real compiled bundle. Related NPC dialogue, question, conversation-facing,
event-route, pet-recruitment and beach/monkey movie tests passed; app vet and
`bin/wonderland-client-next.exe` build passed. The server protocol and exchange
transaction behavior are unchanged. No running client/server was restarted.

## Full pre-push validation — 2026-11-03

For `0aa6f0f` (shared combat formulas and rapid player/pet targeting), both full
module suites ran to completion with cache bypassed. The server command was
`go test -race -count=1 -timeout=60m -json ./...`, with
`WONDERLAND_TEST_ASSETS_DB` pointing to the installed `var/assets.db`. It recorded
1,972 passing test/subtest entries, two failing expectations and 15 opt-in skips.
The client command was `go test -count=1 -timeout=60m -json ./...`; it recorded
627 passing entries, one failing expectation and 47 opt-in skips. No race warning,
panic or build failure was reported. Logs are retained locally under ignored
`var/validation/prepush-0aa6f0f/`.

The three outdated expectations were corrected and verified separately; no
production code changed during validation:

- The maintained item export contains 7,325 records. Its source SHA-256 matches
  the updated translated client's `Item.dat`; the previous assertion pinned
  7,312. The full `internal/assets` package passed again under `-race`.
- The multi-slot pickup golden still expected the old slot-based grant. It now
  independently asserts raw native floor-removal bytes followed by the slotless
  acquisition bytes. All `TestMultiSlot*` server tests passed under `-race`.
- The real Roca sprite in the rebuilt assets lacks sitting action 51. The
  companion integration now also verifies the existing standing fallback when
  sitting is unsupported, independent pet facing during owner turns, available
  lying animation and recovery packets. Its focused rerun passed. Eight-direction
  pose behavior remains covered by the world tests in the completed client suite.

Full root/client vet, tracked-source gofmt, Prettier and HTMLHint passed.
Ten-second protocol and EVE fuzz runs passed (about 6.8 million and 3.3 million
executions). Both executable candidates built. The asset compiler validated
72,854 files, all reused, and the real compiled-bundle login/shared-world smoke
test passed separately. GPU, live-account, original-reference and snapshot-only
checks remain opt-in; their skips are not gameplay acceptance evidence. No
running server/client process was restarted or database rebuilt.

## Shared player, pet and enemy stat formulas — 2026-11-03

Affected checks passed for `internal/game`, `internal/battle` and
`internal/world`, plus focused server combat, encounter, capture, pet,
progression, reward, GM and round-chain tests. Independent golden values cover
all four elements and Floating's level-four `194/105` HP/SP and `10/12/8/10/12`
combat stats. Battle construction checks identical attributes across players,
pets and enemies, catalog-supplied pet elements/enemy attributes, explicit
scripted HP/attack overrides and reinforcement SP packets. Existing native
golden packet expectations remain independent raw bytes.

Client inventory tests and the affected `TestBattle*`, `TestGonlineCombat*`,
pet and inventory selection tests passed. The inventory regression checks the
catalog Wind element and Floating's shared stats. Vet passed for
`./internal/game ./internal/battle ./internal/server ./internal/world` and, from
`client/`, `./wlo/inventory ./wlo/app`. Both `bin/wonderland-next.exe` and
`bin/wonderland-client-next.exe` built successfully. No database migration or
running-process restart was performed. See [combat stats](COMBAT_STATS.md).

## Participation base EXP rate — 2026-11-03

Participation mode now uses eight EXP per monster level, including eight for a
level-one monster. Focused engine and server settlement tests verify the new
base, unchanged simple-mode rate, combo bonus, inverse shares and battle-long
revive assistance. The zero-level fallback treats malformed/unspecified levels
as level one for participation mode. Vet and the server build pass.

## Combo EXP pool bonus — 2026-11-03

Participation EXP and server settlement checks pass with +40% for two distinct
landed combo units, +50% for three, and ten additional percentage points per
extra attacking unit. Tests cover solo/no bonus, support-only assistants,
duplicate actors/supporters, two through eight combo units, fractional pool
rounding and conserved inverse-level payouts. Updated server victory/escape/
defeat/disconnect checks pass with the new shares. Simple EXP remains unchanged.

## Battle-long revive assistance — 2026-11-03

Focused participation, pet-switch and server settlement checks pass. Successful
revives remain eligible through repeated kills and attacks; repeat revives
deduplicate the caster, living-target no-ops do not qualify, returning pets keep
their own revival history, replacements do not inherit it, and a fresh battle
clears revival assistance. Ordinary healing retains next-attack lifetime.

## Participation EXP — 2026-11-03

Complete battle/game/config suites pass. Tests cover normalized inverse weights,
exact pool conservation, deterministic remainders, the 200-base/three-participant
example, solo EXP, killing versus nonlethal strikes, actual buff/heal/cast
integration, active/expired/removed buffs, spent/no-op healing and deduplicated
assistance. Race-enabled server checks pass for victory-only awards, separate
player/pet shares, switched-out pets, defeat/escape/disconnect forfeiture, startup
policy capture and existing native/chain combat integrations. Changed-package
vet and `bin/wonderland-next.exe` build pass. No processes were restarted.

## Battle gold startup policy — 2026-11-03

Configuration tests cover omitted/enabled/disabled `battle_gold_enabled` and
invalid types. Server integration verifies that enabled/disabled victory gold
balances persist, EXP remains awarded, and mutating exported configuration after
startup does not alter the captured policy. Quest-victory and native/chain round
integration checks pass. Config/server vet and the updated server build pass.
The default is enabled; changing the startup switch requires restart.

## Immediate acquisitions and legacy inventory — 2026-11-03

Passed complete affected battle/game/protocol and client inventory/battle suites.
Focused server/store checks under the race detector cover multi-owner loot,
second-recipient SQL rollback, deduplication, immediate capture persistence,
disconnect retention, capture net use, native/chain delivery, pickup/drop order
and amity-only knockout penalties. Client app checks cover the independent live
AC23:6 bytes, native/chain acquisition and item flights, floor spawn/pickup,
destruction marker 7, recruitment notices and result accounting. Updated the
existing ground-item packet expectations to the captured order, keeping raw
expected protocol bytes independent of production constants. Changed-package vet
and both candidate builds pass. Visually checked `var/battle-acquisition.png`.
No running server/client was restarted, and no database migration is added here.

## Wiki capture rules — 2026-11-03

Battle and game suites pass, including capture level boundaries, net allowance,
weakening, seals, native restriction flags, stat budgets and story-pet isolation.
The full assetsql suite passes, including preserving v16-to-v17 migration and
repeat-upgrade edits. Server capture/settlement/native/chain integrations pass;
capture-net checks cover success, failure, final use and SQL rollback. Asset
checks pass except `TestMaintainedItemJSONCensus`, whose 7,312-item expectation
predates the recent asset rebuild; this unrelated expectation is unchanged.
Changed-package vet and the server build pass. The running server and databases
were not restarted or migrated; use the documented offline migration before
deploying `bin/wonderland-next.exe`.

## Multi-hit damage number flight — 2026-11-03

Focused client damage/combo tests pass in native and round-chain modes. Tests
verify alternating, widening, side-mirrored multi-hit lanes, contact-aged arcs,
unchanged single-hit motion, damage totals and stale-target rejection. Rendered
and checked `var/battle-damage-fan.png`. Client app vet and the build of
`bin/wonderland-client-next.exe` pass. The arc approximates the legacy curve;
no server calculation or protocol changes were made.

## Battle skill selector and captured pet level — 2026-11-03

Passed the complete client `wlo/skills` suite and focused app checks for radial
skill selection, recent skills, one-click targeting, general Skills browsing,
pet commands, timer handling and battle exit. The native battle selector was
rendered and visually checked at `var/battle-skills-window.png`. Its standalone
checks cover learned-skill filtering, scrolling, recent-skill capacity and
ordering, actor isolation and login reset. Focused server checks verify captured
monsters recruit and persist at level one with fresh HP/SP and matching client
progression packets, plus save-failure atomicity and battle completion. Vet
passed for client app/skills and server. Built `bin/wonderland-client-next.exe`
and `bin/wonderland-next.exe`; running processes were not restarted.

## Monster elemental targeting — 2026-11-03

Passed `go test ./internal/battle -count=1`, `go vet ./internal/battle`, and
focused server integrations for `TestGonlineCombat*`, wild-monster victory,
world-entry victory persistence and native/chain SP restoration. Deterministic
ticket enumeration verifies all four elemental relationships, exact 4:2:1
selection weights, equal player/pet chances with reversed roster order,
basic/skill/debuff selection, and exclusion of unavailable replacement pets.
Existing support AI and independent packet goldens remain passing. No protocol,
database or client asset changes are needed for this targeting policy.

## Combat support and automatic targeting — 2026-11-03

Passed the complete `internal/battle` suite under `-race`, focused asset support
and targeting tests, and server combat integration tests under `-race`.
Coverage includes native grade fallback, every automatic pattern and ordering,
sparse formations, authored overrides, PvP allegiance, pets, defeated and joining
recipients, capped SP restoration, selective/general cleansing, monster support
AI, one SP cost/proficiency use per cast, independent native result bytes, native
and chain transport, and restored SP persistence after disconnect.
Affected client battle/skill/targeting/playback checks, server-package vet,
formatting and whitespace checks also passed. Built `bin/wonderland-next.exe`
from `./cmd/wonderland`; the running server was not restarted.

```powershell
go test -race ./internal/battle -count=1 -timeout=4m
go test ./internal/assets -run '^Test(Skill|NativeSupport|Native.*Effect)' -count=1 -timeout=2m
go test -race ./internal/server -run '^Test(CombatMana|BattleItem|GonlineCombat|BattlePet|TeamBattle|NativePvP|PvPDisconnect|BattleDisconnect)' -count=1 -timeout=5m
go vet ./internal/assets ./internal/battle ./internal/server
Push-Location client
go test ./wlo/battle ./wlo/skills ./wlo/app -run 'Test.*(Target|Mana|Skill|Round|Combat|Result)' -count=1 -timeout=4m
Pop-Location
```

Automatic shapes and support rules follow Private Server `0cb6915`; a live
official-client animation comparison remains pending. No new SQL schema or
asset rebuild is required.

This is the maintained guide for automated checks, test data, graphics/snapshots,
benchmarks, native-client acceptance and recorded testing findings. Commands run
from the repository root unless they explicitly enter `client/`. Feature guides
retain implementation/source evidence and link here for execution and outcomes.
Asset import/build integrity checks stay with their operational workflows in
[asset import](ASSET_IMPORT.md) and [client architecture](CLIENT_ARCHITECTURE.md).
Machine-readable historical evidence remains in [source-inventory.json](source-inventory.json).

- [Test selection and correctness](#test-selection-and-correctness)
- [Environment requirements](#environment-requirements)
- [Development checks](#development-checks)
- [Pre-push server validation](#pre-push-server-validation)
- [Test data and opt-ins](#test-data-and-opt-ins)
- [Subsystem checks](#subsystem-checks)
- [Graphics, snapshots and benchmarks](#gpu-parity-and-snapshots)
- [Testing findings from the documentation audit](#findings-from-the-2026-11-01-audit)
- [Native-client acceptance and recorded outcomes](#native-client-acceptance)
- [Historical validation reports](#historical-validation-reports)
- [Benchmarks](#benchmarks)
- [Resource exhaustion review](#resource-exhaustion-review--2026-11-01)

## Test selection and correctness

During development, run only affected modules and integrations. Use `-run` within
large packages such as `internal/server` and `client/wlo/app`, including before
commits. Run full server validation only before a push. Use `-race` for changed
concurrency, shared caches, session ownership or resource lifetimes. Include relevant native-data
compatibility checks for protocol/event changes. Confirm that a selection
actually runs tests: `[no tests to run]` does not validate that package.

Keep independent raw-byte expected packets in golden tests. Arbitrary fixture
IDs may stay numeric; preserve authored data tables and source/reference hashes.
Review constant meanings and namespaces as well as wire values. Do not rewrite
expected bytes to mirror production constants. See [development conventions](DEVELOPMENT.md).

Exercise transaction rollback, stale ownership, full containers, replay and
post-commit receipt failures for resource changes. Buffered-state changes also
need walking followed by purchase/trade/warp, stale snapshots, deletion,
reconnect, failed checkpoint retry and concurrent-session coverage. Test dirty
field checkpoints without rewriting inventory/pet/quest children.

A fixture or synthetic protocol test does not establish native-client acceptance.
Record manual steps PASS, FAIL, BLOCKED or NOT TESTED only from observed outcomes;
preparation starts NOT RUN. A timeout is an incomplete run, not a pass. Keep
historical results labeled with their scope and source revision.

## Environment requirements

The root module is the server; `client/` is a separate Go module. Root test/vet
commands never include the client. Build both with Go 1.25 or newer; server
SQLite checks require CGO and a working C compiler. Use the graphics libraries
and display requirements in [client setup](CLIENT.md#run).

Client/UI tests need a usable desktop/display even when they render CPU samples;
real GPU parity also requires an OpenGL context. Local login/world integration
tests open loopback TCP listeners. Sandboxes must permit those listeners; do not
classify `socket: operation not permitted` as a protocol regression.

Keep credentials for opt-in live-server tests separate from regular accounts.
Creation/deletion tests mutate slot 1. Prefer isolated `cmd/live-test` databases.
No manual acceptance or asset tests should reset the regular gameplay store.

## Development checks

Select a relevant recipe under [subsystem checks](#subsystem-checks), for example:

```sh
go test -race ./internal/game ./internal/server -run '^Test(Potential|Compounding|Trade|Resting)' -count=1
```

For client interaction work, run from `client/`:

```sh
go test ./wlo/app ./wlo/world -run '^Test(Battle|Hotbar|Compound|Route|ActorWalk|Settings)' -count=1
```

Use `-count=1` when fresh execution is needed. Narrow packages/selections further
for small changes. Source-only runs use repository exports; compiled paths and
other opt-ins are listed below.

Server packet builders and their pure golden tests live in
`internal/server/wire`; bounded log capture and packet trace checks live in
`internal/server/logging`. Socket queue, expiry and close tests live in
`internal/server/transport`. Include the affected component when changing it:

```sh
go test -race ./internal/server/logging ./internal/server/wire ./internal/server/transport -count=1
```

Session/persistence integration tests remain in `internal/server`, grouped by
feature. The [component guide](../internal/server/README.md) maps handlers to
their files. Existing test names and `-run` selections remain valid.

## Pre-push server validation

Run the full server checks only before a push, from the repository root with a
current imported assets database:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race -timeout 60m ./...
go vet ./...
```

Run the separate client module as well:

```sh
cd client
go test -timeout 60m ./...
go vet ./...
```

Use client `-race` when concurrency or shared lifetimes change. Full installed
story/catalog checks can exceed Go's default ten-minute package timeout; `60m`
is an outer limit, not an expected duration or evidence of load acceptance.

PowerShell, from the repository root:

```powershell
$env:WONDERLAND_TEST_ASSETS_DB = (Resolve-Path var/assets.db).Path
go test -race -timeout 60m ./...
go vet ./...
Push-Location client
go test -timeout 60m ./...
go vet ./...
Pop-Location
```

These commands omit explicitly opt-in GPU, original-asset and live-account tests
unless their environment variables are set. Report skips separately. Normal
build verification remains `make build` / `make build-client` (or the direct
commands in [setup](GETTING_STARTED.md)); client builds validate edited assets.

### Formatting and lint

```sh
gofmt -l .
npx prettier --check "internal/admin/web/**/*.{html,css,js}"
npx htmlhint --config .htmlhintrc internal/admin/web/index.html
```

`gofmt -l` should print no files. Run `golangci-lint run ./...` when installed to
match the optional Makefile check. Node/npm is needed for web lint, not startup.
Makefile test/race/vet targets cover only the root module; their defaults do not
set optional data paths or extend the installed-suite timeout.

### Fuzzing

From the root module:

```sh
go test ./internal/protocol -run '^$' -fuzz FuzzRead -fuzztime 10s
go test ./internal/assets -run '^$' -fuzz FuzzEVE -fuzztime 10s
```

## Test data and opt-ins

Use absolute paths, especially before changing directories. Some packages
normalize relative SQL paths from the module root; others use their own test
working directory. Do not rely on a shared relative-path convention.

| Variable | Input / scope |
| --- | --- |
| `WONDERLAND_TEST_ASSETS_DB` | Current read-only imported `assets.db`; SQL integration/story/fixture checks. Current schemas are in [database upgrades](ASSET_DATABASE.md). |
| `WONDERLAND_TEST_DATA` | Original native `Data` directory for server-format tests; some `internal/clientassets` tests instead expect client `data/`. Set it for the selected package, not every suite indiscriminately. |
| `WONDERLAND_TEST_CLIENT_DATA` | Original client `data/` for independent native export/decoder checks. |
| `WONDERLAND_CLIENT_DATA` | Original WLRI installation root, including `pic/`, `data/` and `jma/`. |
| `WONDERLAND_SPRITE_CLIENT` | Original installation used by authenticated sprite checks. |
| `WONDERLAND_TEST_DATA_ROOT` | Loose exported repository `data/` for explicit client asset checks. |
| `WONDERLAND_TEST_CLIENT_BUNDLE` | Absolute compiled core `client-assets.zip`; keep its named companion packs together. |
| `WONDERLAND_TEST_GPU` | Set to `1` to opt into real GPU/context comparisons. |
| `GPU_DIFF_DIR` | Optional output directory for failing CPU/GPU comparison images. |
| `SNAPSHOT_DIR`, feature snapshot variables | Output paths for opt-in rendered samples; see [snapshots](#snapshot-outputs). |
| `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` | Explicit live-server test host/account; ports 6414 and 6416. |
| `WLO_CREATE_NAME`, `WLO_DELETE_CODE` | Opt into slot-1 creation/deletion using the live account. |

Original native assets are not bundled; opt-in checks may skip without them.
Restore ignored editable media using [asset import](ASSET_IMPORT.md). Keep
native sources separate from editable exports and compiled packs: they test
different contracts. An import/build verifier checks integrity, not gameplay.

## Subsystem checks

### Automated coverage

Tests cover golden wire bytes, fragmented TCP frames, truncated payloads, native record offsets, EVE boundaries, Big5 names, animation timing, login over a duplex connection, administrator authorization, persistent accounts, duplicate registrations, bans, transaction rollback, inventory capacity and quest reward replay. Creation tests cover name reservations, malformed requests, starter grants, map acknowledgment ordering, deletion codes, and reconnect persistence; native asset checks exercise all 60 body/head/element combinations. Peer tests cover concurrent arrival, appearance before movement, map isolation, disconnect cleanup, and failed recipients. World tests cover golden map-info bytes, PreEvent rule order and conditions, every portal lookup priority, and a warp between maps with persistence, departure, acknowledgment and cooldown. Trade tests cover the native AC25 wire (golden offer records), locked offers, metadata preservation, cancellation notices, concurrent acceptance and rollback of both characters. Native asset tests are opt-in because game assets are not bundled. Native-data path handling differs by package; prefer the absolute paths in this guide. Friendship tests cover cross-map requests, native lists and appearance fields, request expiry, capacity, concurrent acceptance, presence across map loads and deletion cleanup. Minigame tests cover start/result packets, native rabbit win/loss rewards, cancellation, replay, chained games, full bags and failed saves. Preference tests cover native settings packets, persistence, malformed commands, failed saves and team/trade request controls. Vehicle tests cover placement/boarding packets, raft wear and wrecking, mounted-slot protection, save failures, portal landing, and login after database reopen.

### Growth and progression checks

```sh
go test ./internal/game ./internal/battle ./internal/server -run 'Test(ElementalGrowth|PlayerFighterConfiguredGrowth|CompiledElementalGrowth)'
```

Potential Pills and compounding also have focused game/server transaction,
probability-boundary, progression and rollback tests; select their `TestPotential`
and `TestCompounding` prefixes as appropriate.

### Compounding checks

Existing sensitive-resource tests cover metadata, rectangular
placement, full bags, locks, ownership gates, save/receipt failure and replay;
Additional tests cover rank sums, book caps, fallback, base preservation, curve ordering,
multi-input SQL consumption, proficiency, forced-catastrophe consumption and
concurrent requests.

```sh
go test -race ./internal/game ./internal/server -run '^Test(Compounding|Compound|Alchemy)' -count=1
```

For client controls, run from the root in a separate shell/block:

```sh
cd client
go test ./wlo/app ./wlo/inventory -run '^TestCompound' -count=1
```

Manual native-client fixtures are in the [compounding acceptance batch](#compounding-acceptance-batch).

### Native-format verification

A committed artificial golden record checks every decoded byte, text reversal,
Big5, negative stats and arithmetic wrap boundaries. Native-data tests compare
all decoded records in file order against independent reference SHA-256 hashes.
No original game data is committed with the golden fixture.

From the repository root in Bash:

```sh
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
WONDERLAND_TEST_CLIENT_DATA="$PWD/../Wonderland-Client/data" \
go test -race ./internal/assets -count=1
```

In PowerShell, set the two environment variables before running the same Go
command. Server SQL integration checks use `WONDERLAND_TEST_ASSETS_DB` pointing to an
absolute path for the current imported catalog. Native-format checks above
verify the independent source decoders. Catalog decoding and protocol test clients do not establish
original game-client compatibility.

### World simulation checks

```sh
go test -race ./internal/world ./internal/assetsql -run 'Test(Terrain|WorldSimulation|WildMonster|Proximity|InstalledSQLWorldSimulationGeometry)'
go test -race ./internal/server -run 'TestWorldSimulation|TestWildMonster|TestProximity|TestWorldVisibility|TestTent|TestVehicle'
WONDERLAND_TEST_ASSETS_DB=/absolute/path/to/migrated/assets.db go test ./internal/assetsql -run TestInstalledSQLWorldSimulationGeometry
```

Coverage includes golden movement bytes, current-position snapshots, single
waypoint return, looping patrols, blocked patrol legs, signed bounds, roaming
leashes, town exclusions, static props, script pauses, monster respawn,
collision rejection before saving, visibility/instance isolation, recipient
failure isolation and SQL upgrade preservation. Installed-grid verification
checks the terrain census and Ship Deck geometry. Native-client and multiplayer
acceptance are still required; synthetic tests do not establish that acceptance.

### Teams and instance checks

Focused automated tests cover pet mode/release requests and authoritative replies,
recruitment validation, invitation acceptance/decline, teammate level versus HP,
SQL instance migration/authority, malformed packets, room capacity, minimum levels,
leadership, disconnect cleanup and unavailable starts. Server tests run under the
race detector. Rendered Teams and Instance windows are compared with the supplied
screenshots.

Manual acceptance: log in with two clients, form a party, confirm both level/HP
rows, transfer leadership, leave, then select/rest/ride/dismiss a disposable pet.
Create a room on an eligible character, join on a second character, refresh the
browser, leave and disconnect the leader. Start must display the pending message
without changing maps or granting rewards.

### Minigame checks

From `client/`:

```sh
go test ./wlo/minigame
go test ./wlo/app -run 'Test(MoleFlow|HunterFlow|AdditionalLocalMinigameFlows|NativeArcadeDispatchAndCleanup|UnportedMinigame)'
```

To render the newly implemented games using extracted WLRI assets:

```sh
ARCADE_SNAPSHOT_DIR=/tmp/wonderland-arcade \
  go test ./wlo/app -run '^TestAdditionalMinigameSnapshots$' -count=1
```

This writes separate explanation and play PNGs for each supported new kind.
It does not purchase anything. The existing `MOLE_SNAPSHOT` and `HUNTER_SNAPSHOT`
checks remain available. After adding native ports or changing their references,
regenerate the client inventory from the repository root:

```sh
python3 tools/decompile/catalog.py
```

Fruit-machine and dodgeball coverage includes `TestMarioFlow`,
`TestArcadeFruitMachines`, `TestThrowFlow` and `TestThrowStartSceneVariant`.
From `client/`, render their explanation/spin/play samples with:

```sh
MARIO_SNAPSHOT=/tmp/mario.png go test ./wlo/app -run '^TestMarioSnapshot$' -count=1
THROW_SNAPSHOT=/tmp/throw.png go test ./wlo/app -run '^TestThrowSnapshot$' -count=1
```

### Login checks

The capture/live-server results below were recorded during earlier login
bring-up. They describe historical evidence and available checks, not a rerun
as part of this documentation reorganization.

`client/wlo/app/reference_test.go` renders the empty server list, the server list with a region selected, and the account form. All three match the captures in `client/reference/screenshots/Login/` exactly.

Other tests:

- `flow_test.go` drives a connection to a fake server, checking action 0, the 1/9 reply, typing, Enter and the exact 63/4 bytes.
- `server_test.go` runs the same flow against a live server, including a rejected and an accepted login. It is opt-in: set `WLO_SERVER_HOST`, `WLO_ACCOUNT` and `WLO_PASSWORD`. It passed against this repository's server.
- `login_test.go` holds golden bytes for 63/4, plus tests for the validator, status records, reframing, SERVER.INI parsing and disconnect texts.
- `seui/editor_test.go` covers the editor's filters, limits and double-byte editing, and the combo box's ordering.

### Character presentation checks

Earlier sprite/selection bring-up recorded the following coverage and comparisons:

- `role_test.go` covers archive lookups against the installed archives, including the weapon order, and the palette rules.
- The opt-in snapshot test renders two starter characters. With body 4/head 0 and body 1/head 0, the hair, hat, clothes, shoes and portraits all line up. One character has neutral colours (444444444 twice); the other has shifted hair and skin. A second frame dresses them in Knight's Cape and Angel Wings+5 (`select_character_costumes.png`); `role_test.go` checks the patch fallback.
- `colors_test.go` covers the digit layout and the shift and clamp.

`selectchar_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders the selection screen for comparison, and the empty screen matches `select_character_empty.png` visually.

These recorded capture comparisons do not establish current native gameplay
acceptance. Use the source test's opt-in output path to reproduce them.

### Character creation checks

Earlier creation bring-up recorded these comparisons and live-account checks:

- `create_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders every step with the reference screenshots' character. Step 2 matches `create_character_stats.png` except 16 pixels in the window corners; steps 3 and 4 differ only in the body's idle frame; the password dialog sits one pixel left and one down of the capture.
- `TestCreatePacket` checks 9/1's layout.
- `TestCreateAgainstServer` (opt-in, with `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` and `WLO_CREATE_NAME`) creates a character on a live server and waits for the world-entry packets. Against this repository's server it passes, and the stored character has the chosen look, element, points, colours and starter outfit. The account needs no characters, and a password of at most 10 characters (the field's limit).

Use a disposable live account: creation requires an empty slot 1, and
`TestDeleteAgainstServer` deletes slot 1 with `WLO_DELETE_CODE`. Supply the
explicit host/account credentials from [test data and opt-ins](#test-data-and-opt-ins).

### Exporter checks

From the repository root:

```sh
python3 -m unittest discover -s tools/data_export -v
WONDERLAND_TEST_CLIENT_DATA="$PWD/../Wonderland-Client/data" \
  go test ./internal/assets ./cmd/item-export -count=1
```

### Original-client resource checks

Run only the native resource package from the repository root. Bash:

```sh
WONDERLAND_CLIENT_DATA="$PWD/../Wonderland-Client" \
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Client/data" \
go test ./internal/clientassets -count=1
```

PowerShell:

```powershell
$env:WONDERLAND_CLIENT_DATA = (Resolve-Path ../Wonderland-Client).Path
$env:WONDERLAND_TEST_DATA = (Resolve-Path ../Wonderland-Client/data).Path
go test ./internal/clientassets -count=1
```

These inputs are original resources, not the runtime SQL database or loose
exported `data/`. Use the separate [full-validation procedure](#pre-push-server-validation)
for full client suites; avoid leaving native-data variables set for unrelated
package selections.

### Inventory and recovery checks

Tests cover native envelopes, independent scenes, admission/ownership, complete
metadata, stale/full-bag rollback, ordinal indices, loading/logout/crash recovery,
reservations, and preserving gameplay v9 and asset v2 upgrades.

Use focused `TestTent`, `TestTrade`, `TestAutosave`, `TestResting` and inventory
selections in the owning game/store/server packages. Add race checks when
session ownership or checkpoint behavior changes. For buffered fields, retain
the cases in [test selection and correctness](#test-selection-and-correctness).

### Inventory checks

Potential-window checks cover its native unselected/player/pet states, button
hover artwork and Escape, sparse pet-slot mapping, canonical seated Robinson
appearance, base attributes versus potential bonuses, aggregate unlocked pill
counts, reusable attempts, duplicate-request gates and replacement pets:

```sh
cd client
go test ./wlo/inventory ./wlo/app -run 'TestPotential|TestItemUsePotential' -count=1
POTENTIAL_SNAPSHOT_DIR=/tmp/potential go test ./wlo/app -run '^TestPotentialNativeWindowAndReplies$' -count=1
```

Create the snapshot directory first. The snapshot test exports unselected,
player, pet, Confirm/Cancel hover and successful-result views. CPU/GPU parity now
covers 79 stages, including these six Potential states, with the compiled bundle.
Sitting previews use native seated frames only; roster portraits remain available
for pets whose artwork has no seated body frames.

From the repository root:

```sh
cd client
go test ./wlo/inventory ./wlo/world ./wlo/login ./wlo/seui ./wlo/app -run 'TestInventory|TestFormula|TestEditor|TestCombo|Test.*BaseStats'
INVENTORY_SNAPSHOT=/tmp/inventory.png go test ./wlo/app -run TestInventoryFlow -count=1
INVENTORY_REFERENCE_SNAPSHOT=/tmp/inventory-reference.png go test ./wlo/app -run TestInventoryOriginalSampleSnapshot -count=1
INVENTORY_INTERACTION_SNAPSHOT=/tmp/inventory-interactions go test ./wlo/app -run TestInventoryInteractionSamples -count=1
```

## Compiled assets and workspace checks

Focused tests cover image/JSON edits, transparency, failed-build atomicity,
compression/reuse, per-image invalidation, lossless page formats, native color keys,
trim/atlas offsets, viewport clipping, LRU byte budgets, ZIP64 offsets, safe mounted paths, independent session state,
shared buffers, background packet processing without redraw, sidebar input and
socket disposal. With a real bundle:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/app -run TestCompiledBundleLoginAndSharedWorld
```

For live acceptance, log in as two different accounts, switch between their
previews, and check movement/chat/Remote continue in the background. Remove one
session, verify the observer sees it disconnect, and continue using the remaining
session. Modify one same-size PNG and a JSON value, rebuild, and inspect the change;
resize that PNG and verify the build fails while the previous bundle remains usable.

## GPU parity and snapshots

Graphics tests require an available display/context:

```sh
cd client
WONDERLAND_TEST_GPU=1 go test ./wlo/render -count=1
WONDERLAND_TEST_GPU=1 \
  WONDERLAND_TEST_CLIENT_BUNDLE=/absolute/path/to/bin/client-assets.zip \
  go test ./wlo/app -run '^TestGPUClientParity$' -count=1
```

The app graphics test runs in a subprocess because Ebitengine owns
process-global graphics state and shuts down its native backend when `RunGame`
returns. This keeps graphics teardown separate from later CPU/UI tests in a full
suite; the subprocess uses the same test binary, including race instrumentation.

The renderer test compares all 65,536 native colors, all alpha levels, clipped
blits, scaling, indexed palettes, lighting, all gauge percentages, sparse compiled
canvases, loose image tiles, cache reuse/eviction and GPU-to-GPU copies against the
CPU renderer through real GPU readbacks. The app test compares entire login,
Ship Deck, inventory, skills, settings and two-session frames, collapsed previews
session removal and clipped, partially scrolled session cards. It also checks that normal composition performs no readbacks
and that removal releases the session target. `GPU_DIFF_DIR` optionally saves
CPU/GPU images when the app comparison fails. Run focused CPU regressions and
race checks on these modules when changing shared caches or lifetime behavior.


### Snapshot outputs

Output images are local comparison artifacts, not automatic acceptance reports.
Use a writable output location; opt-in tests can skip when its variable is absent.
Common selectors in `client/wlo/app` include:

| Area | Variables / selection |
| --- | --- |
| Login, creation, map, Social, mailbox, minimap, trade | `SNAPSHOT_DIR`; use the relevant snapshot test, not the entire app package. |
| Inventory and item use | `INVENTORY_SNAPSHOT`, `INVENTORY_REFERENCE_SNAPSHOT`, `INVENTORY_INTERACTION_SNAPSHOT`, `ITEM_USE_SNAPSHOT`. |
| Battle and shortcuts | `BATTLE_SNAPSHOT`, `BATTLE_HOVER_SNAPSHOT`, `HOTBAR_SNAPSHOT`. |
| Compounding and Remote | `COMPOUND_SNAPSHOT`, `REMOTE_SNAPSHOT`. |
| Minigames | `MOLE_SNAPSHOT`, `HUNTER_SNAPSHOT`, `MARIO_SNAPSHOT`, `THROW_SNAPSHOT`, `ARCADE_SNAPSHOT_DIR`. |
| Teams/instances | `TEAM_SNAPSHOT`, `TEAM_DISMISS_SNAPSHOT`, `TEAM_INVITATION_SNAPSHOT`, `TEAM_CONTROLS_SNAPSHOT`, `TEAM_RENAME_SNAPSHOT`, `INSTANCE_SNAPSHOT`. |
| Workspace | `WORKSPACE_SNAPSHOT`, `WORKSPACE_FOUR_SNAPSHOT`, `WORKSPACE_COLLAPSED_SNAPSHOT`. |
| Chat | `EMOTICON_SNAPSHOT`, `EMOTICON_PREVIEW_SNAPSHOT`, `CHAT_RESIZE_SNAPSHOT`. |

The source tests define additional outputs and exact selections. A native capture
can confirm layout without proving server rules or native packet acceptance.

## Benchmarks

The following measurements were recorded in earlier optimization work; timings
and pack sizes are machine/input dependent. They are not current acceptance or
whole-client FPS claims.

The focused first-terrain benchmark for Ship Deck measured 108–127 ms and about
154 MB of allocations when parsing the editable ground export. The first compiled
lookup measured about 6 ms and 2.7 MB, including ZIP directory initialization;
subsequent fresh record lookups measured about 0.14 ms and 50 KB. These numbers
measure terrain retrieval, not total startup, world rendering or peak resident RAM.
Reproduce with:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/world -run '^$' -bench BenchmarkFirstTerrainRecord -benchtime=1x -count=3
```

On an Intel Core i5-13600K, `BenchmarkWorldBackgroundDrawing` for Ship Deck at
camera `(700, 500)` measured **0.774 ms** for the editable-image path and
**0.030 ms** for compiled tile drawing. Both loops allocated zero bytes per draw.
This is a warm-cache, background-only comparison, not an overall FPS measurement.
Direct copies from prepared tile rows avoid a temporary viewport buffer. Run:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE="$PWD/../bin/client-assets.zip" go test ./wlo/world -run '^$' -bench BenchmarkWorldBackgroundDrawing -benchtime=1s
```


## Findings from the 2026-11-01 audit

These observations apply to commit `1a1672e` and the documentation audit that
followed it. Documentation-only reorganization does not constitute a fresh
execution of every historical test below.

| Check | Observed result and scope |
| --- | --- |
| Server development checks | Focused race tests for game/assets and server compounding, Potential Pills, GM rebirth, gacha, resting, movement and clinics passed. |
| SQL catalog checks | Focused structured round-trip/authority, named effects, gacha compatibility, quest visibility and combat-trial checks passed. |
| Client development checks | Focused app/world/settings checks passed; complete small battle/skills package checks also passed. |
| Full client run before commit | Other packages passed; `TestLoginFlow` could not bind a loopback socket inside the sandbox, then passed with socket access. This does not include all optional GPU/live tests. |
| Full root race run before commit | Incomplete: the server package reached the ten-minute limit while loading installed story data. The run also exposed relative SQL-path failures and a blocked shutdown listener. Do not describe it as a full-suite pass. |
| Corrected reruns | Absolute-path assetsql/livetest checks passed. Focused server race checks, including the interrupted Fred story case, shutdown and world-entry socket cases, passed with needed loopback access. The whole server suite was not rerun with the extended timeout. |
| Documentation audit | Local links/anchors, JSON and generator syntax, reproducible client catalogs, whitespace and preserved source hashes/status/history passed. |

### Read-only Go cache

The sandbox's default `/home/dango/.cache/go-build` was read-only. Errors included
cache write failures and misleading `testing/internal/testdeps` / `runtime/race`
"cannot find package" messages. Selecting a writable `GOCACHE` resolved the build
failures; they were not evidence that those standard packages were missing.
For example, run from the root module:

```sh
mkdir -p /tmp/wonderland-go-cache
GOCACHE=/tmp/wonderland-go-cache go test ./internal/game -run '^TestPotential' -count=1
```

Use an equivalent writable location on other systems. Do not modify the global
cache configuration merely to run an isolated check.

### Loopback permissions and timeouts

`TestLoginFlow`, the graceful shutdown test and authenticated world-entry tests
need local TCP listeners. A denied listener requires an environment with loopback
access; rerun the failed test with that access rather than masking the assertion.
Check cache permissions and data paths before diagnosing a compiler or database
regression. `var/assets.db` existed, but a relative path failed in packages whose
working directory was not the repository root; absolute paths resolved it.

The installed server suite remained CPU-active before timing out. The timeout
interrupted a story test during SQL catalog loading; that specific test passed
on a focused rerun. Extend the package timeout for the next full installed run,
and retain a clear incomplete status until it finishes. Longer limits do not
establish multiplayer load, soak, crash-loss or native-client acceptance.

## Native-client acceptance

The `live-test` command prepares database fixtures and manual acceptance
checklists for the original launcher/aLogin. It does not emulate the client,
start a server, or declare a feature passed. Accounts and character prerequisites
are persisted through the normal Go store; assets come exclusively from the
read-only SQL catalog.

### Prepare and run

From the repository root:

```sh
go run ./cmd/live-test -list
mkdir -p var/live-tests
go run ./cmd/live-test -config config.local.json \
  -scenario fishing-skilled -output var/live-tests/fishing-skilled-01
```

Use a current assets database (schema v16, including the item dimensions
introduced in v11). If testing
with an upgraded copy, add `-assets-database /absolute/path/to/assets.db`.
See [asset database upgrades](ASSET_DATABASE.md) if preparation reports a schema
or content error. No asset upgrade or gameplay reset is performed by this tool.
Config paths follow the regular server's working-directory rules; invoke the
command from the project root.

The new directory contains:

- `wonderland.db`: fresh, isolated gameplay database with a tester and observer.
- `config.json`: original listener/startup policies with absolute test database
  paths. The regular gameplay database is never opened or copied.
- `instructions.md`: randomized 10-character usernames/passwords (within the original client's
  8–10-character range), deletion
  codes, selected characters, positions, server startup command, expected outcomes
  and evidence checklist. An independently generated admin token is included.

The directory is owner-only (0700); configuration and instructions are 0600.
Default `var/` output stays gitignored. Console output gives the instructions path
rather than printing credentials. Keep the generated instructions private when
sharing logs or screenshots.

Stop the regular server before using the generated startup command, because its
listener addresses are retained. Configure the legacy launcher with your usual
local-server setup; the instructions list login/world/status endpoints. The
launcher selects the endpoint—it is not automatically reconfigured by this tool.
For remote-client testing, change the source config's bind addresses appropriately
before preparing a run.

Open `instructions.md`, start the test server using its command, then log in with
the supplied credentials. Use a second legacy-client instance for the observer
when checking broadcasts. The observer has no test items or quest progress and
starts beside the tester. The framework grants no GM privileges.

### Scenarios

| ID | Prepared state and acceptance target |
| --- | --- |
| `native-commands` | Two ungrouped Starter Beach characters; appearance, gestures, movement, mall synchronization and reconnect. Unreached title/job/bath commands stay NOT TESTED. |
| `carnie-return` | Starter Beach (11016), where the native Carnie teleport is available; saved visit origin, native teleport/exit, movement and reconnect without autosave conflicts. |
| `fishing` | Configured rod, no learned fishing skill, SQL-derived walkable shoreline; native start/stop, timed catches and interruption. |
| `fishing-skilled` | Same, with the configured fishing skill at grade one; proficiency and reconnect. |
| `fishing-advancement` | Learned skill at grade one with EXP one below the SQL threshold; first-catch advancement, cumulative native display and reconnect. |
| `fishing-full-bag` | Learned skill and every bag cell occupied by a full rod stack; due catches retain proficiency without delivering items. |
| `inventory-raft` | One 4 × 3 raft at anchor slot 1; valid/invalid moves and reconnect without duplicate items. |
| `raft-no-space` | Alternating full rod stacks leave fragmented free cells; raft pickup fails until a contiguous rectangle is freed. |
| `robinson-recovery` | Completed raft dialogue checkpoint, pending recruitment, no Robinson/raft; actor interaction completes recruitment once. |
| `compound-unskilled` | Stackable materials, no learned skills; Primary level 0, consumption, result replies and reconnect without teaching alchemy. |
| `compound-junior` | Three materials with different first/base-sum winners; Junior level 1 outranks Primary level 30, secondary loss and rank fallback. |
| `compound-books` | Four material stacks and Books 1/4, all three tiers learned; one book per native recipe, 3/4/5-input requests, Superior level 1, highest-book additive shift and secondary retention. |
| `compound-fallback` | SQL-verified impossible material/rank pair; every accepted attempt consumes inputs and produces catastrophic junk. |
| `compound-full-bag` | Full inventory with stacked materials; rejected delivery changes nothing, then clearing slot 50 permits one result. |
| `compound-advancement` | Primary level 10 one EXP below the native Formula.dat threshold; first committed attempt advances to 11, subsequent progress and reconnect. |

Shoreline coordinates, rod/skill IDs and catch timing come from installed SQL
rules. Preparation fails when required definitions or geometry are unavailable.
The test does not shorten timers or alter catch weights. Skill tests require
waiting for the configured advancement threshold. Existing authored fishing
uncertainties remain documented in [FISHING.md](FISHING.md); a working client
interaction alone cannot prove exact original probabilities.

### Results and repeatability

Each run starts **NOT RUN**. Mark checklist steps PASS, FAIL or NOT TESTED and
record timestamps, observed outcomes, screenshots and relevant lines from
`server-diagnostics.jsonl`. Debug logging uses the existing server diagnostics;
it is not a full network capture. Use packet captures when an uncertain native
layout cannot be resolved from those logs. Record packet evidence for the
inferred AC7 adapter rather than assuming its acceptance.

Keep the same database/config for reconnect and persistence checks. Afterward,
log out both accounts and stop the server gracefully. Keep failed runs for
investigation. For a clean repeat choose a new output directory; existing
output directories are refused, so previous evidence and characters survive.
Deleting an old run directory is a separate operator action, after its server
has stopped. Preparation failures clean up only the newly created directory.

Fishing captures should show `[23,53,bagSlot,presentation]` when starting with
a rod and `[23,54]` when stopping. The chosen rod must match the selected bag
slot, including when a stronger rod is present elsewhere. The presentation byte
does not change SQL catch rules. Observer rod state uses AC23:123/122 and
must replay after map entry. Delivered catches should show an item flight toward
the character and an AC23:51 acquisition chat line. The current flight adapter
reuses native ground pickup presentation; crowded/reserved ground slots skip
only that visual. Fishing EXP uses AC8:1 stat 111, cumulative on the wire;
SQL progress remains per grade. Reconnect and verify both the grade and overlay.
A full bag should display its warning and retained proficiency without a false
acquisition notice or flight.

The checklist is the acceptance record. Automated fixture tests verify prepared
state, isolation, credential authentication, private file modes, failed setup and
refusal to overwrite; they do not turn manual checkboxes into a live pass.

### Adding scenarios

Add a named `Scenario` in `internal/livetest/scenarios.go` with explicit steps and
expected outcomes, then extend `Character` with its prerequisites. Use descriptive
content constants and SQL-derived definitions/terrain. Keep observer preparation
free of tester rewards. Never edit active sessions or the regular gameplay
store; sensitive test state is created through transactional store APIs before
the server starts.

Add relevant prerequisite tests in `internal/livetest/scenarios_test.go`. Run:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race ./internal/livetest ./cmd/live-test
```

Native SQL integration checks skip when that variable is absent. They remain
fixture checks, not live-client acceptance.

### Recorded live outcomes

On 2026-10-04, the tester reported these results using the legacy client:

| Run | Result | Evidence |
| --- | --- | --- |
| Carnie Return 02 (`carnie-return-02`) | PASS, tester-reported | “seem to be running properly” |
| Fishing Skilled 02 (`fishing-skilled-02`) | PASS, tester-reported | “seem to be running properly” |
| Carnie Return 01 | BLOCKED by external conditions | Tester attributes failure to outside forces. |
| Fishing Skilled First | BLOCKED by external conditions | Tester attributes failure to outside forces. |

The specific external causes were not supplied. Preserve the earlier runs for
investigation; they do not establish server regressions. The passing reports
confirm overall scenario operation. Individual checklist steps, including catch
flight, observer rod rendering, private notifications and reconnect progress,
remain unverified separately unless their observations are recorded.

### Native command acceptance and fishing parity batch

The following fresh runs were prepared on 2026-10-04. Each has its own isolated
`wonderland.db`, tester/observer credentials and startup command in
`var/live-tests/<run>/instructions.md`. Initial status was **NOT RUN**; the results below were subsequently reported:

| Run | What to verify |
| --- | --- |
| `native-commands-01` | Login/self synchronization, native walking/waypoints, peer gestures and pose cleanup, mall catalog/balance aliases, reconnect and peer replay. |
| `fishing-unskilled-01` (PASS) | Catching without learning the skill, configured timing, owner-only notifications, peer rods, cancellation and reconnect cooldown. |
| `fishing-full-bag-01` (PASS) | No item or success notice for discarded catches, retained proficiency, and delivery after freeing inventory space. |
| `fishing-advancement-01` (PASS) | First due catch advances grade one to two, cumulative display and reconnect, then ordinary progression. |

Run one server at a time using the exact generated startup command. The
prepared configurations referenced the then-current schema-v11 read-only catalog at
`/tmp/wlo-multislot-db-verified/assets.db`; retain that file while testing. To
regenerate against your current imported database, pass its absolute path with
`-assets-database` and select a new output directory. Rod IDs, thresholds and
prepared skill progress are printed in the private instructions. Timers and
reward weights remain those from the selected catalog.

Native title, reborn-job, bath, heartbeat, cutscene and matrix checks count only
when their requests are reached through the legacy client's actual UI flows.
The fixture does not manufacture unlocks or send substitute protocol requests.
Record each unreached request as **NOT TESTED**. In particular, ordinary AC6
walking does not establish acceptance of the inferred AC7 map/X/Y layout.

For fishing, record item IDs, catch times, inventory before/after, skill grade
and EXP, reconnect display, observer rod state and notification privacy. These
runs test the configured fishing behavior; observed samples do not establish
exact original reward probabilities. The already reported successful
`fishing-skilled-02` and `carnie-return-02` runs remain preserved.

#### Batch results and native checklist guidance

On 2026-10-04 the tester reported all checks passing for Fishing Unskilled,
Fishing Full Bag and Fishing Advancement. Advancement also played the native
level-up animation and voice line. These are live passes for the configured
behavior; exact historical reward probabilities remain unresolved.

Native Commands remains a partial pass: ordinary movement and pose controls
work; AC7 was not observed. Observer expressions failed. The log recorded ten
AC32:1 requests and only one outgoing expression packet; the server incorrectly
suppressed transient expressions using the held pose field. Native
`FUN_00432478`/`FUN_00430700` treats expressions as timed animations; AC32:2
`FUN_00438684` controls persistent poses. The corrected expression path and
bounded expression packet diagnostics require an observer retest after restart.

For the numbered native checklist:

- **5:** Open Item Mall, check items/balance, close and reopen, then walk. No
  purchase is required. Report whether the UI works; packet review is developer work.
- **6:** Reconnect the tester with the observer online, then reverse roles.
  Each client should see exactly one of the other character.
- **7:** **Skip.** Mark **NOT TESTED**. There is no client action to perform.
- **8:** Walk, take a screenshot, reconnect, and compare position, appearance
  and Inventory HP/SP.
- **9:** Open **Social → Setup**, save a custom title/nickname, blood type and
  valid birthday, then reopen and reconnect. Retry with a required field missing;
  a warning must leave the connection alive and retain previous blood type/birthday.
  Awarded titles, reborn jobs and bath flows still need separate fixtures.

#### Social setup and movement follow-up

The tester confirmed steps 5 (mall) and 6 (peer reconnect) pass. Social Setup
failed with AC10:1 lengths 8 and 10 rejected as malformed; those are native
length-prefixed nicknames, not contact IDs. Native sender disassembly at
0x2c3ddb establishes `[10,1,length,nickname]`; 0x2c3ea0 establishes
`[10,2,socialCode,bloodType,birthYearOffset,birthMonth,birthDay]`. The birth
UI adds 1900 to its stored year byte. Schema v16 preserves existing rows and
stores these fields. Invalid filled-form values return a warning without a
disconnect or overwriting the saved blood type/birthday. Nickname and profile
are separate native requests and commit separately. This correction needs live
acceptance; do not count the earlier failure as resolved by unit tests.

The native movement log also contains AC7 position resets between AC6 requests.
Go was checking a straight leg from the previous announced destination, which
may still be ahead of the avatar, then resetting the client to that destination
on rejection. Native 15-byte AC6 announcements now validate the endpoint while
the client resolves its route; rejected endpoints leave session state unchanged
and receive no position-reset packet. Short request adapters and server NPC
straight-leg checks retain their current collision validation. Vehicle bounds
checks remain. This is a compatibility correction, not a server-side pathfinder
or speed/anti-cheat implementation. Live retarget/unreachable-click testing is
pending; decoded rejection coordinates are logged at debug level.

#### Native movement disconnect follow-up

Tester disconnected at 21:53:20 on a 15-byte incoming AC6:2. Decompilation
identifies it as a stopped-current-position report. The server now updates the
buffered character position and sends the corrected endpoint to visible peers,
without echoing movement to the sender. Restart, log in Tester and Observer on
the same map, begin walking, and select a pose before reaching the destination.
Also click to change facing while a pose is active. Confirm:

- A received AC6:2 logs `stopped: true` and no malformed rejection.
- Tester stays connected without restarting movement or snapping to the old target.
- Observer sees Tester finish at the corrected endpoint and receive the pose update.
- After waiting for a checkpoint, or logging out normally, reconnect at that endpoint.

Keep this retest pending until observed in aLogin. Invalid endpoints remain
ignored, and the eight trailing check bytes are not yet validated.

#### Latest acceptance correction

The tester corrected the preceding report: friend requests, profile settings and
emotes now work properly. Record these as tester-reported functional passes and
remove the blockers introduced by that typo. This confirmation does not provide
individual results for every accept/reject, persistence or invalid-form subcase;
retain those detailed checks as unverified where no earlier result exists.
AC6:2 stopping still needs an explicit observer/reconnect retest. The previously
reported fishing passes remain valid; AC7 and unreached title, reborn-job and
bath flows remain NOT TESTED.

### Compounding acceptance batch

Prepare each scenario in a separate new directory. For example:

```sh
go run ./cmd/live-test -config config.local.json \
  -scenario compound-books -output var/live-tests/compound-books-01
```

Run the six `compound-*` scenarios above independently. Start with
`compound-unskilled`, then Junior, books, fallback, full bag and advancement.
Stop the previous test server before starting the next configuration. Each
report contains its own 10-character credentials, starting ingredient quantities,
item IDs/names, ranks, material codes, selected skill state and allowed junk pool.
Left-click items in the native Compound bag, choosing one unit when prompted.
Use its named bag slots, not items produced by a previous attempt. aLogin allows
only one Alchemy Book per recipe; use Book 1 and Book 4 in separate attempts. Repeat with a
new output directory to replenish ingredients; preparation refuses overwrites.

Definitions come from the installed assets database. Preparation requires
eligible 1×1 stackable ordinary bulk materials (native types 31–38) of rank at least 5, the relevant alchemy skills,
the selected books and at least one eligible catastrophic reward. Junior and
book fixtures require a reachable normal item with both material bases, so they
can verify successful material retention as well as failures. The forced
fallback fixture checks every candidate at or below the maximum Superior ceiling
for the chosen base pair, with the higher-ranked material first so Superior's
rank-sum primary agrees with the checklist. It does not alter probabilities,
inject a random seed or modify `assets.db`. Fixture tests also verify the forced
fallback for every possible delta using independent controlled rolls.

The book checklist compares each committed log row against
`rank_ceiling = max(1, base_rank + delta)`, where logged `delta` already includes
the highest book. Subtract `book_bonus` to check the original tier range.
Separate live attempts have different random rolls: Book 1 need not produce an
item exactly one rank above a previous no-book attempt. Initial catastrophes
skip delta selection and may log a zero delta/ceiling; candidate exhaustion can
also force catastrophe after the delta is rolled. Junk need not fit the normal
rank ceiling or secondary requirements. Exact weighted probabilities require
statistical analysis, not a handful of successful UI attempts.

Enable **Use Junior Alchemy** for Junior tests and select Superior for Superior
tests. Capture AC23:14/87/101 requests and AC23:9/8/13 replies. Use screenshots for the client
result presentation, inventory quantities and skill display. Keep reconnect
checks on the same prepared database. If aLogin cannot display per-grade EXP,
stop the test server and inspect the isolated database with a read-only query:

```sh
sqlite3 -readonly var/live-tests/compound-advancement-01/wonderland.db \
  'SELECT character_id, skill_id, grade, exp FROM character_skills WHERE skill_id IN (15997,15998,15999);'
```

These tests verify client acceptance and durable mechanics. They start **NOT RUN**;
passing fixture tests do not establish a live-client pass. See
[compounding policy](COMPOUNDING.md) for the compiled rates and full rules.

#### Prepared compounding runs

On 2026-10-06, the finalized fixtures were prepared in these private, ignored
directories. Each remains **NOT RUN**. Open the corresponding `instructions.md`
for credentials and the exact server command; no server was started.

| Scenario | Run directory |
| --- | --- |
| Unskilled | `var/live-tests/compound-unskilled-02/` |
| Junior | `var/live-tests/compound-junior-02/` |
| Books | `var/live-tests/compound-books-02/` |
| Forced fallback | `var/live-tests/compound-fallback-02/` |
| Full bag | `var/live-tests/compound-full-bag-02/` |
| Advancement | `var/live-tests/compound-advancement-02/` |

The `-02` fixtures are superseded by the corrected `-03` batch below. The initial `-01` preparations
remain untouched; the finalized Junior/book fixtures additionally guarantee an
attainable normal two-base result.

#### Corrected native compounding fixtures

The tester reported that the earlier materials (27012/27013) were blocked by
alogin as not compoundable. They are native lucky-bag items; the broad metadata
filter alone did not make them suitable ordinary ingredients. Runs `-01`/`-02`
therefore do not establish compounding acceptance. The unskilled logs contain
no AC23:14 attempts or committed calculations. AC23:115 was incorrectly inferred
to be compounding; `FUN_0021bd90` actually sends AC23:14.

Use `var/live-tests/compound-<scenario>-03/instructions.md` for the corrected
batch (unskilled, junior, books, fallback, full-bag, advancement). Only ordinary
bulk material types 31–38 are eligible fixture inputs. The checklists use native
left-click selection, with one unit if prompted. aLogin's `FUN_0021bd90` limits
recipes to one book; the five-input test uses four material stacks plus Book 4.
Multiple-book highest-bonus logic remains supported by Go and its automated tests.
These fresh runs start NOT RUN; no server was started during preparation.


## Historical validation reports

### Migration verification recorded on 2026-10-04

Compared current handlers and their callers with the recorded reference revision;
25 reviewed source hashes match after CRLF normalization. Ran **175 unique
top-level tests** with the race detector across game, assets, assetsql, protocol,
world, server and store. A stale AC37/AC64 dispatch test matrix was corrected and
its affected checks rerun successfully. This was a focused battery, not the full suite.

| Area                    | Verified outcome                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Allocation              | Direct/header/target/batch formats preserve greedy 4/2/1-byte amounts, missing/zero defaults and partial-batch behavior. Invalid/overflowing entries retain points; pet overflow now matches the reference guard. Character overflow is deliberately rejected as well. HP/SP remain unchanged apart from clamping to maxima; skill unlocks and allocations commit before snapshots. |
| Generic replies         | All handshake/entity subcommands keep their owning layouts. Unknown bank subcommands now return the private AC45:8 balance snapshot without mutation. No global ACK fallback is invented.                                                                                                                                                                                           |
| Native scene readiness  | Source AC3 short ACK/map broadcast conflicts with native AC3 character/map snapshots (receive address 0x2dfdb3); it remains unsupported. AC12/89/92 synchronization, replay and loading gates are tested independently.                                                                                                                                                             |
| Guild journal           | AC39:1 works without guild membership; repeated requests clear old AC24:4 entries, replace active marks and send completion flags without leaking to peers.                                                                                                                                                                                                                         |
| Inventory rules         | All 256 stack/drop type values match source. Wear guards, metadata, locks, whole grants and full-container rollback pass. Source Tradeable and use-type metadata have no reachable restriction callers; no new trade/body/level restriction is imposed.                                                                                                                             |
| Pet outcomes            | Feeding, alias/capacity/initialization checks for vouchers, sparse client slots, rebirth eligibility/reset/bonus and desertion below 20 pass. Failed saves do not consume vouchers or change published rosters. An unrelated mount survives desertion.                                                                                                                              |
| Recipe order            | Historical first-match alchemy tables preserve authored → Compound2 → Compound order through typed SQL reload. Gameplay now intentionally uses the owner-approved [rank/base model](COMPOUNDING.md) instead. Two-input manufacturing retains first workbench match and atomic costs/output.                                                                                                                                                                |
| Persistence and stories | Restart, rollback, duplicate requests, ownership, receipt failures and full containers are covered; native ship/storm/Monkey/Xaolan/Niss/Fred/Elin scenarios run against imported SQL assets. Buffered walking survives committed resource operations.                                                                                                                              |

The SQL census covered 1,119 maps, 8,181 NPCs, 209 ground items, 2,790 resolvable
warps and 786 manufacturing formulas. Of 7,313 NPCs with events, fresh-character
branch selection found 4,872 runnable branches, 30 with unsupported operations,
and 2,411 with no matching branch for that state. Unsupported here means the
ordinary opcode executor refuses the shape; complete water-gathering branches
have a separate validated controller. The reference also rejects unresolved
reward kinds. This census is selection/decoding coverage, not execution of every
story or proof that every state has been ported. Unresolved shapes remain pending.

Regression entry points are `migration_verification_test.go` in game, server and
assetsql, alongside the affected existing suites. To repeat the new checks:

```sh
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" go test -race \
  ./internal/game ./internal/assetsql ./internal/server \
  -run 'Test(Migration.*|CommandRegistry.*|AllocationUnlock.*|Bank.*|PetAllocation.*)' -count=1
```

That recorded verification used asset v10 and gameplay v15. Repeating it with
the current server requires the current schemas documented in
[database upgrades](ASSET_DATABASE.md). Verification used
disposable fixtures and asset database copies; installed databases were unchanged.
Interactive aLogin captures, multiplayer acceptance and load/soak/crash testing
remain separate work below.

### Battle validation history

The following evidence was recorded during the earlier battle-port work. It is
not a full-suite result from the 2026-11-01 audit.

Independent raw-byte fixtures cover formation, turn ownership, repeated readiness,
variable actions, malformed/truncated packets, batched stats, recovery and capture.
All 1,832 exported motions are decoded. Additional tests cover scene commands,
world movement exclusion, direct skill/pet targeting, transient HUD state,
background workspace progression, damage distribution and once-only sounds.
CPU/GPU parity includes formation, radial controls, action/damage, the previous
action icon, exit and roaming NPC movement (20 states).
Server rules have an independent recovery-wire golden and low-maximum revival case.

The earlier battle-port pass recorded: full root and client Go suites,
root/client `go vet`, focused battle/protocol/server/app race tests, and 18-stage
CPU/GPU parity with both loose assets and the compiled bundle. Tests used a
60-minute outer timeout rather than the previous ten-minute limit. The final
focused app race run, including the stale-target corrections, completed in
239 seconds.

`make build-client CLIENT_BUNDLE=/tmp/wonderland-battle-assets.zip
CLIENT_OUTPUT=/tmp/wonderland-battle-client` succeeded, packaging 72,829 files
from approximately 6,017 MiB of source assets and compiling the client binary.
Existing authored sprite exports were used without regeneration. Final request
and stale-target corrections have additional focused regression/race coverage.
The server binary also built successfully with
`go build -o /tmp/wonderland-battle-server ./cmd/wonderland`.

The subsequent roaming/previous-action pass has focused world/battle/hotbar
regressions, session-ownership race coverage and 20-state CPU/GPU parity. See
[client roaming](CLIENT_ROAMING.md) for native packet evidence and the explicitly
named initial-zero-speed compatibility case.

### Administration validation history

The earlier administration pass recorded the following; it is historical
evidence, not a rerun during this documentation change:

A headless Chromium smoke renders all 24 administration views and checks read-only
inspectors, including the combined map/quest actor report. Interactive native aLogin acceptance still requires manual validation; the automated checks cover API access,
transaction behavior, native packet layouts and affected gameplay paths.

### Archive validation history

The following native-archive census and `client/ui` verification results were
recorded during earlier inspection work. They are historical evidence, not a
claim that current gameplay or native-client acceptance passed. The normal
client's screens and remaining work are described in [the client guide](CLIENT.md)
and [the roadmap](CLIENT_ROADMAP.md).

- Local migration verification passed for all 4,070 files against the SHA-256 manifest. Staging/ZIP/checksum tests, client tests and vet/build pass; the base image census and a 11016.jpg PNG export pass using only local assets. Upload packages have been prepared locally, not published.
- Directory and payload-signature checks pass for `map.JMG`, `images.BMg`, and `item.BMg` in all three client installations. These three base archives are byte-identical across the installations (SHA-256 comparison).
- Full image decoding passes for 326 map images, 2,190 item icons and 268 of 269 base UI images. `HiTurtle_2.bmp` declares 255 palette entries but uses missing palette index 255. It is retained unchanged, reported explicitly by the test, and displayed as a decode error by the workbench.
- A native 11016.jpg frame was rendered through Ebitengine and visually inspected. No player, NPC, UI composition or gameplay compatibility is implied by rendering a background.
- Image payloads are capped at 64 MiB and decoded dimensions at 128 million pixels. Unsupported/corrupt resources display an error and remain browsable. Additional expansion archives have not yet received the same complete image census.
- JMA/JXA and JXAN asset decoding is implemented in `internal/clientassets` and `cmd/sprite-export`. All selected client resources are exported under `data/sprites`; see [the sprite export documentation](../data/sprites/README.md). The native SHA padding and AES key expansion quirks are covered by independent golden vectors and authenticated native sprite tests. Three orphan JXAN files have no corresponding pixel archives in the source directory.

### Source update verification — 2026-11-01

The update ledger covers 38 commits and all 431 changed files through `1ff5bba`.
Source hashes were rechecked byte-for-byte against the pinned archive, and every
Go evidence path resolves. Prototype/platform exclusions are documented in
[SOURCE_UPDATES.md](SOURCE_UPDATES.md); they are not counted as working features.

Focused race checks cover native companion/job rebirth, deterministic boxes,
mission/PreEvent conditions, generalized gathering, proficiency and reinforcement
waves, NPC shopping, personal/persistent bazaars, native tent trees/tools,
manufacturing batch/refund/restart, travel and Laura's one-time exit gift.
Independent packet expectations cover 24-bit speech, reinforcement records,
furniture trees, item/pet/vehicle browse layouts and cancellation acknowledgments.
Client regressions retain 24-bit talk references and ID-over-offset lookup precedence.
Failure paths include full bags, stale quotes/funds, repeated claims, SQL rollback,
scarce-stock races and preservation of instance metadata. The final focused
battery uses:

```sh
GOCACHE=/tmp/wonderland-precommit-go-cache go test -race \
  ./internal/assetsql ./internal/world ./internal/game ./internal/battle \
  ./internal/store ./internal/server \
  -run 'Test(CatalogDefinition|NPCRoutesMigration|QuestLifecycle|MissionVariables|RebirthLevelDeduction|EventSpeech|PoisonDefault|Tent|NativeManufactur|Manufacturing|Bazaar|NativeStall|AlternativeStall|CompactCreation|SpeechTrace|Repair|GMReborn|RebirthJob|CompanionRebirth|Boxed|EventGathering|TeleportScroll|KelanReturn|BambooDragonfly|Reinforcement|StormAndBeach|Robinson|Hotel|CombatSkill|BattleSkill)' -count=1
```

Disposable SQLite backup copies of installed asset v13/gameplay v17 were upgraded
through `database-migrate`, including content and integrity verification. The
result is asset v14/gameplay v18, with 19 boxes, 124 evolution links, 7,130 NPC
buy prices, three fallback shop catalogs, the exit reward and travel tables.
Native poison definitions migrate to 10%/minimum 15. Round trips cover all ten
new catalog roots; separate checks preserve edited tent/poison defaults and
existing owned state. The upgrade also projects 1,512 event-route entries (2,029
points) from retained map sections. A migration regression checks reordered/added
NPCs, edited walking steps, signs/speeds and repeat-upgrade preservation. Native
ship/Fred scenarios and installed manufacturing/tent reference checks passed against the upgraded copy.

The installed databases remain at v13/v17; they were not overwritten. The sibling
working checkout and original source inventory hashes are unchanged. Tests use
`GOCACHE=/tmp/wonderland-precommit-go-cache` to avoid the read-only host cache.
The client port catalog was regenerated (12,130 functions, 900 citations,
228 regions, zero unclassified regions, 29/91 receive commands with branches).
Command coverage is not full interface or feature acceptance.

Both module `go vet` runs and the focused client race checks pass (app: 203.646s).
The complete client race run reached its
30-minute outer timeout during active asset loading, without preceding assertion
failures or race reports; it is incomplete, not a pass. Final validation uses the
full server race suite, the full client suite without `-race`, and focused client
race checks for changed dialogue/event/voice/lookup paths. Loopback socket access
is enabled. The full server race suite passed (server: 793.903s; assets SQL:
77.541s), as did the full client suite (app: 251.513s). These runs preceded the
owner’s instruction to reserve full server validation for pushes. Commands used:

```sh
WONDERLAND_TEST_ASSETS_DB=/absolute/path/to/upgraded-copy/assets.db \
  GOCACHE=/tmp/wonderland-precommit-go-cache go test -race -timeout=60m ./...
cd client
GOCACHE=/tmp/wonderland-precommit-go-cache go test -timeout=60m ./...
GOCACHE=/tmp/wonderland-precommit-go-cache go test -race -timeout=10m \
  ./wlo/app ./wlo/world \
  -run 'Test(Dialogue|NPCTalk|TalkWrap|CatTalk|Question|StepCompleteAcknowledged|RouteEvent|EventTurns|EventMusic|TalksPreserves)' -count=1
```

Interactive aLogin captures, the new Go-client native-form interfaces,
multiplayer/load/soak and crash acceptance remain pending. No default-suite result
implies those optional or live checks passed.


### Resource exhaustion review — 2026-11-01

The timed-out client race run was actively registering skin images in
`picdb.LoadDir`, not waiting on a deadlock. A focused CPU profile attributed about
75% of sampled time to race runtime/external instrumentation. The same installed
client-entry/dialogue test took 1.558s without `-race` and 25.689s with it after
hardening. These are individual local measurements, not production latency or
load acceptance. Each app test constructs an isolated client and repeats cold
resource preparation; production workspace sessions share their resources.

The review found and addressed these amplification paths:

| Finding | Exposure and change |
| --- | --- |
| Quadratic skin registration | The directory was reread/sorted for each image. `LoadDir` now resolves filenames in one enumeration and retains them for deferred loading. Three one-iteration samples at 1,000 tiny images improved from median 121.3ms / 130.0MB allocated to 8.60ms / 6.70MB. Case-insensitive names, PNG precedence, first-directory overrides and compiled/loose rendering are preserved. |
| Unbounded native verification expansion | Editable `sprites.json` trusted its declared expansion size and had no JSON allocation ceiling. Input is capped at 128 MiB and decoded payloads at 512 MiB before decompression. All 107 installed exports fit; the largest is about 67 MiB of JSON and 198 MiB decoded. At most two native decoders run per workspace manager, and close cancels queued work. |
| Oversized/repeated UI decoding | PNG/BMP configuration and compiled dimensions are checked before pixel allocation: maximum side 16,384 and maximum area 16,777,216 pixels. Failed deferred decoding is attempted once per resource load, and skin preparation propagates errors. |

The asset findings require malformed or unusually large local/supplied assets;
this pass did not demonstrate a remote player's ability to supply those bytes.
Bounds cover the reviewed UI/native-verification paths, not every client decoder.
Native verification files are still retained until manager close; an aggregate
disk/cache budget across distinct archives remains a follow-up.

A separate potential multiplayer denial-of-service path was identified: synchronous
peer writes in [broadcastWorld](../internal/server/world.go) occur while holding
`worldMu`, and the old [Session.send](../internal/server/session.go) permitted ten seconds
per write. A non-reading peer whose socket buffer fills can delay unrelated
world actions; multiple slow peers can amplify it. Existing packet/connection
limits do not remove outbound backpressure. This is a code-level finding, not
a completed live exploit/load reproduction. The subsequent
[outgoing network isolation](#outgoing-network-isolation--2026-11-01) change
implements bounded per-session writer queues with disconnect-on-overflow,
ordered delivery and actual write receipts for mail markers. Deterministic
stalled-socket regressions verify that unrelated world actions now progress.

Focused validation covers oversized/sparse JSON, excessive declared expansion,
checksum/length mismatch, oversized PNG headers, decode failure caching,
PNG/case precedence, compiled-image parity and native close/cancellation races.
The queued-close race check passes 20 repetitions. Resource package race checks
pass (picdb: 1.176s; sprites: 5.497s), as do the focused app/shared-workspace checks
(49.485s), JSON guard tests and affected-package `go vet`. Commands:

```sh
go test -race ./internal/clientassets -run '^TestSpriteJSON' -count=1
cd client
go test -race ./wlo/picdb ./wlo/sprites -count=1
go test -race ./wlo/sprites -run '^TestCloseCancelsQueuedNativeDecode$' -count=20
go test -race -timeout=5m ./wlo/app \
  -run '^Test(WorkspaceSharedAssetsAndIndependentSessions|NativePresentationAndAllocation)$' -count=1
go test ./wlo/picdb -run '^$' -bench '^BenchmarkLoadDir$' -benchtime=1x -count=3 -benchmem
```

The full server/client results above precede this resource-hardening follow-up.
Only affected tests are rerun here; full server validation is reserved for a push.


### Migration ownership verification — 2026-11-01

Definitions, embedded defaults, validation and additive asset migrations live
with their owning features. Gameplay v18 DDL lives with teleport points, tent
tools, manufacturing and bazaars; the schema runners coordinate their existing
version and transaction boundaries. Source-labelled modules/tests were replaced
by domain names. The reference ledger is [source-update-inventory.json](source-update-inventory.json).

Focused race checks pass for catalog migration/rollback/administration, NPC routes,
tent/poison defaults, event/mission conditions, speech packets, rebirth/proficiency,
travel, manufacturing, tent tools and bazaars. Preservation checks reenter the v14
upgrade with edited box quantities/NPC prices and deliberately empty skill
links. Existing edits and deletions survive. Generated SQL models are byte-identical
after regeneration; all 431 reference hashes and evidence paths still verify.
Affected-package `go vet` passes. No full server suite was run for this refactor.

```sh
go test -race ./internal/assetsql ./internal/world ./internal/game ./internal/server \
  -run '^Test(CatalogDefinition|StructuredCatalogMigration|StructuredAdministration|NPCRoutesMigration|TentDefaultsMigration|PoisonDefault|QuestLifecycleAndCharacterSubjects|MissionVariablesAndPreEventGroups|EventSpeech24BitGolden|CombatSkillEXP|RebirthLevelDeductionAndLevel199|Boxed|EventGathering|TeleportScroll|KelanReturn|BambooDragonfly|MapTransition|NativeNPCShop|NPCShop|NativeRebirthJob)' -count=1
go test -race ./internal/store ./internal/server \
  -run '^Test(TeleportPoints|Tent|NativeManufacturing|Manufacturing|Bazaar|CharacterMetadataV13|NativeStall|AlternativeStall)' -count=1
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/game ./internal/store ./internal/server
```


### Pull request integration — 2026-11-01

Five native-client contributions were reconciled with the newer server and Go
client implementation. The PR head revisions are preserved as merge parents.

| PR | Resulting behavior |
| --- | --- |
| [#1](https://github.com/DangoKnight/Wonderland-Gonline/pull/1) | Region kind 3 resolves authored portal events instead of colliding warp IDs. |
| [#2](https://github.com/DangoKnight/Wonderland-Gonline/pull/2) | Subject kind 3 accepts exact body IDs 1–4 as well as gender codes 7/8. |
| [#3](https://github.com/DangoKnight/Wonderland-Gonline/pull/3) | Existing native trade, exact metadata, locked offers and atomic pair commits retained; protocol documentation reconciled with independent fixtures. |
| [#4](https://github.com/DangoKnight/Wonderland-Gonline/pull/4) | Native five-byte slot/price offers, styles, scene-list completion and bag-slot browse keys supported alongside the six-byte compatibility form; guarded pair purchases retained. |
| [#5](https://github.com/DangoKnight/Wonderland-Gonline/pull/5) | Native item prices supplement SQL initialization, fallback buy/sell choices are separate, and existing pet-hotel acknowledgments are retained. |

Focused race checks cover portal areas, every body/gender combination, trading
replay/metadata/rollback/concurrent acceptance, native and legacy stalls, native
cart forms, SQL price initialization/edit preservation and repeated hotel use.
No GitHub status checks, workflow runs or reviews were present on the reviewed
heads. Final pre-push validation passes: the full server suite with the race
detector and the migrated installed asset database (server package: 735.793s),
the full client suite (app package: 179.440s), and `go vet ./...` in both modules.
The initial isolated-checkout run was invalidated by missing client fixtures
and temporary disk exhaustion; these results come from the complete workspace
after removing those temporary copies. Offline migration and database integrity
checks also pass on disposable copies; the regular databases were not upgraded.
Live native-client acceptance has not been rerun for these integrations.

### Story follow-up verification — 2026-11-01

Private Server commits `d1020d3` and `5c36839` add Waterfall Grotto mirror
continuation and Fairy Altar nonholding movies. Focused server race checks pass
(7.407s): all four mirror events continue through subsequent beam/door rules
without replaying earlier branches; runtime actor swaps survive event completion
and reset on a new visit. The unmodified native EVE scripts also pass repeated
turns in both directions for all four events. Fairy Altar events 2/3 emit movie
mode 1, preserve the completion barrier and resume dialogue without a warp;
unrelated events/maps retain their existing mode. Movie expectations use raw
packet bytes independent of production builders.

Affected world race checks pass (1.012s), covering forward selection, PreEvent
and quest visibility, and mission rule grouping. `go vet` passes for game, world
and server. No schema/data changes or installed database upgrades were needed.
Full server validation remains reserved for a push. Native-script execution and
wire checks do not establish live native-client HUD or puzzle acceptance.

```sh
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
go test -race -timeout=5m ./internal/server \
  -run '^Test(NativeWaterfallMirrorChoices|WaterfallMirrorContinuationAndVisitReset|FairyAltarMovieResumesDialogue)$' -count=1
go test -race ./internal/world \
  -run '^Test(Story|PreEventVisibility|QuestVisibility|MissionVariablesAndPreEventGroups)' -count=1
go vet ./internal/game ./internal/world ./internal/server
```

### Story system migration — 2026-11-01

Existing story-specific selection, continuation, visibility, condition repairs,
recovery, movie/choice modes, recruitment/conversion parameters, atomic reward
checkpoints, battle fallbacks and starter cinematic steps now use typed asset
schema v15 definitions. [STORY_SYSTEM.md](STORY_SYSTEM.md) documents authoring,
SQL ownership, seed preservation and execution limits. Gameplay remains v18.

Focused race validation passes:

- Native EVE story parity (65.419s): eleven Breillat variants and exchanges,
  Fred, Elin, S.Monkey, Robinson recovery, Xaolan rescue, Waterfall mirrors and
  ship/storm transitions. Independent wire bytes and authored scripts retain
  their expectations. Subsequent focused exchange/ship checks pass (27.311s).
- Definition migration, structured storage, invalid-edit rollback, predicates,
  visibility, ordering and outfit checks pass. Generated SQL models reproduce
  byte-for-byte. Historical source hashes remain unchanged and capability-owner
  evidence paths resolve.
- Installed assets v13 upgrade to v15 passes on a disposable copy (18.315s),
  including loading the complete default projection and repeat migration.
  Existing edits and intentionally empty story definitions survive reentry into
  the v15 upgrade; an injected seed write failure rolls the schema back.
- Typed administrator publication/reload passes (64.317s): optimistic versions,
  active-interaction guards, immutable old snapshots and refreshed policy indexes.
  Existing general definition-edit rollback tests also pass.
- Event/pet/battle integration checks pass (20.230s), including legacy reward
  ownership, replay/capture exclusions and raw speech packets. Custom quest kill
  counters and live administration checks pass separately (6.125s).
- New content at unrelated map/event IDs runs through policies alone. Clearing
  definitions does not restore compiled seeds. Automatic continuation cycles
  release their interaction. Movie-handshake triggers remain map scoped.
  Recruitment preserves fresh SQL state and buffered walking, and replay does
  not grant another companion. Atomic exchanges can be authored independently
  of appearance-conversion content.

Affected-package `go vet` and the server build pass. The regular databases remain
unchanged. No full server suite was run for this development migration; it is
reserved for pre-push validation. Live native-client HUD, puzzle and multiplayer
acceptance remain separate from script/wire tests.

Representative commands (run from the repository root):

```sh
go test -race ./internal/assets ./internal/assetsql ./internal/world ./internal/game \
  -run '^Test(Story|Structured|CatalogDefinition|QuestDefinitions|PreEventVisibility|QuestVisibility|Elin|Replace|ReloadEvents|FindBranch|MissionVariablesAndPreEventGroups|StarterScene)' -count=1
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
  go test -race -timeout=8m ./internal/server \
  -run '^Test(InstalledBreillat|Breillat|NativeXaolanRobbery|Xaolan|NativeFredFinale|NativeMonkeyRescue|NativeElin|NativeRobinson|NativeWaterfall|NativeStarterShip|Storm|Spider|StarterShip|Monkey|Story|NissRest)' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/assetsql -run '^TestStoryInstalledMigration$' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/server -run '^TestStorySQLPublicationAndReload$' -count=1
go test -race ./internal/server \
  -run '^Test(Story|NPCConversation|NPCEventRefusals|RegionsAndDoor|AreaRegion|EventSpeech|RecruitSelect|PetGrowthAndConditions|LegacyCombat|LegacyCompanion|GMFreshPetRecruit|ScriptedSharedProp|Waterfall|Fairy|Storm|Spider|Starter)' -count=1
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/server ./internal/game
go build -o bin/wonderland ./cmd/wonderland
```

### Story runtime optimizations — 2026-11-01

The SQL-backed story migration now preorders candidate indexes, compiles grouped
predicates, uses allocation-free first-match and ordinary multi-match iteration,
caches unconditional event repairs and batches SQL projection reads. The SQL
batch is local to one read and transaction; it does not cache player outcomes or
replace transactional resource checks. See [STORY_SYSTEM.md](STORY_SYSTEM.md#execution-and-persistence)
for reload and immutable-program ownership.

Permanent benchmarks use the default authored story definitions and disposable
story-only SQL tables. Medians of three 300ms runs on an Intel i5-13600K,
Go 1.27.3, linux/amd64, GOMAXPROCS=20:

| Operation | Before | After | Allocations before → after |
| --- | ---: | ---: | ---: |
| First movie policy | 175.6ns | 48.12ns | 2 → 0 |
| Gated visibility policy | 692.2ns | 82.19ns | 3 → 0 |
| Unmatched policy | 35.71ns | 37.37ns | 0 → 0 |
| Multi-match visibility iteration | 665.5ns | 164.2ns | 3 → 0 |
| Unconditional repair of a known event | 277.2ns | 14.37ns | 5 → 0 |
| Complete story SQL projection | 7.01ms | 3.47ms | 46,527 → 26,448 |

The projection now uses 45 queries instead of 391, with approximately 1.60MB
allocated instead of 2.92MB. SQL is loaded at startup or administrative publication,
not per story step. These are isolated operation measurements, not end-to-end
server latency or concurrent-player capacity results. The miss path stays close
to its prior cost; startup compilation moves work out of repeated gameplay reads.

Focused race checks pass for authored global/local precedence, selection before
actions mutate quest state, iterator early termination and lists above 256
candidates, compiled/ad hoc gate grouping, current SQL-character rechecks,
cross-player immutable repair reuse, conditional edit ordering and reload cache
invalidation. Batched SQL round trips preserve sibling ownership, row order and
nil/empty children; each required table is fetched once. Tests also retain
ordinal/query/cancellation failures, skip unused child tables, read edits afresh
within the same transaction and verify rollback. The combined affected-package
selection passes (assetsql: 5.491s).

Native-story race parity passes (67.882s), including the migration's existing
raw-script and wire expectations. Administrator publication/reload passes
(70.724s), and installed v13-to-v15 migration plus repeated loading passes on a
disposable database copy (17.929s). Generated SQL output reproduces byte-for-byte;
affected-package/generator vet and the server build pass. The optimization adds
no schema version or migration. Regular databases remain unchanged. Full server
validation remains reserved for a push, and live native-client acceptance has
not been rerun.

```sh
go test ./internal/world ./internal/assetsql \
  -run '^$' -bench '^BenchmarkStory' -benchtime=300ms -count=3 -benchmem
go test -race ./internal/assets ./internal/assetsql ./internal/world ./internal/game \
  -run '^Test(Story|Structured|CatalogDefinition|QuestDefinitions|PreEventVisibility|QuestVisibility|Elin|Replace|ReloadEvents|FindBranch|MissionVariablesAndPreEventGroups|StarterScene)' -count=1
# Native and installed-SQL checks use the migration commands immediately above.
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/server ./internal/game ./tools/asset-schema-generator
go build -o /tmp/wonderland-story-server-check ./cmd/wonderland
```

### Server component layout — 2026-11-01

The server root now has 34 implementation files and 39 integration-test files,
down from 238 Go files. Related command adapters and tests are grouped by
feature. Independent log capture/tracing and 32 packet builders live in
`logging` and `wire` subpackages. Pure speech, text-mail and tent-tree goldens
and trace tests moved beside their implementations with their raw expected bytes
intact. Root constructor/session/administration APIs remain compatible.

A one-off declaration comparison against the pre-layout workspace found all
1,206 original functions and tests. After accounting for moved package names,
their signatures and bodies are unchanged except that `New` now calls the
logging handler constructor, which wraps the same handler and buffer. Session
ownership, world locking, authored tables and SQL transaction boundaries remain
unchanged. Source-inventory owner/test paths now resolve to the new files;
historical source hashes and revisions are preserved.

Validation passes:

- Logging and wire package race checks: 1.007s and 1.009s. Existing root tests
  also retain concurrent bounded/redacted snapshots and capture isolation.
- Focused server race checks: 60.734s, covering dispatch/gates, log integration,
  world visibility/concurrency, trading/stalls, NPC shops, banking, tents,
  companion control, Potential Pills, mall checkout, Lucky Draw, story policies,
  reloads, mail, native stopping, compounding, manufacturing, fishing and recovery.
- Native-story race parity: 60.591s, including Breillat, Xaolan, Fred, Elin,
  S.Monkey, Robinson, Waterfall and ship/storm sequences against original EVE.
- Compilation of all server/component tests, affected server/admin/command vet,
  and the server executable build.

Full server validation remains reserved for a push. Regular databases were not
modified, and live native-client acceptance was not rerun. Commands:

```sh
go test -race ./internal/server/logging ./internal/server/wire -count=1
go test -race -timeout=6m ./internal/server \
  -run '^Test(CommandRegistry|RejectedCommand|PacketTrace|AdminLog|RuntimeSettings|WorldVisibility|ConcurrentWorld|Trade|NativeStall|NPCShop|Bank|Tent|PetControl|PotentialPill|MallCheckout|LuckyDraw|Story|GMReload|TextMail|NativeMovementStop|Compounding|Manufactur|Fishing|Resting)' -count=1
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
  go test -race -timeout=8m ./internal/server \
  -run '^Test(InstalledBreillat|Breillat|NativeXaolanRobbery|Xaolan|NativeFredFinale|NativeMonkeyRescue|NativeElin|NativeRobinson|NativeWaterfall|NativeStarterShip|Storm|Spider|StarterShip|Monkey|NissRest)' -count=1
go vet ./internal/server/... ./internal/admin ./cmd/wonderland ./cmd/live-test
go build -o /tmp/wonderland-server-layout-check ./cmd/wonderland
```

### Outgoing network isolation — 2026-11-01

Accepted login/gameplay connections now enqueue immutable frames into a bounded
per-session transport, with one socket writer outside `worldMu`. Limits are
4,096 frames and 2 MiB of encoded data including the in-flight frame. A frame's
ten-second deadline starts at enqueue, covering both queue residence and partial
writes. Overflow closes that recipient; output is never silently dropped while
keeping an inconsistent session connected. Queue storage grows lazily, and no
goroutine is created per packet.

Tracked receipts distinguish queue admission from completed writes. Native text
mail and GM notification markers advance only after the final packet has been
written. In-flight messages suppress duplicate live sends; failed SQL markers
remain dirty for retry. Claimed GM resources stay committed across write failure
and reconnect. A coalesced completion worker visits only sessions with pending
mail, continues batches after map readiness, and never makes the socket writer
take world ownership or perform SQL. Final disconnect notices can drain under
their existing deadlines; shutdown can abort the drain and joins writers outside
gameplay locks. Closing sessions reject new gameplay after acquiring ownership.

Focused validation passes:

- A blocked `net.Pipe` recipient stays unread while the initiating movement and
  a command on another map both complete within their one-second test limits.
  This directly exercises the old shared-lock failure condition.
- Transport race tests pass 20 repetitions (3.968s): byte/frame bounds include
  active writes; queued copies resist caller mutation; partial writes retain FIFO
  framing; concurrent producers retain per-producer order; timeout/close/overflow
  complete receipts; drain preserves final packets; expired queued data does not
  receive a renewed deadline. Concurrent send/drain/close runs are included.
- Queue-specific server checks pass (2.392s): actual-write mail markers, injected
  marker failure/retry without duplicate sends or healthy-client disconnect,
  continuation beyond the first 32 messages, final-packet gift receipts,
  disconnect/reconnect without another grant, closing-session gates, GM notices,
  and accepted-session cleanup while its writer is blocked.
- The broader affected server race selection passes (58.433s), retaining
  dispatch, world visibility, login/duplicate-login/idle, mail, trading/stalls,
  shops, banking, tents, companion controls, Potential Pills, mall/Lucky Draw,
  story/reload and recovery expectations. Protocol, logging and wire package
  race checks also pass (1.011s, 1.008s and 1.010s).
- Shutdown lifecycle validation passes with loopback access (1.488s). The first
  sandbox attempt could not bind its local listener; that environmental failure
  is not counted as a passing test. Affected-package vet and server build pass.

The regular databases and their schemas remain unchanged. Full server validation
is reserved for a push. This establishes deterministic slow-socket isolation,
not live native-client acceptance, production latency or player capacity. SQL
operations and game mutations retain existing world ownership, including the
five-second context used for receipt processing.

```sh
go test -race -timeout=3m ./internal/server/transport -count=20
go test -race -timeout=3m ./internal/server -run '^TestQueued' -count=1
go test -race -timeout=5m ./internal/server \
  -run '^Test(Queued|CommandRegistry|CreateEnter|WorldVisibility|ConcurrentWorld|FailedWorld|NativeMovementStop|TextMail|AdminMail|DuplicateLogin|ConcurrentLogin|LoginWire|IdleSocket|RejectedCommand|PacketTrace|Trade|NativeStall|NPCShop|Bank|Tent|PetControl|PotentialPill|MallCheckout|LuckyDraw|Story|GMReload|Resting)' -count=1
# Requires permission to bind loopback listeners:
go test -race ./internal/server -run '^TestGMShutdownCountdownUsesGracefulRunCleanup$' -count=1
go test -race ./internal/protocol ./internal/server/logging ./internal/server/wire -count=1
go vet ./internal/server/... ./internal/admin ./cmd/wonderland ./cmd/live-test
go build -o /tmp/wonderland-outbound-server-check ./cmd/wonderland
```

### Contact, movie and rebirth follow-up — 2026-11-01

Private Server `0cb6915` is imported with focused validation only. Independent
raw packet expectations cover contact validation, movie modes and acknowledgment
barriers, kind-3 entry precedence over colliding warp IDs, and one-time UI release
after event warps. Checkpoint tests cover transactional rebirth rollback/retry,
preserved trial steps/completion times, fresh SQL repair, character selection,
already-complete no-write behavior, custom content IDs and cleared policies.

The affected asset, SQL, world and server race selection passes (1.062s, 2.587s,
1.032s and 14.390s). Native story parity, including map 12132 actor 4's contact
transition to map 12141, passes (67.386s). Installed assets v13 upgrade to v16 and
repeated loading pass on a disposable copy (18.306s); SQL publication/reload also
passes (65.557s). Migration fixtures retain edited rules and nil/empty collections,
preserve later deletion on repeats, and roll schema/data/version back on injected
write failure. Generated SQL models reproduce byte for byte; affected-package vet
and the server build pass.

The seven changed source files have exact-byte hashes in the
[follow-up ledger](source-update-inventory.json). SQLite schema and row comparison
finds no shared-definition changes: player state and chest primary-key churn are
excluded. Regular databases and the sibling checkout remain unchanged. Full server
validation is reserved for a push; live native-client acceptance was not run.

```sh
go test -race -timeout=4m ./internal/assets ./internal/assetsql ./internal/world ./internal/server \
  -run '^Test(Story|RebornStory|ActorContact|Movie|EventWarpRestores|AreaEntryScripts|FairyAltar|GMReborn|RebirthJob|CommandRegistry|AreaRegion|RegionsAndDoor|ReloadEvents)' -count=1
WONDERLAND_TEST_DATA="$PWD/../Wonderland-Private-Server/Data" \
  go test -race -timeout=8m ./internal/server \
  -run '^Test(InstalledBreillat|Breillat|NativeXaolanRobbery|Xaolan|NativeFredFinale|NativeMonkeyRescue|NativeElin|NativeRobinson|NativeWaterfall|NativeStarterShip|NativeActorContact|Storm|Spider|StarterShip|Monkey|NissRest|FairyAltar)' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/assetsql \
  -run '^TestStoryInstalledMigration$' -count=1
WONDERLAND_TEST_ASSETS_DB="$PWD/var/assets.db" \
  go test -race -timeout=5m ./internal/server \
  -run '^TestStorySQLPublicationAndReload$' -count=1
go vet ./internal/assets ./internal/assetsql ./internal/world ./internal/server/... ./internal/game ./tools/asset-schema-generator
go build -o /tmp/wonderland-private-0cb6915-server-check ./cmd/wonderland
```

### Private Server import pre-push validation — 2026-11-01

The full root suite passes with the race detector against a disposable assets
database upgraded from v13 to v16 (server package: 831.308s; assets SQL: 95.568s).
The full client suite passes without the race detector (app: 197.844s), and
`go vet ./...` passes in both modules. These checks cover the `0cb6915` import
and the preceding story modularization and outgoing-network isolation commits.

The run reports 1,789 passing test/subtest events and 14 skipped tests in the root
module, plus 416 passing events and 37 skipped tests in the client. Explicitly
opt-in original-asset, GPU, snapshot and live-account checks remain skipped;
automated suites do not establish live native-client acceptance. The regular
databases remain unchanged. Disposable snapshots were migrated with
`cmd/database-migrate`, including integrity/content verification and retained
backups; the test run used the resulting assets database.

```sh
WONDERLAND_TEST_ASSETS_DB=/tmp/wonderland-prepush-0cb6915/assets.db \
  GOCACHE=/tmp/wonderland-precommit-go-cache \
  go test -race -json -timeout=60m ./...
GOCACHE=/tmp/wonderland-precommit-go-cache go vet ./...
# From client/:
GOCACHE=/tmp/wonderland-precommit-go-cache go test -json -timeout=60m ./...
GOCACHE=/tmp/wonderland-precommit-go-cache go vet ./...
```

### Windows database setup — 2026-11-02

Native Windows asset opening and offline snapshot/integrity checks now use
`file:///D:/...` paths, matching the gameplay store. Without the leading slash,
SQLite rejected the drive letter as an invalid URI authority. The existing
runtime-read and WAL-copy tests reproduced the failure before the fix; their
fixture filenames now also include spaces and `#` to verify URI escaping.

The focused Windows checks pass with the installed MinGW GCC compiler:

```powershell
$env:CGO_ENABLED = '1'
go test ./internal/assetdb ./internal/dbmigration ./cmd/database-migrate -run '^(TestRuntime.*|TestCopy.*|TestUpgradeInPlace.*|Test.*Run.*)$' -count=1
go build -o bin/wonderland.exe ./cmd/wonderland
.\bin\wonderland.exe -config config.local.json -inspect-data
```

The configured databases already use asset schema v16 and gameplay schema v18.
Read-only SQLite integrity and foreign-key checks pass for both. The asset
catalog loads 7,312 items and 1,119 maps; four gacha packs remain disabled for
missing item definitions. Gameplay contains no accounts or characters. Existing
databases and configuration were preserved; no rebuild or migration was needed.
These checks do not establish native-client gameplay acceptance.

### Client dialogue layering — 2026-11-02

The Social Set Up confirmation was drawn as part of the world scene, underneath
ordinary window forms. Normal dialogue now renders after those forms, and clears
the covered control hover so its existing click handler receives the press.
Movie dialogue keeps its scene/zoom rendering path; timed notices remain on top.

The overlap was reproduced with Social, Inventory and Settings. A visual
regression checks opaque dialogue text, buttons and frame pixels while allowing
the skin's intentional transparency, then verifies confirmation clicks retain
the underlying window. The focused Social, dialogue, movie and logout selection
passes on Windows (33.148s). GPU parity passes across 21 states, including the
Social confirmation (4.355s). The generated `social_confirmation.png` was
visually inspected under ignored `var/dialogue-layer-check/`.

From `client/`, with the installed editable assets:

```powershell
$env:CGO_ENABLED = '0'
$env:SNAPSHOT_DIR = '../var/dialogue-layer-check'
New-Item -ItemType Directory -Force $env:SNAPSHOT_DIR | Out-Null
go test ./wlo/app -run '^Test(DialogueAbovePanels|SocialConfirmationSnapshot|FriendDelete|SetUpSave|NPCTalk|Question|NoticeSnapshot|BeachMovieEvent|StormMovieEffects|MonkeyRescueMovies|MonkeyMovieDepthAndIllustration|SnowMovie|.*Logout.*)$' -count=1 -timeout 4m
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/app -run '^TestGPUClientParity$' -count=1 -timeout 5m
```

### Client right-click pose facing — 2026-11-02

Right-clicking the scene now turns a stationary pose toward the cursor and sends
the changed action through AC32:2. Window right-click actions retain priority.
The native `FUN_0043b6ac` instructions at `0x43b702`–`0x43b770` were inspected
directly in the WLRI `aLogin.exe` with SHA-256
`ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`.
They call the facing helper, collapse two-facing actions to halves, add the
current action family and invoke the held-pose routine. The native source and
independent wire fixtures remain unchanged.

The focused client checks pass on Windows (app: 20.169s; world: 0.441s). They
cover all eight sitting directions, camera offsets and off-center avatars,
two-facing lying/stance/action families, alternate sitting, standing, unchanged
facings, independent expressions and blocked input states. Expected AC32 bytes
are independent literals. The regular databases and asset sources are untouched.

From `client/`:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/app ./wlo/world -run '^Test(RightClickPose.*|EmoteTab|TurnAction|DirectionPoint|Facing|NativePoseBatchAndMalformed)$' -count=1 -timeout 3m
go build -o ../bin/wonderland-client.exe .
```

Client catalog regeneration was attempted with
`py tools/decompile/catalog.py --exe ../Wonderland-Client/aLogin.exe`, but the
local `var/decompiled/reference/index.json` input is absent. The generated
inventory/catalog were preserved. Native interactive acceptance remains pending.

The decompilation inputs were subsequently restored under `var/decompiled`,
with the matching WLRI executable under `var/ghidra`. Regeneration now succeeds:
12,130 functions, 937 classes, 228 regions, 901 cited functions and 29/91 receive
commands with a client branch. The new citation includes the right-click pose
routine `FUN_0043b6ac`, whose body and main-form callers are present. The
generator writes UTF-8/LF output and portable source paths on Windows. These
inventory figures describe reference coverage, not feature completeness.

### Ship Deck movie completion - 2026-11-02

The captain's storm movie requires the AC186 handshake before AC20:6 completion.
The server's original gate and raw-byte regression have been restored. The fix
is in the client: AC186:12 echoes its identifier in AC186:9, and the server's
AC186:9 reply is consumed without echoing or completing playback. There are no
map/character/movie-ID exceptions or changes to asset definitions or databases.

The client storm regression exercises the complete exchange and checks the
ordered outgoing packets `[186,9,1,0]`, then `[20,6]`. Separate protocol fixtures
cover other identifiers, loading without a world, opaque trailing fields,
truncated/oversized packets, unsupported subcommands and reply-loop prevention.
The server regression retains its premature-completion rejection and covers the
persisted beach destination and rescue cleanup after the required handshake.

From the repository root, then from `client/` respectively:

```powershell
$env:CGO_ENABLED = '1'
go test -race ./internal/server -run '^Test(StormAndBeachRescue|StarterShipSceneTransitions|NativeStarterShipCabinTransitions|NativeSynchronizationGolden)$' -count=1 -timeout=5m

$env:CGO_ENABLED = '0'
go test ./wlo/app -run '^Test(MovieHandshakeWire|MovieHandshakeMalformed|StormMovieEffects|BeachMovieEvent|MonkeyRescueMovies)$' -count=1 -timeout=5m
```

The earlier server-only workaround is superseded by this client fix. Rebuild
the client and keep the server's original handshake requirement. The owner
subsequently confirmed that the Ship Deck story now works properly.

### Following pet emotes and resting recovery - 2026-11-02

Supported companion sprites mirror owner poses for local and remote players;
unsupported animations retain normal following/standing. Native
`FUN_00430680` supplies MoveKind 0's four-facing sitting conversion, verified
against the real Roca sprite. The real-sprite integration also checks ordinary
AC32 owner requests and native AC8:2 pet HP/SP updates. Sitting and lying
screenshots under ignored `var/companion-rest-check/` were visually inspected.

Player and active-pet recovery remains 2% of their individual equipped HP/SP
maxima, minimum one, every three seconds. Focused server/store tests pass under
the race detector, covering a full owner with an injured pet, inactive pets,
caps, interaction/loading/battle gates, delayed checkpoints, rollback/retry,
stale pet vitals, clean checkpoints and SQL-result merges. Recovery checkpoints
update only state/vital columns; triggers reject inventory, pet and skill-row
replacement. No schema or installed-data changes are required.

The existing GPU/CPU full-client parity selection also passes. The asset compiler
validated and reused all 72,829 entries (6,017.4 MiB of source); both Windows
executables were rebuilt. The superseded server-workaround executable was removed.

From the repository root:

```powershell
$env:CGO_ENABLED = '1'
go test -race ./internal/server ./internal/store -run '^Test(Resting.*|SavedTransactionPreserves.*|Autosave.*|PositionCheckpoint.*|CleanCheckpointSkipsSQL|BufferedMovementFullCommitAdoptsCheckpointCoordinates)$' -count=1 -timeout=5m
```

From `client/`:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/world ./wlo/app -run '^Test(Companion.*|RightClickPose.*|AltOneKneelsAndSendsNativePose|NativePoseBatchAndMalformed|EmoteTab|TeamPet.*|MovieHandshakeWire|MovieHandshakeMalformed)$' -count=1 -timeout=4m
```

### Generated client exports stay local - 2026-11-02

Client-derived files under `data/` and the generated client resource catalogs/
PNG dimension baseline are ignored and removed from Git tracking while local
copies remain. Server-only definitions, documentation, exporter code, authored
compatibility policies and fixtures remain tracked. Historical Git/LFS objects
are unaffected.

`regenerate.py` now exports native client tables as well as media, requiring an
explicit `--client` installation path. It builds a complete manifest
using copies of the repository-owned server exports, without reading an original
Private Server installation. The importer restores missing ignored JSON as well
as media and preserves existing local files. Source-path comparisons support
both Windows and POSIX paths.

Focused Python exporter/restoration tests pass (20 tests; the existing symlink
case skips where Windows lacks symlink privileges):

```powershell
$env:PYTHONUTF8 = '1'
& ./var/asset-import-venv/Scripts/python.exe -m unittest discover -s tools/data_export -p 'test_import_untracked.py' -v
& ./var/asset-import-venv/Scripts/python.exe -m unittest discover -s tools/data_export -p 'test_export.py' -v
```

The resource-catalog and first-build dimension-contract regressions also pass:

```powershell
$env:CGO_ENABLED = '0'
go test ./cmd/client-assets ./internal/clientbundle -run '^Test(StagePackageAndVerify|Build.*)$' -count=1 -timeout=3m
```

The index audit excludes 545 generated paths (1,324,409,012 local bytes). Their
local lengths and modification times remain unchanged; all 21 server-only
exports reconstruct against their preserved source hashes without Private Server
originals. No asset regeneration or database reset is performed by this change.

### Sprite extraction across Windows drives - 2026-11-02

An actual backed-up client-asset rebuild exposed a containment check that treated
`filepath.Rel` failing across Windows volumes as an extraction failure. The
exporter's temporary directory and original installation may reside on separate
drives. Both sprite-export entry points now allow distinct volumes while still
rejecting the source directory and its descendants on the same volume, including
drive-letter case changes. Existing pixel, provenance and edit-preservation
fixtures remain unchanged.

`go test ./cmd/sprite-export -count=1 -timeout=3m` passes on Windows, including
the cross-volume and containment regressions.

The complete local rebuild then passed: SHA-256 backup verification covered
26,036 files (5,742,968,012 bytes), and native verification reconstructed 40
source resources. Verification covered 19,717 audio tracks, 6,257 sprites,
816,578 frame records and every editable sprite pixel/placement, plus 3,796
media sources. Picture export published 16,168 images after its own atlas checks.
The 121 unresolved custom media formats and 18 missing-companion records match
the backed-up source exports. Their retained bytes/metadata remain available.

After installing the verified tree while preserving tracked server definitions
and guides, the bundle compiler processed 72,829 files (71,320 reused) and the
Windows client executable rebuilt. Installed `cmd/asset-build` verification,
the focused dialogue/movie/companion selection and
`TestCompiledBundleLoginAndSharedWorld` passed. The redundant rollback copy was
removed only after all its files matched the independent backup; the original
backup and SHA-256 manifest remain under ignored `var/backups/`.

### Windows pre-push validation - 2026-11-02

Full server race tests with the installed assets database, full client tests and
both modules' `go vet` checks pass. Authenticated sprite tests used the explicitly
selected matching installation through `WONDERLAND_SPRITE_CLIENT`. The server
package's uncached run took 984.989s; the unchanged package was reused on the final
full-suite run after correcting other packages' Windows checks.

The checks exposed a rooted PNG-path validation gap on Windows: editable sheet
names now require a local relative path and reject POSIX-rooted names on every
platform. Database-role tests use a hard-link alias when Windows cannot create
symlinks. POSIX file-mode assertions run only where those mode bits are supported;
profile-content and fixture-isolation assertions still run on Windows. A migration
fixture now closes its final reopened database before temporary-directory cleanup.

Original screenshot comparisons skip only when their external capture file is
absent. Existing captures still use the same pixel assertions, and other read/
decode errors remain failures. No replacement reference images were generated.
An old ignored temporary Go source snapshot was renamed to a non-Go suffix to
keep it out of normal package discovery.

### Independent companion pose facing - 2026-11-02

Following pets mirror the owner's pose family using their own facing, retained
across lying and sitting variants. Owner turns leave that facing unchanged for
local and peer companions. Exact sprite frames take precedence over the native
four-frame sitting fallback, regardless of the NPC MoveKind flag. The installed
Roca sprite has only sitting actions 47, 49, 51 and 53; this is confirmed by both
the native action directory and the editable frame table.

The focused selection below passes, including all eight pet facings, missing
pose fallback, spawning during a pose, movement resumption and the rendered Roca
integration with independent raw AC32 requests. Affected-package vet also passes.
The client port catalog was regenerated with no resulting changes. No server
behavior, database state or sprite artwork was changed.

From `client/`, on Windows:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/world ./wlo/app -run '^Test(Companion.*|RightClickPose.*|NativePoseBatchAndMalformed|AltOneKneelsAndSendsNativePose|TeamPet.*)$' -count=1 -timeout=4m
go vet ./wlo/world ./wlo/app
```

### Daily Lucky Draw client - 2026-11-02

The client implements the existing AC104:1 catalog, spin and result contract
with native form artwork, a reward preview and the 2800 ms presentation spin.
Independent raw packets cover ordered rewards, cumulative usage, the winning
slot and ordinary AC23:5 inventory delivery. Malformed catalogs/results leave
state unchanged. Tests cover all twenty slots, empty catalogs, duplicate clicks,
closing/reopening while pending, cancellation before send, full-inventory
refusal, unrelated banners, missing/late replies, send failures, server midnight
refreshes, interaction gates, character changes and disconnects. No server or
database changes are required.

Focused tests and affected-package vet pass. GPU/CPU parity now includes the
draw result and twenty-reward preview (23 stages), passing with loose exports
and compiled packs. The compiled-bundle smoke test also verifies the native
artwork and independent Lucky Draw state between sessions sharing resources.
Result/preview captures under ignored `var/lucky-draw/` were visually inspected;
these are implementation captures, not original-client reference baselines.
The regenerated port catalog records AC104 receive coverage.

From `client/`, on Windows:

```powershell
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/lottery ./wlo/app -run '^Test(LuckyDraw.*|GPUClientParity|Companion.*|RightClickPose.*|NativePoseBatchAndMalformed|Lost.*)$' -count=1 -timeout=5m
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/lottery ./wlo/app -run '^Test(LuckyDraw.*|CompiledBundleLoginAndSharedWorld|GPUClientParity)$' -count=1 -timeout=5m
go vet ./wlo/lottery ./wlo/app
```

### Battle contact timing and departures - 2026-11-02

The displayed battle roster applies HP/SP changes at authored MBTM contact cues,
including multiple hits, healing, critical damage, misses and capture. Independent
wire fixtures verify that subsequent AC51 absolute stats and AC53 defeat packets
cannot move the displayed result ahead of the hit. Player and pet HUD vitals use
the same presentation; command validation retains the immediate decoded roster.

Victory and flee fixtures cover AC11:1 departures followed by an early AC11:0,
with the last attack finishing before motion 60041 and the map fade. New formations
cancel old departure state. Native asset checks verify outward positions on both
halves and that departure omits the return leg without modifying the source
motion. Existing battle, Remote and pet HUD checks pass. CPU/GPU parity adds
seven battle/contact/departure/fade stages (33 total). Focused checks pass with
loose exports and compiled packs; affected-package vet passes. Departure captures
in ignored `var/battle-presentation/` were visually inspected; the turn countdown
is cleared during departure. No server packets or authored motion data changed.

From `client/`, on Windows:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/battle
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/app -run '^Test(Battle.*|PetStatus.*|Remote.*|GPUClientParity)$' -count=1 -timeout=5m
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(BattleVitalsAtContact|BattleMultiHitDefeatAtFinalContact|BattleCaptureAndMissAtContact|BattleDepartureWaitsForFinalAttack|BattleNewFormationCancelsDeparture|BattleTransitions|GPUClientParity|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/battle ./wlo/app
```

### Active-pet HUD - 2026-11-02

The player status panel now includes the compact native pet portrait, level,
HP/SP bars and experience bar/percentage. Layout and portrait anchors were
traced through `FUN_0029d3a4` and `FUN_0029dbf0` and checked against the supplied
legacy screenshot. The existing 007-family icon path retains template colors;
Teams' top-left icon placement is unchanged. The view reads the active pet's
current roster entry rather than Inventory's local selection.

Focused tests pass for independent layout pixels, live HP/SP, overfull-bar
clamping, active-pet changes, Rest, companion snapshots and removed roster entries.
Existing Teams portrait/action and companion-emote checks pass. CPU/GPU parity
now covers pet HUD display, updated stats with a numeric tooltip, and Rest
(26 total stages), including compiled packs. Affected-package vet passes.
An implementation capture at ignored `var/pet-status.png` was visually inspected;
no original-client reference baseline was replaced. The client catalog was
regenerated. No server packets, persistent data or artwork changed.

From `client/`, on Windows:

```powershell
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/hud ./wlo/role ./wlo/app -run '^Test(PetStatus.*|GPUClientParity|TeamPetActionsAndAuthoritativeReplies|Team.*Portrait.*|CompanionEmoteAndRecoveryIntegration|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/hud ./wlo/role ./wlo/app
```

### Battle reveal, audio and zoom - 2026-11-02

The existing map darken now continues into a 15-step battle-field reveal.
Brightness checks cover both halves of entry and the existing exit fade;
queued actions, their sounds, command menus and turn timers wait for reveal.
The reveal continuation is a requested presentation change, separately identified
from the native transition evidence.

Native MBTM camera points and fixed sound tables are checked against independent
expected values. Tests verify general/weapon/body cue ordering, once-only playback,
attacking-weapon ownership of reaction sounds, native voice roulette, player versus
companion voice groups, and actual WAV/OGG decoding from loose and compiled packs.
Camera checks cover authored paths, holds and field bounds. Zoom follows horizontal
attack progress, respects the persisted Zoom Mode setting, restores the full view
after the action and releases its renderer target on exit. No source audio, motion
tables, protocol bytes, server logic or database state changed.

Focused battle/settings/Remote/pet/dialogue regressions and affected-package vet
pass. CPU/GPU parity now checks 37 stages, including the camera/zoom at contact
and multiple reveal frames, with loose exports and compiled packs. The zoom
capture in ignored `var/battle-media/zoom.png` was visually inspected: the scene
enlarges while the HUD keeps its normal size. This is an implementation capture,
not a replacement native reference. Remaining native phase, parallax and weapon
resolver limits are recorded in [battle parity](BATTLE_CLIENT_PARITY.md).

From `client/`, on Windows:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/battle ./wlo/skills ./wlo/settings
go test ./wlo/world -run '^Test(NPCTemplates|PropSound|Companion.*)$' -count=1 -timeout=3m
$env:WONDERLAND_TEST_GPU = '1'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(Battle.*|Settings.*|PetStatus.*|Remote.*|DialogueVoicesAndOGG|GPUClientParity|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/battle ./wlo/skills ./wlo/settings ./wlo/world ./wlo/app
```

### Round damage, overkill removal and spawn return - 2026-11-02

Native AC53:3 selects the upward removal state, not an ordinary knockout.
The old server emitted it for every death. Tests now distinguish ordinary
lethal damage, low remaining HP, exactly 150%, strictly above 150%, odd maximum
HP, cumulative hits, healing, round resets and overflow-safe combo totals.
Independent raw packets verify only overkills send AC53:3, and simultaneous
area removals share one 300 ms flight. Existing combo damage goldens remain;
the ordinary-knockout fixture now explicitly forbids the removal packet.

Server tests cover Starter Beach and saved record-point returns, invalid point
fallback, atomic saved vitals/pet results/location, checkpoint adoption, save
failure, map-load control restoration and one member leaving mid-round while
teammates receive later actions. Old-map peers receive the committed departure.
Disconnecting during the flight also commits the return destination, without
attempting to send a warp to the closing connection.
The focused removal and disconnect checks pass under the race detector.
No schema migration or development database rebuild is needed.

Client tests cover retained ordinary corpses, contact-before-flight timing,
removal during another unit's action, duplicates, early scene close, revival
exclusion, cleared hit boxes and reinforcement identity reuse. Remote counts
ordinary knockouts from authoritative HP changes instead of depending on the
overkill marker. Hotbar fixtures wait for the battle entry fade before exercising
their unchanged raw request goldens. CPU/GPU parity includes the native overkill
flight and disappearance (42 stages). The implementation capture at ignored
`var/overkill/flight.png` was visually inspected. Affected-package vet passes.

From the repository root, on Windows:

```powershell
go test ./internal/battle ./internal/protocol
go test ./internal/server -run '^Test(Overkill.*|QuestBattle.*|TeamBattle.*|.*Battle.*Reinforcement.*|.*Battle.*Capture.*|.*Battle.*Disconnect.*|.*Battle.*Skill.*)$' -count=1 -timeout=5m
$env:CGO_ENABLED = '1'
go test -race ./internal/battle ./internal/server -run '^Test(Overkill.*|TeamBattleFleeAndDisconnect)$' -count=1 -timeout=5m
go vet ./internal/battle ./internal/protocol ./internal/server
```

From `client/`:

```powershell
$env:CGO_ENABLED = '0'
go test ./wlo/battle
go test ./wlo/app -run '^Test(BattleOverkill.*|BattleOrdinaryKnockoutRemainsOnField|Hotbar.*|RemoteBattle.*)$' -count=1 -timeout=4m
$env:WONDERLAND_TEST_GPU = '1'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(Battle.*|RemoteBattle.*|GPUClientParity|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/battle ./wlo/app
```

### Negotiated combat round transport - 2026-11-02

Independent raw fixtures cover capability negotiation, choice batches, complete
round envelopes and framing. Bounds/truncation checks cover incomplete bodies,
out-of-order/overlapping chunks, oversized declarations and nested extensions.
Server tests verify unchanged native handshake replies, registry gates, mixed
legacy/Gonline delivery, the complete chain before the first wait, one atomic
owned choice batch, rejected foreign/duplicate actors, stale tokens, actual
round execution and renewed prompts. Skill progress survives native fallback
without double grants; failed saves publish no chain. Owner overkill truncates
its chain and saves its spawn while teammates continue. Focused negotiation,
playback, overkill and disconnect checks pass under the race detector.

Client checks cover opt-in/version fallback, connection reset, one request for
character plus pet, partial timeout choices, sequential contact damage, combo
playback, overkill/reinforcement ordering, stale chains, deferred readiness and
map changes, and validation before any damage is applied. Legacy battle/hotbar/
Remote regressions pass. With compiled packs, CPU/GPU parity checks 46 stages,
including queued full-round contact and release of the next prompt. Affected
server and client package vet passes. No schema/asset rebuild is required.
These are automated compatibility checks, not live original-client acceptance
or load/throughput measurements.

From the repository root:

```powershell
go test ./internal/protocol ./internal/server -run '^Test(CombatRound.*|GonlineCombat.*|CommandRegistry.*|LoginWireFlow|MigrationGenericAcknowledgements|.*BattleSkill.*|.*BattlePetSkill.*)$' -count=1 -timeout=5m
$env:CGO_ENABLED = '1'
go test -race ./internal/protocol ./internal/server -run '^Test(CombatRound.*|GonlineCombat.*|Overkill.*|TeamBattleFleeAndDisconnect|LoginWireFlow|MigrationGenericAcknowledgements)$' -count=1 -timeout=5m
go vet ./internal/protocol ./internal/battle ./internal/server
```

From `client/`:

```powershell
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_GPU = '1'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(GonlineCombat.*|Battle.*|Hotbar.*|RemoteBattle.*|RemoteAuto.*|GPUClientParity|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/battle ./wlo/app
```

### Battle victory voices and single-click skills - 2026-11-02

Victory audio follows native `FUN_00395f84`: winning-side candidates, living
local-owner gate, one selected speaker, 10/20/30% `_bta3/2/1` variants and 40%
silence. Tests decode the actual Ogg recordings, check roulette boundaries,
final-action timing, duplicates, early scene close, pet speakers, losing/fallen
owners and deferred Gonline endings. The server's AC11:12 operand now names the
actual winning side; independent raw packets cover attacker/defender wins and
native PvP/quest settlement. The ordinary PvP fixture explicitly avoids an
overkill so its loser remains present for settlement.

Real UI press/release tests cover single-click targeting plus the unchanged raw
AC50 request, dragging onto the hotbar plus its unchanged AC40 request, and
out-of-battle browsing/double-click behavior. Focused battle, sound, skill,
hotbar and negotiated-playback checks pass with exported and compiled assets;
affected-package vet passes. No asset/database rebuild is needed. Native knockout
recordings were identified separately; a dedicated whole-battle defeat line was
not found in the traced finish/departure handlers.

```powershell
# Repository root
go test ./internal/battle ./internal/server -run '^Test(Departure.*|NativePvP.*|QuestBattle(Victory|Defeat)|GonlineCombat.*)$' -count=1 -timeout=4m
go vet ./internal/battle ./internal/server
# client/
$env:CGO_ENABLED = '0'
go test ./wlo/skills ./wlo/app -run '^Test(Battle.*|GonlineCombat.*|Hotbar.*|SkillsOutsideBattle.*)$' -count=1 -timeout=5m
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(BattleVictory.*|BattleSkill.*|BattleVoice.*|BattleAdditionalSound.*|GonlineCombatVictory.*|SkillsOutsideBattle.*|HotbarDrag.*)$' -count=1 -timeout=4m
go vet ./wlo/skills ./wlo/app
```

### Combat disconnect settlement - 2026-11-02

Race-enabled regressions cover withdrawal of owner/pet and pending choices,
exclusion from subsequent targeting/damage, and continued combat for an online
teammate after its leader disconnects. Native and chain cases calculate a real
damage/SP-spending round, pause playback, disconnect the owner, and verify SQL
already holds the final round vitals plus pet HP/SP and one amity penalty.
Removal records follow committed playback and precede the next prompt.

Concurrent last-player disconnects save each owner and stop playback. A failed
SQL save retains an immutable snapshot and baseline; retry commits once without
publishing a failed pet change. A disconnect during a committed defeat departure
does not overwrite defeat recovery or repeat the pet penalty. A knocked-out
remaining teammate receives a defeat rather than an impossible command phase.
Existing escape, overkill/spawn, PvP forfeit, item, encounter-reservation and
logout/autosave regressions are included. No asset or database rebuild is needed.

```powershell
go test -race ./internal/battle ./internal/protocol -count=1 -timeout=4m
go test -race ./internal/server -run '^Test(BattleDisconnect|Disconnect|.*Disconnect.*|PvP.*|NativePvP.*|.*Overkill.*|.*Escape.*|GonlineCombat.*|BattleItem.*|BattlePet.*|TeamBattle.*|Wild.*|WorldSimulation.*|BufferedMovementPurchaseAndDisconnect|FailedBattleResult.*)' -count=1 -timeout=6m
go vet ./internal/battle ./internal/server
```

### Battle item use - 2026-11-02

Slow-choice and dual-item regressions: the native `E119` turtle sheet is rendered
with four 250 ms frames starting at ten seconds remaining. Tests cover both turn
limits, own/peer submission receipts, pet indicators, hidden/dead/waiting fighters,
playback, expiry and next-round reset. The actual exported sheet is rendered and
checked; `BATTLE_SLOW_SNAPSHOT` optionally saves a battle screenshot.
The reported Spicy Hot Pot (32176) scenario has player and pet target the pet from
the same stack of 50. Client tests include native receipts between selections;
server race tests verify the round executes and durably consumes exactly two
items, for full and injured pets in native and complete-round transports. This
depends on the full-vitals selection fix above; binaries built before that fix
must be rebuilt and restarted. Tests did not restart the running development server.

```powershell
# root
go test -race ./internal/server -run '^TestBattleItem' -count=1 -timeout=3m
# client/
go test ./wlo/app -run '^Test(BattleSlow|BattleItem|BattleTimer)' -count=1 -timeout=3m
```

Item-target corrections also pass focused tests for the native magic cursor,
clicking all eight allied player/pet formation cells at full vitals, recovery
after damage received between selection and execution, and Bomb enemy selection
with ally rejection. Independent native item projections cover Bomb, Strong Bomb,
Darts and both shuriken variants. Full-vitals selection follows the decompiled
`FUN_0038972c` and its eligibility helpers; no live legacy-server capture was made.

Verified the native AC50:2 builder directly in the matching aLogin executable
(`FUN_002c2394`, branch `0x002d4247`–`0x002d44ed`), including item ID,
opaque trailer, bag slot and compatibility flag. Subsequent live legacy-server
and legacy-client-to-Gonline captures on 2026-11-03 established the additional
40-byte send-path trailer and mode-zero recovery. See
[the capture findings](BATTLE_ITEM_CAPTURE.md). Raw captured 52-byte requests
are replayed through the engine and server, including player/pet item use from
one stack and durable count reduction. Client regressions verify the captured
mode-zero SP recovery at contact; existing 12-byte goldens remain unchanged.
The Grape Syrup/Nian Noodle screenshot references also verify the outlined
cyan SP and orange HP sheets. Item contact tests cover both sheets, including
mode-zero HP recovery on a pet recipient; HP recovery no longer uses plain text.

Focused tests pass for native request/result goldens, simultaneous HP/SP recovery
at contact, revival, creature motions, pet commands, speed ordering, unclamped
overkill damage, invalid/stale/locked selections, last-stack reservations, SQL
failure and cross-owner pet rollback. Native and negotiated round delivery each
emit one private consumption receipt; completed choice-token replay has no effect.
Actual UI double-click, item shortcuts, Escape cancellation, turn reset and
compiled motions 10024/10025/15138 pass. Existing eight-byte choice goldens remain
unchanged; mixed eight/twelve-byte batches have separate independent goldens.

Affected battle/protocol suites and server combat integrations pass under the
race detector. Client battle, hotbar, inventory and Gonline combat regressions
pass with the compiled asset bundle. Affected-package vet passes. No database
migration, asset rebuild or live server restart was performed.

```powershell
# Repository root
go test ./internal/assets -run '^TestNativeBattleItem' -count=1
go test -race ./internal/battle ./internal/protocol -count=1 -timeout=4m
go test -race ./internal/server -run '^Test(BattleItem|GonlineCombat|BattlePet|CombatPet|.*Escape.*|.*Overkill.*)' -count=1 -timeout=5m
go vet ./internal/assets ./internal/battle ./internal/protocol ./internal/server
# client/
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app ./wlo/battle ./wlo/inventory -run '^Test(Battle.*|GonlineCombat.*|Hotbar.*|Inventory.*)$' -count=1 -timeout=5m
go vet ./wlo/app ./wlo/inventory ./wlo/battle
```

### Multi-attribute allocation encoding - 2026-11-03

The client formerly packed two-byte amounts in counted AC8:1 entries. The
reference-compatible server decoder consumes four bytes whenever available,
which swallowed the following stat ID and made multi-attribute allocations
unaffordable or incomplete. The client now writes four-byte amounts for each
entry. Server parsing behavior and independent migration goldens are unchanged.

Focused tests cover all 31 nonempty attribute subsets with different amounts,
passing generated client requests through the actual server decoder and game
allocation rules. A server integration test verifies STR/WIS increases and point
deduction in SQL, unchanged current HP/SP, and the authoritative points reply.
Existing client budget, duplicate-submission, confirmation, timeout and reset
checks also pass.

```powershell
# root
go test -race ./internal/game ./internal/server -run '^Test(StatAllocation.*|MigrationAllocationWidthsAndBatches|AllocationUnlockCommitsWithStats|FailedAllocation.*)$' -count=1 -timeout=3m
# client/
go test ./wlo/inventory ./wlo/app -run '^Test(InventoryPoint.*|NativePresentationAndAllocation)$' -count=1 -timeout=3m
```

### Post-combat results message - 2026-11-02

The client snapshots player/pet progression at formation and accumulates only
confirmed native updates through the battle close and final map/stat refresh.
Tests cover EXP/level/amity/gold changes, repeated absolute EXP receipts,
aggregated material receipts, consumed stock, malformed inventory packets,
losses, no empty popup, departed/replaced pets, long-list scrolling, expiry at
six seconds even while hovered, panel/text clicks retaining the timed notice
without world click-through, scrollbar navigation, disconnect/character
isolation, and queued Gonline settlement.
The summary makes no resource changes or requests; wire assertions preserve the
existing additive item packets. No server rule, protocol, database or asset
format changes are required.

Focused results and affected battle/Remote/compiled-bundle checks pass with
compiled packs. CPU/GPU parity now checks 50 stages, including post-fade results,
long reward lists and the final scrolled entries. The actual implementation
capture at ignored `var/battle-results/summary.png` was visually inspected: all
player, pet, gold and material rows fit and are readable. This extends the native
six-second message with a bounded, scrollable list; it does not claim the remaining
animated level-up/capture artwork. Affected-package vet passes.

From `client/`:

```powershell
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
go test ./wlo/app -run '^Test(BattleResults.*|GonlineCombatResults.*)$' -count=1 -timeout=4m
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/app -run '^Test(Battle.*|GonlineCombat.*|RemoteBattle.*|GPUClientParity|CompiledBundleLoginAndSharedWorld)$' -count=1 -timeout=5m
go vet ./wlo/app ./wlo/inventory
```

The results notice now matches the supplied legacy example: immediately below
the top command icons (Y=34), sized to its centered yellow English lines, without
a title or close button. Five visible rows retain wheel/rail scrolling for longer
reward lists, and stale loot notices are replaced so they cannot cover the result.
Focused `BattleResults.*`, `GonlineCombatResults.*` and `GPUClientParity` checks
pass with compiled assets; the three-line English reference capture at
`var/battle-results-reference.png` and the longer summary at
`var/battle-results-summary.png` were visually inspected. Affected-package vet passes.

### Battle modules, pet switches and idle timeouts - 2026-11-02

Native Team evidence (`FUN_00277728`) gates Rest/Battle requests on the player's
unconsumed command and closes selection without an AC50 Defend. Tests open
Social, Team, Inventory, Skills, Compound and Settings under real AC20:12 and
AC6:2 battle locks while retaining gameplay restrictions. Raw requests verify
pet changes send only AC19; Ride/dismiss and repeated/late commands stay blocked.
Empty timeouts send nothing, partial Gonline selections flush only their original
native AC50 request, and explicit Defend remains distinct from idle damage.

Engine/server checks cover both formation sides, independent native participant
goldens, outgoing HP/SP/amity persistence, switching back without refilling,
capture settlement isolation, transactional save failure, ineligible pets,
already-submitted/dead players, ordered exit/join playback in both transports,
next-turn readiness, and selection racing the turn timeout. The battle engine
suite and focused server race checks pass. Affected client battle/transport,
hotbar/Remote and Team checks pass with compiled assets; 53-stage CPU/GPU parity
includes the three newly open battle modules. The actual Team-over-battle capture
at ignored `var/battle-team.png` was visually inspected. Affected-package vet
passes. No schema, assets or capability-version changes are needed.

```powershell
# Repository root
go test ./internal/battle -count=1 -timeout=2m
$env:CGO_ENABLED = '1'
go test -race ./internal/server -run '^Test(BattlePet.*|GonlineCombat.*|CommandRegistryCompatibility|NativePvPEntryActionsSettlementAndPersistence|QuestBattle(Victory|Defeat))$' -count=1 -timeout=4m
go vet ./internal/battle ./internal/game ./internal/server
# client/
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/app -run '^Test(BattleModules.*|BattleTeam.*|BattleTimeout.*|GonlineCombatTimeout.*|GPUClientParity)$' -count=1 -timeout=3m
go test ./wlo/app -run '^Test(BattleTimeout.*|GonlineCombat.*)$' -count=1 -timeout=3m
go vet ./wlo/app ./wlo/team
```

### Pet-switch animation sequencing - 2026-11-02

Regression fixtures reproduce exit, replacement and attack packets arriving in
one native TCP burst or a zero-delay round chain. Both paths preserve the old
pet until departure completes, animate the new pet entering its mirrored cell,
and hold attacks/contact damage, readiness and settlement until entry finishes.
Tests retain queued gold/items across scene close, avoid duplicate rewards,
reset transitions on a new formation, and advance background sessions without
drawing. Initial-roster fixtures now put their pets before the first command
prompt rather than introducing them as mid-battle arrivals.

The entry controller is traced through `FUN_0039108c` / `FUN_00378d80`; its
80-pixel horizontal projection is a documented compatibility approximation.
Focused battle/transport/input checks and motion tests pass; affected-package
vet passes. CPU/GPU parity covers 60 stages, including departure, entry and
the first subsequent attack contact. Actual sprite captures under ignored
`var/battle-pet-switch/` were visually inspected. No server/protocol change or
asset rebuild is required.

```powershell
# client/
$env:CGO_ENABLED = '0'
$env:WONDERLAND_TEST_CLIENT_BUNDLE = (Resolve-Path ../bin/client-assets.zip).Path
$env:WONDERLAND_TEST_GPU = '1'
go test ./wlo/battle -count=1 -timeout=2m
go test ./wlo/app -run '^Test(BattlePet.*|GonlineCombat.*|GPUClientParity)$' -count=1 -timeout=3m
go vet ./wlo/battle ./wlo/app
```

### Native pet-switch capture correction - 2026-11-02

The preceding client animation fix did not correct the server's native switch
packets. A completed round on the legacy server and a completed round from aLogin
through a transparent Gonline relay exposed these differences:

| Packet/state | Successful legacy server | Gonline before correction |
| --- | --- | --- |
| Owner AC53:5 | Before exit/selection | After selection |
| Outgoing AC11:1 | Four bytes | Five bytes |
| Owner AC19 receipt | Between exit and participant | Before exit |
| Incoming AC11:5 entry mode / owned slot | 5 / 2 | 1 / 0 |
| Immediate incoming HP/SP and AC53:5 | Absent; vitals are in participant | Sent |
| First attack after participant | About 342 ms | Under 1 ms |
| Incoming pet targeted during switch round | Unavailable | Targeted by second enemy |

`FUN_0039108c` assigns the mode-5 unavailable flag, `FUN_0038ca44` excludes it
from drawing, targeting also rejects it, and `FUN_0039783c` resets it on AC52:1.
The server now preserves this native order and owned slot, keeps selection
receipts private in both transports, and excludes joining pets from action/direct
target/area target/AI selection for that round. Joining pets still count as living
for the battle outcome if their owner is knocked out. A documented 350 ms floor
separates the switch batch from actions; this single observed interval does not
establish every native animation's duration.

Independent raw goldens reproduce the actual reference participant bytes.
Regressions cover both formation sides, next-round targeting, owner knockout,
transaction failure, mixed-client receipt isolation and the selection/timeout
race. Focused battle/server checks pass under `-race`. Client fixtures exercise
both transports through departure, the hidden waiting state, current-round
attacks and readiness-triggered entry. Compiled-asset snapshots of waiting and
next-turn entry were visually inspected. CPU/GPU parity passes all **67** stages;
affected server/client vet and separate executable builds pass. The running
server was not restarted, so a manual legacy-client retest of the rebuilt server
remains acceptance work.

The combat-only diagnostic traces and snapshots remain ignored under
`var/pet-switch-capture/`. The relay is stopped, temporary PktMon filters are
removed, and the legacy client's original server list is restored.

```powershell
# repository root
go test -race ./internal/battle ./internal/server -run 'Test(BattlePet|Timeout|GonlineCombat|NativePvP|Overkill|Formation|Target|Monster|Reinforcement)' -count=1 -timeout=3m

# client/; compiled assets available as documented above
go test ./wlo/battle ./wlo/app -run '^Test(Battle.*|LegacyPetSwitch.*|Round$|GonlineCombat.*|Hotbar.*|RemoteBattle.*|GPUClientParity)$' -count=1 -timeout=3m
```

### Concurrent pet presentation and opt-in round transport - 2026-11-02

This supersedes the preceding hidden incoming-pet presentation while retaining
switching-round protection. Gonline retains the departing pet's fighter and body as a separate
visual copy while the incoming pet walks onto the field. Ground entry and exit
run concurrently in native bursts and round chains; attacks and settlement wait
for both tracks. The incoming pet stays visible while its native Waiting flag
blocks choices and targeting until next-round readiness. Readiness does
not replay entry. Area coverage excludes the joining pet, for both whole-side
and authored offset patterns, matching direct/AI primary-target protection.

`TestBattlePetSwitchConcurrentDepartureAndArrival` verifies independent bodies,
overlapping movement, ground travel, delayed damage and no extra requests.
`TestBattlePetLegacyWaitingModeAcrossTransports` plays an attack against the
owner while the visible incoming pet stays protected, then enables choices without repeating
entry. Area-attack tests verify that damage excludes the joining pet and can hit
it after the switching protection ends. New-formation and queued-settlement regressions remain covered. Compiled
sprite snapshots under ignored `var/pet-switch-concurrent/` were visually
inspected; CPU/GPU parity passes all 67 stages with simultaneous actor checks.

`combat_round_chain` is a startup-file boolean, default false when omitted.
Configuration tests cover omitted/false/true. Server regressions check captured
startup policy, suppressed offers while disabled, rejected unsolicited or stale
acceptance, explicit negotiation when enabled, and native playback for clients
that do not negotiate even with the switch enabled. Focused battle/server race
checks include area attacks, pet switching, negotiation and native login.
Native pet-only prompts also open the owner's next turn after a player knockout;
foreign-pet prompts and duplicate readiness cannot open additional turns.

```powershell
# repository root
go test ./internal/config -count=1
go test -race ./internal/battle ./internal/server -run 'Test(BattlePet|Area|GonlineCombat|LoginWireFlow)' -count=1 -timeout=3m

# client/; compiled assets / graphics environment as above
go test ./wlo/app -run '^Test(BattlePet.*|GPUClientParity)$' -count=1 -timeout=3m
```

### Owner/pet escape scope and native failure - 2026-11-02

An explicit player or pet escape now withdraws its owner/pet pair and discards
their remaining commands. Only a successful team-leader escape ends the whole
battle; member escapes let other participants finish the round and choose their
next turn. The round captures current party leadership under `worldMu`.
Overkill and disconnect remain separate removal paths. A knocked-out teammate
still receives ordinary defeat settlement if the last living member escapes.
The legacy AC11:1:3 request enters the normal validated escape/command path.

The selected reference rule uses the strongest living opponent: equal/higher
actor level guarantees success; lower levels use `75 - floor(enemy level / 6)`
percent. Boundary tests retain the reference high-roll success convention and
exclude dead opponents from the maximum. Raw native failure bytes select motion
60042 with zero stat results. Extracted MBTM records confirm its run, pose-64
trip and recovery. Failure grants no defense or proficiency, consumes no SP,
and keeps Flee 60041 as the repeat request.

Successful member departures commit character vitals, pet vitals/amity/captures
and roster adoption before success packets or a round envelope. Escaping owners'
chains end at their departure; other clients receive a participant close and
continue. Owner and pet departures animate together. Failed saves publish no
owner success, and observers clean up the disconnected identities. Empty battles
release their timer/encounter even if the last teammate disconnects during flight.

Focused engine/server checks pass under `-race`: chance boundaries, either actor,
leader/member scope, native/full-round mixtures, save failure, overkill, logout,
disconnect/departure races and pet switches. Client regressions cover own/peer
departure with both transports, failure without damage/removal and repeat-command
normalization. Compiled player and Roca falling sprites under ignored
`var/battle-escape/` were visually inspected. CPU/GPU parity now covers **69**
stages, including the native failed-escape fall. These are automated compatibility
checks; no new manual legacy-client capture was performed for this change.

```powershell
# repository root
go test -race ./internal/battle ./internal/server -run 'Test.*(Escape|Flee|RoundDamageAndOutcomes|GonlineCombat|Overkill|BattlePet)' -count=1 -timeout=3m
go vet ./internal/battle ./internal/server

# client/; compiled asset and graphics environment as above
go test ./wlo/app -run '^Test(BattleEscape.*|BattlePet.*|GPUClientParity)$' -count=1 -timeout=3m
go vet ./wlo/battle ./wlo/app
```

### Full pre-push validation - 2026-11-02

Clean Windows runs passed the full root module with `-race -count=1 -timeout=60m`
and the full client module with `-count=1 -timeout=60m`, compiled bundles,
exported data and real GPU checks enabled. The root run passed 1,878 tests/subtests
across 28 tested packages; the client passed 576 across 24 tested packages.
Packages without tests are counted separately. Both modules passed `go vet`.

Validation exposed and corrected three integration problems: Ebitengine's audio
context must be shared process-wide even across independent workspace caches;
all four GPU fixtures need isolated `RunGame` lifecycles; and companion emote
integration must load Roca's real metadata instead of the simplified world
fixture. Concurrent first-time audio initialization and the affected voice/emote
checks passed under `-race`. GPU parity retains all 69 stages, with separate
portrait, hover and movie-illustration checks.

The sprite exporter now recognizes the exact `ca19ee...` and `190dae...` executable
layouts at their verified key-literal addresses, preserving the earlier layout
fallback. Installed archive nonces and tags authenticated both layouts; key
bytes were not logged or added to source. Independent crypto vectors remain
unchanged. The translated item census/hash golden uses its matching 7,312-item
reference, rather than the newer 7,325-item input. NPC, skill, mark, EVE and MBTM
inputs are identical between these installed revisions. Complete native asset
and sprite packages passed separately with their correct original inputs.

Additional checks passed: all original artwork archives, every exported font
glyph against its original font, the remaining available game/UI snapshots,
editable-asset manifest verification, Go formatting, Prettier, HTMLHint and the
Python exporter/media suite (24 passed, one Windows symlink-privilege skip).
Ten-second fuzz runs completed 1,061,019 protocol inputs and 606,139 EVE inputs
without failures. Optional `golangci-lint` was not installed.
Server and client executable builds passed. The incremental bundle build
validated all 72,829 files (6,020.3 MiB of source) and reused every entry.

Full-run skips include opt-in native/snapshot checks subsequently run separately,
unavailable original screenshot/legacy editable-sprite fixtures, live-account
checks without credentials and Windows symlink checks without the required
privilege. They are not claimed as executed live/native-client acceptance.
Detailed logs and rendered samples remain ignored under
`var/validation/2026-11-02-prepush/`.

Use the full commands above with absolute `WONDERLAND_TEST_ASSETS_DB`,
`WONDERLAND_TEST_CLIENT_DATA`, `WONDERLAND_SPRITE_CLIENT`,
`WONDERLAND_TEST_CLIENT_BUNDLE` and `WONDERLAND_TEST_DATA_ROOT` paths. Set
`WONDERLAND_TEST_GPU=1` for the client run. Native item goldens require the
matching reference revision; other installation variants should be tested
separately without changing those independent expected bytes/hashes.

### Full pre-push validation - 2026-11-03

Validated merge `91a8570`, including combat follow-up `df552ca`. The documentation
merge preserves both main's incoming-handler reference and the client's Lucky
Draw section.

Completed root-module race coverage: **1,933 unique tests/subtests passed across
28 tested packages**, with 12 opt-in/platform skips and eight packages without
tests. This is aggregate coverage across the full run and its isolated retry,
not a claim that one uninterrupted root command passed. Windows memory pressure
interrupted `internal/server` near the end; comparison against its independently
listed 584 top-level tests identified 88 interrupted/unexecuted cases. All 88
passed in a fresh race-enabled process. The final inventory is **584/584 completed,
zero remaining**, and no assertion or race failure remained.

The complete client module passed **595 tests/subtests across 24 tested packages**,
with compiled bundles, exported data and real GPU checks enabled. Its 35 skips
include optional snapshots and native artwork subsequently exercised separately,
as well as unavailable reference/live fixtures. Both modules passed full `go vet`
and executable builds. Separate client battle/inventory state race suites passed;
the item/round and result/slow-turn application race batches passed 27 and 18
tests/subtests respectively, with compiled assets enabled.

The initial temporary drive filled, and simultaneous asset-heavy race processes
exceeded Windows' memory commit limit. Retried with temporary storage on `D:`,
the ignored diagnostic Go utility outside package discovery, and heavy race
batches running sequentially. A ten-minute application race attempt was incomplete
during image decoding; the completed batches used the documented 60-minute outer
limit. These retries changed neither production code nor expected test values.

Additional checks passed: native asset decoders and encrypted sprites, original
artwork archives and every exported font glyph, available UI/minigame snapshots,
editable sprite verification (6,257 sprites and 816,578 frames), exported asset
checksums, Go formatting, whitespace, Prettier, HTMLHint, and Python exporter/media
tests (24 passed, one Windows symlink-privilege skip). Ten-second fuzz runs passed
1,694,063 protocol inputs and 1,006,539 EVE inputs. The incremental client bundle
build validated all 72,829 files (6,020.3 MiB source), reusing every entry.

One local provenance caveat remains: strict source reconstruction against the
manifest's recorded updated-client path reports an `Item.dat` source hash mismatch.
The existing 7,312-item export and its recorded hash match both older installed
client variants exactly; the updated installation contains a different file.
Export integrity and the matching native-reference tests passed. No original
files, exported payloads or independent golden hashes were overwritten to hide
this discrepancy. Source-path reconstruction against that newer variant is not
claimed as passed.

Live-account tests, unavailable original screenshot/legacy sprite-pack fixtures
and privileged Windows symlink checks remain unexecuted. Automated results do
not establish live Legacy-client acceptance. Detailed logs, the complete test
inventory, retry selections, final aggregate summary and rendered samples remain
ignored under `var/validation/2026-11-03-prepush/`.

### Updated client asset rebuild - 2026-11-03

Rebuilt and installed the supplied updated WLRI assets: 7,325 items, 6,280 sprites
and 820,994 frames, 16,195 pictures, 19,717 OGG tracks and 3,796 media sources.
All 40 native data sources reconstruct byte-for-byte against the updated
installation, including the previously mismatched `Item.dat`. Sprite preservation
checks passed for all 222 sources; every editable frame passed pixel and placement
comparison with its decoded original. Media checks validated 3,246 outputs and
917 complete reconstructions. Exported asset checksums passed.

Four source weapon JXAs have fewer records than their corresponding JMAs:
`002w3`, `003w3`, `004w3` and `005w3` lack 6, 6, 6 and 2 action records respectively.
The exporter now preserves the actual JXA header/records and all sprite frames,
records `missing_action_records`, and leaves unavailable animations undefined.
Malformed/truncated tables and excess records remain errors. Regression coverage
checks independent raw-byte reconstruction, rejection boundaries, and nonempty
frame preservation through editable export and atlas optimization.

Passed `go test ./cmd/sprite-export ./internal/spritepack -count=1 -timeout=5m`,
focused native sprite tests, exporter vet, and the Python exporter/media suite
(24 passed, one Windows symlink-privilege skip). The independent sprite verifier
also checks the missing-action inventory. Existing 121 unresolved custom media
resources and 18 absent companion-archive records remain recorded.

The rebuilt bundle contains 72,854 files, reusing 66,441 entries (6,037.9 MiB
source). Its new image dimension baseline accompanies this verified asset revision;
the previous baseline and core bundle are backed up. The runtime compiler cache
was relocated to the installed `data/` path for subsequent incremental builds.
Client smoke checks passed for direct/compiled login, compiled-versus-editable
worlds and sprites, pet portraits and battle item motions. A separate installed
bundle check compared every compiled frame of all 20 sprites without source
action records and verified the updated item count.

The previous 26,034 extracted files (5,752,495,913 bytes), checksum inventory,
core bundle and local client catalogs remain under
`var/backups/client-assets-20261103T065213Z/`; the old data tree is its `data/`
directory. Tracked server definitions were checked byte-for-byte across cutover.
Server asset/gameplay databases and the original client installation were not
modified. Job records and verification logs are ignored under
`var/asset-rebuild-20261103T065213Z/` and `var/current-asset-rebuild.json`.

### Multi-hit and combo damage display - 2026-11-03

Native AC50:1 records retain a local packet batch identity through animation
queuing, including embedded native packets in round chains. Single-hit damage
combines into one popup; one multi-hit skill pools the combo total into its
contact pattern. Identical multi-hit skill IDs combine, while different multi-hit
skills retain separate groups and leave singles in their own combined group.
Target cells, roster generations, HP/SP resources and separate packets remain
independent. Mixed damage/recovery preserves original stat order and clamping.

Completed regressions cover every requested combination, different skills with
equal timing, missed members, critical styling, small and wide totals, maximum
uint64 splitting, raw native batch bytes and invalid-packet atomicity. Native and
round-chain integration tests verify grouped HP changes at each contact, duplicate
frame suppression and actual bitmap digit spacing. Real Water Hit and Combo x3
metadata exercise four- and three-contact patterns. Late-frame catch-up and
replacement-target checks passed. Existing authored percentages and raw wire
goldens remain intact; repeated full weights normalize across contacts and missing
weights split evenly.

Affected battle/item/pet/departure/overkill/round-chain integration tests passed,
along with the complete client battle and minigame packages. Focused packet,
splitter, grouping and recovery-order checks passed under `-race`. Vet passed
for `./wlo/battle ./wlo/app ./wlo/minigame`; built
`bin/wonderland-client-next.exe`. The rendered
`var/battle-damage-combo.png` was inspected for readable, nonoverlapping numbers.
The client port inventory was regenerated. Server calculations, protocol layouts,
databases and asset tables were unchanged by this correction.

From `client/`, repeat the focused checks with:

```powershell
go test ./wlo/app -run '^Test(Battle(Damage|MultiHit|Vitals|Capture|HitReaction|Departure|Overkill|Pet|NewTurn|Actions|Item)|GonlineCombat(Sequential|Legacy|Native|Deferred))' -count=1 -timeout=6m
go test ./wlo/battle ./wlo/minigame -count=1 -timeout=3m
go test -race ./wlo/battle ./wlo/app -run '^Test(DamagePartsConserveWideTotals|ActionPacketsHaveIndependentLocalBatches|BattleDamageComboGrouping|BattleDamageGroupingBoundaries|BattleDamagePopupStackAndIdentity|BattleDamagePreservesInterleavedRecovery)$' -count=1 -timeout=3m
go vet ./wlo/battle ./wlo/app ./wlo/minigame
```

## Dialogue indicator and pet initial attributes — 2026-11-04

The long Robinson line is rendered with the actual two-frame next-indicator
picture. Normal, portrait and wide dialogues reserve a six-pixel gap below the
last text row. The rendered sample `var/dialogue-indicator-fixed.png` was inspected.
Focused HUD and dialogue integrations pass.

Pet initial-attribute definitions use classic WLO tables first, Rhodes tables
for missing companions, and half-template fallback otherwise. Independent
authored tests check all 30 rows and desired totals. Tests cover bounded rolls,
capture rounding, fresh Robinson recruitment, restored reserve progression,
SQL persistence, invalid totals and preserving asset migrations. Installed-data
quest tests pass against the upgraded assets database. The v18 assets-only
upgrade kept the previous database in
`var/backups/pre-migrate-20261104-005406`; gameplay data was not migrated.

Relevant checks, from the repository root:

```powershell
go test ./internal/game ./internal/world -count=1 -timeout=3m
go test ./internal/assetsql -run 'TestPetInitial|TestStructuredCatalog|TestCatalogDefinition' -count=1 -timeout=3m
$env:WONDERLAND_TEST_ASSETS_DB=(Resolve-Path var/assets.db).Path
go test ./internal/server -run 'TestNativeRobinson|TestMonkeyNative|TestMonkeyAuthored|TestStoryRecruitment|TestNativeFredFinale|TestRecruitSelect|TestRobinsonRecruitment' -count=1 -timeout=3m
```

From `client/`:

```powershell
go test ./wlo/hud ./wlo/app -run 'TestTalk|TestLongDialogueClickIndicatorLayout' -count=1 -timeout=3m
go test ./wlo/app -run 'Test.*Talk|Test.*Dialogue|TestBreillat' -count=1 -timeout=3m
```

## Stat-boosting pet food — 2026-11-04

The 24 native type-20 food definitions are projected into structured SQL.
Focused tests pass for random attribute selection, overflow protection, the
1,200-use lifetime limit, batch clamping, invalid targets, native AC23/AC67
requests, HP/SP maximum gains, RNG/SQL rollback and counters retained in party,
reserve and hotel storage. Existing amity-feeding and Potential Pill checks pass.
Both installed databases were upgraded with verified backups at
`var/backups/pre-migrate-20261104-011038`: assets v19 and gameplay v20. Existing
pet counters initialize at zero; existing stats and inventory are preserved.

```powershell
go test ./internal/server ./internal/store ./internal/assetsql ./internal/game ./internal/assets -run 'TestPetStatFood|TestPetFood|TestCharacterMetadata|TestPetFeeding' -count=1 -timeout=3m
# From client/
go test ./wlo/inventory -run 'TestPetStatFood|TestItemUse' -count=1 -timeout=3m
```

## Guaranteed pet-food compounding - 2026-11-04

Focused checks pass for food-only inputs/outputs, equal-probability food result
selection without a failure roll, native Primary/Junior/Superior commands without
skill requirements or EXP, compatibility AC40, same-food pairs, mixed/extra
ingredient rejection, locked slots, full inventory, random/save rollback and
persisted native receipts. Client validation rejects mixed recipes without
optimistic consumption. No database migration is needed.

```powershell
go test ./internal/server ./internal/assets ./internal/game -run 'TestPetFoodCompounding|TestCompounding|TestCompound|TestAlchemy|TestPetStatFood' -count=1 -timeout=3m
# From client/
go test ./wlo/inventory -run 'TestPetFoodCompounding|TestCompound|TestAlchemy' -count=1 -timeout=3m
```
## Pet-food growth mode - 2026-11-04

Pet food shares the level-up weighted allocator and captured startup growth
mode. Exhaustive interval tests cover template weights, unknown-template
fallback and equipment-adjusted combat weights. Integration tests cover both
configured modes, per-food recalculation in batches, unchanged lifetime caps,
current vitals and transactional rejection/rollback. The level-up conditions
fixture now checks two gained CON points relative to recruitment's starting
value, allowing the existing half-template recruitment policy.

```powershell
go test ./internal/game ./internal/server -run 'TestPetFood|TestPetStatFood|TestPetGrowth' -count=1 -timeout=3m
```
## Character-creation translated layout - 2026-11-04

Element selectors now retain their full translated bitmap dimensions for both
painting and hit testing, with a matching nine-slice selection frame. Its three-pixel border is fully excluded from the transparent center to prevent interior tiling over labels. The title
uses one row of its two-frame atlas. Existing creation/packet tests pass; the
opt-in creation wizard snapshots were rendered and the element screen visually
checked at `var/create-layout/create_3_fire.png` and `create_3_water.png`.

```powershell
# From client/; create the snapshot output directory first.
$env:SNAPSHOT_DIR=(Resolve-Path ../var/create-layout).Path
go test ./wlo/login ./wlo/app -run 'TestCreate|TestBuildLoginGolden' -count=1 -timeout=3m
```
## Password-field alignment - 2026-11-04

Creation/deletion password editors now match the authored 104-by-14 field boxes
at x=149 and clip the complete Panel26 image, removing the overlapping larger
outline and duplicate border. Existing creation/password checks pass. The
rendered `var/create-layout/create_5.png` was visually checked for all four
fields and their labels.

```powershell
# From client/
go test ./wlo/login ./wlo/app -run 'TestCreate|TestBuildLoginGolden|Test.*Password' -count=1 -timeout=3m
```
## Verified full monster names - 2026-11-04

Extracted all 1,190 entries from 12 saved WLOHub pages, checking page identity,
advertised totals, duplicate IDs and source checksums. The native export and
pre-patch SQL independently produce the same 558 verified name restorations
and 210 unresolved candidate records. All three authored CSV outputs reproduce
byte-for-byte. Tests pass for parser entities/whitespace, duplicate pages,
ambiguous/mismatched names, authored examples, client templates, preserving
SQL names/other fields/missing records and repeat migrations. Asset migration
v20 was verified against every patched SQL row; the original assets database
is backed up at `var/backups/pre-migrate-20261104-052141`. Gameplay was unchanged.
Both server and client candidate executables build.

```powershell
python -m unittest discover -s tools/data_export -p test_extract_monster_names.py -v
go test ./internal/assets ./internal/assetsql ./internal/dbmigration -run 'TestNPCName|TestNativeNPC|TestCatalogDefinition|TestStructuredCatalog|TestCopy|TestUpgradeInPlace' -count=1 -timeout=3m
# From client/
go test ./wlo/world -run 'TestNPCFullMonsterNames|TestShipDeckNPCs|TestStandFacing' -count=1 -timeout=3m
```

Reproduction commands, matching limits and all reference artifacts are in
[MONSTER_NAMES.md](MONSTER_NAMES.md).
## Approved monster names and Quest cleanup - 2026-11-04

Accepted the 192 review entries with a single proposed name. Quest removal
collapses 12 further multi-option entries to the same display name. The compiled
patch now covers 762 NPC IDs, with six genuinely different candidate sets left
for review. Raw source facts remain unchanged. Tests cover approved mismatches,
annotation-only ambiguity, preservation of custom names, upgrading previous
Quest-labelled names (including names now equal to their native prefix), and
client templates. Focused checks from the previous section pass. The backed-up
v21 assets upgrade verified all 762 SQL names and no remaining Quest names;
originals are at `var/backups/pre-migrate-20261104-052954`.
## Explicit monster-name choices - 2026-11-04

The six final owner choices are authored in name_choices.csv and applied by the
extractor after validating the selected name against its reference candidates.
All 768 mappings were verified in the local SQL catalog, leaving no unresolved
candidate entries. Focused assets/migration, client template and Python parser
checks pass, including all six exact names and rejection of invalid choices.
The v22 assets-only upgrade retained its original database at
`var/backups/pre-migrate-20261104-053555`. Both candidates rebuild successfully.
## Full item names - 2026-11-04

Extracted all 6,882 entries from 69 saved WLOHub item pages. The conservative
icon/rank/description matcher restores 696 item names and retains 50 uncertain
candidate rows. It excludes complete names and enhancement-prefix collisions.
All four extraction artifacts reproduce byte-for-byte. Tests pass for parsing,
matching, unknown ranks, duplicate pages, preserved raw records/custom names,
v23 SQL migration and client inventory definitions. The installed assets-only
upgrade verified all 696 names and retained its original database in
`var/backups/pre-migrate-20261104-054400`. See [ITEM_NAMES.md](ITEM_NAMES.md).

```powershell
python -m unittest discover -s tools/data_export -p test_extract_item_names.py -v
go test ./internal/assets ./internal/assetsql ./internal/dbmigration -run 'TestItemName|TestNPCName|TestCatalogDefinition|TestStructuredCatalog|TestCopy|TestUpgradeInPlace|TestNativeItem|TestItemCatalogJSON' -count=1 -timeout=3m
# From client/
go test ./wlo/app -run 'TestFullItemNames|TestItemUse|TestPetStatFood' -count=1 -timeout=3m
```
## Pineapple Monster name - 2026-11-04

NPC 17001 now uses Pineapple Monster instead of the native short name Pineapple.
Explicit owner choices may restore shorter source names without widening the
automatic ten-byte prefix rule. Parser, authored table, client-template and SQL
migration checks pass. Assets v24 was upgraded with an original backup at
`var/backups/pre-migrate-20261104-054909`; the compiled table now has 769 entries.
## Collection-spot definitions - 2026-11-04

Imported 160 WLOHub reference spots plus two owner coordinates. SQL contains
162 definitions, 251 listed rewards, 82 distinct native items and 30 vacuum
entries. Location/reward metadata is fully resolved for 107 entries; other
records preserve unknown fields explicitly. Tests cover page parsing, vacuum
classification, ambiguous/out-of-bounds maps, reward matching, partial definitions,
owner-authored rewards, exact seed counts and preserving edited/empty SQL tables.
Seed and CSV reports reproduce byte-for-byte. The v25 assets-only migration kept
originals at `var/backups/pre-migrate-20261104-061037`. This is a definition import;
vacuum gameplay timing/tool rules remain unspecified. See COLLECTION_SPOTS.md.

```powershell
python -m unittest discover -s tools/data_export -p test_extract_collection_spots.py -v
go test ./internal/assets ./internal/assetsql ./internal/dbmigration -run 'TestCollectionSpot|TestCatalogDefinition|TestStructuredCatalog|TestCopy|TestUpgradeInPlace' -count=1 -timeout=3m
```

## Companion starting equipment - 2026-11-04

Twenty normal companion equipment mappings were imported from the saved pets
page. Focused checks cover source stages and item matching, authored mappings,
native slot validation, equipment-adjusted starting vitals, aliases, recruitment
and vouchers, failed-transaction rollback, immediate AC15:8 roster publication,
reserve preservation, and edited/empty/idempotent SQL migration. Existing capture,
bazaar, GM recruitment and story migration integration selections pass. The
older voucher fixture now expects the established half-template initial stats;
the v16 story unit test runs its owning migration against its Story-only fixture.

```powershell
python -m unittest discover -s tools/data_export -p test_extract_pet_equipment.py
go test ./internal/game ./internal/world ./internal/server ./internal/assetsql -run 'TestFreshPet|TestPetStartingEquipment|TestPetSpecialEquipment|TestMigrationVoucher|TestStory.*Migration|TestGMRecruitPet|Test.*Recruit' -count=1 -timeout=3m
go test ./internal/server ./internal/store ./internal/assetsql -run 'TestPetSpecialEquipment|TestMigrationVoucher|Test.*Recruit|Test.*Capture|Test.*Bazaar|TestNativeRobinson|TestNativeFredFinale|TestNativeRoca|TestStory.*Migration|TestCatalogDefinition|TestStructuredCatalog' -count=1 -timeout=3m
```

The extractor reproduced all three artifacts byte-for-byte. Installed-catalog
and native companion checks passed with `WONDERLAND_TEST_ASSETS_DB` pointing to
the offline v26 database. All 20 native equipment mappings were read back from
SQL. Both `bin/wonderland-server-next.exe` and `bin/wonderland-client-next.exe`
build successfully. No client UI code changed in this feature.

## Remaining enemy-name review - 2026-11-04

Reviewed 2,063 candidate/capacity-length records against the saved reference and
native metadata. Added 129 ID-specific names, bringing the patch to 898. Evidence
and retained cases are documented in ENEMY_NAME_REVIEW.md. Checks cover native
and source identity guards, missing leading/middle words, exact HP/SP evidence,
false prefix/species matches, retained spelling/context, SQL custom-name and
missing-record preservation, idempotent upgrades and client template loading.

```powershell
python -m unittest discover -s tools/data_export -p test_extract_monster_names.py
python -m unittest discover -s tools/data_export -p test_review_enemy_names.py
go test ./internal/assets ./internal/assetsql ./internal/dbmigration -run 'TestNPCName|TestCatalogDefinition|TestStructuredCatalog|TestCopy|TestUpgradeInPlace' -count=1 -timeout=3m
# From client:
go test ./wlo/world -run TestNPCFullMonsterNames -count=1 -timeout=3m
```

These focused checks pass. The extractor and review reproduce all CSV/manifest
artifacts byte-for-byte. Assets v27 was backed up at
`var/backups/pre-migrate-20261104-064512`; all 898 names were verified in SQL.
Exactly 129 new name fields changed. A hash of every other NPC column across
all 4,930 rows remained identical. Server/client next executables both build.

## Full pre-push validation - 2026-11-04

Validated `27b8f8d` and the subsequent validation-only test fixes across both Go
modules. The complete server run used `-race -count=1 -timeout=60m -json ./...`
with the installed v27 SQL catalog. The complete client run used
`-count=1 -timeout=60m -json ./...`, with GPU, loose-data and compiled-bundle
checks enabled. Final results, combining the full run with complete reruns of
the corrected packages, cover every repository package:

| Module | Passing test/subtest entries | Skipped entries | Passing packages | Packages without tests |
| --- | ---: | ---: | ---: | ---: |
| Server | 2,072 | 12 | 28 | 8 |
| Client | 710 | 36 | 24 | 1 |

Initial validation found three issues, all resolved before pushing:

- Vet rejected external unkeyed `game.Attributes` literals in three pet-stat
  test files. Keyed fields retain exactly the same independent expected values;
  the affected fixtures pass under the race detector.
- The background-only Team/logout comparison included the logout portrait/name
  after painting was unified for closing animations. The test now isolates the
  decoration and retains separate portrait placement/static-pose checks. The
  **entire client application package** passed its subsequent GPU-enabled rerun.
- The opt-in translated-catalog test requires the original 7,312-record fixture,
  not the updated 7,325-record extraction. The **entire assets package** passed
  its subsequent race run with the designated original fixture. An ignored local
  scratch program was also discovered by `./...`; ignored `var` research tools
  were isolated behind their own local module boundary. Package-list comparison
  confirms that every actual repository package was covered, with no remaining
  failures. Scratch programs are not shipped or treated as product test packages.

Additional checks passed: full server/client vet, tracked Go formatting,
Prettier, HTMLHint, both ten-second protocol/EVE fuzz runs, and both executable
builds. Client UI/world/inventory/role/skills/team/login race suites passed 184
entries; app dialogue/confirmation race regressions passed another 19 entries.
The Python exporter suite ran 37 tests, with one Windows symlink-privilege skip.
The asset compiler validated all 72,854 files (6,037.9 MiB source), all reused.
`TestCompiledBundleLoginAndSharedWorld` passed against the actual compiled packs.
No race warning, runtime panic or remaining product-package failure was reported.

The Go skips cover original-reference inputs not enabled or unavailable for the
specific test, live-account operations, optional snapshots and one unavailable
Windows symlink privilege; they are not manual gameplay acceptance evidence.
Optional `golangci-lint` was not installed. Initial logs
retain the resolved failures; final package results and complete-package reruns
are recorded separately under ignored `var/validation/prepush-20261104/`, with
`final-summary.json` checking results against both module package inventories.

## Wiki quest implementation validation - 2026-11-04

The subsequent quest work uses the preserved assets v38 catalog (gameplay v20).
This was focused development validation, separate from the earlier pre-push run.

The race-enabled server selection passed across `internal/server`, `assetsql`,
`world`, `assets` and `battle`. It covers all 350 installed resource contracts,
25 mixed skill/companion bundles, 15 escort contracts, disguise/actor policies,
constellation receipts, equipment and inventory gates, Sledge Wolf alternatives,
SQL rollback/stale-state/replay checks and failed post-commit delivery. Complete
`internal/world` and `internal/battle` race suites also passed. Story/catalog and
offline-copy/upgrade migration regressions passed under the race detector,
including operator edits, historical deletions, empty collections and repeated
upgrades. The final focused server selection ran in 114.456 seconds.

Client application/world/role regressions passed for constellation acknowledgements,
native NPC presentation, disguises, event/dialogue behavior and Breillat. Both
`WONDERLAND_TEST_DATA_ROOT` and `WONDERLAND_TEST_CLIENT_BUNDLE` were enabled;
`TestCompiledBundleLoginAndSharedWorld` loads the actual star/sky artwork and checks
that shared resources do not share cutscene state. The app selection ran in
26.750 seconds. Server and client executable builds and affected-package vet pass.
The six Python quest-extractor tests pass, and the reviewed bundle/parent seeds
reproduce exactly from the cached reference catalog. Changed Go files are formatted
and `git diff --check` passes.

The final static action audit covers 1,701 events bound to all 345 notepad entries,
with no reported action/capability/formation gaps. This is not a live playthrough
of 345 quests. Native formation coordinates, constellation particle/panel parity,
the separate marriage ceremony and detailed multi-map/team acceptance remain
explicitly unverified; see [quest research](QUEST_RESEARCH.md).


## Translation / quest branch split validation - 2026-11-04

The Client translation commit was tested independently in a clean detached
checkout: native-name and SQL migration regressions pass, and the review tool
compiles and passes vet. The confirmed-name importer and client name-loading
checks pass; the extracted patch reproduces 991 changed names and preserves 85
explicit unchanged confirmations. Tentative names are excluded.

On quest-development, assets v38 supports both the translation-only v28 upgrade
and existing development v37 catalogs. Tests prove the initial three quest
contracts are supplied to v28 and a deliberately deleted quest policy remains
absent after the v37 name refresh. The local backed-up upgrade changed exactly
93 NPC names with all other fields identical across 4,930 records.

The final focused quest/name selection passed under the race detector across
server, assets, assetsql, world and battle (server: 118.862 seconds). The client
quest/name selection passed with loose and compiled assets enabled (app: 32.792
seconds). The ten quest research parser/extractor tests, affected-package vet,
format checks and both executable builds pass. No push was requested; this is
focused pre-commit validation, not a new full-server pre-push run.

## Both-branch pre-push validation - 2026-11-04

The translation branch `Client-rewrite` (`c6d15ad`) and quest branch
`quest-development` (`0401b3c` plus the validation-only fix below) were validated
independently. The Client checkout used its own upgraded v28 assets database;
the quest checkout used the installed v38 database. Gameplay data was not reset.

Server modules were tested with `-race -count=1 -timeout=60m`; client modules
used the documented `-count=1 -timeout=60m` command, with GPU, loose-data and
actual compiled-bundle client checks enabled. An optional full-client race run
was stopped because its artwork tests thrashed under the memory limit; its
partial log is retained and is not counted as completed coverage. Heavy package
runs were serialized with `-p 1` after Windows memory-quota and full-system-drive
errors interrupted the initial parallel validation. Complete package reruns
replace interrupted or failed package results; the original logs are retained.
The regenerable Go build cache was cleared and subsequent build caches and test
temporary files were directed to the workspace drive.

Final results cover every package in both branches' module inventories:

| Branch / module | Passing test/subtest entries | Skipped entries | Passing packages | Packages without tests |
| --- | ---: | ---: | ---: | ---: |
| Client-rewrite / server | 2,074 | 12 | 28 | 8 |
| Client-rewrite / client | 710 | 36 | 24 | 2 |
| quest-development / server | 2,140 | 12 | 28 | 9 |
| quest-development / client | 715 | 36 | 24 | 2 |

There are no remaining test failures, race reports or missing packages. Original
client reference paths were restored in the isolated checkout; the full UI
package and the two affected native SQL/sprite comparison tests then passed.
Remaining skips cover optional reference fixtures, snapshot outputs, live-account
operations and Windows symlink privileges. The Python suites ran 38 Client and
48 quest tests, each with one symlink-privilege skip.

Validation found one test expectation issue: installed migrations append
constellation metadata in a different order from fresh seeds. The migration
comparison now canonicalizes only constellation metadata, preserving every
executable rule's relative order and the independent repeated-migration check.
No gameplay behavior changed for this correction.

Additional checks cover both modules' vet and executable builds, tracked Go
formatting, Prettier, HTMLHint, review-tool JavaScript syntax and export behavior,
both branches' Python exporter suites, and ten-second protocol, EVE and native
item fuzz runs. The asset compiler checked all 72,854 files (6,037.9 MiB source),
all reused. Optional `golangci-lint` was unavailable.

Logs and the package-inventory summary are retained under ignored
`var/validation/prepush-branches-20261104/`. Automated checks do not establish
live-account or manual quest acceptance; the remaining gameplay limitations in
[quest research](QUEST_RESEARCH.md) still apply.

## Native manual collection validation - 2026-11-04

Collection research and implementation are on `Client-rewrite`. Focused race
checks pass for server event gathering, SQL claims, world branch planning,
native timer packets and the assets v29 content migration. The installed North
Island 60000/33 test uses the real EVE event: click, committed pool reward,
180-second timer, step-complete acknowledgement, replay protection, cooldown
dialogue and full-inventory rejection. Existing gathering restart, concurrent
claim, rollback and failed-delivery checks pass.

The migration upgrades an isolated Client v28 catalog and removes exactly 25
resolved generated disables, with no added disables or changes to retained
reasons. Tests retain operator reasons, mixed diagnostics, missing entries,
incomplete pools and later operator disables across repeated initialization.
The local quest-development v38 catalog and gameplay data are untouched.

Focused client race tests pass for clicking the actual North Island grass
sprite, the literal captured acquisition/timer/completion exchange, no actor
flight, movement release, malformed atomic timer updates, ordinary NPC dialogue
and completion acknowledgement. Affected-package vet and server, migration-tool
and client executable builds pass. Logs and validation binaries are under
ignored `var/validation/collection-usage/`. This is focused development
validation, not a full pre-push battery or a new live vacuum test.

## Shared resource cooldown validation - 2026-11-04

Focused race tests pass across assets, asset SQL, store, world and server for
global collection claims and independent per-source durations. Concurrent claims
from different accounts and separate database connections produce one winner.
Tests cover separate actors sharing a legacy timer, ground-item namespaces,
exact expiry boundaries, full-inventory rollback and database reopen recovery.
Server integration checks cover neighboring collection and ground sources with
different configured cooldowns and restoring ground availability on map entry.
World regression checks preserve pending respawn broadcasts when another player
arrives after expiry but before the world ticker runs.

Assets v29-to-v30 migration checks preserve authored overrides and intentionally
empty definitions across repeated upgrades. An isolated installed catalog upgrade
passes real North Island collection, water gathering, world entry, visibility,
area entry and ground-pickup integration tests with the race detector. Existing
quest-development v38 assets and live gameplay databases remain untouched.
Focused client collection timer/acquisition race tests, affected-package vet and
server, migration-tool and client executable builds pass. Logs and
disposable outputs are under ignored `var/validation/shared-resources/`.

## Vacuum client scheduling validation - 2026-11-04

Focused client race tests pass for native start/stop/click bytes, deadline
boundaries, single in-flight retries, changed respawn durations, failed attempts,
starting on exhausted spots, movement, equipment removal, map/battle cancellation
and connection isolation. A 30-second timer gives Gonline a 40-second wait; a
240-second timer gives 250 seconds. Other servers keep the fixed 190-second
Legacy interval. The matching executable confirms the decompiled scheduler's
190,000-millisecond threshold and its exact eight resource-template references.

The initial client run exposed an incorrect NPC-kind assumption; the corrected
template-list implementation passes all vacuum tests with the race detector.
Existing captured manual collection, malformed timer and combat negotiation/reset
regressions also pass. Server/protocol race tests verify the literal collection
capability bytes and availability with combat chains disabled while preserving
combat opt-in. Affected-package vet and client/server executable builds pass.
Ignored `var/validation/shared-resources/vacuum-*.log` retains the test history;
validated binaries are `client-vacuum.exe` and `wonderland-vacuum.exe` there.
No full pre-push battery or real-server vacuum validation was run for this change.

## Complete collection pre-push validation - 2026-11-04

Validated production commit `aafdeff` and test-fixture correction `44e5a4a`.
The full server module ran with `-race -count=1 -timeout=60m -p 1 ./...` against
the isolated v30 catalog. The full client module ran with GPU, native-data and
compiled-bundle checks enabled. Final coverage includes every module package:

| Module | Passing test/subtest entries | Skipped entries | Passing packages | Packages without tests |
| --- | ---: | ---: | ---: | ---: |
| Server | 2,089 | 12 | 28 | 8 |
| Client | 722 | 36 | 24 | 2 |

The initial server run found an outdated login wire assertion and a simulation
fixture cleanup race. The login golden now consumes the independent collection
capability before authentication. Field fixtures settle playback and cancel battle
timers before closing SQL or restoring timing hooks. Both fixes passed three
focused race repetitions, followed by a complete passing server-package race run.
Coverage combines that complete rerun with the other passing full-module packages;
no test/package failures or race warnings remain in the final results.

Full vet, tracked Go formatting, HTMLHint, Prettier, ten-second Read/EVE/NativeItems
fuzz runs, server/client/migration builds and the entity-name review/export check
pass. Python exporters pass 38 tests with one Windows symlink-privilege skip.
Go skips retain their documented optional-reference, live-account and snapshot
requirements; they are not live gameplay acceptance evidence. Logs, binaries and
the strict package-coverage audit are under ignored
`var/validation/prepush-collection-20261104/`, including `final-summary.json`.

## Quest/client consolidation validation - 2026-11-05

Merged quest development into `Client-rewrite`, then into `main`. Validation
used an isolated, consistently copied assets database upgraded to schema v46;
running gameplay and assets databases were not modified.

The complete server module ran with `-race -count=1 -timeout=60m -p 1`.
Passing full-package reruns cover assets, asset SQL and encrypted-client assets.
A complete server integration rerun verified the timer cleanup. Its remaining
waterfall ACK and vehicle-storage failures were corrected and replaced by
passing focused race runs covering vehicle,
shop, sale, storage, hotel, clinic and quest integrations. The complete client
module passed with GPU, native-data and compiled-bundle checks enabled.

The final audit combines complete package coverage with the passing corrected
reruns; original failed attempts remain available for diagnosis.

| Module | Passing test/subtest entries | Skipped entries | Passing packages | Packages without tests |
| --- | ---: | ---: | ---: | ---: |
| Server | 2,270 | 10 | 28 | 9 |
| Client | 786 | 37 | 24 | 2 |

Validation corrected the stale 7,312-item catalog census (now 7,325), comparisons
of independent story lookup kinds, SQL setup for clinic and honey hand-in tests,
and a waterfall test missing the native choice-completion ACK. World fixtures
now cancel idle battle timers before database teardown, preventing later tests
from racing their battle timing hooks. Repeated pet/potential and partial-battle
fixture regressions pass under the race detector.

A real storage regression was fixed: moving the ridden vehicle out of the bag
now normalizes mount ownership inside the resource transaction and publishes
the native dismount only after commit. Tests verify durable bag/storage/mount
state, peer replication and no state changes or receipts on a failed save.

Both modules' vet and executable builds pass; the final server was rebuilt after
the resource fix. Python exporters pass 48 tests with one Windows symlink skip.
Prettier, HTMLHint, tracked Go formatting, review-tool JavaScript syntax/export,
and ten-second protocol/EVE/native-item fuzz runs pass. The asset compiler
checked all 72,854 files (6,037.9 MiB source), all reused. The encrypted-sprite
fixture uses its matching original translated executable. Optional live-account,
snapshot and platform tests remain skipped; golangci-lint is unavailable.

Logs, validation binaries and the strict package audit are retained under
ignored `var/validation/prepush-merged-20261105/`, including `final-summary.json`.
These automated results do not establish live gameplay acceptance.
