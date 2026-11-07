# Client runtime performance investigation

Measured on Windows on 2026-11-07 using the current compiled asset bundle and
GPU renderer. The reported occasional freeze is an unconfirmed observation;
other running processes may explain it. This investigation establishes concrete
CPU and cache costs, not a reproduced half-second freeze.

## Implemented rendering optimizations

The GPU renderer now updates mutable artwork in place using a stable source
identity and revision. Gateway surfaces, waypoint markers, exit indicators and
area titles no longer leave obsolete animation textures in the immutable asset
LRU. A reusable upload buffer also avoids repeated conversion-buffer allocations.
Unchanged revisions do not reload or upload; changed alpha and image dimensions
refresh blending metadata and cache accounting. Drawing remains ordered, and
the CPU renderer retains its existing pixel operations.

Gateway generation now shares precomputed ring geometry/falloff and calculates
row-invariant plume parameters once per row. It preserves the continuous effect,
timing, colors and geometry. Thirty-two independently captured RGB565 reference
frames across the complete animation loop match exactly. A three-run matched
microbenchmark improved from a median **2.526 ms** to **1.286 ms** per generated
gateway frame, with zero per-frame allocations in both versions.

The same offline samples were rerun with the optimized client:

| Measurement | Before | After |
| --- | ---: | ---: |
| Stationary gateway, GPU asset cache after 15 s | 128.0 MiB | 9.6 MiB |
| Stationary gateway, mean simulation/update cost | 7.47 ms | 1.74 ms |
| Stationary gateway, update p99 | 14.78 ms | 3.19 ms |
| Stationary gateway, cumulative allocations in 15 s | 336.7 MiB | 143.9 MiB |
| One session walking/map change, GPU asset cache | 85.7 MiB | 15.0 MiB |
| One session walking/map change, mean update cost | 1.95 ms | 0.99 ms |
| Two sessions walking/map change, GPU asset cache | 93.1 MiB | 17.5 MiB |
| Two sessions walking/map change, mean update cost | 1.90 ms | 1.01 ms |

All optimized samples recorded zero GPU readbacks. These are short local
observations using the methodology below, not whole-game FPS guarantees or
total process/driver memory measurements. They do not measure the audio path.

Validation passed: render/surface packages with `-race`, focused gateway/marker/
exit tests, full client CPU/GPU pixel parity, area-title lifecycle/native rules,
compiled shared-workspace and marker integrations, affected-package vet and a
client build. New GPU regressions queue many revisions before readback and cover
alpha changes, unchanged revisions, failed-load retry, resizing, cache limits
and eviction. No database migration or asset re-extraction is needed. Restart
the client to load the code changes. Cold audio decoding and scene preparation
remain opportunities for separate optimizations.

Updated profiles and results are under `after-gpu-*` in the evidence directory;
`optimization-summary.json` records the before/after measurements. Gateway
benchmark logs and the captured pre-optimization signatures remain alongside
their matching profiling binaries.

## Pre-optimization measurements

Offline profiling used isolated temporary profiles, no live accounts or server
connections, a 1020x600 canvas, 60 simulation updates/second and presentation
paced toward 120 FPS. One- and two-session samples walked in Chicken Farm and
changed to North Island after seven seconds. A third sample remained stationary
beside Chicken Farm's procedural gateway. Each ran for about 15 seconds, with
CPU profiling enabled and the default 20 logical Go processors on this machine.
Audio playback and final-window enlargement filters were not enabled in those
frame samples. They therefore do not measure the complete live-client workload.

| Sample | Update p99 | Largest update | Go heap at end | GPU asset cache at end | GPU readbacks |
| --- | ---: | ---: | ---: | ---: | ---: |
| One session, walking/map change | 6.29 ms | 74.69 ms | 320.7 MiB | 85.7 MiB | 0 |
| Two sessions, walking/map change | 6.35 ms | 70.57 ms | 276.1 MiB | 93.1 MiB | 0 |
| One session, stationary gateway | 14.78 ms | 59.90 ms | 220.4 MiB | 128.0 MiB | 0 |

The largest observed draw-to-draw interval was 123 ms, including scheduling and
work outside the measured drawing method. No half-second freeze occurred.
One/two-session walking samples allocated about 310/340 MiB during the timed
period; the stationary gateway sample allocated 337 MiB. These are cumulative
allocations, not retained or peak memory. Map changes and first-use graphics
uploads are included. Heap readings are snapshots at different GC phases, so
the smaller two-session heap does not mean an additional session saves memory.
GPU cache figures exclude render targets, driver allocations and scaling buffers.

## Pre-optimization findings

### Procedural frames occupy the immutable GPU texture cache

[The GPU device](../client/wlo/render/device.go) keys CPU-surface textures by
source, source rectangle and revision. Every generated gateway frame increments
its surface revision, creating a fresh texture entry. Older revisions stay in
the shared 128 MiB LRU until normal eviction. The
[exit indicator](../client/wlo/world/exit_indicator.go) and
[waypoint marker](../client/wlo/world/marker.go) likewise include a changing
revision in their sprite cache keys.

