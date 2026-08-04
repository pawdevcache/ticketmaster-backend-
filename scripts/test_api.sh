#!/usr/bin/env bash
#
# End-to-end check of every route in the Ticketmaster API.
#
# Run it against any environment:
#
#   scripts/test_api.sh                        # http://localhost:8080
#   scripts/test_api.sh https://api.example.com
#   make test-api BASE_URL=https://api.example.com
#
# Exits non-zero if anything fails, so it works as a CI gate.
#
# Assumptions, all overridable by environment variable:
#
#   ADMIN_EMAIL / ADMIN_PASSWORD   an existing admin account to sign in as
#   ADMIN_REGISTRATION_KEY         the secret POST /api/admin/register expects
#
# Accounts it creates carry a per-run suffix and are deleted at the end, so the
# suite can be run repeatedly against the same database. It also creates and
# then deletes catalogue records; a failure part-way through can leave some
# behind, so prefer a scratch database for routine runs.
#
# Password-reset assertions are skipped automatically when the deployment does
# not echo the reset token (ENV=production), since there is no other way for a
# black-box test to learn it.
#
# Auth-bearing calls each use a distinct X-Forwarded-For so the rate limiter's
# per-caller buckets never collide; the 429 case at the end deliberately
# reuses one.
set -u

B=${1:-${BASE_URL:-http://localhost:8080}}
J='Content-Type: application/json'
ADMIN_EMAIL=${ADMIN_EMAIL:-admin@gmail.com}
ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}
ADMIN_REGISTRATION_KEY=${ADMIN_REGISTRATION_KEY:-admin123}

# Unique per run so a second run against the same database does not collide
# with the accounts the first one created.
RUN=$$-$(date +%s 2>/dev/null || echo 0)
USER_EMAIL="test-user-$RUN@example.test"
EVE_EMAIL="test-eve-$RUN@example.test"
OPS_EMAIL="test-ops-$RUN@example.test"
ADMIN_CREDS="{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"

pass=0; fail=0; skip=0; failed=()

ck() { # ck <name> <want> <got>
  if [ "$2" = "$3" ]; then pass=$((pass+1)); printf '  \033[32mPASS\033[0m %-52s %s\n' "$1" "$3"
  else fail=$((fail+1)); failed+=("$1"); printf '  \033[31mFAIL\033[0m %-52s got %s want %s\n' "$1" "$3" "$2"; fi
}
sk() { skip=$((skip+1)); printf '  \033[33mSKIP\033[0m %-52s %s\n' "$1" "$2"; }
code()  { curl -s -m 30 -o /dev/null -w '%{http_code}' "$@"; }
body()  { curl -s -m 30 "$@"; }
# First "id" in the payload — a booking embeds its event, and a greedy match
# would pick the event's id instead of the booking's.
id()    { grep -oE '"id":"[a-zA-Z0-9]+"' | head -1 | sed 's/"id":"//; s/"//'; }
has()   { if echo "$1" | grep -q "$2"; then echo yes; else echo no; fi; }
ip()    { echo "X-Forwarded-For: 10.$((RANDOM%250+1)).$((RANDOM%250+1)).$((RANDOM%250+1))"; }

if ! curl -s -m 15 -o /dev/null "$B/health"; then
  echo "cannot reach $B/health — is the API running?" >&2
  exit 2
fi
echo "testing $B"
echo

echo "=== 1. service (4 routes) ==="
ck "GET /"                       200 "$(code $B/)"
ck "GET /health"                 200 "$(code $B/health)"
ck "GET /docs"                   200 "$(code $B/docs)"
ck "GET /openapi.yaml"           200 "$(code $B/openapi.yaml)"

echo "=== 2. admin sign-in ==="
AR=$(body -X POST $B/api/admin/login -H "$J" -H "$(ip)" -d "$ADMIN_CREDS")
A=$(echo "$AR" | sed -E 's/.*"token":"([^"]*)".*/\1/'); AH="Authorization: Bearer $A"
ck "POST /api/admin/login"       200 "$(code -X POST $B/api/admin/login -H "$J" -H "$(ip)" -d "$ADMIN_CREDS")"
ck "  returns an admin role"     yes "$(has "$AR" '"role":"admin"')"
ck "GET /api/admin/me"           200 "$(code $B/api/admin/me -H "$AH")"

echo "=== 3. discovery writes — create (4 routes) ==="
CLS=$(body -X POST $B/discovery/v2/classifications -H "$AH" -H "$J" -d '{"segment":"Music","genre":"Rock"}')
CID=$(echo "$CLS" | id)
ck "POST /discovery/v2/classifications" 201 "$(code -X POST $B/discovery/v2/classifications -H "$AH" -H "$J" -d '{"segment":"Arts","genre":"Theatre"}')"
VEN=$(body -X POST $B/discovery/v2/venues -H "$AH" -H "$J" -d '{"name":"National Arena","city":"Colombo","capacity":500}')
VID=$(echo "$VEN" | id)
ck "POST /discovery/v2/venues"          201 "$(code -X POST $B/discovery/v2/venues -H "$AH" -H "$J" -d '{"name":"Side Hall","city":"Kandy"}')"
ATT=$(body -X POST $B/discovery/v2/attractions -H "$AH" -H "$J" -d "{\"name\":\"The Band\",\"type\":\"performer\",\"classificationId\":\"$CID\"}")
AID=$(echo "$ATT" | id)
ck "POST /discovery/v2/attractions"     201 "$(code -X POST $B/discovery/v2/attractions -H "$AH" -H "$J" -d '{"name":"Support Act"}')"
EVT=$(body -X POST $B/discovery/v2/events -H "$AH" -H "$J" -d "{\"name\":\"Rock Night\",\"date\":\"2026-09-01T19:30:00Z\",\"venueId\":\"$VID\",\"attractionIds\":[\"$AID\"],\"classificationId\":\"$CID\",\"priceMin\":50,\"priceMax\":90,\"ticketsTotal\":100}")
EID=$(echo "$EVT" | id)
ck "POST /discovery/v2/events"          201 "$(code -X POST $B/discovery/v2/events -H "$AH" -H "$J" -d "{\"name\":\"Second Show\",\"venueId\":\"$VID\",\"priceMin\":10,\"ticketsTotal\":5}")"
ck "  new event defaults to onsale"     yes "$(has "$EVT" '"status":"onsale"')"

echo "=== 4. discovery reads (8 routes) ==="
ck "GET /discovery/v2/events"            200 "$(code $B/discovery/v2/events)"
ck "GET /discovery/v2/events/{id}"       200 "$(code $B/discovery/v2/events/$EID)"
ck "GET /discovery/v2/venues"            200 "$(code $B/discovery/v2/venues)"
ck "GET /discovery/v2/venues/{id}"       200 "$(code $B/discovery/v2/venues/$VID)"
ck "GET /discovery/v2/attractions"       200 "$(code $B/discovery/v2/attractions)"
ck "GET /discovery/v2/attractions/{id}"  200 "$(code $B/discovery/v2/attractions/$AID)"
ck "GET /discovery/v2/classifications"   200 "$(code $B/discovery/v2/classifications)"
ck "GET /discovery/v2/classifications/{id}" 200 "$(code $B/discovery/v2/classifications/$CID)"
ck "  paged envelope present"            yes "$(has "$(body $B/discovery/v2/events)" '"totalElements"')"
ck "  keyword filter works"              yes "$(has "$(body "$B/discovery/v2/events?keyword=Rock")" 'Rock Night')"
ck "  city filter works"                 yes "$(has "$(body "$B/discovery/v2/events?city=Colombo")" 'Rock Night')"
ck "  regex is escaped (.* matches none)" '"totalElements":0' "$(curl -s -m 30 --get --data-urlencode 'keyword=.*' $B/discovery/v2/venues | grep -o '"totalElements":[0-9]*')"

echo "=== 5. discovery writes — update (8 routes) ==="
UPD=$(body -X PUT $B/discovery/v2/events/$EID -H "$AH" -H "$J" -d '{"priceMin":55}')
ck "PUT /discovery/v2/events/{id}"        200 "$(code -X PUT $B/discovery/v2/events/$EID -H "$AH" -H "$J" -d '{"priceMin":55}')"
ck "  partial update keeps other fields"  yes "$(has "$UPD" '"name":"Rock Night"')"
ck "PATCH /discovery/v2/events/{id}"      200 "$(code -X PATCH $B/discovery/v2/events/$EID -H "$AH" -H "$J" -d '{"status":"onsale"}')"
ck "PUT /discovery/v2/venues/{id}"        200 "$(code -X PUT $B/discovery/v2/venues/$VID -H "$AH" -H "$J" -d '{"capacity":650}')"
ck "PATCH /discovery/v2/venues/{id}"      200 "$(code -X PATCH $B/discovery/v2/venues/$VID -H "$AH" -H "$J" -d '{"state":"WP"}')"
ck "PUT /discovery/v2/attractions/{id}"   200 "$(code -X PUT $B/discovery/v2/attractions/$AID -H "$AH" -H "$J" -d '{"type":"band"}')"
ck "PATCH /discovery/v2/attractions/{id}" 200 "$(code -X PATCH $B/discovery/v2/attractions/$AID -H "$AH" -H "$J" -d '{"name":"The Band Live"}')"
ck "PUT /discovery/v2/classifications/{id}"   200 "$(code -X PUT $B/discovery/v2/classifications/$CID -H "$AH" -H "$J" -d '{"genre":"Indie"}')"
ck "PATCH /discovery/v2/classifications/{id}" 200 "$(code -X PATCH $B/discovery/v2/classifications/$CID -H "$AH" -H "$J" -d '{"genre":"Indie Rock"}')"
FORGE=$(body -X PATCH $B/discovery/v2/events/$EID -H "$AH" -H "$J" -d '{"ticketsSold":999,"id":"hacked"}')
ck "  forged id/ticketsSold ignored"      yes "$(has "$FORGE" "\"id\":\"$EID\",")"

