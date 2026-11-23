# PowerWhois Web

Vue 3 + Quasar. Use Node.js 24 and npm.

- `npm ci`
- `npm run dev` — local 8090, backend proxy to 127.0.0.1:8080
- `npm test` and `npm run lint`
- `npm run build` — output `dist/spa`

Use the root [deployment guide](../../docs/DEPLOYMENT.md). The existing `.env` is not automatically a safe place for browser credentials; do not expose secrets through client build variables.
