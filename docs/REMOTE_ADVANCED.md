# Advanced Remote

The client uses `THL_AutoPlayFormPlus`'s existing localized skins, native 240×489
layout and independent player/pet configurations. Auto Fight enables execution;
opening or editing settings sends no gameplay requests. Confirming the advanced
window commits its draft to the main Remote form; main Confirm activates it.
Closing without confirmation discards the draft. Main Confirm saves settings
locally; automation itself and its counters remain transient.

## Local profiles

Confirmed settings are stored in `remote-<character ID>.json`, alongside the
client's `settings.json` and hotbar profiles. This versioned file contains main
checkboxes, supply slots, thresholds, configured logout limits, discard item IDs
and both advanced configurations. Files are validated before use and replaced
atomically on save. A failed save reports an error and does not start Remote.
Malformed or unsupported files report an error and leave the default controls.

Character entry restores its profile and populates Remote when opened. It does
**not** start automation, even if the saved Auto Fight checkbox is checked.
Running status, pending requests, elapsed time, counted deaths and sequence
cursors are never stored. Switching characters discards the previous controls
and restores the new character's profile, or defaults if none exists.

The native client also stores these preferences locally. In
`170_THL_ItemUnit_TH_BlackBoardForm_TH_ComputerForm.c`, `FUN_001fe79c`
(other build `FUN_001e469c`) writes main settings and both advanced records under
`User\\`; `FUN_001fed8c` (`FUN_001e4c8c`) reads them. `FUN_001ffaf4` derives the
profile name from the character ID XOR `0x74cbb1`. Main Confirm's
`FUN_001e6b38` calls the save routine; opening the main form invokes the loader.
Native code additionally notifies the server through AC23:78 for main settings;
that packet does not serialize the full advanced profile. Gonline uses readable
JSON rather than duplicating the native obfuscation or adding a server settings
message. Normal gameplay actions retain their existing native protocol.

Starting Auto Fight during combat, or entering combat with it enabled, also
enables the battle's bottom-right AUTO control. The two switches are independent:
stopping Remote leaves AUTO running, and clicking AUTO off leaves Remote running.
While Remote Auto Fight remains enabled, it re-enables AUTO at the next decision
stage, or immediately if the current stage is still accepting actions. Cancelling
AUTO during playback does not re-enable it until decision time.
Remote owns submissions while active, so AUTO cannot send competing choices for
the same turn. Turn both off to resume manual selection; commands already sent
to the server remain submitted for that round. AUTO still resets at battle end.

## Native evidence

`var/decompiled/reference/units/169_TGiftExchangeCardForm_THL_AutoPlayFormPlus_THL_FixTrafficForm.c`:

- `FUN_001ba580`: constructor, checkboxes, nine assignment cells, target controls,
  three numeric fields and Player/Pet/Description tabs.
- `FUN_001d6294`: restores each tab's controls and item/skill artwork.
- `FUN_001d5b1c`: confirms two independent 0x52-byte records into client state.
- `FUN_001d5d48`: validates target selection using the assigned skill's native
  target domain. Gonline also checks this domain before sending actions.

`var/decompiled/reference/units/231_TFightField_TFightHum_TFightManage.c`:

- `FUN_003b66dc`: chooses advanced actions for the current player or pet.
- `FUN_003b4584`, selector 3: searches for a fighter with zero HP.
- Recovery compares current HP/SP to an **absolute** configured value and maximum
  HP/SP; these settings are separate from the main form's percentage sliders.
- The three sequence entries use an advancing cursor; default entries supply
  fallback actions after the sequence is exhausted.

## Execution

Rules are evaluated in native order:

1. Party-death action, triggered by a knocked-out ally.
2. Enemy-HP condition (`<` or `>`), evaluated against the next living enemy.
3. HP recovery below the configured value, excluding full-health/dead targets.
4. SP recovery below the configured value, excluding full-SP/dead targets.
5. The next usable entry from the three-action opening sequence.
6. First usable default skill, then second default skill.
7. Ordinary attack if no advanced rule can execute.

Player and pet have separate sequence cursors, restarted for each battle.
Assignments may select enemy, player, pet or allied targets. Native skill target
metadata further restricts those selections; self-only actions resolve to the
acting unit. Defense and escape use their native command codes. Skills must be
learned by the actor and affordable; unavailable skills/items fall through.
For conditional skills, the condition's subject and the eventual action target
are distinct: a party death can trigger a revive on that ally, an attack on an
enemy, or a self-only action. Native target metadata controls that redirection.

Left-click assignment cells to cycle Attack → Defend → Flee → Attack, as in
`FUN_001d6f84`. An empty cell starts with Attack; right-click clears any
assignment. Drag learned skills and unlocked battle-use items from Skills or
Inventory to assign them. Default cells accept skills only. The target button cycles the native
target choices. The Description tab retains the localized native help skin.

Battle items use the existing AC50:2 protocol and skills use AC50:1 (or native
Defend/Flee requests). Inventory and vitals remain server-authoritative. Per-round
item reservations prevent both actors choosing the same final unit in a stack;
reservations do not alter displayed inventory and reset for the next round.
No server extension, database migration or asset extraction is required.

Focused tests cover confirmation/cancellation, independent player/pet commands,
skill affordability, recovery priority, finite item reservations, opening
sequence exhaustion, enemy-HP conditions, SP recovery and revival targeting.
Rendered settings were inspected against the native layout. Live Legacy-server
automation acceptance has not been tested for this change.
