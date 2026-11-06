# Compiled client string tables

`en.json` and `zh-TW.json` are tracked, UTF-8 client assets. They contain the
compiled UI labels, names, descriptions, dialogue and quest text indexed by
stable keys. Chinese entries without a match fall back to the English catalog
at runtime. Server-sent literal messages remain unchanged.

Regenerate both files from the repository root after extracting the client data
and updating the translation sources:

```powershell
go run ./cmd/asset-build -data data -text-only
```

Commit the regenerated catalogs alongside their translation-source changes.
The compiler sorts keys and emits one entry per line for readable diffs.
Edit the source UI catalogs and imported override tables, rather than these
generated files; regeneration replaces their contents.

The native exports, keyed record projections, images, audio and other generated
assets remain ignored. See [translation compilation and language sources](../../../docs/IGG_TRANSLATIONS.md).