echo "=== 6. user accounts (3 routes) ==="
ck "POST /api/register"          201 "$(code -X POST $B/api/register -H "$J" -H "$(ip)" -d '{"name":"Sam","email":"'"$USER_EMAIL"'","password":"pw12345"}')"
UR=$(body -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"pw12345"}')
UT=$(echo "$UR" | sed -E 's/.*"token":"([^"]*)".*/\1/'); UH="Authorization: Bearer $UT"
ck "POST /api/login"             200 "$(code -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"pw12345"}')"
ck "  role is user, not admin"   yes "$(has "$UR" '"role":"user"')"
ck "  self-promotion ignored"    yes "$(has "$(body -X POST $B/api/register -H "$J" -H "$(ip)" -d '{"name":"Eve","email":"'"$EVE_EMAIL"'","password":"pw12345","role":"admin"}')" '"role":"user"')"

echo "=== 7. bookings (4 routes) ==="
BK=$(body -X POST $B/api/bookings -H "$UH" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":3}")
BID=$(echo "$BK" | id)
ck "POST /api/bookings"          201 "$(code -X POST $B/api/bookings -H "$UH" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":1}")"
ck "GET /api/bookings"           200 "$(code $B/api/bookings -H "$UH")"
ck "GET /api/bookings/{id}"      200 "$(code $B/api/bookings/$BID -H "$UH")"
ck "  booking embeds its event"  yes "$(has "$BK" '"venueName":"National Arena"')"
ck "  total = qty x priceMin"    yes "$(has "$BK" '"total":165')"
ck "DELETE /api/bookings/{id}"   200 "$(code -X DELETE $B/api/bookings/$BID -H "$UH")"
ck "  cancel returns the booking" yes "$(has "$(body $B/api/bookings/$BID -H "$UH")" '"status":"cancelled"')"

