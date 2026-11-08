# Admin frontend

Vue 3 single-file components, Vue render functions for tool definitions, Tailwind
CSS 4 and Vite. The Go API remains responsible for authentication, validation,
transactions and version conflicts. No CDN or additional runtime service is used.

From the repository root, with Node.js 22.18+ (or current Node.js LTS) installed:

```sh
npm ci
npm run admin:dev
npm run admin:test
npm run format:check
npm run lint
npm run admin:build
go test ./internal/admin
```

The dev server binds to loopback and proxies API/registration requests to
`127.0.0.1:8080`. Run the Go server separately when using real data. The dev
server uses the same API and permissions as production.

Commit `package-lock.json`, frontend sources and the complete generated
`internal/admin/web` directory together. `admin:build` replaces that directory;
do not hand-edit its generated HTML, JavaScript or CSS. A normal Go build embeds
the checked-in bundle and needs neither Node.js nor npm installation. Build
after source changes, before testing the Go embedding or rebuilding the server.

`App.vue` owns the in-memory token, connection status, navigation and stale-read
cancellation. `views.js` retains the admin workflows as Vue render functions.
Reusable components own form values, pending mutations, filtering, sorting,
pagination and modal focus. JSON saves retain server-provided versions. Failed
saves keep the editor open; successful saves close before a refresh, preventing
a failed refresh from inviting a duplicate purchase, grant or balance change.

The public `/register` entry uses `Registration.vue` in the same compiled app;
POST `/register` and its existing server validation remain unchanged.

The frontend test suite covers all navigation pages, native Vue component
interactions, API authorization errors, duplicate submissions and versioned
character edits. Go tests cover API behavior and public embedded entry points.
