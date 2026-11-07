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
| 1 | Throw (dodgeball) | Playable: teams, charging, throws, computer players, lives and result; see [Throw](#throw) | `FUN_001ae788`, `FUN_001af7d8` |
| 2 | Scotd | Playable movement, strikes, enemy lifecycle and result; native collision/animation fidelity remains partial | `FUN_0017be1c`, `FUN_0017c0dc` |
| 3 | Mole / rabbit / turtle | Playable | `FUN_001b3090`, `FUN_001b4820` |
| 4 | Hunter | Playable; some missing native effects | `FUN_0017e2c8`, `FUN_0017f714` |
| 5 | Sheep / pig dreams | Playable rules, native dream sheets and click regions; actor animations remain incomplete | `FUN_00138f3c`, `FUN_001391b0`, `FUN_0013a28c` |
| 6 | Egg draw | Native points request, reward display and opening animation | `FUN_001743d0`, `FUN_001749f4`, `FUN_00174cd8` |
| 7 | Claw / catch doll | Pending: crane controls; the server sends raw prize item IDs, with no native prize table | `FUN_0014089c`, `FUN_00141c44` |
| 8 | Slot machine | Native points request, server reels and reward display | `FUN_001772c0`, `FUN_00178730` |
| 9 | Boxing | Pending: three punches and purchase flow; raw prize item IDs, as kind 7 | `FUN_00142ccc`, `FUN_00143864` |
| 10 | Slot machine 2 | Native request with no selected ticket, server reels and reward display | `FUN_00178d24`, `FUN_001799cc` |
| 11 | Mario (fruit machine) | Native reels, lamps, price prompts and server outcomes; see [Fruit machines](#fruit-machines) | `FUN_0014618c`, `FUN_00149694` |
| 12 | Mario 2 (fruit machine) | As kind 11, with point or token payment | `FUN_00146558`, `FUN_00149694` |
| 13 | Lucky falling objects | Playable: pointer movement, catches, bombs and timed result; emitter actor remains incomplete | `FUN_0016c4fc`, `FUN_0016d170`, `FUN_0016d4c8` |
| 14 | Mario 3 (fruit machine) | As kind 12 | `FUN_00146594`, `FUN_00149694` |
| 15 | Numbered memory | Playable: memorize unique digits, then click in ascending order | `FUN_0014a558`, `FUN_0014a98c`, `FUN_0014ac84` |
| 16 | Poke (card pairs) | Pending: server-dealt faces over AC57:9 and raw prize item IDs; see [Poke](#poke) | `FUN_00169630`, `FUN_0016a2e4` |
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

## Sheep and pig dreams

The EVE launch carries a UInt16 parameter in D2 and a variant byte in D3.
Kelan's sleeping-pig event (map 12000, event 17) uses parameter 11000 and
variant 2, producing `[57,1,5,248,42,2]`; variant 1 is the sheep game.
The server preserves authored D3 instead of forcing the default variant 1.
Scripts with D3 zero retain the historical default. `FUN_003bd398` passes
the final byte to `FUN_00138f3c`. The client selects the matching explanation,
life icons, dream sheets and sleeper template. During active play it selects
the hammer cursor; the explanation and completed round use ordinary UI cursors.

Dream-game rendering follows `FUN_001398a8` and `FUN_00139c60`: native life,
time and score pictures, variant-specific life icons, vertically sliced dream
artwork, and the fixed-position `Hammer_1` sheet at (375,200). The hammer is
three stacked frames, selected by `FUN_001391b0`'s 30 ms ticks: ready through
tick 3, raised at 4–7, striking at 8–10, then ready again. Valid target clicks
restart a ready hammer; repeated strikes do not score or charge lives twice
for the same cue. Diagnostic target rectangles and cue-number labels are not
drawn. The sleeper uses the native (475,550) anchor and waking action on loss.

Focused tests cover hammer frame boundaries, click/scoring independence and
the clean dream area. Set `SHEEP_SNAPSHOT` and run
`go test ./wlo/app -run TestPigDreamReferenceSnapshots -count=1` from `client/`
to render the explanation, ready and striking scenes using the actual assets.
Initial visual checks are in `var/validation/pig-dream/`. Full native sleeper
animation and impact-effect parity remain separate from these corrections.

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
by this initializer.

## Server-owned paid games

`client/wlo/minigame/arcade.go` contains the egg and slot presentation and AC71
state machine. `internal/server/minigames.go` implements their authoritative
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
| 11 | `71, 11` (points; serializer 0x2da5e1) |
| 12 / 14 | `71, kind, mode` (1 points, 2 Fun Tokens, 3 Play Tokens) |

Egg token payment and slot ticket selection are still pending. The client does
not send those modes before their native inventory/confirmation flows are ported.
There is no practice mode or local reward fallback.

Private Server's AC71 handler returns only a fixed placeholder score. Go now
implements native purchases for **6, 8, 10, 11, 12, 14, 19 and 22**. Opening via AC75:4 or an
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
| 11 | 25 | Not accepted |
| 12 | 100 | 20 |
| 14 | 5 | 1 |

Fees and token counts come from native `FUN_00176e88`. Egg and slot modes 2/3
select Fun Token (34007) / Play Token (34008). Kind 8/19 includes a fourth-byte
inventory voucher slot; kind 10 includes that slot as its third byte. Zero means
points payment. **Inferred voucher rule:** one voucher (34360) in the selected
slot replaces one play's point fee. Mixed voucher/token payment is rejected.
Bonus points are never used. The Go client exposes points payment for eggs and
slots, and points or token payment for fruit machines 12/14; the server also
accepts native egg/slot token and slot voucher requests.

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

**Balance assumptions:** the initial implementation did not have original server
odds and award quantities. The subsequently supplied official-site snapshots in
`var/slots` publish rewards and probabilities; see the investigation below.
Initial quantities are one; reel patterns are provisional presentation outcomes,
not recovered original server rules. Initial throttles are 3 seconds for eggs
and fruit machines, and 4.5 seconds for slots; SQL permits 1–60 seconds. The wire has no purchase ID,
so this animation throttle prevents rapid repeats, but cannot distinguish a late
retransmission from a new confirmed play after the interval or a reconnect.

Payment removal and inventory delivery share one gameplay SQL transaction.
Insufficient funds/items, reserved items or insufficient inventory capacity roll
back the entire play. Inventory must fit every positive-weight reward, preventing
selective free rerolls. Success packets are sent only after commit; a socket error
does not undo a completed play. Egg success is `71,kind,1,index,quantity`; slot
success is `71,kind,1,index,reel1,reel2,reel3,quantity`. Denial is `71,kind,2`, with
an explanatory banner. Native clients may label all denials “Not enough Points”.

Upgrade existing asset databases with the preserving offline upgrade procedure in
[ASSET_DATABASE.md](ASSET_DATABASE.md). Native-client acceptance testing remains
necessary; packet/transaction tests cover the wire layouts and persistence.
The fruit machines' prices come from their arming callbacks (`FUN_00146b98`,
`FUN_00147e40`, `FUN_00147fc8`) and token callbacks (`FUN_00147890` 20,
`FUN_001479dc` 1). Schema v13 appends missing seed machines to existing databases
and leaves existing rows untouched, including disabled machines. It can restore
deleted egg/slot definitions too; disable a machine rather than deleting its row
to keep it unavailable through this upgrade.

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

See [minigame checks](TESTING.md#minigame-checks) for engine/packet tests and
snapshot commands. Native source references and gameplay limits remain here.

## Fruit machines

Kinds 11, 12 and 14 are the "Little Mary" fruit machines (`MarioGround`,
`client/wlo/minigame/mario.go`). Events launch kind 11 and 12 in the casino
(map 11094, events 25 and 30) and kind 14 at the Great Wall (map 12120,
event 7). Each machine is the same class with its own prize table
(`PTR_DAT_004ca80c`, `004c9888`, `004c9ec0`) and price.

- **Start**: the constructor asks the balance with AC34:1; 35/4 sets the shown
  points (+0xac). The explanation form paints the price in blue at
  (0x186, 0x77). Its Start shows the machine's form: Leave
  (`Hunting_Exit_Btn` at 700, 573), the red `Mario_Start` button at
  (683, 409) and invisible hint buttons 0x18 below each prize icon (Mario 1
  shows the item description, the others the name).
- **Paying**: the first press asks the price in a talk window
  (`FUN_0034d150`). With Fun or Play Tokens in the bag, Mario 2 and 3 first
  offer `Points`, `Token` or `Fun Token` (`FUN_0034d00c`). Confirming checks
  the shown balance or token count and a free bag slot, then arms the
  machine and plays `BGM0019`. Only the next press sends AC71. After one
  confirmed game the machine re-arms by itself (the start form's +0x140).
- **Replies**: 2 disarms with "Not enough Points!", 5 reports a full bag, 3
  (`FUN_00149040`) and 4 (`FUN_001488cc`) carry the prize index, three reel
  positions (mapped to strip frames by `FUN_0014966c`'s tables) and the
  quantity; reply 4 has a lamp byte before the quantity.
- **Reels** (`FUN_00149694`, one step per 30 ms frame): the three 420 × 70
  strips turn (top and bottom forward, middle backwards, 4/5/6 frames a
  picture) until the top one has changed four times. Then each reel stops,
  in order, on its target after at least four more changes, with `9911`; `9912`
  repeats every 800 ms while they turn. A jackpot (reply 3 with a prize)
  flashes `Mario_Bingo`, a lamp win blinks its halves, then the reward
  message shows for 3.5 s with `wav0152`.
- **Leaving** asks "Leave the game? No points will be used" while a game is
  armed. The machine always reports a loss; prizes come only from AC71.

Reel positions 1-8 show the legend's eight animals (angel, jellyfish, frog,
squirrel, chick, dog, mushroom, cactus); the legend's ninth row is a cactus
on the middle reel and the tenth any other line. The seed follows that
legend. Which reply the original server chose is not recorded; the server
sends three matching reels with a prize as reply 3 and every other line as
reply 4, lit for prizes 1-10 (an inferred compatibility rule).
Packet/reel checks and explanation/spin/stopped-reel rendering are documented
in [minigame checks](TESTING.md#minigame-checks).

## Throw

Kind 1 is a dodgeball game (`client/wlo/minigame/throw.go`; constructor
`FUN_001ae788`, set-up `FUN_001ae968`, update `FUN_001af7d8`, draw
`FUN_001af424`). It runs entirely on the client and reports its result with
57/1.

- **Sides**: the player's side stands on the right from (600, 450), 25 pixels
  apart diagonally, and the launch template's NPCs on the left from
  (200, 450). Alone, the player gets three of the template's figures as
  computer-run teammates. In a party the members line up, leader first, but
  only the player moves: no packet synchronises the others, who stand still.
  Each side has as many figures as the player's side, each with three lives
  (hearts above the head, `FUN_001b1b0c`).
- **Controls** (`FUN_001af944` press, `FUN_001afa90` release): a press on the
  player charges. Power rises every 60 ms up to 9, then every 30 ms up to
  15, and shows on `Str_Bar`. The release plays the throw, and the ball
  leaves on frame 3 toward power × 15, 25 or 40 plus 230 pixels to the left
  (`FUN_001b03b0`). A press elsewhere in the right court (500-800 × 430-550)
  runs there at 0.3 px/ms; the longer axis moves at full speed and the
  shorter one in proportion (`FUN_001aff54`).
- **Computer players** (`FUN_001b0b38`): every 1-2 seconds each picks one of
  seven moves. Two of them aim for a second and throw with a random power,
  at a random opponent or (for teammates, two times in three) straight
  ahead. Two run to a random spot of their court; the rest wait.
- **Balls** (`FUN_001b0688`): up to 50 fly at 0.5 px/ms on the battle leap
  arc, `a = (0.5 − |left/length − 0.5|) × 360`, height `2 × (a − 0.003a²)`.
  A ball hits a figure standing near its target while it passes them
  (`FUN_001b1700`). The hit costs a life with `SEB0036` and the `S10203`
  burst, and the figure flinches for a second. A ball that reaches its
  target misses with `SEB0051` and `S10004`.
- **Result** (`FUN_001b1cbc`): three seconds after the player loses the last
  life (a loss) or every opponent is down (a win). Leave
  (`ThrowGame_Exit_Btn` at 700, 20) gives up with a loss. Music is `BGM0018`.
- **Scene**: battle scene 59000 + the start packet's scene byte, drawn as
  battles draw it. The EVE launch operations carry no scene, and the
  recorded captures cover only kinds 3 and 4. The server therefore sends
  Jungle (90) for the Mayan Tribe (map 12150) and Beach (50) for Large
  Island (60008); these are compatibility choices. Other maps send 0, which
  has no record and so no scene.

The Throw start packet is `[57,1,1,templateLo,templateHi,parameter,scene]`:
the scene is at offset 6 including command 57 (the sixth byte after that command).

Sprite frame counts are not consulted: the release frame is reached by time
(100 ms a frame), so a throw sprite with fewer than four frames still
throws. Scene-variant/flow checks and explanation/play snapshots are documented
in [minigame checks](TESTING.md#minigame-checks).

## Poke

Kind 16 deals eight cards (`PokeCardBack`, `PokeCard_1`-`15`) in two rows. The
player pays 10 points or 2 tokens through AC71 (`71, 16, mode`), and the server
answers 57/9:6 to deal. Each flipped card is requested with `57, 9, 3, card`,
and the server returns its face and the step (`57, 9, 3, card, face, step`).
Steps 2 and 4 compare a pair (`FUN_0016a44c`), and step 4 ends the round with
`57, 9, 4`. Leaving sends `57, 9, 2`; leaving after a pair is answered by a
consolation item with AC71 16/4. The client never holds the faces or a prize
table: the server deals and rewards with raw item IDs. Claw (7) and Boxing (9)
likewise receive raw item IDs (`FUN_00141cd0`, `FUN_001435f0`). Published reward
names and probabilities are now available in `var/slots/02.htm`; the missing
ports still require verified item mappings and interactive server rules.

## Published mall probabilities: local `var/slots` investigation

Investigated 2026-11-07. These saved ChineseGamer Wonderland pages are Big5/CP950,
not UTF-8. Decode them before matching Chinese names. The accompanying image
folders mostly contain site artwork; the HTML tables contain the useful data.
This investigation records evidence only; it does not alter live SQL catalogs,
default rewards, odds or game availability.

| Saved source | Content | SHA-256 |
| --- | --- | --- |
| `var/slots/01.htm` | Bag/box/chest/wishing rewards, forging, Chaos Crystal and Potential probabilities | `fd5ba9547557200afdbd783fba4f3bcad475e229a936087b9f0811fe627100f6` |
| `var/slots/02.htm` | All ten displayed mall games' prize tables and probabilities | `b356d8d19f89aa2b737860ee9e2f5e8626d2940d63fe7734544378a83630e649` |
| `var/slots/飄流幻境 Wonderland Online.htm` | Another saved rendering of the product-content page; overlaps `01.htm`, not independent corroboration | `38e69a373a26d1bf8d2c57bee5be547b7346be041a2082c86fb731e7d29ef0e9` |

The tables have names, descriptions, quantities and probabilities, but no numeric
item IDs. Match definitions by identity and effects against the untranslated
Chinese catalog, then cross-check the executable's prize-index tables. Page order
is not a wire prize index. Regional/version differences and source typos must be
resolved rather than replacing the native table to fit a translated name.
For example, the first egg table's vehicle voucher names and descriptions disagree
about the vehicle; names alone are insufficient.
There is also a concrete compatibility conflict: the Potion Box page lists a
Lottery Ticket for unmatched reels, but the current executable's kind-10 index
zero maps to item 32095, Fugu Hot Pot. Kinds 8 and 19 instead map index zero to
30063, Lottery Ticket. Preserve these native identities until version/protocol
evidence resolves the mismatch; importing the page's ticket into kind 10 would
make the client display a different item from the actual server grant.

### Coverage and probability meanings

| Mall game | Native kind | Evidence in `02.htm` |
| --- | ---: | --- |
| Baby Dolls / 抓娃娃機 | 7 | Seven doll categories; each has five prizes at 60%, 20%, 16%, 3%, 1%. These are conditional prize probabilities for that category, not the chance of catching that doll. |
| Fortune Egg / 轉蛋機 | 6 | Ten individual rewards at 35%, 24%, 20%, 12%, 4%, then five at 1%. |
| Diamond Hit / 爆擊彩鑽機 | 19 | Eight symbol probabilities and eleven combination outcomes, including a consolation ticket. Critical diamonds span 8% to 32%. |
| Diamond Cast / 彩鑽拉霸機 | 8 | Same symbol probabilities/combination rules; enhancement diamonds from +14 through +50, plus a consolation ticket. |
| Potion Box / 藥水拉霸機 | 10 | Same symbol probabilities/combination rules; 1.5x through 8x EXP potions, plus a consolation ticket. |
| Jigsaw / 拼拼樂 | 14 | Eight picture-category probabilities and eleven outcomes, including partial matches and a consolation ticket. |
| Lucky Wheel / 轉轉樂 | 21 | Four prize tiers: 1%, 2%, 8%, 89%, spanning 5, 10, 10 and 20 items respectively. Within-tier item probabilities are not specified. |
| Lucky Cards / 戳戳樂 | 16 | Thirty-two reward rows under a single explicit equal-probability cell; quantities vary by row. |
| Golden Glove / 拳擊樂 | 9 | Twenty-seven explicitly weighted rewards. First three: healing potion 20%, pizza 12%, phoenix feather 10%; remaining entries are pet armlets at 1–4%. |
| Fortune Egg 2 / 轉蛋機二代 | 22 | Ten individual rewards with the same 35/24/20/12/4/1/1/1/1/1 percentages as the first egg; different identities. |

The page additionally lists 翻翻樂, a separate fourteen-reward table with weights
19, 14, 13, 11, 9, 7, 7, 6, 5, 3, 2, 2, 1, 1 percent. It is not automatically
the same as Lucky Cards or Lucky Wheel. Its native entry point remains unresolved;
do not assign it to either kind from the similar English names.

Fortune Egg's first outcomes are pizza 35%, phoenix feather 24%, hot sweet potato
20%, and a player stat-return scroll 12%. Fortune Egg 2 substitutes a selectable
glaze-stone voucher at 24%, with hot sweet potato still 20%. All egg quantities
listed are one. Verify each mapped ID before importing these weights.

The seven claw categories are little monkey, Labrador, Persian cat, Shiba Inu,
rice chicken, chick beast and Taiwan macaque. Their common 60% result is a pet
1% EXP pill; the other four outcomes are category-specific companion vouchers.
Their 60/20/16/3/1 probabilities each total 100%, but the page supplies neither
category-selection odds nor the mechanical success rate of the crane controls.

### Reel rules

For the three slot machines, the page gives angel 9%, crab 10%, tabby cat 11%,
frog 12%, sheep 13%, squirrel 14%, jellyfish 15%, starfish 16%. The lowest prize
requires the right reel to be a starfish; the next requires middle and right
starfish; three starfish award the next tier. Higher awards require three of
another matching symbol; all other combinations yield a ticket. Evaluate triples
before partial starfish matches. The page does not establish numerical reel
positions or whether the reels are statistically independent.

If the three reels are independent, the inferred mutually exclusive weights are
as follows. These are derived probabilities, not additional published percentages:

| Outcome | Probability |
| --- | ---: |
| Right starfish, middle not starfish | 13.44% |
| Middle/right starfish, left not starfish | 2.1504% |
| Three starfish | 0.4096% |
| Three jellyfish | 0.3375% |
| Three squirrels | 0.2744% |
| Three sheep | 0.2197% |
| Three frogs | 0.1728% |
| Three tabby cats | 0.1331% |
| Three crabs | 0.1000% |
| Three angels | 0.0729% |
| Consolation ticket | 82.6896% |

Jigsaw instead lists angel, jellyfish, frog, snail, chicken, Shiba, mushroom and
cactus with probabilities 9% through 16%. Its partial-match rules are same-category
middle/bottom, and cactus middle/bottom; full matching pictures take precedence.
The page does not explain how picture categories map to the three stop positions
or which differently drawn pieces complete a picture. Confirm that with the
native Mario3 code/assets before deriving weights or manufacturing reel patterns.

### Differences from Gonline and remaining work

`internal/assetsql/arcade_defaults.json` currently assigns weight one to every
reward. Fresh catalogs therefore give each egg prize 10%, and each three-slot
prize 1/11, rather than the published distributions. Fruit-machine seeds likewise
use equal weights and omit consolation index zero. Administrators can already
set SQL-owned weights, quantities and reel positions; existing edited catalogs
must be preserved by any future default migration. The supplied pages substantially
improve reward evidence but do not prove the currently connected regional server
uses identical odds. No live database was inspected in this investigation.

The client should continue to animate server-selected outcomes. Published names
and odds do not replace missing interactive protocol/state machines for kinds 7,
9, 16 and 21. The existing decompilation remains the source for controls, payment
requests, step replies, cancellation and animation. The page does not publish
point/token costs, timing or handling of interrupted paid rounds. Kind 16's equal
reward-row weights also do not establish the faces/deal distribution of its
interactive card game. Kind 21's four HTML probability cells use `rowspan`; treating
them as per-item odds would multiply the intended tier rates. Equal selection
inside those tiers would be an explicit policy assumption, not a recovered rule.

`01.htm` provides additional enhancement evidence: point-forging attempts at
levels 1/2/3/10/50/100/200 are respectively
100/17.24/10.88/2.17/0.19/0.06/0.02 percent (the full table has 200 rows, rounded
to two decimal places). `internal/server/crafting.go:forgeRoll` currently uses a
flat coin flip, independent of progress. This is a substantial remaining mismatch;
retain the published table rather than fitting an invented curve. Scroll-family
upgrades are a separate mechanism and should not inherit the point-forging odds.
The Potential table, in contrast, matches `game.PotentialSuccessPercent` for
ordinary pills: 100/100/100/70/60/55/55/50/35/20/18/15 percent for attempts +1…+12.

The same product page publishes bag contents with probabilities, including
Pet Upgrade Bag (夥伴強化福袋). Its older list is not identical to the updated
client's 26-entry preview. Keep the screenshot/native Lbd display evidence and
these published probability tables separately until every differing item and
version is reconciled. Other useful sections include gold chests, the Kelan
wishing pool, Chaos Crystal ranks, and Kyoto lottery/fortune rewards.
