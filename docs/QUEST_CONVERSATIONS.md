# Immediate reward conversations

The server keeps a conversation open when its dialogue advances a quest
checkpoint and the same NPC's next normal interaction would immediately deliver
a reward. The follow-up runs after the preceding line is acknowledged, without
an intermediate movement unlock or dialogue-close packet. This applies to both
clients through the existing native protocol.

The review reads the SQL catalog's EVE branches and quest-definition titles.
An initial structural scan found 191 potential dialogue/checkpoint-to-reward
branch pairs. These are candidates, not 191 unconditional changes: their extra
conditions can still require items, later quest stages or other work. Reviewed
programs include Lost Calendar, Tempting Goat Cheese, A Guard Drinking with
Friends and Kind Dan. No quest IDs, maps or NPC identities are hard-coded into
the new behavior.

The running conversation retains a transient before-state only for dialogue
branches that change quest checkpoints. At the end, the interpreter compares
the same NPC's normally selected event/branch before and after that change.
This also covers an unconditional reward fallback previously masked by the
first-time dialogue, and follow-ups in another event linked to the same NPC.
Question callbacks retain this before-state through the same conversation;
introductory quest-item grants also do not force a second interaction.
Explicit authored StoryContinue links take precedence.

The destination must contain a supported reward, with only conversation,
presentation, checkpoint and delivery operations. Normal branch order and all
native/SQL prerequisites remain authoritative. Different NPCs, unchanged
checkpoints, already-selected reward branches and intervening battles, travel,
gathering, services or minigames are not skipped. Existing reward capacity,
transaction, disabled-event and automatic-transition checks remain in use.
Failed delivery retains normal retry behavior; duplicate acknowledgments do
not repeat rewards.
Automatic reward continuation visits each event branch at most once per
conversation, preventing reward loops even if a receipt leaves a branch eligible.

This is a reusable interpreter behavior, not a rewrite of EVE programs or quest
rewards. Existing asset/gameplay databases need no migration. Rebuild/restart
the server. Focused tests cover same-event and linked-event follow-ups,
unconditional fallbacks, other NPCs, unmet item conditions, battle boundaries,
unchanged marks, full inventories, failed saves and duplicate acknowledgments,
alongside the installed native story/quest integration tests.
