# SK compat probes

Two complementary scripts that catch the kinds of bug we kept finding
manually while the user clicked around. Run them together against any
deployed Qoo.IM that has the SK frontend bundle wired up.

## What each one catches

| Probe                | Catches                                                               |
| -------------------- | --------------------------------------------------------------------- |
| `static_probe.py`    | • Endpoints SK calls but we don't register (404 from NoRoute)         |
|                      | • Routes registered under the wrong HTTP verb                         |
|                      | • Responses missing the `{success, code:200, …}` envelope             |
|                      | • Responses returning HTML where SK expects JSON                      |
| `browser_walk.py`    | Body-shape mismatches and modal auxiliary fetches that only show up   |
|                      | once the SPA actually mounts. Logs in, walks every admin page, clicks |
|                      | every visible 新建/导出/导入/搜索 button, captures every /api/* 4xx.  |

## Running

Both default to a public droplet at `BASE=http://188.166.247.233`.
Override via the `BASE` env var.

```bash
# Static — needs the SurveyKing bundle source for parsing:
SK_BUNDLE=/home/ivmm/otheruse/surveyking/server/api/src/main/resources/static \
  python3 tests/sk_compat/static_probe.py
# → /tmp/sk_probe_findings.md

# Browser walk — needs Python's `playwright` installed plus a chromium.
python3 tests/sk_compat/browser_walk.py
# → /tmp/sk_browser_walk.md  + /tmp/qooim_browser_walk/*.png
```

Findings are graded `BLOCKER / MAJOR / MINOR / INFO`. A clean run
prints `0 findings`.

## When to run

- Before declaring a SK-compat increment "done".
- After bumping the SK static bundle (new build = new endpoint set).
- After any change that touches `internal/api/sk_*.go` or `router.go`.

The static probe is fast (~30s); the browser walk is ~1 min for a full
sweep of 14 admin routes.
