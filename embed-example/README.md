# Superset Embed Example

This folder contains a minimal example showing how to embed a Superset dashboard using the Superset Embedded SDK and a backend endpoint that requests a Guest Token from Superset.

Structure

- `backend/main.go` - minimal Go HTTP server exposing `/superset/guest-token` which proxies a request to Superset's `/api/v1/security/guest_token/`.
- `frontend/` - minimal Next.js files showing how to call `embedDashboard` with a `fetchGuestToken` helper and a toggle button to switch ACTIVE/INACTIVE filters.

Before running

1. Set `SUPERSET_BASE_URL` and `SUPERSET_API_KEY` environment variables for the Go server (or adapt auth). The Go handler uses `Authorization: Bearer $SUPERSET_API_KEY` to authenticate with Superset.
2. In Superset Admin > Embed: add Allowed Domains (comma separated): `http://localhost:3000` so the demo page can load the embedded iframe.
3. Create the dataset and dashboard in Superset (use your SQL). Add a native `status` filter (values `ACTIVE` / `INACTIVE`) to the dashboard.

Quick start

1. Copy `.env.example` to `.env` files for backend/frontend and fill values.

Backend (Go)

```bash
# from embed-example/backend
go run main.go
```

Frontend (Next.js)

```bash
cd embed-example/frontend
npm install
npm run dev
```

Open `http://localhost:3000/embed-demo` to view the demo.

Note
This example scaffolds the integration and a toggle button; depending on your Superset Embedded SDK version you may need to adapt `applyStatusFilter` in `frontend/components/SupersetEmbed.tsx` to match the SDK's method for programmatically setting native filters (e.g., `setDataMask` or `postMessage` with the correct payload). The demo uses dashboard id `1fe3a335-46b9-4b44-b1e0-dd32f8b117c7` by default.
