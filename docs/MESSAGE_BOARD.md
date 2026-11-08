# Public message board (service 15)

Welling map 12050, NPC click 36 (template 18044 at 1862,1245), invokes
EVE event 57 with action 7 / service 15. The nearby second board uses the
same template and service. This is a community notice window, not Talk.dat
dialogue. The former unknown-service fallback silently resumed the event.

The server opens the native public notice window after AC20:8,
then serves stored page entries and details. Gameplay schema 22 preserves
posts in `message_board_posts`; initialization never seeds fictional notices
or replaces existing posts. Text BLOBs hold native Big5-compatible bytes.
Records sort newest first, ten per page. Schema 23 adds a monotonic ID allocator
without replacing existing posts; deleted IDs cannot identify newer notices.
Messages expire exactly 24 hours after posting. Expired rows are excluded from
lists, counts, direct reads, duplicate-post checks and posting quotas. Their SQL
records remain preserved; expiry does not reuse IDs or require a running timer.
Gonline's client implements the Board list, Notice detail and posting windows,
pagination, refresh, sender mail and author-only deletion. Both new client and
server use the same AC82 protocol; there is no Gonline-only board protocol.

Posting is free: no fee is established by the native submission path. Subjects
allow 30 native bytes and messages 151 (`FUN_001fe600` sets Memo.MaxLen=0x97;
the copy routine's 0x98-byte capacity is not the editable limit). Each character
may keep three notices. Ownership, quota, ID allocation and insertion/deletion
are checked transactionally in the gameplay database. Identical retries return
the existing notice, and creation does not rewrite buffered character state.
There is no assumed expiry or invented paid-posting rule.

The client retains the board behind its detail or composer. Non-authors cannot
delete, and Send Mail opens the existing mail composer addressed to the sender.
Failed submissions retain the draft. Responses after closing, changing maps or
disconnecting cannot reopen an old notice; a five-second timeout releases busy
controls. English/Chinese notices use the compiled localization catalogs.

## Visual references

The screenshots supplied in `var/board` on 2026-11-07 confirm this is the
public AC82 notice board. Keep the following client layout when implementing
the window; the ignored screenshots are local research, not runtime assets.

- The main "Board" window is 654 by 365 pixels, with a dark-green list,
  yellow entry text and Subject / Sender / Time columns. The screenshot has
  one entry; native code reserves ten rows of 20 pixels each. Below the list,
  Previous / page count / Next sit above Add / Update / Close.
- Selecting a row opens a separate "Notice" window, 418 by 252 pixels,
  above the list. Its dark-green text area shows the subject followed by
  the message, in white. The parent board remains open behind it.
- The detail buttons are Delete / Send Mail / Close. Delete is disabled
  for the photographed non-author. Both windows have their own close icon.
- The sender and timestamp belong to the list row, not the detail body.
  Display timestamps as `YYYY/MM/DD HH:MM:SS`, following the reference.

The screenshots verify appearance but do not establish request packing,
posting fees, expiry or mutation acknowledgements. Those still require the
native sender logic or a live capture before implementing paid operations.

## Native evidence

The local full Alogin decompilation identifies the public notice form as
`Notice`, 654 by 365 pixels (`FUN_001ff0b8`). This is distinct from the
five-slot `Form_BlackBoard_1` used by AC72 and furniture message boards.
`FUN_001ff4b8` requests AC82:2 when the notice window opens.
The AC82 dispatcher opens it on AC82:9.

`FUN_001ff95c` reads up to ten entries from AC82:1. Each entry is:

- Author byte length (u32), author bytes (maximum 14).
- Title byte length (u32), title bytes (maximum 30).
- Posted date (f64 Delphi TDateTime), author ID (u32), post ID (u32).

`FUN_001fff90` reads AC82:3/4 pagination responses as total count (u32)
and current page (u8). The real-server capture uses AC82:4 before listing
entries, including refreshes and post completion. `FUN_001ffd14` requests
AC82:3 for the selected row.
`FUN_001ffd84` reads AC82:2 details: author and title length-prefixed as
above, date (f64), content byte length (u32), author ID (u32), content bytes.
AC82:7 displays the native missing-entry notice.

The 2026-11-07 Legacy-client relay capture confirms the opening request is
`82,2,1,1`: page (u8), then Notice form context (u8, public value 1).
Omitting the context previously caused a disconnect. Direct x86 inspection of
the serializer fills the branches elided by the decompiler:

- `0x2db6a1`: AC82:2 carries page (`Notice+0xa58`) and context (`+0xa6c`).
- `0x2db5f5`: AC82:1 appends context after u32 subject length, subject bytes,
  u32 content length and content bytes from `FUN_001fe8a8`.
- `0x2db787`: AC82:3 carries ID (u32, `+0xa48`), page (u32, `+0xa60`),
  selected date (f64, `+0xa50`) and context (u8).
- `0x2db88f`: AC82:4 carries the same metadata, with the final context read
  from the detail form (`+0x10c`, set to 1 by the AC82:2 dispatcher).

The broader meaning of the context byte remains unresolved; it is named
`MessageBoardWireContext1`. Gonline sends the native layouts. The server retains
the earlier Gonline context-free layouts for compatibility. It uses the post ID
for lookup and checks SQL ownership for deletion; client page/date metadata
cannot authorize a mutation.

The real-server capture later that evening confirms the 19-byte read requests
and detail replies, including sender-mail integration. A captured row has author
12021 and message 1; the following read requests message 1, not author 12021.
Reversing these fields previously made Legacy request a nonexistent notice.
The trace also records successful posting as `82,8,1` (close composer), followed
by `82,4,count:u32,page:u8` and the updated AC82:1 rows. Exact public list/read/
detail packets are replayed in regression tests. Some outgoing actions were
absent from the Windows capture: posting/deletion request packing still relies
on the executable serializer evidence above. A full live test against Gonline
remains useful after rebuilding; this reference capture was against Legacy.
Response layouts come directly from the native deserializers and have independent
raw-byte golden tests. The original 2,048-byte SQL content capacity remains for
preserving existing administrator content; newly posted notices use 151 bytes.
If paid posting is introduced later, commit payment and posting in one transaction.
