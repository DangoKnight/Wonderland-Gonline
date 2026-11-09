# Tent client support

The client consumes the existing native tent protocol. Double-clicking a tent
in the inventory opens it; right-clicking its inventory item requests retrieval.
Clicking an exterior tent approaches and enters it. Inside, the player can walk,
leave through the doorway, drag inventory furniture into the room, and move
placed furniture by dragging it. Stair furniture activates the existing server
floor-switch handler. Furniture changes remain server-authoritative.

Exterior signs are depth-sorted with characters. Map-entry sign lists replace
the previous list, including empty lists. Private room entry always creates a
fresh scene, even when two owners use the same interior map ID. Furniture and
collision grids belong to the individual session; decoded picture resources can
be shared. Moving furniture updates collision without rebuilding unchanged room
art. Closing an occupied tent uses the server's explicit return warp; the
eviction notice alone never invents a destination.

## Evidence

The retained independent packet goldens are
`client/wlo/housing/testdata/legacy_tents.json`. They come from the Rhodes3
capture `tents-20261109-02`, which covered opening, map re-entry, owner/guest
entry, furniture movement and closure with a guest inside. The local full trace
is intentionally untracked. User screenshots under `var/screenshots/tents`
provide separate visual references, rather than exact frame matches.

| Direction | Packet | Meaning |
| --- | --- | --- |
| Client to server | AC23:96, inventory slot | Use the tent item |
| Client to server | AC65:1, owner ID | Enter a tent |
| Client to server | AC65:2 | Retrieve own tent |
| Client to server | AC65:3 | Leave the interior |
| Server to client | AC65:1, 16-byte sign | Add/update an exterior |
| Server to client | AC65:3, repeated 16-byte signs | Replace exterior list |
| Server to client | AC65:4, owner ID | Remove exterior |
| Server to client | AC65:5 | Owner removed the occupied tent; AC12 supplies return |
| Server to client | AC12 | Warp, with optional current floor and room-kind/floor pairs |
| Server to client | AC62:4 | Owner ID followed by 59-byte furniture records |
| Both | AC62:3 | Furniture index, grid X/Y/Z, rotation |
| Server to client | AC62:14/15 | Current floor/wallpaper item |
| Client to server | AC62:14, furniture index | Activate stairs |

Native housing uses a 60-by-30 isometric grid. The geometry initializer and
player constructor establish the origin used by `housing.Pixel`; room kinds
1/2/3 select the authored floor rectangles. AC12's trailing pairs are room kind
and floor, not feature flags. Default room images are direct picture resources
10005, 10004 and 10000 respectively. They must not be resolved as ordinary map
IDs: those IDs can refer to unrelated art through Ground.MMG.

Furniture pictures come from Item.dat record offsets 409/411, with footprints
at 402/401. Native TSe_itemObject painting centers the complete image on the
projected cell and adds 15 pixels vertically; transparent padding participates
in the anchor. Default floor 39062 and wallpaper 39064 use the baked room art.
Their adjacent item IDs are unrelated objects and are not fallback textures.
Tent exteriors use the 009 sprite family, including its patch archive.

## Remaining coverage

Core entry, exterior snapshots, movement and guest eviction have captured-packet
regression tests. Live Gonline/Legacy mixed-client acceptance remains necessary.
The doorway anchor follows the native image and screenshot; its exact native
trigger rectangle still needs tracing. Custom surface tiling and unusual
stacked/attached furniture need additional visual checks. Furniture context
menus, rotation/retrieval controls, manufacturing forms, pillar editing, dolls,
locks and music-selection interfaces are separate unfinished client work.
