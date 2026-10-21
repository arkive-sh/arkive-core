# Arkive web frontend

SvelteKit owns and server-renders the web pages. A `+page.server.ts` is only needed when a page needs server-only data; login currently renders without one. Browser API calls go directly to Go; set `CORE_API_ORIGIN` to the browser-accessible Go API origin and allow the frontend origin with Go's `CORS_ALLOWED_ORIGINS`. Pages not yet added to SvelteKit return its normal 404 instead of falling through to Go.

Run the Go server with `make dev`, then run `npm run dev:web` from the repository root and open `http://localhost:5173/login`; the API defaults to `http://localhost:8080`. In Docker Compose, SvelteKit is public on port 8080 and Go's API is available on port 8081.