echo "=== 7a. payments and holds (1 route) ==="
# Only meaningful with PAYMENTS set; with payments off a booking confirms on
# creation and there is nothing to pay.
PAYSTATE=$(body $B/api/bookings -H "$UH" | grep -o '"status":"pending"' | head -1)
if [ -z "$PAYSTATE" ]; then
  sk "POST /api/bookings/{id}/pay" "payments are off (PAYMENTS=off)"
  ck "  booking confirms immediately"  yes "$(has "$(body -X POST $B/api/bookings -H "$UH" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":1}")" '"status":"confirmed"')"
  ck "  pay endpoint answers 404"      404 "$(code -X POST $B/api/bookings/$BID/pay -H "$UH")"
else
  PB=$(body -X POST $B/api/bookings -H "$UH" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":1}")
  PBID=$(echo "$PB" | id)
  ck "  booking is held, not sold"     yes "$(has "$PB" '"status":"pending"')"
  ck "  a payment intent is issued"    yes "$(has "$PB" '"clientSecret"')"
  ck "  unpaid hold is refused entry"  409 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$(echo "$PB" | grep -oE '"ticketCode":"[a-f0-9]+"' | sed 's/"ticketCode":"//; s/"//')\"}")"
  ck "POST /api/bookings/{id}/pay"     200 "$(code -X POST $B/api/bookings/$PBID/pay -H "$UH")"
  ck "  booking is now confirmed"      yes "$(has "$(body $B/api/bookings/$PBID -H "$UH")" '"status":"confirmed"')"
  ck "  paying again is idempotent"    200 "$(code -X POST $B/api/bookings/$PBID/pay -H "$UH")"
  ck "  another user cannot pay it"    404 "$(code -X POST $B/api/bookings/$PBID/pay -H "Authorization: Bearer $A")"
