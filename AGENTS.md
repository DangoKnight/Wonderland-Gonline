# Project instructions

Read [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) before changing this project.
Follow its numeric naming convention: give protocol codes, event codes, flags,
limits and content references descriptive names in their owning packages. Use
explicit compatibility values and documented fallback names for unresolved codes.
Preserve independent raw-byte golden tests and authored data tables.

## Approval requests

The user often denies approval requests accidentally. If a request is denied
without an explicit instruction to stop, repeat the request through the normal
approval flow instead of treating the denial as a final refusal. A denial does
not grant permission: wait for approval before executing the requested action.
Respect an explicit refusal or instruction to stop.
