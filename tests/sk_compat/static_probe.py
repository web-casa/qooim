"""SK ↔ Qoo.IM static + dynamic probe v2.

Stages:
  1 STATIC: parse bundle for every (verb, /api URL) callsite.
  2 DYNAMIC: probe each with realistic body, classify response.
  3 OPPOSITE-VERB: ensure the OTHER verb 404's plain (caller registered
    the right verb).
  4 Write Markdown punchlist.
"""
import json, os, re
from collections import defaultdict
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

BUNDLE = Path("/home/ivmm/exam-run/web/dist")
BASE = os.environ.get("BASE", "http://188.166.247.233")
TIMEOUT = 6

CALL_RE = re.compile(
    r'this\.(get|post|put|patch|delete|upload|download)'
    r'(?:WithPagination|)?\(\s*"(/api/[^"]+)"',
    re.IGNORECASE,
)


def extract_calls():
    seen = {}
    for f in BUNDLE.glob("*.js"):
        try:
            txt = f.read_text(errors="ignore")
        except Exception:
            continue
        for m in CALL_RE.finditer(txt):
            verb_raw = m.group(1).upper()
            verb = "POST" if verb_raw == "UPLOAD" else verb_raw
            if verb_raw == "DOWNLOAD":
                verb = "GET"
            url = m.group(2).split("?")[0]
            seen[(verb, url)] = (verb_raw, seen.get((verb, url), (None, 0))[1] + 1)
    return [(k[0], k[1], v[0], v[1]) for k, v in sorted(seen.items())]


def http(method, path, body=None, headers=None):
    h = {"Content-Type": "application/json"}
    if headers:
        h.update(headers)
    data = json.dumps(body).encode() if body is not None else None
    req = Request(BASE + path, data=data, method=method, headers=h)
    try:
        r = urlopen(req, timeout=TIMEOUT)
        return r.status, dict(r.headers), r.read()
    except HTTPError as e:
        return e.code, dict(e.headers), e.read()
    except URLError as e:
        return -1, {}, str(e).encode()


def login():
    st, _, body = http("POST", "/api/public/login", {"username": "admin", "password": "123456"})
    if st != 200:
        return ""
    try:
        return json.loads(body)["data"]["token"]
    except Exception:
        return ""


def realistic(verb, url):
    if verb == "GET":
        return None
    if "list" in url:
        return {"current": 1, "pageSize": 5}
    if any(s in url for s in ["delete", "destroy", "trash", "restore", "softDelete"]):
        return {"ids": ["x"]}
    if "create" in url:
        return {"name": "probe", "mode": "survey"}
    if "update" in url:
        return {"id": "x", "name": "probe"}
    if "register" in url:
        return {"username": "_zzdup_", "password": "p"}
    if "validate" in url or url.endswith("/loadProject") or url.endswith("/saveAnswer"):
        return {"id": "x"}
    if "load" in url:
        return {"id": "x"}
    return {}


def classify(verb, url, st, headers, raw):
    ctype = (headers.get("Content-Type") or "").lower()
    is_html = b"<!doctype" in raw[:32].lower() or b"<html" in raw[:32].lower()
    text_404 = b"404 page not found" in raw[:80]

    if st == 404 and (text_404 or is_html):
        return ("BLOCKER", "MISSING_ENDPOINT", f"{verb} {url} → 404 (route not registered)")
    if is_html and verb in ("GET", "POST"):
        return ("BLOCKER", "HTML_INSTEAD_OF_JSON", f"{verb} {url} returns HTML for an /api/* path")
    if st >= 500:
        return ("BLOCKER", "SERVER_ERROR", f"{verb} {url} → {st} {raw[:80]!r}")
    parsed = None
    if "application/json" in ctype:
        try:
            parsed = json.loads(raw)
        except Exception:
            pass
    if parsed is None:
        if st < 400:
            return ("MAJOR", "NOT_JSON", f"{verb} {url} → {st} body isn't JSON ({ctype or 'no ctype'})")
        return None
    if "code" not in parsed:
        return ("MAJOR", "ENVELOPE_NO_CODE", f"{verb} {url} JSON missing numeric `code`")
    if "success" not in parsed:
        return ("MAJOR", "ENVELOPE_NO_SUCCESS", f"{verb} {url} JSON missing `success`")
    code = parsed.get("code")
    if isinstance(code, int):
        if code in (200, 400, 401, 403, 404):
            return None
        return ("MINOR", "ODD_CODE", f"{verb} {url} → code={code}")
    return ("MAJOR", "CODE_NOT_INT", f"{verb} {url} code is {type(code).__name__}: {code!r}")


