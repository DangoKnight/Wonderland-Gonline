"""Skill.dat field labels, with disk offsets and explicit evidence strength.

Native manager offsets include a four-byte prefix; subtract four for disk
offsets. These labels do not change the cipher, bytes or gameplay formulas.
"""

# Authored format/evidence table. Compatibility interpretations must not be
# promoted to verified native semantics merely because the server uses them.
SKILL_FIELDS = {
    21: ('type', 'Skill type', 'Native type domain, separate from the UI/effect layer.',
         'native', 'FUN_004815dc reads manager +0x19.'),
    22: ('id', 'Skill ID', 'Skill identity used in protocol requests and catalogs.',
         'native', 'FUN_00480b74 / FUN_00480f8c; Gonline ParseSkills.'),
    24: ('sp', 'SP cost', 'Base SP cost; not current actor SP.',
         'native', 'FUN_0048155c reads manager +0x1c; skill UI SP display.'),
    26: ('element', 'Element', 'Native element: 0 neutral; 1 Earth; 2 Water; 3 Fire; 4 Wind; 7 caster affinity.',
         'reference', 'Private Server ElementType layout; saved unique-skill reference and authored bindings: 15212/18007/25055/30065 use caster affinity (7).'),
    27: ('attack_category', 'Attack range/category', 'Presentation/range category; not skill damage power.',
         'native', 'FUN_003822d8 checks +0x1f; Gonline battle_camera.go.'),
    29: ('effect_layer', 'Effect/UI layer', 'Physical, magical, support or life category; context-specific effect classes.',
         'native', 'FUN_00481584 reads manager +0x21; Gonline skill tabs and effect conversion.'),
    32: ('power_per_level', 'Power per skill grade',
         'Native grade coefficient used by the empirical whole-attack power model; see docs/DAMAGE.md.',
         'compatibility', 'Private Server SkillInfoRaw.Decimal1; docs/DAMAGE.md.'),
    40: ('stat_multiplier', 'Unresolved native coefficient (legacy stat_multiplier)',
         'Compatibility field name retained; not applied as an ATK/MATK multiplier in the empirical damage model.',
         'compatibility', 'Private Server SkillInfoRaw.Decimal2; docs/DAMAGE.md.'),
    49: ('additional_damage', 'Additional damage coefficient', 'Native additive coefficient retained as provenance.',
         'reference', 'Private Server SkillInfoRaw.AdditinalHarm; Gonline ParseSkills.'),
    51: ('effect_rounds', 'Effect duration in rounds', 'Native duration input used by compatibility effect conversion.',
         'compatibility', 'Private Server NumberOfTurns; internal/assets/native_effect_rules.go.'),
    52: ('effect_code', 'Effect/status code', 'Interpret together with effect_layer; not a globally unique status enum.',
         'compatibility', 'Private Server Effect; internal/assets/native_effect_rules.json.'),
    54: ('secondary_skill_id', 'Secondary skill reference', 'Raw SkillID2 reference; its precise relationship is unresolved.',
         'reference', 'Private Server SkillInfoRaw.SkillID2; no prerequisite semantics assumed.'),
    97: ('table_order', 'Skill table order', 'Native ordered skill index used by snapshots and progress packets.',
         'native', 'FUN_0048161c reads manager +0x65; Gonline skill snapshot decoder.'),
    99: ('target_policy', 'Target policy', '0 enemy, 1 ally, 2 either, 3 self, 5 other ally; remaining values unresolved.',
         'native', 'FUN_00481640 (+0x67), FUN_0038972c; client/wlo/skills/catalog.go.'),
    100: ('icon_id', 'Skill icon ID', 'Icon_sk<ID>s UI image reference; not the battle effect animation ID.',
          'native', 'TRe_SkillForm; client/wlo/skills/catalog.go nativeIconField.'),
    105: ('maximum_grade', 'Maximum skill grade', 'Maximum native proficiency/skill grade, not actor level.',
          'native', 'TRe_SkillForm; client/wlo/skills/catalog.go nativeMaximumGradeField.'),
    106: ('additional_effect_code', 'Additional effect code', 'Reference status qualifier; exact native interpretation remains unresolved.',
          'reference', 'Private Server AdditionalEffect support classifiers.'),
    109: ('target_pattern_grade_1_3', 'Target pattern for grades 1-3', 'Native shape code, using the documented compatibility grade band.',
          'compatibility', 'Private Server GetPatternForGrade; Gonline Skill.NativePatternAt.'),
    110: ('target_pattern_grade_4_6', 'Target pattern for grades 4-6', 'Zero falls back to the preceding nonzero band in compatibility mode.',
          'compatibility', 'Private Server GetPatternForGrade; Gonline Skill.NativePatternAt.'),
    111: ('target_pattern_grade_7_9', 'Target pattern for grades 7-9', 'Zero falls back to the preceding nonzero band in compatibility mode.',
          'compatibility', 'Private Server GetPatternForGrade; Gonline Skill.NativePatternAt.'),
    112: ('target_pattern_grade_10', 'Target pattern for grade 10', 'Final native shape band; native grade selection still needs live verification.',
          'compatibility', 'Private Server GetPatternForGrade; docs/COMBAT_TARGETING.md.'),
    113: ('injury_type', 'Injury type (reference name)', 'Reference TypeOfInjury byte; enum and runtime behavior remain unresolved.',
          'reference', 'Private Server SkillInfoRaw.TypeOfInjury / LoadSkillDatabase.'),
    118: ('battle_voice_id', 'Battle voice WAV ID', 'Native skill-specific battle voice selector.',
          'native', 'Skill.dat manager +0x7a; client/wlo/skills/catalog.go nativeBattleVoiceField.'),
}

