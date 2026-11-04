$BASE = "http://localhost:8080"
$TMP = "C:\Temp\sl_idem_test"
New-Item -ItemType Directory -Force -Path $TMP | Out-Null

$ts = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$email = "idem$ts@example.com"

function Write-NoBOM($path, $str) {
    [System.IO.File]::WriteAllBytes($path, [System.Text.Encoding]::UTF8.GetBytes($str))
}

Write-NoBOM "$TMP\reg.json" "{`"email`":`"$email`",`"password`":`"SuperSecret123!`"}"
curl.exe -s -X POST "$BASE/v1/auth/register" -H "Content-Type: application/json" --data-binary "@$TMP\reg.json" | Out-Null

$loginRaw = curl.exe -s -X POST "$BASE/v1/auth/login" -H "Content-Type: application/json" --data-binary "@$TMP\reg.json"
$TOKEN = ($loginRaw | ConvertFrom-Json).access_token

$idemKey = "unique-key-$ts"
Write-NoBOM "$TMP\link.json" '{"url":"https://github.com"}'

$r1 = curl.exe -s -X POST "$BASE/v1/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: $idemKey" --data-binary "@$TMP\link.json"
$r2 = curl.exe -s -X POST "$BASE/v1/links" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" -H "Idempotency-Key: $idemKey" --data-binary "@$TMP\link.json"

$c1 = ($r1 | ConvertFrom-Json).code
$c2 = ($r2 | ConvertFrom-Json).code
Write-Host "Call 1 code: $c1"
Write-Host "Call 2 code: $c2"
if ($c1 -and $c1 -eq $c2) { Write-Host "IDEMPOTENCY OK" -ForegroundColor Green }
else { Write-Host "IDEMPOTENCY MISMATCH ($c1 vs $c2)" -ForegroundColor Red }

