#!/usr/bin/env bash
# Walks every Qoo.IM endpoint surface against a deployed instance.
# Prints PASS/FAIL per check and exits non-zero on any failure.
#
# Usage:
#   DROPLET_IP=1.2.3.4 ./smoke.sh
#   DROPLET_IP=1.2.3.4 ADMIN_PW=123456 ./smoke.sh   # custom admin pw
set -uo pipefail

: "${DROPLET_IP:?need DROPLET_IP}"
PORT="${DO_PORT:-80}"
ADMIN_PW="${ADMIN_PW:-123456}"
BASE="http://$DROPLET_IP:$PORT"

fail=0
pass() { printf "  \033[32mPASS\033[0m %s\n" "$1"; }
miss() { printf "  \033[31mFAIL\033[0m %s — %s\n" "$1" "$2"; fail=$((fail+1)); }

echo "== smoke against $BASE =="

# health
H=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/healthz")
[ "$H" = "200" ] && pass "/healthz=200" || miss "/healthz" "got $H"
H=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/readyz")
[ "$H" = "200" ] && pass "/readyz=200" || miss "/readyz" "got $H"

# version
V=$(curl -sS "$BASE/api/version")
[[ "$V" == *'"name":"Qoo.IM"'* ]] && pass "version env" || miss "version" "$V"

# login
L=$(curl -sS -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"account\":\"admin\",\"password\":\"$ADMIN_PW\"}")
TOK=$(printf '%s' "$L" | python3 -c 'import sys,json;print(json.load(sys.stdin).get("token",""))' 2>/dev/null || true)
[ -n "$TOK" ] && pass "login → token (${#TOK}c)" || { miss "login" "$L"; exit 1; }

AUTH=(-H "Authorization: Bearer $TOK")

# /me
M=$(curl -sS "${AUTH[@]}" "$BASE/api/me")
[[ "$M" == *'"name":"Admin"'* ]] && pass "/me Admin principal" || miss "/me" "$M"

# list endpoints
for path in projects repos templates dashboards; do
  RESP=$(curl -sS "${AUTH[@]}" "$BASE/api/$path")
  [[ "$RESP" == *'"items":'* ]] && pass "GET /api/$path" || miss "GET /api/$path" "$RESP"
done

# project lifecycle
PID=$(curl -sS -X POST "$BASE/api/projects" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"name":"smoke","mode":"survey","status":1,"survey":"{\"title\":\"smoke\"}"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin).get("id",""))')
[ -n "$PID" ] && pass "POST /api/projects → $PID" || { miss "POST /api/projects" "no id"; exit 1; }

# public render
P=$(curl -sS "$BASE/api/survey/$PID")
[[ "$P" == *'"name":"smoke"'* ]] && pass "public survey render" || miss "public survey" "$P"

# guest answer
A=$(curl -sS -X POST "$BASE/api/survey/$PID/answer" -H 'Content-Type: application/json' \
  -d '{"answer":{"q":"a"},"temp_save":1}')
[[ "$A" == *'"id":'* ]] && pass "public answer submit" || miss "submit" "$A"

# report
R=$(curl -sS "${AUTH[@]}" "$BASE/api/projects/$PID/report")
[[ "$R" == *'"finished":1'* ]] && pass "report finished=1" || miss "report" "$R"

# xlsx export — must start with PK\03\04
X=$(curl -sS "${AUTH[@]}" "$BASE/api/projects/$PID/answers.xlsx" | head -c 4 | xxd -p)
[ "$X" = "504b0304" ] && pass "answers.xlsx PK header" || miss "xlsx" "$X"

# ai chat must 404 unless QOOIM_AI_TOKEN was set on the droplet
AI=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE/api/ai/chat" "${AUTH[@]}" \
  -H 'Content-Type: application/json' -d '{"messages":[{"role":"user","content":"hi"}]}')
