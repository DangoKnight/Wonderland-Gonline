# Stat-boosting pet food

Orchid Candy, Fairy Honey and the other 22 native type-20 foods permanently add
one point to a random STR, CON, INT, WIS or AGI attribute on a compatible owned
party pet. Attribute selection shares the level-up allocator and the startup
`pet_growth_formula` setting: `base_stats` weights the template's base attributes
(the pet's own attributes when no template exists), while `combat_stats` weights
STR/CON/INT/WIS/AGI by current ATK/DEF/MAT/MDF/SPD, including equipment. Weights
are recalculated for every food in a batch, with a minimum weight of one.
At UInt16 maximum an attribute is excluded rather than overflowing. No item is
used when all attributes are capped.

Each pet has a lifetime limit of **1,200 successful uses**, shared across all
compatible food types. `game.Pet.StatFoodUses` persists in the gameplay database
as `character_pets.stat_food_uses` for party, reserve and hotel pets. Existing
pets begin with zero recorded uses because these foods were previously
unimplemented. The counter stays with the pet through ordinary ownership,
storage and progression operations; resting, dismissal to reserve, relogging
or rebirth does not reset it.

The server rechecks pet identity, compatibility, item ownership, lock state,
available quantity and remaining allowance inside the same SQL transaction that
consumes food, adds attributes and advances the counter. A batch uses only the
remaining allowed quantity. Wrong targets, full allowance, random-source errors
and failed SQL transactions cannot consume food or grant points. Current HP/SP
gain any positive increase to their maxima, preserving the previous deficit.
Permanent food use is unavailable during combat or other locked gameplay.

## Definitions and compatibility

Assets schema v19 adds `catalog_pet_foods` and its ordered target rows. Offline
initialization projects each native type-20 item's target word from decoded
Item.dat offset 413 (excluding the four-byte in-memory prefix). All 24 target
words match their supplied descriptions. Ice Snowflake additionally supports
Dark Snow Girl (31235), as its authored description explicitly states.
Runtime and administration use the structured SQL definitions, not description
parsing or raw record decoding. Repeat migration preserves edited definitions.
Companion aliases use the existing `game.SamePet` rules.

Inventory selection uses ordinary AC23:15 with the selected pet's client slot;
the Gonline client rejects a player target. Native AC67:1 and AC23:96 can feed
the active battle pet. Successful replies remove the consumed quantity with
AC23:9, update pet stats through ordinary AC8:2 progression packets and play the
normal AC23:15 use effect. A head banner reports the saved use count. No custom
wire command is introduced. Amity food retains its separate existing behavior.

## Compounding

Exactly two pet foods, one unit from each of two distinct bag slots, produce
one randomly selected pet food. The inputs may have the same item ID. Every
available food in the SQL food catalog has equal probability, including the
input types. Split a stack first when using two units of the same stack.

This recipe has no failure/junk roll, rank/base rules, alchemy skill requirement
or alchemy EXP gain. Foods cannot be mixed with ordinary ingredients or books,
and cannot appear as ordinary alchemy results. The Gonline client rejects mixed
recipes; the server independently enforces the rule for all clients. Native
AC23:14/87/101 and compatibility AC40 share the same transaction. Both inputs
and the output commit together before native success replies. Invalid requests,
unavailable output space, entropy errors and failed saves leave both foods intact.
Compounding does not change any pet's lifetime food-use counter.

## Migration and verification

Gameplay schema v20 adds the counter with default zero and an SQL check from
zero through 1200. Assets schema v19 adds food compatibility definitions.
The backed-up local upgrade completed while the server was stopped; originals
are in `var/backups/pre-migrate-20261104-011038`.

Focused tests cover all attribute selections, overflow exclusion, final-use
batch clamping, wrong targets, RNG/SQL rollback, native requests, current-vital
gains, counter persistence across party/reserve/hotel storage, SQL cap rejection
and preserving asset migration. See [TESTING.md](TESTING.md).