fi

echo "=== 7b. ticket check-in (1 route) ==="
# A ticket only admits once it is paid for, so with payments on these bookings
# have to complete the payment step before they can be scanned.
paid_booking() {
  local r bid
  r=$(body -X POST $B/api/bookings -H "$UH" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":${1:-1}}")
  bid=$(echo "$r" | id)
  if echo "$r" | grep -q '"status":"pending"'; then
    code -X POST $B/api/bookings/$bid/pay -H "$UH" >/dev/null
    r=$(body $B/api/bookings/$bid -H "$UH")
  fi
  echo "$r"
}
CIB=$(paid_booking 1)
CODE=$(echo "$CIB" | grep -oE '"ticketCode":"[a-f0-9]+"' | sed 's/"ticketCode":"//; s/"//')
ck "  booking carries a ticket code"     32 "${#CODE}"
ck "  code is not the booking id"        no "$(has "$CODE" "$(echo "$CIB" | id)")"
ck "POST /api/admin/tickets/check-in"    200 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$CODE\"}")"
ck "  second scan is refused"            409 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$CODE\"}")"
ck "  says which refusal"                yes "$(has "$(body -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$CODE\"}")" 'already_used')"
ck "  checkedInAt is stamped"            yes "$(has "$(body $B/api/bookings -H "$UH")" '"checkedInAt"')"
ck "  unknown code -> 404"               404 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d '{"code":"deadbeefdeadbeefdeadbeefdeadbeef"}')"
ck "  missing code -> 400"               400 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d '{}')"
ck "  check-in needs an admin"           403 "$(code -X POST $B/api/admin/tickets/check-in -H "$UH" -H "$J" -d "{\"code\":\"$CODE\"}")"
CANB=$(paid_booking 1)
CANCODE=$(echo "$CANB" | grep -oE '"ticketCode":"[a-f0-9]+"' | sed 's/"ticketCode":"//; s/"//')
curl -s -m 30 -o /dev/null -X DELETE $B/api/bookings/$(echo "$CANB" | id) -H "$UH"
ck "  cancelled ticket refused"          409 "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$CANCODE\"}")"
# Two doors scanning the same ticket at once must not both admit.
RB=$(paid_booking 1)
RCODE=$(echo "$RB" | grep -oE '"ticketCode":"[a-f0-9]+"' | sed 's/"ticketCode":"//; s/"//')
# curl -w prints no trailing newline, so each result needs its own line or
# they concatenate into one unsearchable string.
RACE=$(mktemp); : > "$RACE"
for i in 1 2 3 4 5; do
  { printf '%s\n' "$(code -X POST $B/api/admin/tickets/check-in -H "$AH" -H "$J" -d "{\"code\":\"$RCODE\"}")" >> "$RACE"; } &
done
wait
ck "  concurrent scans admit exactly 1"  1 "$(grep -c '^200$' "$RACE")"
ck "  the rest are refused"              4 "$(grep -c '^409$' "$RACE")"
rm -f "$RACE"

echo "=== 8. admin bookings (4 routes) ==="
ck "GET /api/admin/bookings"     200 "$(code $B/api/admin/bookings -H "$AH")"
AB=$(body "$B/api/admin/bookings?status=confirmed" -H "$AH")
ABID=$(echo "$AB" | sed -E 's/.*"bookings":\[\{"id":"([^"]*)".*/\1/')
ck "GET /api/admin/bookings/{id}"            200 "$(code $B/api/admin/bookings/$ABID -H "$AH")"
ck "POST /api/admin/bookings/{id}/cancel"    200 "$(code -X POST $B/api/admin/bookings/$ABID/cancel -H "$AH")"
ck "DELETE /api/admin/bookings/{id}"         204 "$(code -X DELETE $B/api/admin/bookings/$ABID -H "$AH")"

echo "=== 9. analytics (1 route) ==="
AN=$(body $B/api/admin/analytics -H "$AH")
ck "GET /api/admin/analytics"    200 "$(code $B/api/admin/analytics -H "$AH")"
ck "  has both chart series"     yes "$(has "$AN" '"revenueByEvent".*"ticketsByCategory"')"
ck "  topEvents cap accepted"    200 "$(code "$B/api/admin/analytics?topEvents=9999" -H "$AH")"

echo "=== 10. password recovery (2 routes) ==="
ck "POST /api/forgot-password"          200 "$(code -X POST $B/api/forgot-password -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'"}')"
ck "  unknown email looks identical"    yes "$(has "$(body -X POST $B/api/forgot-password -H "$J" -H "$(ip)" -d '{"email":"nobody@x.com"}')" 'if that email has an account')"
# Capture the token LAST: requesting a new one invalidates every earlier token,
# so a token grabbed before the calls above would already be superseded.
FP=$(body -X POST $B/api/forgot-password -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'"}')
RT=$(echo "$FP" | grep -oE '"resetToken":"[a-f0-9]+"' | sed 's/"resetToken":"//; s/"//')
# A deployment with ENV=production does not return the token, and a black-box
# test has no other way to obtain it. Skip rather than report a false failure,
# and keep the password unchanged so the sections below still work.
CURRENT_PW="pw12345"
if [ -z "$RT" ]; then
  sk "POST /api/reset-password"         "no resetToken in response (ENV=production?)"
  sk "  new password works"             "depends on the reset above"
  sk "  token is single-use"            "depends on the reset above"
  sk "  reset revoked the old session"  "depends on the reset above"