case "$AI" in
  404) pass "/api/ai/chat=404 (no provider)";;
  200) pass "/api/ai/chat=200 (provider configured)";;
  *)   miss "/api/ai/chat" "got $AI";;
esac

# cleanup
DEL=$(curl -sS -o /dev/null -w '%{http_code}' -X DELETE "$BASE/api/projects/$PID" "${AUTH[@]}")
[ "$DEL" = "204" ] && pass "DELETE /api/projects/:id=204" || miss "delete" "$DEL"

# ---------- Console (Gate 1-3 + 5) ----------------------------------------
CJ=$(mktemp)
trap "rm -f $CJ" EXIT

H=$(curl -sS -o /dev/null -w '%{http_code}' -c "$CJ" "$BASE/console/login")
[ "$H" = "200" ] && pass "GET /console/login=200" || miss "console-login" "got $H"

CTOK=$(grep qooim_console_csrf "$CJ" | awk '{print $7}')
[ -n "$CTOK" ] && pass "console csrf cookie minted" || miss "csrf" "missing"

H=$(curl -sS -o /dev/null -w '%{http_code}' -b "$CJ" -c "$CJ" -X POST "$BASE/console/login" \
  -d "username=admin&password=$ADMIN_PW&csrf=$CTOK")
case "$H" in
  302) pass "POST /console/login=302 (admin)";;
  *)   miss "console-login post" "got $H";;
esac

if grep -q "qooim_console_session" "$CJ"; then
  pass "console session cookie set"
  for page in dashboard system/users system/roles system/depts system/positions system/dicts; do
    H=$(curl -sS -o /dev/null -w '%{http_code}' -b "$CJ" "$BASE/console/$page")
    [ "$H" = "200" ] && pass "/console/$page=200" || miss "/console/$page" "got $H"
  done
  BR=$(curl -sS -b "$CJ" "$BASE/console/sk-bridge" | grep -c 'var token = "Bearer eyJ')
  [ "$BR" -ge 1 ] && pass "/console/sk-bridge renders Bearer token" || miss "sk-bridge" "no token"
  D=$(curl -sS -b "$CJ" "$BASE/console/sk-bridge?next=//evil.com" | grep -oE 'var dest = "[^"]*"')
  [ "$D" = 'var dest = "/"' ] && pass "sk-bridge ?next=//evil → /" || miss "sk-bridge open-redirect" "got $D"
else
  miss "console session" "cookie missing — InsecureCookies misconfigured?"
fi

# ---------- Answer UI (Gate 4) -------------------------------------------
H=$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/answerui/demo?demo=1")
[ "$H" = "404" ] && pass "/answerui/demo (prod)=404" || miss "demo prod" "got $H"

RM=$(mktemp)
echo "fake-png" > "$RM"
PUP=$(curl -sS -X POST -F "file=@$RM;filename=spike.png" "$BASE/api/public/upload")
PID2=$(printf '%s' "$PUP" | python3 -c 'import sys,json;d=json.load(sys.stdin);print(d.get("data",{}).get("id",""))' 2>/dev/null || true)
rm -f "$RM"
if [ -n "$PID2" ]; then
  pass "public upload id=${PID2:0:10}"
  N=$(curl -sS -D - -o /dev/null "$BASE/api/file?id=$PID2" | grep -ci 'x-content-type-options: nosniff')
  [ "$N" -ge 1 ] && pass "/api/file: nosniff header set" || miss "nosniff" "missing"
fi

RM=$(mktemp)
echo '<script>alert(1)</script>' > "$RM"
PHM=$(curl -sS -o /dev/null -w '%{http_code}' -X POST -F "file=@$RM;filename=evil.html" "$BASE/api/public/upload")
rm -f "$RM"
[ "$PHM" = "400" ] && pass "public upload rejects .html" || miss ".html upload" "got $PHM"

echo
[ "$fail" = 0 ] && { echo "all checks passed"; exit 0; }
echo "$fail check(s) failed"; exit 1
