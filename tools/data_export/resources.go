// Package dataexport carries the offline extraction scripts in native clients.
package dataexport

import "embed"

// Resources contains only the modules used by regenerate.py, not research tools.
//
//go:embed regenerate.py export.py import_untracked.py media.py media_formats.py ui.py styles.py skill_fields.py skill_effects.py lucky_draw.py extract_audio.py text_languages.py extract_igg_translations.py extract_chinese_text.py
var Resources embed.FS