In the stationary sample, the GPU asset cache grew from **9.5 MiB** at the first
sample to **51.4 MiB**, **93.2 MiB**, and finally **128 MiB**, without changing the
scene. It uploaded 132.1 MiB in total. This is bounded cache pollution, not an
unbounded GPU leak, but it can evict useful static assets and force later uploads.

Give dynamic sources a stable identity and update their existing texture when
the revision changes. Keep immutable sprite/page caching separate. Preserve
render-command ordering, transparency metadata, cache accounting and GPU/CPU
pixel parity. Do not merely remove revisions: that would display stale frames.

### Gateway animation is expensive CPU work

[Gateway generation](../client/wlo/world/gateway_light.go) calculates an energy
image on the CPU every rendered world frame. Many exponential/power operations
repeat inside nested pixel loops. The static gateway sample spent **5.62 of
7.77 sampled CPU seconds (72%)** in gateway generation; the walking sample spent
about 34% of sampled CPU time there. These percentages are shares of sampled CPU,
not whole-machine CPU utilization or GPU time.

Optimize invariant ring data, move row-invariant plume calculations outside the
inner pixel loop, and replace integer powers with multiplication where output
parity permits. A GPU implementation could remove the CPU pixel upload entirely,
but needs a compatible CPU path and visual validation. Preserve the continuous
animation and authored footprint rather than reducing its animation rate.

### First-use sound decoding can block simulation

[Sound effects](../client/wlo/app/sound.go) synchronously decode and resample an
uncached effect before playback. Ambient entry can call the same path. Separate
offline cold-decode measurements from the compiled bundle found:

| Effect | Cold decode | Decoded PCM size |
| --- | ---: | ---: |
| `wav0152` | 6.5 ms | 0.089 MiB |
| `wav0149` | 98.5 ms | 1.06 MiB |
| `wav0050` | 200.1 ms | 2.14 MiB |
| `wav0053` | 180.8 ms | 1.95 MiB |

These timings demonstrate a hitch-capable path, but do not establish that any
of these effects caused the observed freeze. Voice decoding already uses a
bounded background worker; music opens a streaming decoder and should not be
mistaken for the full-PCM effect path. Preload predictable effects during loading
and use bounded background decoding for cold effects, retaining timely cached
playback and session cancellation. The decoded sound/voice cache currently has
no byte cap; add a byte-bounded policy without invalidating samples still used
by playing audio streams.

## Other costs and useful existing behavior

- Prebuilt assets still need compressed-page decoding, palette conversion,
  resident CPU buffers and GPU textures. Sprite CPU pages allow up to 256 MiB,
  compiled image pages 64 MiB and GPU asset textures 128 MiB; these limits are
  separate and exclude retained sprite indices, scene artwork, audio and targets.
- [Scene object loading](../client/wlo/world/objects.go) materializes compiled
  pictures into CPU images, converts them to RGB565 and reconstructs stippled
  shadows. Four recent scenes retain shared artwork. Investigate keeping more
  of this data tile-backed or preprocessing it during compilation before simply
  raising or shrinking caches.
- Sprite archive/record loading still runs synchronously on first use under the
  manager lock. A small compiled-data sample found a 46.6 ms archive open and
  individual indexed sprite reads of roughly 0–2 ms. It does not demonstrate a
  general 500 ms sprite-decoding problem. Editable exports have a more expensive
  native-matching path, but normal `go run .` selects `bin/client-assets.zip`
  when available; Go compilation itself is not recurring gameplay work.
- Sessions already share resources. Background sessions render at four frames
  per second while networking/simulation continue at 60 Hz. Their rendering is
  intentionally not another full-rate foreground scene.
- Normal sampled GPU rendering performed zero readbacks. There is no evidence
  here for a general per-frame GPU-to-CPU copy bottleneck.
- Presentation at 120 FPS still recomposes the workspace/sidebar at that rate,
  while simulation/world rendering runs at 60 Hz. Reuse unchanged composition
  where practical; preserve camera animation, pointer response and session layout.
- Higher-resolution bicubic/edge-aware filtering adds final GPU work. This
  sample did not measure it and does not attribute the reported pause to it.

## Scope and evidence

The initial investigation did not change runtime code. The subsequent rendering
changes and their validation are recorded above; game assets, gameplay rules
and real user profiles were not modified. Temporary offline probes were removed
after execution; their source, matching test binaries,
CPU/heap profiles, JSON measurements and logs remain in ignored local
`var/client-runtime-20261107/`. The source is
`runtime_probe_test.go.txt`; it uses existing app test helpers. Heap profiles
include initialization allocations, whereas the JSON allocation deltas cover
only the timed loop. Do not compare their totals as the same measurement.

The initial three GPU frame samples and separate cold-effect/sprite probes passed;
the three optimized GPU samples and focused regression checks also passed. No
full suite was rerun for this client-only change. Remaining candidates include
bounded asynchronous effect preparation and scene decoding. A persistent stall recorder can be added
later if the occasional freeze becomes reproducible or operationally significant.
