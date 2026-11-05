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

New capabilities should be added as named bits in `internal/protocol`, selected
through this common hello. Do not add a new per-feature offer/accept exchange.
Independent wire goldens, configuration-gate tests, feature-subset tests,
reconnection tests and repeated/forged-message tests cover this protocol.
