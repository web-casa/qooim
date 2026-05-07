"""SK public answer page walk — `/s/<projectId>`.

The two existing walkers (browser_walk + project_walk) cover the
admin side. Participants hit a different surface: `/s/<id>` loads
the same SK SPA but the bundle's umi router resolves it to the
public answer page, which fires its own /api/public/* calls.

This walker:
  1. picks the first PUBLISHED project visible to admin (so the
     loadProject path returns 200 — drafts 404 by design, see the
     "网络连接失败" toast we hit earlier in the session),
  2. visits /s/<projectId> as an anonymous visitor (no token),
  3. captures every /api/* call's status + body shape,
  4. reports anything 4xx/5xx, HTML-on-JSON-route, or JS errors.
"""

from __future__ import annotations

import json as J
import os
import urllib.request
from collections import defaultdict
from pathlib import Path

from playwright.sync_api import sync_playwright

BASE = os.environ.get("BASE", "http://188.166.247.233")
OUT = Path("/tmp/qooim_public_walk")
OUT.mkdir(exist_ok=True)


def admin_token() -> str:
    req = urllib.request.Request(
        BASE + "/api/auth/login",
        data=J.dumps({"account": "admin", "password": os.environ.get("ADMIN_PW", "123456")}).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req) as r:
        return J.loads(r.read())["token"]


def first_published_project(tok: str) -> str:
    req = urllib.request.Request(
        BASE + "/api/projects",
        headers={"Authorization": "Bearer " + tok},
    )
    with urllib.request.urlopen(req) as r:
        d = J.loads(r.read())
    items = d.get("items") or d.get("data") or d
    if not isinstance(items, list):
        items = []
    for p in items:
        if p.get("status") == 1:
            return p["id"]
    raise RuntimeError("no published project found — publish one first (PUT /api/projects/<id> {\"status\":1})")


def main():
    tok = admin_token()
    pid = first_published_project(tok)
    print(f"using published project {pid}")

    findings = defaultdict(list)
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        # Anonymous context — NO localStorage token, just the anon
        # participant's view.
        ctx = browser.new_context(viewport={"width": 1366, "height": 800})
        page = ctx.new_page()

        def on_response(resp):
            url = resp.url
            if "/api/" not in url:
                return
            st = resp.status
            ctype = resp.headers.get("content-type", "")
            looks_html = "text/html" in ctype.lower()
            flag = None
            if looks_html:
                flag = "HTML_ON_API"
            elif st >= 500:
                flag = "5xx"
            elif st >= 400:
                # 401 on /currentUser is expected pre-auth — skip the
                # noise; we want SHOULD-WORK calls that fail.
                if "/currentUser" in url and st == 401:
                    return
                flag = str(st)
            if flag:
                try:
                    body = resp.text()[:300]
                except Exception:
                    body = ""
                findings["public"].append({
                    "flag": flag,
                    "url": url.replace(BASE, ""),
                    "status": st,
                    "body": body,
                })

        def on_pageerror(err):
            findings["public"].append({"flag": "JS_ERROR", "msg": str(err)[:300]})

        page.on("response", on_response)
        page.on("pageerror", on_pageerror)

        url = BASE + "/s/" + pid
        print(f"-- GET {url}")
        try:
            # SK's JS bundle is ~16 MB; over a slow link domcontentloaded
            # can comfortably take 30s. Use `commit` (fires on first byte)
            # so we don't fail the navigate phase, then sleep for chunks
            # to load + initial /api/* calls to fire.
            page.goto(url, wait_until="commit", timeout=60000)
            page.wait_for_timeout(8000)
        except Exception as e:
            findings["public"].append({"flag": "NAVIGATION", "msg": str(e)[:200]})

        try:
            page.screenshot(path=str(OUT / "public.png"), full_page=True)
        except Exception:
            pass
        browser.close()

    md = ["# SK public answer walk · findings\n"]
    items = findings.get("public", [])
    seen = set()
    deduped = []
    for it in items:
        key = (it.get("flag"), it.get("url") or it.get("msg"))
        if key in seen:
            continue
        seen.add(key)
        deduped.append(it)
    md.append(f"## /s/{pid} ({len(deduped)})")
    for it in deduped:
        if it["flag"] == "JS_ERROR":
            md.append(f"- **JS_ERROR** {it['msg']}")
        elif it["flag"] == "NAVIGATION":
            md.append(f"- **NAV_FAIL** {it['msg']}")
        else:
            md.append(f"- {it['url']} → {it['status']} {it['flag']}")
            md.append(f"  body[:300]={it['body']!r}")
    out = "/tmp/sk_public_walk.md"
    Path(out).write_text("\n".join(md))
    print(f"wrote {out} ({len(deduped)} findings)")


if __name__ == "__main__":
    main()
