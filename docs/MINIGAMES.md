# Native client minigames

The client ports the WLRI aLogin sport manager (`FUN_003bd398`, build
`ca19ee087b60`). The manager accepts AC57/1, hides the world HUD, opens the
native explanation form, and restores the HUD and map music on AC57/2.
Start does not spend points. Leave reports the event result once through AC57/1.
Closed-game callbacks and replies must not affect a subsequent game.

## Coverage

This is an inventory of the 22 sport kinds in the native manager. It does not
count unrelated feature forms as completed games.

| Kind | Native game | Client coverage | Main native reference |
| --- | --- | --- | --- |
| 1 | Throw | Pending: shared NPC/map scene data, charging, projectile and team controls | `FUN_001ae788`, `FUN_001af7d8` |
| 2 | Scotd | Playable movement, strikes, enemy lifecycle and result; native collision/animation fidelity remains partial | `FUN_0017be1c`, `FUN_0017c0dc` |
| 3 | Mole / rabbit / turtle | Playable | `FUN_001b3090`, `FUN_001b4820` |
| 4 | Hunter | Playable; some missing native effects | `FUN_0017e2c8`, `FUN_0017f714` |
| 5 | Sheep / pig dreams | Playable rules, native dream sheets and click regions; actor animations remain incomplete | `FUN_00138f3c`, `FUN_001391b0`, `FUN_0013a28c` |
| 6 | Egg draw | Native points request, reward display and opening animation | `FUN_001743d0`, `FUN_001749f4`, `FUN_00174cd8` |
| 7 | Claw / catch doll | Pending: crane controls and AC71 outcomes | `FUN_0014089c`, `FUN_00141c44` |
| 8 | Slot machine | Native points request, server reels and reward display | `FUN_001772c0`, `FUN_00178730` |
| 9 | Boxing | Pending: three punches, purchase flow and AC71 replies | `FUN_00142ccc`, `FUN_00143864` |
| 10 | Slot machine 2 | Native request with no selected ticket, server reels and reward display | `FUN_00178d24`, `FUN_001799cc` |
| 11 | Mario | Pending: reel controls and paid game lifecycle | `FUN_0014618c`, `FUN_00146840` |
| 12 | Mario 2 | Pending: reel controls and paid game lifecycle | `FUN_00146558`, `FUN_00146840` |
| 13 | Lucky falling objects | Playable: pointer movement, catches, bombs and timed result; emitter actor remains incomplete | `FUN_0016c4fc`, `FUN_0016d170`, `FUN_0016d4c8` |
| 14 | Mario 3 | Pending: reel controls and paid game lifecycle | `FUN_00146594`, `FUN_00146840` |
| 15 | Numbered memory | Playable: memorize unique digits, then click in ascending order | `FUN_0014a558`, `FUN_0014a98c`, `FUN_0014ac84` |
| 16 | Poke | Pending: native selection controls and server outcomes | `FUN_00169630`, `FUN_0016afc4` |
| 17 | Pumpkin | Playable ghost lanes, aiming, lives, timer and result; native collision/animation fidelity remains partial | `FUN_00161e5c`, `FUN_00162d94`, `FUN_00163538` |
| 18 | Gobang | Pending: lobby, seats, turns and AC57/11 synchronization | `FUN_001654c4`, `FUN_00166cf4` |
| 19 | Slot machine 3 | Native points request, server reels and reward display | `FUN_0017a3b0`, `FUN_0017bb14` |
| 20 | Digging | Pending: grid selection, purchase mode and server prize decoding | `FUN_00167fac`, `FUN_00168748`, `FUN_00168810` |
| 21 | Tile turning | Pending: board controls and server outcomes | `FUN_0014c228`, `FUN_0014cf04` |
| 22 | Egg draw 2 | Native points request, reward display and opening animation | `FUN_001743d0`, `FUN_001749f4` |

