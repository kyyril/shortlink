$BASE = "http://localhost:8080"
$TMP = "$env:TEMP\sl_test"
New-Item -ItemType Directory -Force -Path $TMP | Out-Null
# Use a timestamp-based suffix so each run is isolated from previous data.
$RUN = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()

function Write-JSON($path, $obj) {
    # Write without BOM so curl can read the file cleanly
    $bytes = [System.Text.Encoding]::UTF8.GetBytes(($obj | ConvertTo-Json -Compress))
    [System.IO.File]::WriteAllBytes($path, $bytes)
}

$PASS = 0; $FAIL = 0

function Check($label, $resp, $expectCode) {
    $obj = $resp | ConvertFrom-Json -ErrorAction SilentlyContinue
    $isError = $resp -match '"type".*"about:blank"'
    $ok = if ($expectCode -eq "2xx") { -not $isError }
          elseif ($expectCode -eq "404") { $obj.status -eq 404 }
          elseif ($expectCode -eq "401") { $obj.status -eq 401 }
          elseif ($expectCode -eq "409") { $obj.status -eq 409 }
          else { $false }
    if ($ok) { Write-Host "[OK]   $label" -ForegroundColor Green; $script:PASS++ }
    else      { Write-Host "[FAIL] $label" -ForegroundColor Red; Write-Host "       $resp"; $script:FAIL++ }
}

Write-Host "`n=== Shortlink API Test ===" -ForegroundColor Yellow

# 1. Health probes
Check "GET /healthz"  (curl.exe -s "$BASE/healthz") "2xx"
Check "GET /readyz"   (curl.exe -s "$BASE/readyz")  "2xx"

# 2. Register new user
Write-JSON "$TMP\reg.json" @{email="apitest-$RUN@example.com"; password="SuperSecret123!"}
Check "POST /v1/auth/register" (curl.exe -s -X POST "$BASE/v1/auth/register" -H "Content-Type: application/json" --data-binary "@$TMP\reg.json") "2xx"

# 3. Duplicate register -> 409
Check "POST /v1/auth/register duplicate -> 409" (curl.exe -s -X POST "$BASE/v1/auth/register" -H "Content-Type: application/json" --data-binary "@$TMP\reg.json") "409"

# 4. Wrong password -> 401
Write-JSON "$TMP\wrong.json" @{email="apitest3@example.com"; password="wrongpassword"}
Check "POST /v1/auth/login wrong pass -> 401" (curl.exe -s -X POST "$BASE/v1/auth/login" -H "Content-Type: application/json" --data-binary "@$TMP\wrong.json") "401"

# 5. Correct login
Write-JSON "$TMP\login.json" @{email="apitest-$RUN@example.com"; password="SuperSecret123!"}
$loginRaw = curl.exe -s -X POST "$BASE/v1/auth/login" -H "Content-Type: application/json" --data-binary "@$TMP\login.json"
Check "POST /v1/auth/login" $loginRaw "2xx"
$TOKEN = ($loginRaw | ConvertFrom-Json).access_token

# 6. Create link with custom code
Write-JSON "$TMP\link1.json" @{url="https://go.dev/doc/effective_go"; custom_code="godocs-$RUN"}
$r = curl.exe -s -X POST "$BASE/v1/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" --data-binary "@$TMP\link1.json"
Check "POST /v1/links (custom_code=godocs3)" $r "2xx"

# 7. Auto-code link with idempotency
Write-JSON "$TMP\link2.json" @{url="https://github.com/kyyril/shortlink"}
$r1 = curl.exe -s -X POST "$BASE/v1/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: idem-xyz-$RUN" --data-binary "@$TMP\link2.json"
$r2 = curl.exe -s -X POST "$BASE/v1/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: idem-xyz-$RUN" --data-binary "@$TMP\link2.json"
$autoCode = ($r1 | ConvertFrom-Json).code
$sameCode = ($r2 | ConvertFrom-Json).code
Check "POST /v1/links (auto code)" $r1 "2xx"
if ($autoCode -eq $sameCode -and $autoCode) { Write-Host "[OK]   Idempotency-Key returns same code: $autoCode" -ForegroundColor Green; $PASS++ }
else { Write-Host "[FAIL] Idempotency mismatch: $autoCode vs $sameCode" -ForegroundColor Red; $FAIL++ }

# 8. List links
Check "GET /v1/links" (curl.exe -s "$BASE/v1/links" -H "Authorization: Bearer $TOKEN") "2xx"

# 9. Get single link
Check "GET /v1/links/godocs-$RUN" (curl.exe -s "$BASE/v1/links/godocs-$RUN" -H "Authorization: Bearer $TOKEN") "2xx"

# 10. Redirect
$httpCode = curl.exe -s -o NUL -w "%{http_code}" "$BASE/godocs-$RUN"
if ($httpCode -eq "302") { Write-Host "[OK]   GET /godocs-$RUN -> 302 redirect" -ForegroundColor Green; $PASS++ }
else { Write-Host "[FAIL] GET /godocs-$RUN returned $httpCode (expected 302)" -ForegroundColor Red; $FAIL++ }

# 11. Stats (wait for async click flush)
Start-Sleep -Milliseconds 700
$statsRaw = curl.exe -s "$BASE/v1/links/godocs-$RUN/stats" -H "Authorization: Bearer $TOKEN"
Check "GET /v1/links/godocs-$RUN/stats" $statsRaw "2xx"
Write-Host "       total_clicks = $(($statsRaw | ConvertFrom-Json).total_clicks)"

# 12. Delete auto-code link
$delCode = curl.exe -s -o NUL -w "%{http_code}" -X DELETE "$BASE/v1/links/$autoCode" -H "Authorization: Bearer $TOKEN"
if ($delCode -eq "204") { Write-Host "[OK]   DELETE /v1/links/$autoCode -> 204" -ForegroundColor Green; $PASS++ }
else { Write-Host "[FAIL] DELETE returned $delCode (expected 204)" -ForegroundColor Red; $FAIL++ }

# 13. Get deleted link -> 404
Check "GET /v1/links/$autoCode (deleted -> 404)" (curl.exe -s "$BASE/v1/links/$autoCode" -H "Authorization: Bearer $TOKEN") "404"

# 14. No auth -> 401
Check "GET /v1/links (no token -> 401)" (curl.exe -s "$BASE/v1/links") "401"

# 15. Prometheus metrics
$m = curl.exe -s "$BASE/metrics"
if ($m -match "shortlink_http_requests_total") { Write-Host "[OK]   GET /metrics (Prometheus)" -ForegroundColor Green; $PASS++ }
else { Write-Host "[FAIL] /metrics missing metric" -ForegroundColor Red; $FAIL++ }

Write-Host "`n=== Result: $PASS passed, $FAIL failed ===" -ForegroundColor $(if ($FAIL -eq 0) {"Green"} else {"Red"})
