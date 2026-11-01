# Player trade (AC25)

Wire format notes for the native exchange window. Every claim here was verified
against the retail client (`aLogin.exe`) with packet captures or the client's own
disassembly; frame samples are quoted next to the statement that relies on them.

Frames use the usual `F4 44 | length | body` envelope XORed with `0xAD`. The body
starts with the action code; the window handler receives a pointer to the code
byte, so offsets below are counted from the code unless stated otherwise.

## Packets

| Packet | Direction | Layout |
| --- | --- | --- |
| `AC25:1` invitation | client → server, then server → both | `target u32`, then a zero byte |
| `AC25:2` offer | client → server | `gold u32`, then one byte per offered inventory slot |
| `AC25:3` offer window | server → client | `gold u32`, then one 30-byte record per item, then 5 zero bytes |
| `AC25:3` confirm / cancel | client → server | single flag: `1` confirm, `2` cancel |
| `AC25:12` window | server → client | fixed 25-byte header, optional rows |

### Invitation (`AC25:1`)

The initiator must receive its own copy with the **partner** id; the client only
opens the window from an incoming `AC25:1`. The partner receives the initiator id.

```
C>S  19 01 12 27 00 00      target 0x2712
S>C  19 01 12 27 00 00      echo to the initiator (partner id)
S>C  19 01 11 27 00 00      invitation to the partner (initiator id)
```

### Offer (`AC25:2`)

The client sends this packet **only when the player confirms**; arranging items
in the window stays client-side. Confirming an empty offer therefore produces a
packet with gold and no slots.

```
19 02 7b 00 00 00 24 2a     gold 123, slots 0x24 and 0x2A
19 02 0a 00 00 00           gold 10, no items
```

Slots are one-based bag indexes. Unknown, empty or locked slots are dropped; the
packet itself must never close the session.

### Offer window (`AC25:3`)

The client computes the item count as `(len - 5) / 29` and then reads 30-byte
records, so the tail keeps five zero bytes:

```
19 03 | gold u32 | record… | 00 00 00 00 00
```

Each record:

| Offset | Field |
| --- | --- |
| `+0` | item id `u16` |
| `+2` | quantity (low byte) |
| `+11` | quantity (the byte the handler actually accumulates) |
| other | damage, price, flags — zeroed by the server |

The handler assembles the quantity from bytes 2, 3, 10 and 11, so the server
writes it to both 2 and 11.

**The client appends rows instead of replacing them.** Sending the same offer
twice duplicates the stack on screen; the server therefore pushes an offer only
when it differs from the last one sent to that side.

### Window (`AC25:12`)

Header (25 bytes, offsets from the code byte):

| Offset | Field |
| --- | --- |
| `[2..5]` | partner id `u32` |
| `[6..7]` | partner id `u16` |
| `[8]` | unused |
| `[9..12]` | viewer gold `u32` |
| `[13..16]` | partner gold `u32` |
| `[17..18]` | confirmation flag, viewer |
| `[19..20]` | confirmation flag, partner |
| `[21]` | unused |
| `[22]` | "already changed" flag |
| `[23]` | rows present (`1`) |
| `[24]` | padding |
| `[25…]` | rows |

Rows are 21-byte field groups; the client copies a single trailing `0x51`-byte
structure after the loop, so item bodies must not follow every row.

The row key is looked up in the item table (`DAT_0077fbfc`), and the byte returned
at `record + 0x32` becomes the grid cell index. For materials that byte is zero,
so `AC25:12` rows do not populate the trade grid; the window is opened by this
packet but its contents come from `AC25:3`.

## Server behaviour

1. `AC25:1` opens a session for both sides and sends the window plus an empty
   `AC25:3` to each.
2. `AC25:2` replaces the sender's offer and pushes `AC25:3` with the partner's
   offer to the other side.
3. `AC25:3 1` marks the sender confirmed; any new offer clears both marks.
   When both sides confirmed, the exchange runs in one transaction.
4. `AC25:3 2` cancels.

## Open questions

- The two confirmation flags and the "already changed" byte were probed with
  every combination: the retail client shows no label, no highlight and does not
  clear its own confirmation. They are treated as cosmetic and left at zero.
- The five trailing zero bytes of `AC25:3` are required for the length formula
  but their meaning is unknown.