Unsupported kinds retain the manager's existing loss response. Other native
forms include watermelon, conundrum, chess, dice and lotto; their entry and
network flows need separate investigation. Lucky (kind 13) is the falling-object
sport, distinct from the server's daily Lucky Draw feature (AC104).

## Throw entry points and source data

Throw (kind 1) has NPC-linked launch operations in the imported WLRI EVE
content; it is not merely an unreferenced sport-manager entry:

| Map | NPC click ID | NPC template | Event | Launch parameter |
| --- | ---: | ---: | ---: | ---: |
| 12150 | 10 | 14605 | 14 | 14605 |
| 60008 | 12 | 14304 | 16 | 14304 |

The event labels are `馬雅玩投擲遊戲` (Maya throwing game) and
`夏威夷沙灘排球` (Hawaii beach volleyball). Both launches are operation index 2
in branch 7, with EVE opcode 9, sport kind 1 and D3 = 1. The NPC event-link
bytes are `0e` and `10`, respectively. Branch conditions still govern when
interaction reaches the launch; these are content references, not a live-client
acceptance test.

The previous `Throw.dat` requirement was incorrect. Native `FUN_001ae968`
initializes teams through `FUN_001aef04` / `FUN_001af02c`, which look up the
shared NPC records. It loads scene pictures through the shared map table,
using map ID **59000 + the scene variant**, with picture references at native
record offsets `0x58`, `0x5a` and `0x5c`. No separate Throw data file is required
by this initializer. The Go Throw engine and verification of its complete start
packet remain pending; absence of `Throw.dat` does not block that work.

## Server-owned paid games

`client/wlo/minigame/arcade.go` contains the egg and slot presentation and AC71
state machine. `internal/server/arcade_payments.go` implements their authoritative
AC71 purchases and rewards:

- Charges, balances, purchases, prize selection and inventory grants belong to
  the server. The client never chooses a random prize or modifies the inventory.
- Clicking Play opens a purchase prompt. Clicking it again confirms the native
  points request. Further clicks are ignored while awaiting the reply or playing
  the outcome animation. Opening the explanation form and clicking Start send
  no purchase request.
- A reply is accepted only for the active, pending game. Truncated, invalid and
  duplicate replies leave the pending operation unchanged. A points denial
  enables another confirmed attempt.
- Egg replies carry a prize-table index and quantity. Slot replies carry a
  prize-table index, **three reel positions, then quantity**. The binary
  dispatcher at `002ee125` confirms the fourth stack argument omitted from the
  decompile. Golden tests use literal packet bytes independently of the encoder.
- Prize-index translation tables are WLRI executable compatibility data, cited
  beside their Go definitions. Item names and icons come from the extracted item
  catalog. Inventory updates remain separate server messages.

Current requests, including the command byte:

| Kind | Request |
| --- | --- |
| 6 / 22 | `71, kind, 1` (points payment) |
| 8 / 19 | `71, kind, 1, 0` (points, no selected inventory ticket) |
| 10 | `71, 10, 0` (no selected inventory ticket) |

Egg token payment and slot ticket selection are still pending. The client does
not send those modes before their native inventory/confirmation flows are ported.
There is no practice mode or local reward fallback.

Private Server's AC71 handler returns only a fixed placeholder score. Go now
implements native purchases for **6, 8, 10, 19 and 22**. Opening via AC75:4 or an
EVE minigame records the machine and map; AC71 must match that ownership. Leaving
through AC57 clears it. Purchases do not consume an EVE result callback. Map/event
transitions clear ownership, and reopening does not reset the animation throttle.

### Payment and rewards

| Kind | Points per play | Tokens per play |
| --- | ---: | ---: |
| 6 | 20 | 4 |
| 8 | 5 | 1 |
| 10 | 2 | Not accepted |
| 19 | 10 | 2 |
| 22 | 20 | 10 |

