"""SK browser walk — per-project tab pages.

The browser_walk.py script visits the top-level list pages (project,
repo, system, …). It misses the bug class that hits when the user
clicks INTO a specific project and SK's tab pages (Data / Report /
Setting / Poster / Flow / Overview) fire their own /api/* calls.

Those per-project routes need a real projectId to load. This walker
takes the first published project, visits each tab in turn, and
reports any /api/* response that's >=400 OR HTML on a JSON route.
"""

from __future__ import annotations

import json
import os
import re
import sys
from collections import defaultdict
from pathlib import Path

from playwright.sync_api import sync_playwright

BASE = os.environ.get("BASE", "http://188.166.247.233")
ADMIN_PW = os.environ.get("ADMIN_PW", "123456")
OUT = Path("/tmp/qooim_project_walk")
OUT.mkdir(exist_ok=True)

# SK admin URL pattern is hash-based; the umi router resolves these
# under the SPA bundle at /. Each tab corresponds to one of the
# `p__survey__*` umi page chunks.
TABS = [
    ("data",     "/survey/{pid}/data"),
    ("report",   "/survey/{pid}/report"),
    ("setting",  "/survey/{pid}/setting"),
    ("flow",     "/survey/{pid}/flow"),
    ("poster",   "/survey/{pid}/poster"),
    ("overview", "/survey/{pid}/overview"),
]


def login_get_token(api_url: str) -> str:
    import urllib.request, json as j
    req = urllib.request.Request(
        api_url + "/api/auth/login",
        data=j.dumps({"account": "admin", "password": ADMIN_PW}).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req) as r:
        body = j.loads(r.read())
    return body["token"]


def first_published_project(token: str) -> str:
    import urllib.request, json as j
    req = urllib.request.Request(
        BASE + "/api/projects",
        headers={"Authorization": "Bearer " + token},
    )
    with urllib.request.urlopen(req) as r:
        d = j.loads(r.read())
    items = d.get("items") or d.get("data") or d
    if isinstance(items, list) and items:
        return items[0]["id"]
    raise RuntimeError("no projects available — create one first")


# current_label is mutable shared state; the response/pageerror
# listeners are installed ONCE per page and look it up so we don't
# need playwright's listener-removal API (which Python is missing).
_current_label = {"v": ""}


def install_listeners(page, findings):
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
            flag = f"{st}"
        if flag:
            try:
                body = resp.text()[:300]
            except Exception:
                body = ""
            findings[_current_label["v"]].append({
                "flag": flag,
                "url": url.replace(BASE, ""),
                "method": resp.request.method,
                "status": st,
                "ctype": ctype,
                "body": body,
            })

    def on_pageerror(err):
        findings[_current_label["v"]].append({
            "flag": "JS_ERROR",
            "msg": str(err)[:300],
        })

    page.on("response", on_response)
    page.on("pageerror", on_pageerror)


def main():
    token = login_get_token(BASE)
    pid = first_published_project(token)
    print(f"using project {pid}")

    findings: dict[str, list] = defaultdict(list)
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        ctx = browser.new_context(
            viewport={"width": 1366, "height": 800},
            base_url=BASE,
        )
        page = ctx.new_page()
        install_listeners(page, findings)

        # Plant the JWT into localStorage on a blank page so SK reads
        # it on subsequent loads. Note we store the RAW token (no
        # "Bearer " prefix); SK's request adapter prepends Bearer
        # itself — see internal/api/console/sk_bridge.go for the
        # canonical write site.
        page.goto(BASE + "/", wait_until="domcontentloaded", timeout=30000)
        page.evaluate(f"localStorage.setItem('Authorization', '{token}')")
        page.evaluate(f"localStorage.setItem('tokenValue', '{token}')")

        for label, tpl in TABS:
            url = BASE + tpl.format(pid=pid)
            print(f"-- {label}  ({tpl.format(pid=pid)})")
            _current_label["v"] = label
            try:
                page.goto(url, wait_until="domcontentloaded", timeout=20000)
                page.wait_for_timeout(2500)  # let lazy chunks load + fire
            except Exception as e:
                findings[label].append({"flag": "NAVIGATION", "msg": str(e)[:200]})
            try:
                page.screenshot(path=str(OUT / f"{label}.png"), full_page=True)
            except Exception:
                pass

        browser.close()

    md = ["# SK per-project walk · findings\n"]
    total = 0
    for label, items in findings.items():
        if not items:
            continue
        # Dedup by (flag, url) for HTTP, and by msg for JS errors
        seen = set()
        deduped = []
        for it in items:
            key = (it.get("flag"), it.get("url") or it.get("msg"))
            if key in seen:
                continue
            seen.add(key)
            deduped.append(it)
        md.append(f"## {label} ({len(deduped)})")
        for it in deduped:
            total += 1
            if it["flag"] == "JS_ERROR":
                md.append(f"- **JS_ERROR** {it['msg']}")
            elif it["flag"] == "NAVIGATION":
                md.append(f"- **NAV_FAIL** {it['msg']}")
            else:
                md.append(f"- {it.get('method','GET')} {it['url']} → {it['status']} {it['flag']}")
                md.append(f"  body[:300]={it['body']!r}")
        md.append("")
    out_path = "/tmp/sk_project_walk.md"
    Path(out_path).write_text("\n".join(md))
    print(f"wrote {out_path} ({total} findings)")


if __name__ == "__main__":
    main()
