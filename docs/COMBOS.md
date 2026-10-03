# Party attack combos

The server joins adjacent single-target attacks against the same enemy when each
neighboring pair differs by at most 99 effective SPD. The fastest participant
starts the combo. There is no two-fighter limit: all four players and four pets
can participate.

For example, SPD 150 → 60 → 30 forms one combo. Without the 60 SPD attacker,
150 and 30 act separately. SPD 294 → 199 → 100 is another valid chain, even
though the fastest and slowest differ by 194.

## Execution rules

- Use effective SPD, including all active skill effects, when ordering the round.
  Keep that snapshot for the collected round; later speed effects affect the next
  round's order.
- Support actions, capture attempts and enemy turns keep their place in the turn
  order and separate combos. Different targets and sides also separate combos.
- Multi-target skills cannot join a combo. Targeting uses the SQL-loaded skill's
  area classification at the user's learned grade, including pets.
- Dead, absent, blocked, unlearned or SP-starved fighters cannot provide a bridge.
  Recheck participants before damage. If a bridge becomes unavailable or attacks
  an ally through confusion, disconnected attackers execute separately.
- Emit one native animation with one hit record per participant. Spend each
  participant's SP and save their skill proficiency through the existing battle
  commit path. Apply combined damage once, capped at the native signed damage
  limit; a knockout does not split the combo across different enemies.
- Roll once per eligible chain against its actual target before executing it.
  Failed chains execute as single attacks in their existing speed order, without
  a combo damage bonus or additional rolls for smaller subsets.
- Apply the shared 30 percent combo damage bonus, individual critical rolls,
  protection effects, and reward calculation.

## Evidence and remaining parity

The user saved these original forum pages under `docs/References/` after their
URLs returned HTTP 403 during investigation:

- [輔掛原理分析與教學 (Online, 91889)](https://forum.gamer.com.tw/C.php?bsn=8897&snA=91889)
  gives the 100 → 199 → 294 bridging example and includes player/pet parties.
- [普掛經驗值實驗 (Online, 88190)](https://forum.gamer.com.tw/C.php?bsn=8897&gothis=527313&page=1&snA=88190)
  records combos of three and four attackers and the fastest participant leading.
- [拖車隊伍配速 (M, 1254)](https://forum.gamer.com.tw/C.php?bsn=31536&snA=1254)
  discusses four players and four pets and excludes multi-target attacks.
- [想問一下 (M, 1045)](https://forum.gamer.com.tw/C.php?bsn=31536&snA=1045)
  discusses the influence of participant and enemy levels on combo probability.

The speed limit is interpreted as strictly less than 100 (99 inclusive), matching
those examples. The posts also describe guaranteed combo probability when the
whole party's average level minus 25 exceeds the enemy's level. They do not
supply the complete probability formula below that threshold. The user requested
this linear compatibility curve instead:

`chance = clamp(0.5 + (partyAverageLevel - enemyLevel) / 50, 0, 1)`

| Party average minus enemy level | Chance |
| --- | --- |
| −25 or less | 0% |
| −12.5 | 25% |
| 0 | 50% |
| +12.5 | 75% |
| +25 or more | 100% |

Use the fractional average of all present players and pets on the attacking side,
including support casters and knocked-out fighters. Exclude absent fighters who
have left the battle. Use current fighter levels plus 99 for each reborn player or pet before
averaging. Apply the same effective-level rule to a reborn target. Visible and
stored levels stay unchanged; the calculation uses integers beyond the native
byte-sized level limit. Do not consume random rolls at guaranteed/zero endpoints or for single
attacks. Speed eligibility remains independent of the probability curve. This
curve is a user-requested policy, not a recovered native probability formula.

The saved sources do not establish whether attackers may cross an intervening
enemy or support turn. The existing ordering barriers remain the compatibility
policy. The existing damage bonus and native client rendering of eight hit
records also still need comparison against the original game. Burst EXP is
outside this change.


Character rebirth is represented by optional saved `reborn` metadata; pets use
their existing flag. Missing flags mean non-reborn. This supports combo level
calculation; character rebirth progression, jobs and roster presentation remain
unported.
