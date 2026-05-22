# Gateway smoke test — run from repo root
$ErrorActionPreference = "Stop"
$Base = "http://127.0.0.1:8080"
$failed = 0

function Pass($msg) { Write-Host "[OK] $msg" -ForegroundColor Green }
function Fail($msg) { Write-Host "[FAIL] $msg" -ForegroundColor Red; $script:failed++ }

Write-Host "`n=== Cursor Gateway Smoke Test ===" -ForegroundColor Cyan

# 1. Health & status
try {
    $h = Invoke-RestMethod "$Base/healthz" -TimeoutSec 5
    if ($h.status -eq "ok") { Pass "healthz" } else { Fail "healthz status=$($h.status)" }
} catch { Fail "healthz: $_" }

try {
    $st = Invoke-RestMethod "$Base/status" -TimeoutSec 5
    Pass "status uptime=$($st.uptime) daemon_profile=$($st.cursor.agent_profile)"
} catch { Fail "status: $_" }

# 2. Config
try {
    $cfg = Invoke-RestMethod "$Base/api/config" -TimeoutSec 5
    $c = $cfg.config.cursor
    if ($c.use_daemon) { Pass "config use_daemon=true" } else { Fail "use_daemon not true" }
    if ($c.default_model) { Pass "config model=$($c.default_model)" } else { Fail "no default_model" }
} catch { Fail "api/config: $_" }

# 3. Web UI page
try {
    $html = (Invoke-WebRequest "$Base/" -UseBasicParsing -TimeoutSec 5).Content
    if ($html -match 'data-tab="compare"' -and $html -match 'data-tab="config"') {
        Pass "Web UI v2 (Compare + Config tabs)"
    } else { Fail "Web UI missing new tabs" }
} catch { Fail "index: $_" }

# 4. Models list
try {
    $models = Invoke-RestMethod "$Base/v1/models" -TimeoutSec 15
    $n = @($models.data).Count
    if ($n -gt 0) { Pass "GET /v1/models ($n models)" } else { Fail "models list empty" }
} catch { Fail "/v1/models: $_" }

# 5. Chat completion (non-stream) — short prompt
$body = @{
    model = "composer-2.5-fast"
    messages = @(@{ role = "user"; content = "Reply with exactly one word: pong" })
    stream = $false
    max_tokens = 32
} | ConvertTo-Json -Depth 5

Write-Host "`n--- Live LLM test (may take 30-120s) ---" -ForegroundColor Yellow
$sw = [System.Diagnostics.Stopwatch]::StartNew()
try {
    $chat = Invoke-RestMethod "$Base/v1/chat/completions" -Method POST -Body $body -ContentType "application/json" -TimeoutSec 180
    $sw.Stop()
    $text = $chat.choices[0].message.content
    $ms = $sw.ElapsedMilliseconds
    if ($text) {
        Pass "chat non-stream ${ms}ms len=$($text.Length) preview=$($text.Substring(0, [Math]::Min(80, $text.Length)))"
    } else { Fail "chat empty content" }
} catch { Fail "chat non-stream: $_"; $sw.Stop() }

# 6. Second chat (daemon warm) — compare latency
$sw2 = [System.Diagnostics.Stopwatch]::StartNew()
try {
    $chat2 = Invoke-RestMethod "$Base/v1/chat/completions" -Method POST -Body $body -ContentType "application/json" -TimeoutSec 180
    $sw2.Stop()
    $ms2 = $sw2.ElapsedMilliseconds
    Pass "chat #2 (warm) ${ms2}ms"
    if ($sw.ElapsedMilliseconds -gt 0) {
        $ratio = [math]::Round($ms2 / $sw.ElapsedMilliseconds, 2)
        Write-Host "      warm/cold ratio: $ratio (lower is better for daemon)" -ForegroundColor DarkGray
    }
} catch { Fail "chat #2: $_" }

# 7. Streaming chat — TTFB
Write-Host "`n--- Stream test ---" -ForegroundColor Yellow
$bodyStream = @{
    model = "composer-2.5-fast"
    messages = @(@{ role = "user"; content = "Say hi in 3 words" })
    stream = $true
} | ConvertTo-Json -Depth 5

$sw3 = [System.Diagnostics.Stopwatch]::StartNew()
$ttfb = $null
$chunks = 0
try {
    $req = [System.Net.HttpWebRequest]::Create("$Base/v1/chat/completions")
    $req.Method = "POST"
    $req.ContentType = "application/json"
    $req.Timeout = 180000
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($bodyStream)
    $req.ContentLength = $bytes.Length
    $stream = $req.GetRequestStream()
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Close()
    $resp = $req.GetResponse()
    $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
    while (-not $reader.EndOfStream) {
        $line = $reader.ReadLine()
        if ($line -and $line.StartsWith("data: ") -and $line -ne "data: [DONE]") {
            if (-not $ttfb) { $ttfb = $sw3.ElapsedMilliseconds }
            $chunks++
        }
    }
    $reader.Close()
    $sw3.Stop()
    if ($ttfb) { Pass "stream TTFB=${ttfb}ms total=${sw3.ElapsedMilliseconds}ms chunks=$chunks" }
    else { Fail "stream no SSE chunks" }
} catch { Fail "stream: $_" }

# 8. Traces API
try {
    $traces = Invoke-RestMethod "$Base/api/traces" -TimeoutSec 5
    $n = @($traces).Count
    if ($n -ge 1) { Pass "api/traces ($n traces)" } else { Fail "no traces recorded" }
} catch { Fail "api/traces: $_" }

# 9. Stats
try {
    $stats = Invoke-RestMethod "$Base/api/stats" -TimeoutSec 5
    Pass "api/stats completed=$($stats.completed) active=$($stats.active)"
} catch { Fail "api/stats: $_" }

Write-Host "`n=== Done: $failed failure(s) ===" -ForegroundColor $(if ($failed -eq 0) { "Green" } else { "Red" })
exit $failed
