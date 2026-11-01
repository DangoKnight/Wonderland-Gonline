# Player trade (AC25)

This describes Gonline's direction-specific protocol for the WLRI client. The
server and Go client retain independent raw-byte fixtures. Native traces identify
`FUN_003c6b18` for opening, `FUN_003680f4` for locking an offer,
`FUN_003c704c` for receiving one, and `FUN_00368000` for final acceptance.

Packet bodies below exclude the framing envelope. All integers are little-endian.

| Packet | Direction | Body after code/subcode |
| --- | --- | --- |
| AC25:1 | Client to server | Partner character ID, U32 |
| AC25:1 | Server to each player | The other player's character ID, U32 |
| AC25:2 | Client to server | Gold, U32, followed by up to 20 distinct bag-slot bytes |
| AC25:3 | Server to partner | Gold, U32, followed by 30-byte item records |
| AC25:3 | Client to server | One byte: 1 accepts, 2 cancels |
| AC25:2 | Server to client | One result byte, including 4 for completed |
| AC25:44 | Server to client | Online-time eligibility; 2 enables trading |

AC25:1 opens both forms immediately when both players are eligible and within
180 pixels in the same scene. The recipient's trade preference controls consent;
there is no second invitation exchange. An offer consists of whole inventory
stacks, and confirming an empty offer still sends the four-byte gold value.

Each partner-offer item record contains:

| Record offset | Field |
| --- | --- |
| 0–1 | Item ID, U16 |
| 2 | Quantity, U8 |
| 3 | Damage, U8 |
| 4–9 | Instance metadata bytes 2–7 |
| 10–11 | Instance metadata bytes 0–1 |
| 12–29 | Instance metadata bytes 8–25 |

There is no duplicated quantity field or appended five-byte padding. Damage and
metadata are preserved. The body length is `6 + 30 * itemCount`.

An offer locks once and cannot be replaced during that exchange. Both offers
must be locked before either final acceptance counts. When both accept, one SQL
transaction reloads and validates both owners, balances and exact item records,
plans the exchange, and saves both characters. Success packets follow commit.
Outgoing items are removed in the plan before incoming items are placed, allowing
valid swaps between full bags. Failure transfers nothing; replay cannot regrant.

See [economy behavior](ECONOMY_SOCIAL.md#player-item-trades),
[client trade controls](CLIENT.md#trade-ported-in-part-clientwloinventorytradego-clientwlohudbusinessgo-clientwloapptradego)
and [testing](TESTING.md) for checks and remaining native acceptance work.
