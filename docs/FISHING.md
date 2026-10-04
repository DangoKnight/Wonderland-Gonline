# Fishing

## Playing

Own a Simple Fishing (37105), Great Fishing (37011), or Luxury Fishing (37082)
rod and stand on land near water at a configured beach. Leave your party, finish
active interactions and dismount first. Double-click the rod in aLogin or use
`/fish`; `/stop` cancels casting. Fishing works without learning a fishing skill.
The existing EVE quest system remains responsible for teaching the skill.

The default catch interval is 60 seconds, following the WLRI 2020 observation.
Each interval delivers one weighted catch. There is no catch-up burst after a
scheduler delay. A full bag discards the item while retaining skill proficiency;
a compatible existing stack can still receive it. Movement, travel, disconnect,
battle, trade, party membership and losing the selected rod end casting.

## Definitions and tuning

Structured asset schema v7 introduced the `Fishing` dataset; v8 increases the
default fish/seafood weights. Admin's definition editor
can change it; server gameplay uses its validated startup catalog, so restart to
activate edits. `internal/assetsql/fishing_defaults.json` seeds new databases and
explicit upgrades only. Editing this seed does not replace existing SQL edits.
Runtime never reads the JSON seed from disk.

- `Enabled`: enables fishing; initialization disables it if required rod/catch
  item definitions are unavailable.
- `IntervalSeconds`: seconds between catches (default 60).
- `Maps`: permitted beach map IDs (defaults 11016, 10035, 10039).
- `Rods`: item ID and maximum catch grade (defaults 6, 8, 10).
- `Rewards`: explicit item IDs, content grades and positive weights. The seed
  contains a conservative WLRI-mapped subset of the beginner-beach table: fish,
  coal, steel, sulfur, magnet, quartz, shale, clays, ink and milk. It does not
  automatically include every item below grade ten, equipment or Sky Sword Box.
- `Skills`: primary/junior fishing IDs 15993/15994. The highest learned grade
  determines weighting and receives proficiency; fishing never auto-learns one.
- `SkillBonusPercent`: each skill grade adds this percentage times the catch's
  content grade to its weight. The initial base weight is `floor(10000/grade²)`, multiplied by **10** for
  the eight fish/seafood rewards (Croaker, Tilapia, Sardine, Salmon, Eel, Tuna,
  Octopus and Cuttlefish);
  effective weight is `base × (100 + skillGrade × catchGrade × bonus)`.
- `CatchRequirements`: catches required at each learned grade before the next
  grade. The seed uses the wiki's 14, 35, 79 … 5410 table as per-grade requirements.
  One catch earns one proficiency point. After the last requirement, advancement
  stops at grade 16; no undocumented levels 16–30 thresholds are invented.

Weights, per-grade threshold interpretation, rod grade ceilings and beach-map
assignment are explicit authored defaults where the references are incomplete.
The older Japanese source reports 120-second catches; choose 120 if desired.
Seasonal 2020/2021 rewards and box recipes are excluded because their dated event
windows have expired. Exact event odds, all-region pools and fishing from boats
remain research work. These defaults do not claim exact original probability
parity. See [saved references](NATIVE_COMMANDS.md#saved-fishing-references).

With no learned skill, fish/seafood comprise approximately **55–56%** of
catches across the three default rods. Grade restrictions and the existing
skill bonus still apply. SQL weights remain editable.

## Protocol and persistence

The aLogin decompilation's `FUN_0046a354` sends **AC23:53** to start and
`FUN_0046a8e4` sends **AC23:54** to stop. Both have no payload. AC23:54 still
serves legacy mall refresh when there is no active cast. Native starts select
the strongest available configured rod because the request omits its bag slot;
AC23 item-use requests retain their explicit slot. AC90:1/2/3 remain compatible
start/attempt-catch/stop requests. Successful legacy casts also receive the source
AC90:2 item/count receipt after each committed catch (count zero when discarded).
A catch request cannot bypass the deadline.
The client owns its native casting animation; the incompatible short AC5:12
reference animation is not emitted. Proficiency uses AC5:11 and grade changes
use AC8:1 stat 110 after persistence. Live aLogin acceptance remains to verify.

The SQL-derived Ground.MMG collision grid provides eligibility: walkable land
with water cell 2 in the native 14×14 neighborhood. Missing terrain fails closed.
Active casts remain in memory. Gameplay schema v13 adds `fishing_progress`,
owned by the character, containing total catches and the next eligible deadline.
Rod ownership, replay/cooldown validation, inventory delivery and learned-skill
progress commit in one transaction. Failures roll back everything; reconnect or
recast cannot reset a future deadline. Starting after an expired deadline waits
a fresh interval. SQL results preserve pending walking, and no packets are sent
before a catch commits. Offline time does not generate rewards.

## Upgrading

Stop the server, then follow [the database copy procedure](ASSET_DATABASE.md).
`go run ./cmd/database-migrate -config config.local.json -output-dir <new-directory>`
creates upgraded copies without replacing the source databases. The v7 → v8
upgrade changes fish weights only where both grade and weight still match the
old defaults. Custom weights/grades, removed rewards and other rules are preserved.
Repeating migration never reapplies the multiplier. Point the startup
configuration at those copies before starting the updated binary. The gameplay
upgrade is additive; no reset or loss of accounts is required.

Focused verification covers weight boundaries, shoreline/bounds, native start/
stop, early/replayed catches, full-bag proficiency, pending walking, SQL rollback,
wrong ownership, rod removal, concurrent claims, reopening and preserved asset
edits. No live client test has been performed by these automated checks.