else
  ck "POST /api/reset-password"         200 "$(code -X POST $B/api/reset-password -H "$J" -H "$(ip)" -d "{\"token\":\"$RT\",\"newPassword\":\"newpw123\"}")"
  # Update the tracked password before asserting on it: from here on, the old
  # one is dead and every later sign-in must use the new one.
  CURRENT_PW="newpw123"
  ck "  new password works"             200 "$(code -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"'"$CURRENT_PW"'"}')"
  ck "  token is single-use"            400 "$(code -X POST $B/api/reset-password -H "$J" -H "$(ip)" -d "{\"token\":\"$RT\",\"newPassword\":\"another99\"}")"
  ck "  reset revoked the old session"  401 "$(code $B/api/bookings -H "$UH")"
fi

echo "=== 11. logout (1 route) ==="
UT2=$(body -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"'"$CURRENT_PW"'"}' | sed -E 's/.*"token":"([^"]*)".*/\1/')
ck "POST /api/logout"            204 "$(code -X POST $B/api/logout -H "Authorization: Bearer $UT2")"
ck "  token dead afterwards"     401 "$(code $B/api/bookings -H "Authorization: Bearer $UT2")"

echo "=== 12. admin accounts (6 routes) ==="
ck "POST /api/admin/register"    201 "$(code -X POST $B/api/admin/register -H "$J" -H "$(ip)" -H "X-Admin-Key: $ADMIN_REGISTRATION_KEY" -d '{"name":"Ops","email":"'"$OPS_EMAIL"'","password":"pw12345"}')"
ck "GET /api/admin/users"        200 "$(code $B/api/admin/users -H "$AH")"
OPSID=$(body "$B/api/admin/users?keyword=ops" -H "$AH" | sed -E 's/.*"users":\[\{"id":"([^"]*)".*/\1/')
ck "GET /api/admin/users/{id}"   200 "$(code $B/api/admin/users/$OPSID -H "$AH")"
ck "  password hash never returned" no "$(has "$(body $B/api/admin/users -H "$AH")" '"password"')"
ck "PUT /api/admin/users/{id}"   200 "$(code -X PUT $B/api/admin/users/$OPSID -H "$AH" -H "$J" -d '{"name":"Ops Team"}')"
ck "PATCH /api/admin/users/{id}" 200 "$(code -X PATCH $B/api/admin/users/$OPSID -H "$AH" -H "$J" -d '{"role":"user"}')"
ck "DELETE /api/admin/users/{id}" 204 "$(code -X DELETE $B/api/admin/users/$OPSID -H "$AH")"

