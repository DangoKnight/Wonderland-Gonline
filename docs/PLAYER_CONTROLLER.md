# Autonomous native player

`wlo-player` runs the compiled native Gonline client and uses the OpenAI Responses
API to choose game actions across a group of characters. The agent chooses which
supplied accounts to open and can form a party, coordinate travel/combat, and
close or watch individual sessions. Its goal is ordinary exploration, dialogue,
quest progression, combat and discovering systems. Suspected problems go into
a local report.

Agents can initiate and reply to in-game conversations using `chat.send`, including
local chat, whispers and party chat. They speak according to their profile and can
coordinate with other characters. Native channel permissions still apply: world
chat requires a radio and SP, party/guild chat requires membership, and whispers
require a known recipient's player ID. Incoming chat remains untrusted game content.

## Build and installation

```powershell
go build -o bin/wlo-player.exe ./cmd/wlo-player
```

Place `wlo-player.exe` beside `wonderland-client.exe`, `SERVER.INI`,
`client-assets.zip` and the companion packs. The client server list determines
the destination. The server must authorize Agent / gameplay API access.

Create a local `.secrets` file beside the executables. Each Username/Password
pair is a game account. Each profile must have an allowed account set. Its agent
sees only those account numbers and chooses which to use within that set.
Add the model API key locally. Keep the file private and outside version control.

```text
Username: your-game-account
Password: your-game-password
OPENAI_API_KEY=your-openai-key
```

`OPENAI_API_KEY` in the process environment takes priority over the file.
Numbered pairs such as `username_01=...` and `password_01=...` are also accepted,
in file order. The username and password suffix must match.
To force the initial account, use `-account 2` with `-character 1` or
`-character 2`. By default `-account 0` lets the agent choose its initial account
and open additional accounts through `controller.openAccount`. If a selected slot is empty, startup creates a character
through the ordinary creation forms and enters once the server confirms it.
For an empty slot, a separate model request chooses the character's name, native
portrait archetype, element, starting stats and all four RGB tint controls. It
uses the controller persona, ongoing goals, native portrait bonuses and existing
character names. There is no fixed Rover/Fire/default-color creation template.
The choices are validated and saved with the private deletion code in
`agent/account-N-character-N/character.private.json` before submission. Retries
reuse that design. Rejected names can trigger a new design, with at most three
design/name attempts per startup. Credentials and the deletion code are never
included in the design request. Keep this private file with your account records.
Existing characters retain their appearance and are selected without requesting
a new design; customization applies when a chosen slot is empty.

Portraits outside the native carousel viewport are brought into view with its
arrows; carousel motion also advances in headless sessions without rendering.
If startup returns `needsAttention`, opening the same account and character again
resumes its existing session through the controller's private startup path.

Character design and gameplay guidance describe the elements' broad tendencies:
Water favors support, Wind favors speed and dodging, Fire favors raw damage, and
Earth favors defense. Agents may use these to plan party roles while keeping
their own preferences and checking actual skills and effects during play.

Names follow the shared server rules and the native wizard's ten-byte editor
limit (4–10 printable ASCII bytes). The five free stat points are allocated in
STR/CON/INT/WIS/AGI order, separately from the archetype's built-in bonuses. Color
choices use the wizard's four RGB tint groups: digits 0–9, with 4 neutral, rather
than absolute 0–255 RGB values. Both the updated controller and native client are
needed for the creation catalog and color-state observations.

```powershell
.\wlo-player.exe --profile Aria --accounts 1,2 --duration 1h
.\wlo-player.exe --profile Meteo --accounts 3,4 --duration 1h
```

The default model is `gpt-5-mini`, configurable with `-model`. Each decision
sends the current game observation, API contract, recent results and a compact
play memory to OpenAI. Model requests incur API charges. The default run ends
after one hour of elapsed gameplay, starting when the first character enters the world.
This is one shared deadline for the whole group. Opening or closing another
account does not restart it. Each account can have one active character at a time.
Walking, cinematics and spectator intervention count toward this hour. Use
`-duration 30m` to select another duration. An optional `-decisions` cap can end
a diagnostic run earlier. This cap counts gameplay/coordinator decisions; the
one-time character-design requests and bounded name retries are additional.
Pending model requests and local API waits are cancelled
when the duration expires, and the hosted connection closes. Token usage is
recorded in the profile directory's `usage.jsonl`. There is no monetary
spending cap in this tool.

## Independent profiles

Run the two commands above in separate terminals to run independent controllers.
Aria prefers exploration, stories and variety. Meteo prefers levels, stats,
equipment and strong teams. These are decision preferences, not guaranteed
behavior or fixed scripts. Each controller has its own host, spectators,
one-hour budget, memory, journal and reports.

`--accounts` saves the allowed set in `agent/profiles/aria/profile.json` (or
`meteo/profile.json`). Later launches need only `--profile Aria`. Account numbers
refer to the entry order in `.secrets`; keep that order stable. The controller
enforces the set before login, including explicit `--account` selections.
Gameplay UI commands are blocked until a managed session enters the world;
the model cannot use login/registration forms to bypass account assignment.
Use disjoint sets for predictable independent play. Overlapping sets are permitted,
but an account reserved by another controller is unavailable until released.
An unconfigured profile cannot start gameplay: it requires an explicit account set.

