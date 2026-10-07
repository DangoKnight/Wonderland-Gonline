# Teams, pets and instance rooms

Open **Teams** with the top toolbar's party button or **Ctrl+T**.

- Pet rows do not select pets or change the selection in Inventory and Skills.
  Following pets use their roster nickname when assigned; rename replies update
  the active follower immediately. Template names are only a fallback.
- Click a pet name to edit it inline. Enter, clicking elsewhere, closing Teams,
  or switching tabs submits AC15:6 (slot followed by raw name bytes). The server
  reply updates the name. Empty or unchanged names send no request; a replaced
  pet slot cancels the edit. The native editor accepts up to 14 alphanumeric
  bytes (`FUN_0026d6bc`, `FUN_00272004`, `FUN_00271eac`). The `Panel29`
  source is 83×13, tiled into the native 86×23 editor; using the editor
  dimensions as the source clip clips the background through the name.
- The 24×24 small pet icon is centered within its 34×34 portrait cell.
- Click its mode button to choose **Rest**, **Battle**, or **Ride**. Changes take
  effect after the server reply. Battle pets require sufficient amity and HP.
- **Dismiss** opens a confirmation. Confirming permanently releases the pet;
  Cancel or Escape keeps it. The pet ID is rechecked before sending the request.
- Use the bottom toolbar's party invitation tool, then click a player to request
  joining their party. Join Team stays pressed and the world cursor changes
  while selecting. Hovering either a player or their following pet highlights
  both together; clicking a pet requests joining its owner. Picking checks the
  opaque pixels of the drawn sprite layers (`FUN_002fe8e8` / `FUN_004106b0`),
  and leaving selection clears the highlight. Selection and requests add no chat
  notifications. Click the tool again or press Escape to cancel. Leave an existing
  party before requesting another. The leader can accept or decline the invitation.
- Teammate rows show name, portrait, level, element and HP/SP gauges. Leaders can
  transfer leadership or dismiss a member. End Party/Leave uses confirmation.
- Team chat checks actual party membership.

The server's native teammate stats now send HP and level in their correct fields.
Private Server's previous mapping displayed a level-1 teammate with 181 HP as
level 181. Rebirth, CON/WIS and corrected growth bonuses also reach native clients.

Party removal detaches the old visible formation before resetting the HUD.
A solo AC13:6 roster must be followed by AC13:4 for that character: the native
client otherwise retains a leader role. Both characters receive this reset when
a two-person party ends, including after a kick or disconnect. Larger parties
retain their remaining members and transfer leadership when necessary.

Battle pets follow their owner’s recent positions at normal walking speed.
The native trail stores five positions, shifts after 20 pixels on either axis,
and targets the third entry (roughly 40 pixels behind during straight walking).
Pets keep their own facing and walking/standing animation, including other
players’ pets. AC7 relocation resets the trail; large discontinuities beyond
the local walk range also reset it as a recovery fallback. Switching pets
starts a fresh trail; name updates preserve the current walk.

Pet riders use directional riding poses (18–25) while the mount walks.
AdjustRidePetPos.txt may instead request standing or seated poses (46–53).
The character’s land walking animation returns after dismounting.

## Instance tab

The browser supports room creation, pagination, joining, leaving and leader
handoff on disconnect. New Instance shows the 34 native definitions from the
current WLRI exports. Names/descriptions come from Mark.dat; capacities, level
requirements, guild restrictions and time limits come from SceneData.dat.

Definitions live in `assets.db`'s `catalog_instances`. Lobby membership lives in
memory and is removed on disconnect. Room capacity and minimum level are checked
on the server. Guild-only instances report unavailable until their membership
rules are verified.

**Dungeon start, objectives, completion and rewards remain pending.** Start reports
unavailable and keeps the players on their current map. Lobby creation does not
charge currency or grant rewards.

### Wire layouts

All multibyte numbers are little-endian. `String` means length U8 plus that many
bytes. These are payloads after transport framing/encryption.

| Request | Input after action/subcode | Reply |
| --- | --- | --- |
| AC85:1 browse | Optional page U8; zero/omitted means first page | AC85:14 count U8, repeated definition ID U16 and availability U8 (1 means available); AC85:1 pages U8, page U8, count U8, repeated room ID U16, creator String, room name String, member count U8, guild String |
| AC85:3 create | Definition ID U16, room name String (maximum 28 bytes) | Membership, room snapshot and refreshed browser, or AC85:2 status U8 |
| AC85:4 details | Room ID U16 | AC85:4 room ID U16, count U8, repeated character name String and character ID U32 |
| AC85:200 join | Room ID U16 | Membership/snapshot to room members, or status |
| AC85:201 leave | Empty | AC85:5 operation 2; empty room snapshot; remaining members receive their updated room |
| AC85:202 start | Empty | AC85:2 status 11 (content pending); nonleader requests get 255 |

Membership uses AC85:5 operation 1, state U8, room ID U16, member count U8,
creator String and creator ID U32. Gonline's AC85:203 snapshot contains room ID
U16, definition ID U16, count U8, then repeated character ID U32, level U8 and
name String. An empty room has zero IDs and zero members.

Codes **200–203 are Gonline extensions**. The native browser/create/detail and
membership reply layouts come from the decompile; native join/leave/start request
layouts remain unverified. These extensions do not claim compatibility with those
native controls. Private Server contains no working instance backend.

## Verification

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

Remaining presentation work includes native party walking formations. Dungeon
objectives and rewards require verified source rules.