echo "=== 13. discovery writes — delete (4 routes) ==="
# Section 8 cancelled and deleted every confirmed booking, so make a fresh one:
# the guard only fires while a CONFIRMED booking exists.
UT4=$(body -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"'"$CURRENT_PW"'"}' | sed -E 's/.*"token":"([^"]*)".*/\1/')
curl -s -m 30 -o /dev/null -X POST $B/api/bookings -H "Authorization: Bearer $UT4" -H "$J" -d "{\"eventId\":\"$EID\",\"quantity\":1}"
ck "DELETE event blocked while booked"  409 "$(code -X DELETE $B/discovery/v2/events/$EID -H "$AH")"
# Every booking, not just confirmed ones: an unpaid hold also keeps seats and
# blocks the event delete below.
for b in $(body "$B/api/admin/bookings?size=100" -H "$AH" | grep -oE '"id":"[a-f0-9]+"' | sed 's/"id":"//; s/"//'); do
  curl -s -m 30 -o /dev/null -X DELETE $B/api/admin/bookings/$b -H "$AH"
done
ck "DELETE /discovery/v2/events/{id}"         204 "$(code -X DELETE $B/discovery/v2/events/$EID -H "$AH")"
ck "DELETE /discovery/v2/attractions/{id}"    204 "$(code -X DELETE $B/discovery/v2/attractions/$AID -H "$AH")"
ck "DELETE venue blocked while events exist"  409 "$(code -X DELETE $B/discovery/v2/venues/$VID -H "$AH")"
E2=$(body "$B/discovery/v2/events?keyword=Second" | sed -E 's/.*"events":\[\{"id":"([^"]*)".*/\1/')
curl -s -m 30 -o /dev/null -X DELETE $B/discovery/v2/events/$E2 -H "$AH"
ck "DELETE /discovery/v2/venues/{id}"         204 "$(code -X DELETE $B/discovery/v2/venues/$VID -H "$AH")"
ck "DELETE /discovery/v2/classifications/{id}" 204 "$(code -X DELETE $B/discovery/v2/classifications/$CID -H "$AH")"

