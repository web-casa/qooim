"""SK browser walk — login, navigate every admin page, click every
visible 新建/编辑/删除/查询/导出/导入/确定/保存/添加 button, capture every
/api/* response with status>=400 OR Content-Type wrong OR body looking
like HTML. Reports per-page punchlist.

Goal: catch dynamic body-shape mismatches the static probe missed.
"""

from __future__ import annotations

import json
import os
import re
import sys
import time
from collections import defaultdict
from pathlib import Path

from playwright.sync_api import sync_playwright, TimeoutError as PWTimeout

BASE = os.environ.get("BASE", "http://188.166.247.233")
OUT = Path("/tmp/qooim_browser_walk")
OUT.mkdir(exist_ok=True)

# Routes from the SK bundle's umi config (extracted earlier).
ROUTES = [
    ("home", "/home"),
    ("project", "/project"),
    ("repo", "/repo"),
    ("repo_template", "/repo/template"),
    ("repo_book", "/repo/book"),
    ("system", "/system"),
    ("system_user", "/system/user"),
    ("system_role", "/system/role"),
    ("system_dept", "/system/dept"),
    ("system_dict", "/system/dict"),
    ("system_position", "/system/position"),
    ("exercise", "/exercise"),
    ("survey_list", "/survey"),
    ("account", "/account/settings"),
]

# Buttons we'll try to click on each page once the listing has rendered.
# We click then immediately back out (Esc / dialog close) — the goal is
# only to trigger any auxiliary fetch the modal does on open.
TRIGGER_BUTTONS = [
    "新建", "新增", "添加", "添加试题",
    "查询", "导出", "导入",
    "刷新", "搜索",
]


def install_listeners(page, label, findings):
    def on_response(resp):
        url = resp.url
        if "/api/" not in url:
            return
        if url.endswith(".map"):
            return
        st = resp.status
        ctype = resp.headers.get("content-type", "")
        looks_html = "text/html" in ctype.lower()
        # 200 with HTML on /api/* is a SPA-fallback bug; otherwise we
        # care about >=400.
        flag = None
        if looks_html:
            flag = ("HTML_ON_API", st)
        elif st >= 500:
            flag = ("SERVER_ERROR", st)
        elif st >= 400:
            flag = (f"HTTP_{st}", st)
        if flag:
            try:
                body_preview = resp.text()[:200]
            except Exception:
                body_preview = ""
            findings[label].append({
                "url": url.replace(BASE, ""),
                "method": resp.request.method,
                "status": st,
                "ctype": ctype,
                "preview": body_preview,
            })

    page.on("response", on_response)


def login(page):
    """Use the API directly so we don't depend on the form parsing."""
    page.goto(BASE + "/", wait_until="networkidle", timeout=30000)
    res = page.evaluate(
        """async (base) => {
            const r = await fetch(base + '/api/public/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({username:'admin', password:'123456'})
            });
            const j = await r.json();
            if (j && j.success && j.data && j.data.token) {
                localStorage.setItem('Authorization', j.data.token);
                return {ok: true};
            }
            return {ok: false, body: j};
        }""",
        BASE,
    )
    return res.get("ok")


def click_buttons_safely(page, label, sleep=400):
    """Click every visible trigger button once. Cancel modal afterwards."""
    for txt in TRIGGER_BUTTONS:
        try:
            loc = page.locator(f'button:has-text("{txt}")').first
            if loc.count() == 0:
                continue
            try:
                loc.click(timeout=1500)
            except Exception:
                continue
            page.wait_for_timeout(sleep)
            # Cancel any modal: try a 'cancel/取消/关闭' button, else Escape.
            for cancel in ("取消", "关闭", "Cancel"):
                cb = page.locator(f'button:has-text("{cancel}")').first
                if cb.count():
                    try:
                        cb.click(timeout=600)
                        break
                    except Exception:
                        pass
            else:
                page.keyboard.press("Escape")
            page.wait_for_timeout(150)
        except Exception:
            continue


def main():
    findings = defaultdict(list)
    with sync_playwright() as pw:
        b = pw.chromium.launch(headless=True)
        ctx = b.new_context(viewport={"width": 1366, "height": 900},
                            ignore_https_errors=True, locale="zh-CN")
        page = ctx.new_page()
        install_listeners(page, "_global_", findings)

        if not login(page):
            print("login failed; aborting")
            sys.exit(2)

        for label, path in ROUTES:
            print(f"-- {label}  ({path})")
            findings.pop(label, None)
            install_listeners(page, label, findings)
            try:
                page.goto(BASE + path, wait_until="networkidle", timeout=20000)
            except PWTimeout:
                pass
            page.wait_for_timeout(1500)
            try:
                page.screenshot(path=str(OUT / f"{label}.png"), full_page=False)
            except Exception:
                pass
            click_buttons_safely(page, label)

        b.close()

    # Report
    out = ["# SK browser walk · per-page 4xx/5xx/HTML-on-api findings", ""]
    total = 0
    for label, items in findings.items():
        if not items:
            continue
        out.append(f"## {label} ({len(items)})")
        out.append("")
        # dedupe by (method,url,status)
        seen = set()
        for f in items:
            key = (f["method"], f["url"], f["status"])
            if key in seen:
                continue
            seen.add(key)
            out.append(f"- {f['method']} {f['url']} → {f['status']} ({f['ctype']})")
            if f["preview"]:
                out.append(f"  body[:200]={f['preview']!r}")
            total += 1
        out.append("")
    if total == 0:
        out.append("✅ Walk found nothing actionable.")
    Path("/tmp/sk_browser_walk.md").write_text("\n".join(out), encoding="utf-8")
    print(f"wrote /tmp/sk_browser_walk.md ({total} unique findings)")
    for label, items in findings.items():
        if items:
            print(f"  {label}: {len(items)} raw")


if __name__ == "__main__":
    main()
