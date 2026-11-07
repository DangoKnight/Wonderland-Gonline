# Item redemption cards

Double-click a native Item.dat type-40 card in the inventory. One-reward cards
redeem immediately. Gender pairs use the player's body gender automatically;
other alternatives open the Gift Exchange Card selector, with four rows per page.
Choose a reward and confirm, or close/Escape without consuming anything.
The selected inventory pet does not change the recipient: item rewards go into
the player's inventory. Existing companion vouchers, VIP and EXP cards keep
their separate effects.

The server removes one card and grants the selected quantity in the same SQL
transaction. Locked, changed, unavailable or undeliverable cards are retained.
Clients never consume cards or grant rewards optimistically.

Assets schema 62 introduces `catalog_item_cards` and
`catalog_item_cards_rewards`. The 236 initial rules come from tracked
`internal/assetsql/item_cards_defaults.json`, also included in the repository
SQL seed. The migration initializes missing tables only; administrator changes
and deletions survive repeated migrations. Update an existing installation with
`go run ./cmd/database-migrate -config config.local.json`, then rebuild/restart
the server. No client extraction is needed to run the server.

Regenerate definitions with `go run ./cmd/item-card-import -assets var/assets.db`.
The English/Chinese catalog, authored overrides, output and source-hash paths
are CLI options. `item_cards_sources.json` records the portable input hashes;
`item_cards_overrides.json` preserves the explicit gender and confirmed reward
rules separately from inferred name matches. Regeneration is an offline authoring
step, never a runtime dependency or automatic replacement of administrator data.

Reward matching uses complete named phrases from the modern Chinese descriptions,
with English descriptions filling unmatched entries. Explicit gender pairs cover
winter clothing/boots/boxes, Purity clothing/hats and Union clothing/hats.
Dark Snow Card correctly offers Dark Snow Girl's voucher/rebirth pill, rather
than the similarly named Snow Girl's items. Pet Upgrade Card offers the known
pet stat foods. Each default row records its source evidence.

Unspecified “other premium items” are not invented. Ambiguous names or generic
descriptions without identifiable item IDs remain unavailable and retain the
card; examples include Nostalgia Cat Card 50053, Nostalgia Rabbit Card 50054
and Class Weapon Card 51702. These need a Gec.Dat catalog or live-server trace
to resolve reliably.

The client form follows decompiled `TGiftExchangeCardForm` (unit 169): native
`Form_GiftExchange_1`, 447×293, checkbox rows, icon/name, paging and exchange/close
controls. Native AC23:133 provides catalog index (1), details (2), redemption
notice (3) and slot reopening (4); AC90:1 requests catalog details. AC90's normal
two-byte fishing messages remain separate. The vehicle-stall compatibility code
also uses 133, so card packets require exact layout validation.

Catalog layouts and form controls are recovered from the decompile. The outbound
exchange request's slot/count/index/item ordering is reconstructed from its
serializer calls, not a live capture. Legacy interoperability still requires a
live exchange test; the implementation does not claim that test has occurred.