def opposite_verb(token, calls):
    """Detect routes registered under the WRONG verb. Definition of
    'wrong': the bundle calls GET, but a POST also returns code=200,
    while the GET returns code=404 from the NoRoute fallback (with the
    canonical message 'endpoint not found'). True positive matches
    that pattern; redundant-verb dual registrations don't (since the
    bundle's verb already returns 200)."""
    used = defaultdict(set)
    for v, u, _, _ in calls:
        used[u].add(v)
    headers = {"Authorization": "Bearer " + token} if token else {}
    NOROUTE_MSG = b'"message":"endpoint not found"'
    out = []
    for url, verbs in sorted(used.items()):
        # Did the verbs SK uses get NoRoute'd?
        sk_was_noroute = False
        for v_sk in verbs:
            st, _, raw = http(v_sk, url, realistic(v_sk, url), headers)
            if st == 404 and NOROUTE_MSG in raw:
                sk_was_noroute = True
                break
        if not sk_was_noroute:
            continue
        # Then if the OTHER verb returns code=200 it's a real bug.
        for v_other in ("GET", "POST"):
            if v_other in verbs:
                continue
            st, h, raw = http(v_other, url, realistic(v_other, url), headers)
            try:
                p = json.loads(raw)
            except Exception:
                continue
            if p and p.get("code") == 200:
                out.append(
                    f"WRONG VERB: SK calls {sorted(verbs)} {url} but only {v_other} responds 200"
                )
    return out


def main():
    print("== Stage 1: bundle ==")
    calls = extract_calls()
    print(f"  {len(calls)} (verb, url) callsites")

    token = login()
    if not token:
        print("  WARN: login failed; auth-gated probes will 401")

    print("== Stage 2: probe each ==")
    findings = defaultdict(list)
    for verb, url, raw_verb, count in calls:
        body = realistic(verb, url)
        h = {"Authorization": "Bearer " + token} if token else {}
        st, hdrs, raw = http(verb, url, body, h)
        cls = classify(verb, url, st, hdrs, raw)
        if cls:
            sev, cat, msg = cls
            findings[(sev, cat)].append(f"  - {msg} (SK calls × {count})")

    print("== Stage 3: opposite-verb ==")
    for m in opposite_verb(token, calls):
        findings[("INFO", "REDUNDANT_VERB")].append(f"  - {m}")

    print("== Stage 4: write findings ==")
    out = ["# SK ↔ Qoo.IM probe v2", ""]
    for (sev, cat), msgs in sorted(findings.items()):
        out.append(f"## {sev} · {cat}  ({len(msgs)})")
        out.append("")
        out.extend(sorted(set(msgs)))
        out.append("")
    if not findings:
        out.append("✅ Probe found nothing actionable.")
    Path("/tmp/sk_probe_findings.md").write_text("\n".join(out), encoding="utf-8")
    total = sum(len(v) for v in findings.values())
    print(f"  wrote /tmp/sk_probe_findings.md ({total} findings)")
    for k, v in sorted(findings.items()):
        print(f"  {k[0]:8} {k[1]:24} {len(v)}")


if __name__ == "__main__":
    main()