Fees and token counts come from native `FUN_00176e88`. Egg and slot modes 2/3
select Fun Token (34007) / Play Token (34008). Kind 8/19 includes a fourth-byte
inventory voucher slot; kind 10 includes that slot as its third byte. Zero means
points payment. **Inferred voucher rule:** one voucher (34360) in the selected
slot replaces one play's point fee. Mixed voucher/token payment is rejected.
Bonus points are never used. The Go client currently exposes only points payment;
the server accepts the native token/voucher requests too.

The seed in `internal/assetsql/arcade_defaults.json` initializes typed
`catalog_arcades`, `catalog_arcades_rewards` tables during explicit creation or
schema v3-to-v4 migration. Existing rows are preserved on repeat migration and
startup never rereads the seed. Machines with missing prize definitions are
seeded disabled. Use the administration **Arcades** definition editor, followed
by its normal catalog publication/reload, to change enabled state, costs, token
counts, cooldowns, weights, quantities or reel positions. Weights start at 1 for
every reward; weight 0 excludes a reward. Selection uses cryptographic randomness
against the total positive weight. Native prize-index/item identities are fixed
and validated because aLogin displays a compiled table.

**Balance assumptions:** original server odds and award quantities are unavailable.
Initial quantities are one; reel patterns are provisional presentation outcomes,
not recovered original server rules. Initial throttles are 3 seconds for eggs
and 4.5 seconds for slots; SQL permits 1–60 seconds. The wire has no purchase ID,
so this animation throttle prevents rapid repeats, but cannot distinguish a late
retransmission from a new confirmed play after the interval or a reconnect.

Payment removal and inventory delivery share one gameplay SQL transaction.
Insufficient funds/items, reserved items or insufficient inventory capacity roll
back the entire play. Inventory must fit every positive-weight reward, preventing
selective free rerolls. Success packets are sent only after commit; a socket error
does not undo a completed play. Egg success is `71,kind,1,index,quantity`; slot
success is `71,kind,1,index,reel1,reel2,reel3,quantity`. Denial is `71,kind,2`, with
an explanatory banner. Native clients may label all denials “Not enough Points”.

Upgrade existing asset databases with the preserving offline copy procedure in
[ASSET_DATABASE.md](ASSET_DATABASE.md). Native-client acceptance testing remains
necessary; packet/transaction tests cover the wire layouts and persistence.
Other AC71 sport kinds and AC72 remain pending; unsupported games do not receive
fabricated rewards.

## Rules and rendering

New local engines live in `client/wlo/minigame`. `Round` handles one result and
Start/Leave, while each engine owns its timers, hit regions and score:

- Scotd: arrow controls move, jump and duck; Space or click strikes. Twenty
  enemies spawn at fifty-frame intervals, each with three hit points. Twenty
  defeats win; damage grants a hundred frames of immunity. Native life count
  is five minus the launch difficulty.
- Pumpkin: four-second countdown, fifty seconds, three lives and thirty hits
  to win. Ten ghost lanes rise and return; a missed returning ghost costs a
  life. Click aims one projectile at a time; the result waits three seconds.
- Dreams: three lives, thirty seconds; odd dream cues score when hit, even ones
  must be left alone. Missing an odd cue costs a life. Surviving wins.
- Lucky: four-second countdown, thirty seconds, three lives, thirty catches to
  win. The character follows the pointer in ten-pixel steps; the emitter moves
  seven pixels per 30ms frame. Catch dimensions come from the native pictures.
  A bomb costs a life. The result waits three seconds.
- Memory: three lives, twelve completed rounds to win, three to five unique
  digits across these rounds. A one-second preview consumes part of the
  ten-second deadline. Wrong or late clicks lose a life. Correct and incorrect
  feedback have separate delays.

Pictures come from the shared extracted `data` directory through the existing
client asset loader. Animation strips draw individual rows. Simple text labels
are a rendering fallback, and some native actor/effect animations are still
pending. These ports have engine and protocol tests; they have not all been
compared interactively with the original running games.

## Focused validation

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
