# Client enemy roaming

The server already owns roaming destinations, pursuit, collision checks and
encounters. The client now consumes AC22:2 instead of leaving those packets
unhandled. It animates each NPC toward the announced destination, turns it to
the walking direction and restores its standing pose on arrival. Position,
depth sorting, hovering and clicking use the moving actor's current position.

Native evidence is `var/decompiled/aLogin_ca19ee087b60_full.c` and the matching
`var/ghidra/aLogin.exe`:

- AC22:2 dispatch at `0x002e55fc` reads click U16, destination X/Y U16 and speed
  U8, then calls `FUN_0041869c`.
- `FUN_0041869c` replaces the destination; a zero speed byte retains the old
  speed. The speed helper `FUN_00482980` is mostly absent from the C output.
  Its binary instructions divide the byte by 8 and multiply by 0.16 pixels/ms.
  The server's speed byte 2 therefore moves 40 pixels per second.
- `FUN_004126b8` supplies elapsed-time motion; the existing player/peer walker
  now also accepts an NPC-specific speed. NPC walkers remain session-owned.

Retargeting advances the old leg to the receive time before replacing it, so a
chase interruption starts from the current position. AC22:4 placement/hiding
cancels the previous leg, preventing it from moving a hidden or respawned actor
back toward a stale endpoint. Background workspace sessions advance normally.
The client does not generate random destinations or send NPC movement requests.

Until the EVE initial-speed byte is projected into client map records, a first
AC22:2 with speed zero uses the named server-speed-2 compatibility fallback.
Subsequent zero-speed packets preserve the preceding speed. Ordinary server
roaming packets always supply speed 2 and do not need this fallback.

Focused tests cover raw packet decoding, native speed, diagonal travel,
interrupted legs, standing on arrival, malformed packets, unknown actors,
respawn cancellation, separate sessions and background packet progression.
The graphics parity fixture includes a moving NPC after battle exit.
