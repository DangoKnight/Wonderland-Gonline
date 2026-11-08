# Connection-wide Gonline mode

Gonline determines its application protocol immediately after TCP connects,
during native discovery. TCP framing and the discovery request remain unchanged.
The server replies with the ordinary description and channel list, then one
versioned application hello. No gameplay feature starts its own negotiation on
a current Gonline Server + Client connection.

## Wire format

All fields use normal Wonderland framing and little-endian integers:

| Field | Bytes |
| --- | --- |
| Command | 1 (AC1) |
| Subcommand | 248 offer, 249 acceptance, 250 confirmation |
| Signature | Four ASCII bytes `GLON` |
| Version | UInt16, currently 1 |
| Feature mask | UInt32 |

The payload is exactly 12 bytes. Bits are:

| Bit | Feature |
| --- | --- |
| 0 | Collection retries at server respawn expiry plus 10 seconds |
| 1 | Full combat round chains and batched choices |
| 2 | Client-side native backdrop resolution |
| 3 | AC5:4 completes the entire initial map snapshot |
| 4 | Shared ground pickups include the collector for observer animations |

The server includes bit 1 only when its captured startup `combat_round_chain`
setting is enabled. The client intersects the offer with its supported features;
bit 2 requires loaded local backdrop metadata. Unknown offered bits are ignored.
The server rejects unoffered bits and unsupported versions. Features activate only
after the server's confirmation exactly matches the client's acceptance.

The server captures its offer once per connection. Repeated discovery sends no
new hello. Identical repeated acceptance is harmless; a different acceptance or
late feature negotiation cannot change the established mode. Negotiation state,
playback and pending backdrop context clear on disconnect/reconnect. No account
or gameplay database fields store this transient transport state.

## Compatibility and interaction traffic

Legacy clients ignore the unknown AC1 subcommands and remain on the native path.
The client does not probe unidentified servers or infer their software from the
server name. Legacy servers continue controlling their own native behavior.
Clients also understand the older separate GLBR/GLCT/GLBD feature advertisements
for older Gonline servers. Current servers advertise only GLON; once GLON starts,
legacy feature messages cannot activate extra features.

Interaction messages still carry their necessary data. For example, AC1:247
GLBD map/X/Y specifies the shared battle-entry position used for local backdrop
selection. It requires no response and does not renegotiate the connection.
Round prompts, choices and result chains likewise use the mode already selected.

With bit 4 selected, shared pickups send observers an extended AC23:2:
`command:u8, subcommand:u8, ground_slot:u16, collector_id:u32, x:u16, y:u16`
(12 bytes). X/Y are the collector's world position at pickup. Observers remove
the ground node and fly its icon along a parabola toward that position, without
granting an item or displaying an acquisition notice. The collector retains the
native pickup/grant sequence. Permanent account-owned tutorial nodes never send
this animation to another account. Hidden collectors receive no visual disclosure.

Unnegotiated observers receive the four-byte native AC23:2 removal. Decompilation
`FUN_003dc72c` checks packet length, not the extra byte's value: adding even a zero
byte requests `FUN_003d74d4` to animate toward the receiving client's own player
through `FUN_003e0548`. No remote collector field is available in that native
message. Full observer flight therefore currently requires Gonline Client.

With bit 3 selected, the server emits the existing AC5:4 map-ready reply after
the complete destination snapshot, including peer positions, vehicle state,
props, bazaars and tent furniture, before starting an arrival script. Without
bit 3, its native packet ordering remains unchanged. No new per-map handshake
or polling request is added. The client's loading screen also waits for local
render preparation and pending sprite decoding; it does not block networking.

New capabilities should be added as named bits in `internal/protocol`, selected
through this common hello. Do not add a new per-feature offer/accept exchange.
Independent wire goldens, configuration-gate tests, feature-subset tests,
reconnection tests and repeated/forged-message tests cover this protocol.