echo "=== 14. authorisation and error paths ==="
ck "write without a token -> 401"        401 "$(code -X POST $B/discovery/v2/venues -H "$J" -d '{"name":"x"}')"
UT3=$(body -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"'"$CURRENT_PW"'"}' | sed -E 's/.*"token":"([^"]*)".*/\1/')
ck "admin route with user token -> 403"  403 "$(code $B/api/admin/users -H "Authorization: Bearer $UT3")"
ck "garbage token -> 401"                401 "$(code $B/api/bookings -H 'Authorization: Bearer nonsense')"
ck "unknown event -> 404"                404 "$(code $B/discovery/v2/events/nope)"
ck "unknown route -> 404"                404 "$(code $B/definitely-not-a-route)"
ck "bad login -> 401"                    401 "$(code -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"'"$USER_EMAIL"'","password":"wrong"}')"
ck "duplicate email -> 409"              409 "$(code -X POST $B/api/register -H "$J" -H "$(ip)" -d '{"name":"Sam","email":"'"$USER_EMAIL"'","password":"pw12345"}')"
ck "malformed JSON -> 400"               400 "$(code -X POST $B/api/register -H "$J" -H "$(ip)" -d '{not json')"
ck "admin register, wrong key -> 403"    403 "$(code -X POST $B/api/admin/register -H "$J" -H "$(ip)" -H 'X-Admin-Key: wrong' -d '{"name":"X","email":"'"$EVE_EMAIL"'-dup","password":"pw12345"}')"
ck "short reset password -> 400"         400 "$(code -X POST $B/api/reset-password -H "$J" -H "$(ip)" -d '{"token":"x","newPassword":"abc"}')"
ck "CORS preflight -> 204"               204 "$(code -X OPTIONS $B/api/login)"

echo "=== 15. rate limiting (shared caller, deliberate) ==="
LAST=""
for i in $(seq 1 11); do LAST=$(code -X POST $B/api/login -H "$J" -H 'X-Forwarded-For: 203.0.113.77' -d '{"email":"a@b.c","password":"x"}'); done
ck "11th login from one caller -> 429"   429 "$LAST"
ck "  a different caller is unaffected"  401 "$(code -X POST $B/api/login -H "$J" -H "$(ip)" -d '{"email":"a@b.c","password":"x"}')"


echo "=== 16. cleanup ==="
# Remove the accounts this run created, so the suite can be run repeatedly
# against the same database without accumulating users. Only confirmed
# bookings block a delete, and section 13 removed those.
for e in "$USER_EMAIL" "$EVE_EMAIL"; do
  uid=$(body "$B/api/admin/users?keyword=$e" -H "$AH" | id)
  if [ -n "$uid" ]; then
    ck "delete test account" 204 "$(code -X DELETE $B/api/admin/users/$uid -H "$AH")"
  else
    sk "delete test account" "not found (already gone)"
  fi
done
echo
echo "==================================================================="
printf '  passed: %s   failed: %s   skipped: %s\n' "$pass" "$fail" "$skip"
if [ "$fail" -gt 0 ]; then printf '  failures:\n'; for f in "${failed[@]}"; do printf '    - %s\n' "$f"; done; fi
echo "==================================================================="
[ "$fail" -eq 0 ]
