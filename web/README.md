# Web bundle

`web/dist/` is a verbatim copy of the [SurveyKing](https://github.com/javahuang/SurveyKing)
v1.12.0 compiled UI (UmiJS / React). SK is MIT-licensed; the original
notice is preserved at `web/SK-LICENSE`.

The bundle is served by `qooim-server` as a single-page app:
- `GET /` returns `index.html`.
- Hashed asset filenames (`*.async.js` / `*.chunk.css` / etc.) are
  served from this directory.
- Any other non-`/api/*` path falls back to `index.html` so client-
  side routing works.

The Qoo.IM HTTP layer adds a thin **SK-compat adapter** under `/api/*`
so the SK frontend's action-style requests
(`POST /api/public/login`, `POST /api/project/list`, …) speak to the
same Go services that back our cleaner REST routes
(`POST /api/auth/login`, `GET /api/projects`, …). See
`internal/api/sk_compat.go`.

This bundle is a **starting point**, not a fork. Replace
`web/dist/` with a Qoo.IM-branded build whenever it's ready.
