# Skill.dat field labels

Skill.dat version 11 uses 148-byte records. Export offsets are zero-based disk
offsets; native manager offsets include a four-byte prefix. Name and description
remain separate text objects. Raw header/record bytes and authored effects are
preserved when labels are applied. These are offline export labels; they do not
change the SQL schema or gameplay formulas.

Confidence distinguishes **native** client evidence, **reference** C# names with
unresolved details, **compatibility** interpretations currently used by Gonline,
and **unresolved** fields. The project uses coefficient 32 per grade in P and
coefficient 40 on the attacking stat; see [damage](DAMAGE.md). Wiki `base_power`
remains reference data and does not drive combat. Contact counts come from MBTM,
stored separately in the structured skill catalog by asset schema v55.

| Disk offset | Export key | Confidence | Meaning |
| --- | --- | --- | --- |
| 21 | `type` | native | Native type domain, separate from the UI/effect layer. |
| 22 | `id` | native | Skill identity used in protocol requests and catalogs. |
| 24 | `sp` | native | Base SP cost; not current actor SP. |
| 26 | `element` | reference | 0 neutral; 1 Earth; 2 Water; 3 Fire; 4 Wind; 7 caster affinity. |
| 27 | `attack_category` | native | Presentation/range category; not skill damage power. |
| 29 | `effect_layer` | native | Physical, magical, support or life category; context-specific effect classes. |
| 32 | `power_per_level` | compatibility | Native grade coefficient used by the empirical whole-attack power model; see DAMAGE.md. |
| 40 | `stat_multiplier` | compatibility | Unresolved native coefficient; compatibility name retained. Not an ATK/MATK multiplier in the empirical model. |
| 49 | `additional_damage` | reference | Native additive coefficient retained as provenance. |
| 51 | `effect_rounds` | compatibility | Native duration input used by compatibility effect conversion. |
| 52 | `effect_code` | compatibility | Interpret together with effect_layer; not a globally unique status enum. |
| 54 | `secondary_skill_id` | reference | Raw SkillID2 reference; its precise relationship is unresolved. |
| 97 | `table_order` | native | Native ordered skill index used by snapshots and progress packets. |
| 99 | `target_policy` | native | 0 enemy, 1 ally, 2 either, 3 self, 5 other ally; remaining values unresolved. |
| 100 | `icon_id` | native | Icon_sk<ID>s UI image reference; not the battle effect animation ID. |
| 105 | `maximum_grade` | native | Maximum native proficiency/skill grade, not actor level. |
| 106 | `additional_effect_code` | reference | Reference status qualifier; exact native interpretation remains unresolved. |
| 109 | `target_pattern_grade_1_3` | compatibility | Native shape code, using the documented compatibility grade band. |
| 110 | `target_pattern_grade_4_6` | compatibility | Zero falls back to the preceding nonzero band in compatibility mode. |
| 111 | `target_pattern_grade_7_9` | compatibility | Zero falls back to the preceding nonzero band in compatibility mode. |
| 112 | `target_pattern_grade_10` | compatibility | Final native shape band; native grade selection still needs live verification. |
| 113 | `injury_type` | reference | Reference TypeOfInjury byte; enum and runtime behavior remain unresolved. |
| 118 | `battle_voice_id` | native | Native skill-specific battle voice selector. |

Each `field_layout` entry includes a label, confidence and, for identified fields,
evidence. Unresolved keys retain `unknown_u*_offset_*` names. Six old byte keys
at 51, 52 and 109-112 are retained as explicit compatibility aliases for existing
Skill JSON consumers and SQL provenance. New exports provide readable names
alongside those aliases. Field 54 is a secondary reference, not a proven skill
prerequisite; field 113 retains the reference name without inventing its enum.

`power_per_level` and `stat_multiplier` now appear in `field_layout` with
`encoding: float64-le`; verification checks floating-point values and aliases
against the retained bytes. Original native reconstruction remains byte-exact.

Fresh extraction labels fields automatically. To relabel an existing export
without reimporting translated names, authored effects or other assets:

```powershell
python tools/data_export/label_skill_fields.py --input data/skill_data.json --manifest data/asset_manifest.json
```

For an existing localized runtime export, run the same command with
`--input data/runtime/text/skill_data.json` and omit `--manifest`. Rebuild the
client bundle when you want to package the updated exports. Native Skill.dat and
live databases are not rewritten. Conflicting values or fields differing from
the retained bytes cause validation failure before replacing the export.

Evidence is recorded in `tools/data_export/skill_fields.py` and emitted beside
each identified field. Principal native references are `FUN_00480f8c` (decoding),
`FUN_0048155c` (SP), `FUN_00481584` (effect layer), `FUN_004815dc` (type),
`FUN_0048161c` (table order), `FUN_00481640` / `FUN_0038972c` (target policy),
and the client skill catalog/voice/camera implementations. The C# reference is
`wlo.pserver.core/Game/SkillRelated/SkillManager.cs`. Independent wire and
relabel-preservation fixtures are in `tools/data_export/test_skill_fields.py`.
