# Notepad client

Double-click Notepad (34038) to open Notebook. The item is not consumed, and
AC23:96 refreshes the native quest journal. The current inventory pet selection
does not change this action.

The menu uses the native `Form_NoteBook` and Quest Log, Monster Book and Star Book
buttons. Quest Log and Monster Book have ten selectable entries per page,
previous/next controls, Close and Escape, and draggable windows. Long quest
descriptions scroll with the wheel over the description pane; the wheel over
the list changes pages. Long list names are abbreviated within their row.

Quest state comes from AC24:1/4/5/6/7. Active marks choose the matching `#NN`
description from compiled Mark.dat; completion flags select the paired journal
entry's `#99` description and a red check. Completed entries remain visible after
the active mark is removed. Titles and descriptions use the selected language.

Monster discovery comes from AC53:9. The book shows discovered native book
indices only, sorted by book index, with duplicate indices collapsed. Its level,
HP/SP, attributes and element are the stored NPC template values. The animated
preview uses native action 13 and the template's sprite/colors.

Star Book uses the original animated `Star_Sky`, constellation music, received
constellation ownership and the colored halves of owned `Star_N` images. It is a
full-screen modal with the original top-right exit button and Escape. It restores
music when closed, and exits on battle entry, map change or logout. Ownership is
never inferred from inventory or client interactions. Owned constellation
placement currently uses a documented stable-slot fallback: the executable's
native coordinate tables have not been verified against the installed build.

References: decompiled unit 214 `CH_NoteBook` / `CH_StarSystem` / `CH_StarSysExit`,
unit 205 `form_taskview_1`, unit 170 `TTH_CellectionForm` (`FUN_001db4e0`), and the
provided Legacy screenshots. The original skins are used for all three module
windows; the star scene also uses its original artwork.

No new protocol or database schema is required. Definitions come from the existing
compiled/localized `mark_data.json` and `npc_data.json` client assets. Existing
server quest flags, discovery persistence and constellation synchronization stay
authoritative. Rebuild/restart the client to use the feature.

Focused tests cover raw native journal/discovery messages, completion retention,
reset, malformed snapshots, description selection, Chinese wrapping, inventory
opening and star-book dismissal. `NOTEBOOK_SNAPSHOTS` can select a local output
directory for `TestNotebookScreens`; this renders all four screens with installed
assets for visual inspection.
