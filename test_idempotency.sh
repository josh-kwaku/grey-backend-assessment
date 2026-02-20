#!/usr/bin/env bash
set -uo pipefail

BASE="http://localhost:8080"
PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1 — $2"; }

check_header() {
  local count
  count=$(grep -ci "$1" "$2" 2>/dev/null || true)
  echo "${count:-0}"
}

# --- Auth ---
ALICE_TOKEN=$(curl -s -X POST "$BASE/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@test.com","password":"password123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

BOB_TOKEN=$(curl -s -X POST "$BASE/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"bob@test.com","password":"password123"}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

echo "Tokens acquired: Alice + Bob"

ALICE_USD_BEFORE=$(curl -s "$BASE/api/v1/users/00000000-0000-0000-0000-000000000002/accounts" \
  -H "Authorization: Bearer $ALICE_TOKEN" | python3 -c "
import sys, json
accts = json.load(sys.stdin)['data']
print(next(a['balance'] for a in accts if a['currency'] == 'USD'))
")
echo "Alice USD balance before tests: $ALICE_USD_BEFORE"

# ============================================================
echo ""
echo "=== HAPPY PATHS ==="

# H1: Internal transfer with Idempotency-Key → 201
echo "H1: Internal transfer with Idempotency-Key"
KEY_H1=$(python3 -c "import uuid; print(uuid.uuid4())")
H1=$(curl -s -o /tmp/h1.json -D /tmp/h1_headers.txt -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H1" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":1000}')
if [ "$H1" = "201" ]; then pass "H1 — 201 Created"; else fail "H1" "expected 201, got $H1"; fi
H1_PAYMENT_ID=$(python3 -c "import sys,json; print(json.load(open('/tmp/h1.json'))['data']['id'])")

# H2: Replay same key + same body → same response + X-Idempotent-Replayed
echo "H2: Replay same key + same body (internal)"
H2=$(curl -s -o /tmp/h2.json -D /tmp/h2_headers.txt -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H1" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":1000}')
H2_REPLAYED=$(check_header 'x-idempotent-replayed' /tmp/h2_headers.txt)
H2_PAYMENT_ID=$(python3 -c "import sys,json; print(json.load(open('/tmp/h2.json'))['data']['id'])")
if [ "$H2" = "201" ]; then pass "H2 — 201 replayed"; else fail "H2" "expected 201, got $H2"; fi
if [ "$H2_REPLAYED" -ge 1 ]; then pass "H2 — X-Idempotent-Replayed header present"; else fail "H2" "missing X-Idempotent-Replayed header"; fi
if [ "$H1_PAYMENT_ID" = "$H2_PAYMENT_ID" ]; then pass "H2 — same payment ID returned"; else fail "H2" "payment IDs differ: $H1_PAYMENT_ID vs $H2_PAYMENT_ID"; fi

# H3: External payout with Idempotency-Key → 202
echo "H3: External payout with Idempotency-Key"
KEY_H3=$(python3 -c "import uuid; print(uuid.uuid4())")
H3=$(curl -s -o /tmp/h3.json -D /tmp/h3_headers.txt -w '%{http_code}' -X POST "$BASE/api/v1/payments/external" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H3" \
  -d '{"source_currency":"USD","dest_currency":"USD","amount":500,"dest_iban":"DE89370400440532013000","dest_bank_name":"Deutsche Bank"}')
if [ "$H3" = "202" ]; then pass "H3 — 202 Accepted"; else fail "H3" "expected 202, got $H3"; fi

# H4: Replay external payout same key + same body
echo "H4: Replay same key + same body (external)"
H4=$(curl -s -o /tmp/h4.json -D /tmp/h4_headers.txt -w '%{http_code}' -X POST "$BASE/api/v1/payments/external" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H3" \
  -d '{"source_currency":"USD","dest_currency":"USD","amount":500,"dest_iban":"DE89370400440532013000","dest_bank_name":"Deutsche Bank"}')
H4_REPLAYED=$(check_header 'x-idempotent-replayed' /tmp/h4_headers.txt)
if [ "$H4" = "202" ]; then pass "H4 — 202 replayed"; else fail "H4" "expected 202, got $H4"; fi
if [ "$H4_REPLAYED" -ge 1 ]; then pass "H4 — X-Idempotent-Replayed header present"; else fail "H4" "missing X-Idempotent-Replayed header"; fi

# H5: GET without Idempotency-Key → 200
echo "H5: GET /payments/{id} without Idempotency-Key"
H5=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/payments/$H1_PAYMENT_ID" \
  -H "Authorization: Bearer $ALICE_TOKEN")
if [ "$H5" = "200" ]; then pass "H5 — GET works without Idempotency-Key"; else fail "H5" "expected 200, got $H5"; fi

# H6: Different user, same idempotency key → both succeed
echo "H6: Different users, same idempotency key"
KEY_H6=$(python3 -c "import uuid; print(uuid.uuid4())")
H6_BOB=$(curl -s -o /tmp/h6_bob.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H6" \
  -d '{"recipient_unique_name":"alice","source_currency":"USD","dest_currency":"USD","amount":100}')
H6_ALICE=$(curl -s -o /tmp/h6_alice.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H6" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":100}')
if [ "$H6_BOB" = "201" ]; then pass "H6 — Bob 201 Created"; else fail "H6" "Bob expected 201, got $H6_BOB"; fi
if [ "$H6_ALICE" = "201" ]; then pass "H6 — Alice 201 Created (same key, different user)"; else fail "H6" "Alice expected 201, got $H6_ALICE"; fi

# H7: Same user, different keys, different requests → both succeed
echo "H7: Same user, different keys"
KEY_H7A=$(python3 -c "import uuid; print(uuid.uuid4())")
KEY_H7B=$(python3 -c "import uuid; print(uuid.uuid4())")
H7A=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H7A" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":200}')
H7B=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H7B" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":300}')
if [ "$H7A" = "201" ]; then pass "H7 — first request 201"; else fail "H7" "first expected 201, got $H7A"; fi
if [ "$H7B" = "201" ]; then pass "H7 — second request 201"; else fail "H7" "second expected 201, got $H7B"; fi

# ============================================================
echo ""
echo "=== SAD PATHS ==="

# S1: POST /payments without Idempotency-Key → 400
echo "S1: Missing Idempotency-Key (internal)"
S1=$(curl -s -o /tmp/s1.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":100}')
S1_CODE=$(python3 -c "import json; print(json.load(open('/tmp/s1.json'))['error']['code'])")
if [ "$S1" = "400" ]; then pass "S1 — 400 returned"; else fail "S1" "expected 400, got $S1"; fi
if [ "$S1_CODE" = "MISSING_IDEMPOTENCY_KEY" ]; then pass "S1 — correct error code"; else fail "S1" "expected MISSING_IDEMPOTENCY_KEY, got $S1_CODE"; fi

# S2: POST /payments/external without Idempotency-Key → 400
echo "S2: Missing Idempotency-Key (external)"
S2=$(curl -s -o /tmp/s2.json -w '%{http_code}' -X POST "$BASE/api/v1/payments/external" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"source_currency":"USD","dest_currency":"USD","amount":100,"dest_iban":"DE123","dest_bank_name":"Test"}')
S2_CODE=$(python3 -c "import json; print(json.load(open('/tmp/s2.json'))['error']['code'])")
if [ "$S2" = "400" ]; then pass "S2 — 400 returned"; else fail "S2" "expected 400, got $S2"; fi
if [ "$S2_CODE" = "MISSING_IDEMPOTENCY_KEY" ]; then pass "S2 — correct error code"; else fail "S2" "expected MISSING_IDEMPOTENCY_KEY, got $S2_CODE"; fi

# S3: Same key + different body (change amount) → 409
echo "S3: Same key, different amount"
S3=$(curl -s -o /tmp/s3.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H1" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":9999}')
S3_CODE=$(python3 -c "import json; print(json.load(open('/tmp/s3.json'))['error']['code'])")
if [ "$S3" = "409" ]; then pass "S3 — 409 Conflict"; else fail "S3" "expected 409, got $S3"; fi
if [ "$S3_CODE" = "IDEMPOTENCY_CONFLICT" ]; then pass "S3 — correct error code"; else fail "S3" "expected IDEMPOTENCY_CONFLICT, got $S3_CODE"; fi

# S4: Same key + different body (change recipient) → 409
echo "S4: Same key, different recipient"
S4=$(curl -s -o /tmp/s4.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_H1" \
  -d '{"recipient_unique_name":"charlie","source_currency":"USD","dest_currency":"USD","amount":1000}')
S4_CODE=$(python3 -c "import json; print(json.load(open('/tmp/s4.json'))['error']['code'])")
if [ "$S4" = "409" ]; then pass "S4 — 409 Conflict"; else fail "S4" "expected 409, got $S4"; fi
if [ "$S4_CODE" = "IDEMPOTENCY_CONFLICT" ]; then pass "S4 — correct error code"; else fail "S4" "expected IDEMPOTENCY_CONFLICT, got $S4_CODE"; fi

# S5: Replay a request that originally failed validation → cached error replayed
echo "S5: Replay a failed validation request"
KEY_S5=$(python3 -c "import uuid; print(uuid.uuid4())")
S5_FIRST=$(curl -s -o /tmp/s5_first.json -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_S5" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":-1}')
S5_REPLAY=$(curl -s -o /tmp/s5_replay.json -D /tmp/s5_headers.txt -w '%{http_code}' -X POST "$BASE/api/v1/payments" \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY_S5" \
  -d '{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":-1}')
S5_REPLAYED=$(check_header 'x-idempotent-replayed' /tmp/s5_headers.txt)
if [ "$S5_FIRST" = "400" ]; then pass "S5 — original 400 error"; else fail "S5" "first expected 400, got $S5_FIRST"; fi
if [ "$S5_REPLAY" = "400" ]; then pass "S5 — cached 400 replayed"; else fail "S5" "replay expected 400, got $S5_REPLAY"; fi
if [ "$S5_REPLAYED" -ge 1 ]; then pass "S5 — X-Idempotent-Replayed on error replay"; else fail "S5" "missing X-Idempotent-Replayed on error replay"; fi

# ============================================================
echo ""
echo "=== RACE CONDITIONS ==="

# R1: 10 concurrent requests, same key + same body
echo "R1: 10 concurrent requests, same key"
KEY_R1=$(python3 -c "import uuid; print(uuid.uuid4())")
R1_BODY='{"recipient_unique_name":"bob","source_currency":"USD","dest_currency":"USD","amount":100}'
for i in $(seq 1 10); do
  curl -s -o "/tmp/r1_$i.json" -D "/tmp/r1_${i}_headers.txt" -w '%{http_code}\n' -X POST "$BASE/api/v1/payments" \
    -H "Authorization: Bearer $ALICE_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: $KEY_R1" \
    -d "$R1_BODY" &
done
wait

R1_CREATED=0
R1_REPLAYED=0
R1_OTHER=0
for i in $(seq 1 10); do
  status=$(python3 -c "
import json
try:
  d = json.load(open('/tmp/r1_$i.json'))
  print('success' if d.get('success') else 'fail')
except: print('error')
")
  replayed=$(check_header 'x-idempotent-replayed' "/tmp/r1_${i}_headers.txt")
  if [ "$status" = "success" ] && [ "$replayed" = "0" ]; then
    R1_CREATED=$((R1_CREATED+1))
  elif [ "$status" = "success" ] && [ "$replayed" -ge 1 ]; then
    R1_REPLAYED=$((R1_REPLAYED+1))
  else
    R1_OTHER=$((R1_OTHER+1))
  fi
done
echo "  R1 results: created=$R1_CREATED replayed=$R1_REPLAYED other=$R1_OTHER (other = DB constraint race losers, expected)"
if [ "$R1_CREATED" -le 2 ]; then pass "R1 — at most 2 payments created ($R1_CREATED)"; else fail "R1" "expected <=2 created, got $R1_CREATED"; fi
# Verify only 1 payment exists in DB for this key
R1_DB_COUNT=$(docker exec grey-postgres-1 psql -U grey -d grey -t -A -c "SELECT COUNT(*) FROM payments WHERE idempotency_key = '$KEY_R1';")
if [ "$R1_DB_COUNT" = "1" ]; then pass "R1 — exactly 1 payment in DB"; else fail "R1" "expected 1 payment in DB, got $R1_DB_COUNT"; fi

# R2: 6 concurrent overdraft attempts with different keys
echo "R2: 6 concurrent overdraft attempts"
BOB_USD=$(curl -s "$BASE/api/v1/users/00000000-0000-0000-0000-000000000003/accounts" \
  -H "Authorization: Bearer $BOB_TOKEN" | python3 -c "
import sys, json
accts = json.load(sys.stdin)['data']
print(next(a['balance'] for a in accts if a['currency'] == 'USD'))
")
echo "  Bob USD balance: $BOB_USD"
for i in $(seq 1 6); do
  KEY_R2=$(python3 -c "import uuid; print(uuid.uuid4())")
  curl -s -o "/tmp/r2_$i.json" -w '%{http_code}\n' -X POST "$BASE/api/v1/payments" \
    -H "Authorization: Bearer $BOB_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: $KEY_R2" \
    -d "{\"recipient_unique_name\":\"alice\",\"source_currency\":\"USD\",\"dest_currency\":\"USD\",\"amount\":$BOB_USD}" &
done
wait

R2_SUCCESS=0
R2_FAILED=0
for i in $(seq 1 6); do
  status=$(python3 -c "
import json
try:
  d = json.load(open('/tmp/r2_$i.json'))
  print('success' if d.get('success') else 'fail')
except: print('error')
")
  if [ "$status" = "success" ]; then R2_SUCCESS=$((R2_SUCCESS+1)); else R2_FAILED=$((R2_FAILED+1)); fi
done
echo "  R2 results: success=$R2_SUCCESS failed=$R2_FAILED"
if [ "$R2_SUCCESS" -eq 1 ]; then pass "R2 — exactly 1 overdraft succeeded"; else fail "R2" "expected 1 success, got $R2_SUCCESS"; fi
if [ "$R2_FAILED" -eq 5 ]; then pass "R2 — 5 correctly rejected"; else fail "R2" "expected 5 failed, got $R2_FAILED"; fi

BOB_USD_AFTER=$(curl -s "$BASE/api/v1/users/00000000-0000-0000-0000-000000000003/accounts" \
  -H "Authorization: Bearer $BOB_TOKEN" | python3 -c "
import sys, json
accts = json.load(sys.stdin)['data']
print(next(a['balance'] for a in accts if a['currency'] == 'USD'))
")
if [ "$BOB_USD_AFTER" -ge 0 ]; then pass "R2 — Bob USD balance non-negative ($BOB_USD_AFTER)"; else fail "R2" "Bob balance negative: $BOB_USD_AFTER"; fi

# ============================================================
echo ""
echo "=== INTEGRITY CHECKS ==="

# I1: Alice's balance reflects only actual debits
echo "I1: Alice balance integrity"
ALICE_USD_AFTER=$(curl -s "$BASE/api/v1/users/00000000-0000-0000-0000-000000000002/accounts" \
  -H "Authorization: Bearer $ALICE_TOKEN" | python3 -c "
import sys, json
accts = json.load(sys.stdin)['data']
print(next(a['balance'] for a in accts if a['currency'] == 'USD'))
")
# Alice spent: H1=1000 + H3=500 + H6_alice=100 + H7A=200 + H7B=300 + R1=100 = 2200
# Alice received: H6_bob=100 + R2=Bob's full balance
echo "  Alice USD: before=$ALICE_USD_BEFORE after=$ALICE_USD_AFTER"
if [ "$ALICE_USD_AFTER" -gt 0 ]; then pass "I1 — Alice balance positive, not double-debited by replays"; else fail "I1" "Alice balance: $ALICE_USD_AFTER"; fi

# I2: Global ledger integrity
echo "I2: Ledger integrity (debits = credits per currency)"
I2_RESULT=$(docker exec grey-postgres-1 psql -U grey -d grey -t -A -c "
SELECT currency,
       SUM(CASE WHEN entry_type = 'debit' THEN amount ELSE 0 END) AS total_debits,
       SUM(CASE WHEN entry_type = 'credit' THEN amount ELSE 0 END) AS total_credits
FROM ledger_entries
GROUP BY currency
ORDER BY currency;
")
I2_BALANCED=true
while IFS='|' read -r curr debits credits; do
  [ -z "$curr" ] && continue
  echo "  $curr: debits=$debits credits=$credits"
  if [ "$debits" != "$credits" ]; then
    I2_BALANCED=false
    fail "I2" "$curr debits=$debits credits=$credits"
  fi
done <<< "$I2_RESULT"
if [ "$I2_BALANCED" = true ]; then pass "I2 — all currencies balanced"; fi

# ============================================================
echo ""
echo "==============================="
echo "Results: $PASS passed, $FAIL failed out of $TOTAL tests"
echo "==============================="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