Edit each profile's `name`, `preferences` and `accounts` fields while it is stopped.
Custom profile IDs are supported; `--profile Nova --accounts 1 --setup-profile`
creates its configuration without starting a game or calling the model. IDs are
case-insensitive ASCII letters, digits, underscores or hyphens. The profile name
does not rename existing game characters. Preferences are sent to the model each
decision and do not relax gameplay permissions or spectator ownership checks.

OS-backed locks prevent two controllers from writing the same profile concurrently
or logging into the same reserved account. The shared directory defaults to
`WonderlandGonline/controller-locks` under the current user's configuration folder
(`%APPDATA%` on Windows). Use the same `--coordination-dir` for controllers running
under different OS users. Empty lock files are expected and remain after shutdown;
the open OS handle determines ownership, and a controller crash releases it.
This is local coordination; manual clients and controllers on other computers
still rely on the server's duplicate-login rejection. An orphaned game host may
remain connected after forcibly killing its controller; a released local lock
does not override the server's active account session.

The unnamed launch uses the `default` profile and also requires an account set on
first use. Existing `agent/group-account-*/` logs and memories are left untouched;
named profiles start with independent memory and never import another profile's
memory automatically.

## Watching and control

Each opened character gets a native spectator window by default. Use those
windows to watch the different members. `controller.watchSession` can reopen
a closed spectator, while `-spectator=false` starts without automatic viewers.
Take control pauses agent submissions for that character.
Return to API resumes them. Decisions made against an earlier map, battle turn
or input ownership generation are discarded. Closing the spectator leaves the
controller and hosted character running. Stop the controller with Ctrl+C to
close its hosted game connection. Restarting reconnects and loads play memory.
One native host owns all the group's game connections. Each model decision
contains an explicit session ID. The controller checks the chosen character's
own map, turn and input ownership before dispatch. The model receives snapshots
of all controlled characters and may use `world.target` to join a controlled
teammate's party or battle. Joining still requires the normal server confirmation.
Workspace changes pause while any spectator owns input.

## Gameplay and reports

The model receives semantic state and visible NPCs. It interacts with NPCs by
their map click ID using `world.interactNPC`, without
screen-coordinate clicks. NPC snapshots expose live interaction eligibility;
moving actors are resolved again when the command executes. Their template IDs
identify definitions and must not be used as interaction targets.
Model output constrains operation names to the filtered live API contract, so
descriptions such as “Interact with Persian Cat” belong in the reason field and
cannot replace `world.interactNPC` as the command identifier.
It can request navigation and asset lookups, inspect forms and choose ordinary
game actions. Login happens
inside the controller. Passwords and API keys are removed from model input and
controller journals. Model output has no filesystem, shell or browser tools.
The prompt allows in-game chat, while directing it to avoid unsolicited mail,
trade or PK and spending IM
or Bonus points. Normal NPC purchases using earned gold are allowed.

Each account/character keeps its private creation profile under `agent/`, for
example `agent/account-1-character-1/`. Each controller profile keeps its play
memory and reports in `agent/profiles/PROFILE/`, independent of its initial
account/slot choice. Characters within that profile share its memory.
Its `journal.jsonl` records decisions, dispatch results and query results with
session IDs. `memory.json` stores the group's ongoing goals and discoveries.
`bugs.jsonl` records suspected problems with the location and recent context.
These are reports of observations, requiring investigation before treating them
as confirmed defects. Local command execution must be checked against later game
state. The controller stops on model/transport failures and never automatically
replays an uncertain transaction. It waits through walking, loading and movies.

The controller is prepared for autonomous use, but exploration quality, combat
timing and long-session recovery require live evaluation with the selected model.

## Controller validation

`go test -race ./cmd/wlo-player` covers session routing, profile memory isolation,
allowed account sets, reservation exclusion and reservation recovery after a
controller crash. It also covers model-selected character designs, wizard control
routing, persisted choices and name rejection recovery. Model decisions use a
local mock server in these tests.

The separate `TestLiveNativeGroup` integration test is disabled unless
`WLO_PLAYER_LIVE_ROOT` points to an exported client directory. It uses the first
two accounts in that directory's `.secrets`, enters slot 1, creates a character
if empty, and temporarily forms a party when the characters can see each other.
Use dedicated test accounts. Private tutorial scenes skip the party subtest;
independent login and closing one session while retaining the other are still
checked. It uses a fixture character design, does not call OpenAI and does not
establish autonomous model gameplay acceptance.

The model request uses the documented [Responses API](https://developers.openai.com/api/reference/resources/responses/methods/create)
and [structured outputs](https://developers.openai.com/api/docs/guides/structured-outputs).
See [HEADLESS_API.md](HEADLESS_API.md) for the game transport contract.