## Native window presentation

Teams and Instances share one draggable window and its position, with two tabs. Companion and teammate
rows retain the extracted portraits, element icons and HP/SP label artwork.
Pet assignment lists Rest, Battle and Ride with hover tooltips; a press outside
the menu closes it. Dismissing
a pet hides Teams and opens the native shaded confirmation with the pet preview. Confirmations render one cached
background, shared by pet dismissal and party invitation prompts. Pet portraits
load their NPC templates even if no overworld NPC has initialized them yet.
Teams uses the dedicated 24×24 sprite in family `007` (`Npc.dat +0x10`, exported
as `unknown_u16_offset_16`), preserving its colours. Robinson uses sprite 7143;
his family `008` sprite 8067 remains the large dialogue portrait. Party and
Instance tabs retain the selected button state through hover and redraw, using
the same sticky-button mechanism as Skills (`FUN_0026dfc4`).
The instance picker shows ten definitions per page, a blue selected row, full
wrapped names and a page counter. Creation uses the selected definition name;
dungeon objectives and rewards remain pending as documented above.

Pet portraits are rasterized once as immutable artwork shared by sessions, then
uploaded through the normal GPU asset cache. They do not allocate a disposable
GPU render target on every frame. Focused tests exercise editable and compiled
assets, plus CPU/GPU portrait parity (`WONDERLAND_TEST_GPU=1`).

## Native pet raster and name placement

The riding render path is `FUN_00412c50`: the mount is drawn through
`FUN_0041c8e8`, followed by the human through `FUN_00433318`. The previous
camera-facing layer reversal was an inference and has been removed. Rider body
and equipment retain their native directional raster ordering.

`FUN_002fe8e8` computes the saddle from the mount frame's canvas height minus
anchor Y, the template's height scale/preset, then adds the body/facing offsets
from `FUN_0040cbb4`. For height flags `(scale, preset)`, the vertical baseline is:

| Flags | Rider Y offset before authored offsets |
| --- | --- |
| 0, 0 | 62 - (canvas height - anchor Y) |
| 0, 1 | 130 - (canvas height - anchor Y) |
| 1, 0 | 94 - 2 × (canvas height - anchor Y) |
| 1, 1 | 330 - 2 × (canvas height - anchor Y) |

Positive offset Y moves down. Standing saddle offsets apply during movement as
well. `FUN_0040e4b4` adds movement corrections only for sprites 1185 and 1456.
The native parser stores `MovePosX/MovePosY`, but that function never reads those
text-file fields. `FixedPosOnFirst` fixes the frame used to calculate positioning;
the pet's visible animation continues. Height-scale 1 doubles the map sprite
through the existing GPU texture cache, preserving palette recolouring.

The 135 compiled native definitions live in
`client/wlo/role/pet_mount_tables_generated.go`. They include the original four
body/eight facing offsets and native seated/fixed-frame selectors. Authored
saddle tables take precedence over `data/ride_pet_positions.json`. Unlisted pets
use the native ±5 horizontal fallback for ordinary riding poses; explicitly
seated native fallbacks have no extra horizontal displacement.

Regenerate the authored tables from a matching local WLRI executable and its
full decompile (the paths below refer to ignored analysis inputs):

```sh
python3 tools/decompile/mount_tables.py \
  --exe var/ghidra/aLogin.exe \
  --decompile var/decompiled/aLogin_ca19ee087b60_full.c \
  --output client/wlo/role/pet_mount_tables_generated.go
```

The generator reads PE table bytes and evaluates the three pure decompiled
selectors in an isolated C harness. It does not execute aLogin.exe. Python, a C
compiler and gofmt are required. The generated header records the executable
SHA256; ordinary client builds do not need the analysis inputs.

`FUN_004147f8` centres mounted names on the owner's world X, independent of the
saddle's horizontal offset. `FUN_004265a4` supplies additional name clearance:
`ceil(first bitmap height × 0.7) + 2` for normal presets, or
`ceil(first bitmap height × 0.1) + 66` for tall presets. Height-scale 1 adds 68.
Both name and nickname receive this clearance, for local and remote riders.
Following pets use their own NPC template and first-frame anchor for name height,
plus the native sprite drop, rather than the previous fixed 72-pixel name lift.
The existing native five-position follow trail remains in use.

Focused tests cover all eight standing/walking facings, native table/file
precedence, seated/default poses, saddle baselines, movement correction,
fixed-frame positioning, following-pet name heights and CPU/GPU scaled sprite
parity. Live acceptance still requires comparing each tested mount with aLogin
in all eight directions, while stationary and walking, on both player sessions.

Pet dismissal and party invitations use `seui.ConfirmationBackground`, the same
Panel22 frame, dithering and blue shade conversion as logout. Dismissal places
the companion on the right and its current name below, with confirmation text
on the left. The window and preview anchors follow the first sprite frame
height, including the native low-body and tall-name template flags. Shorter pets
therefore use shorter dialogs. Both previews hold their first frame: NPC action
11 for dismissal and human action 13 for logout, as selected by
`FUN_0034c814`. Their animation state is independent of inventory previews.
No solid blue backing is drawn behind the native dither mask.

Instance pagination arrows use the native 17×17 state frames in
`Btn_ArrowL_5` / `Btn_ArrowR_5`, for both the browser and creation form
(`FUN_0026ab14`, `FUN_00183d44`). Idle, hover and pressed states each read
one complete row without including pixels from the neighboring state.