# Existing Skill JSON consumers and SQL provenance use these six keys. Keep
# explicit aliases when exporting new readable names; never rewrite SQL state.
SKILL_LEGACY_KEYS = {
    51: 'unknown_u8_offset_51', 52: 'unknown_u8_offset_52',
    109: 'unknown_u8_offset_109', 110: 'unknown_u8_offset_110',
    111: 'unknown_u8_offset_111', 112: 'unknown_u8_offset_112',
}


def annotate_skill_layout(layout):
    layout[:] = [field for field in layout if field['offset'] not in (32, 40)]
    layout.extend({'name': SKILL_FIELDS[offset][0], 'offset': offset,
                   'bytes': 8, 'encoding': 'float64-le'} for offset in (32, 40))
    for field in layout:
        info = SKILL_FIELDS.get(field['offset'])
        if info:
            field['name'], field['label'], field['description'], field['confidence'], field['evidence'] = info
            if field['offset'] in SKILL_LEGACY_KEYS:
                field['legacy_name'] = SKILL_LEGACY_KEYS[field['offset']]
        else:
            field['label'] = f"Unresolved {field['bytes'] * 8}-bit field at disk offset {field['offset']}"
            field['confidence'] = 'unresolved'
    layout.sort(key=lambda field: field['offset'])


def label_existing_export(document):
    """Relabel an existing export without reimporting names, effects or bytes."""
    import copy
    result = copy.deepcopy(document)
    if result.get('format') != 'native-records' or result.get('record_bytes') != 148 or result.get('native_version') != 11:
        raise ValueError('expected a version-11 Skill.dat export')
    for field in result['field_layout']:
        info = SKILL_FIELDS.get(field['offset'])
        if not info:
            continue
        old, new = field['name'], info[0]
        for row in result['records']:
            fields = row['fields']
            value = fields[old]
            if new in fields and fields[new] != value:
                raise ValueError(f'conflicting skill field aliases: {old}, {new}')
            fields[new] = value
            if old != new and old != SKILL_LEGACY_KEYS.get(field['offset']):
                del fields[old]
            if field['offset'] in SKILL_LEGACY_KEYS:
                legacy = SKILL_LEGACY_KEYS[field['offset']]
                if legacy in fields and fields[legacy] != value:
                    raise ValueError(f'conflicting skill field aliases: {legacy}, {new}')
                fields[legacy] = value
    annotate_skill_layout(result['field_layout'])
    return result
