# EdgeCDN one-shot start (Windows / PowerShell).
#
#   .\start.ps1            build everything, start core (serves web/dist at /)
#   .\start.ps1 -Edge      also start the local edge node (uses edge/config.json)
#   .\start.ps1 -NoBuild   skip go/pnpm builds (reuse existing binaries)
#   .\start.ps1 -Port 9080 run core on a different port
#
# Linux/macOS: run the same steps manually (see README.md "部署").

param(
    [switch]$Edge,
    [switch]$NoBuild,
    [string]$Port = "8080"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$tools = Join-Path $root ".tools"
$bin = Join-Path $tools "bin"
New-Item -ItemType Directory -Force $bin | Out-Null

# --- locate Go (prefer .tools/go, fall back to PATH) ---
$goExe = $null
$candidate = Join-Path $tools "go\bin\go.exe"
if (Test-Path $candidate) { $goExe = $candidate }
else { $cmd = Get-Command go -ErrorAction SilentlyContinue; if ($cmd) { $goExe = $cmd.Source } }
if (-not $goExe) { Write-Error "Go not found: put it in $tools\go or add go to PATH"; exit 1 }
$env:PATH = (Split-Path $goExe) + ";" + $env:PATH
$env:GOCACHE = Join-Path $tools "go-cache"
$env:GOPATH = Join-Path $tools "gopath"
$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOSUMDB = "off"

# --- stop stale instances ---
Get-Process core, edge -ErrorAction SilentlyContinue | ForEach-Object {
    Write-Host "stopping stale $($_.Name) ($($_.Id))"
    Stop-Process -Id $_.Id -Force
}
Start-Sleep -Milliseconds 500

# --- build ---
if (-not $NoBuild) {
    # go/pnpm write progress to stderr; don't let EAP=Stop treat that as fatal.
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
    Set-Location (Join-Path $root "core")
    Write-Host "==> building core"
    & $goExe build -o (Join-Path $bin "core.exe") ./cmd/core
    if ($LASTEXITCODE -ne 0) { exit 1 }
    Set-Location (Join-Path $root "edge")
    Write-Host "==> building edge"
    & $goExe build -o (Join-Path $bin "edge.exe") ./cmd/edge
    if ($LASTEXITCODE -ne 0) { exit 1 }
    Set-Location $root
    if (-not (Test-Path (Join-Path $root "web\dist\index.html"))) {
        Write-Host "==> building web frontend"
        Set-Location (Join-Path $root "web")
        pnpm install --registry=https://registry.npmmirror.com
        if ($LASTEXITCODE -ne 0) { exit 1 }
        pnpm build
        if ($LASTEXITCODE -ne 0) { exit 1 }
        Set-Location $root
    }
    } finally {
        $ErrorActionPreference = $prevEap
    }
} else {
    Write-Host "==> skipping build (-NoBuild)"
}

# --- start core ---
$coreArgs = @("-config", (Join-Path $root "core\config.json"), "-log", (Join-Path $tools "core.log"))
if (Test-Path (Join-Path $root "web\dist\index.html")) {
    $coreArgs += @("-static", (Join-Path $root "web\dist"))
}
Write-Host "==> starting core on :$Port (log: .tools\core.log)"
# NOTE: launch with -WindowStyle Hidden (own console) and NO -RedirectStandardOutput/-Error:
# managed std redirection keeps the daemon attached to this script's stdout pipe and
# the launcher blocks on exit. The daemon writes its own log file via -log.
Start-Process -FilePath (Join-Path $bin "core.exe") -ArgumentList $coreArgs -WorkingDirectory $root -WindowStyle Hidden

# wait for health
$ok = $false
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Milliseconds 500
    try {
        $r = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/api/v1/health" -TimeoutSec 2
        if ($r.code -eq 0) { $ok = $true; break }
    } catch { }
}
if (-not $ok) { Write-Error "core did not become healthy on :$Port (see .tools\core.log)"; exit 1 }
Write-Host "    core healthy. console: http://127.0.0.1:$Port  (admin / admin123)"

# --- start edge (optional) ---
if ($Edge) {
    $edgeCfg = Join-Path $root "edge\config.json"
    if (-not (Test-Path $edgeCfg)) {
        Write-Warning "edge/config.json not found - start core first, create a node in the console, write its token into edge/config.json, then re-run with -Edge"
        exit 0
    }
    Write-Host "==> starting edge (log: .tools\edge.log)"
    Start-Process -FilePath (Join-Path $bin "edge.exe") -ArgumentList @("-config", $edgeCfg, "-log", (Join-Path $tools "edge.log")) -WorkingDirectory $root -WindowStyle Hidden
    Start-Sleep -Seconds 2
    if (Get-Process edge -ErrorAction SilentlyContinue) {
        Write-Host "    edge running"
    } else {
        Write-Warning "edge exited - check .tools\edge.log"
    }

    # --- start dummy test origins (backends for the demo sites) ---
    function Test-PortOpen([int]$p) {
        return [bool](Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue)
    }
    if (-not (Test-PortOpen 9000)) {
        $nodeExe = (Get-Command node -ErrorAction SilentlyContinue).Source
        if ($nodeExe -and (Test-Path (Join-Path $tools "origin.js"))) {
            Start-Process -FilePath $nodeExe -ArgumentList (Join-Path $tools "origin.js") -WorkingDirectory $tools -WindowStyle Hidden
            Write-Host "    dummy origin (node) on 127.0.0.1:9000"
        }
    }
    if (-not (Test-PortOpen 19099)) {
        $pwshExe = (Get-Command pwsh -ErrorAction SilentlyContinue).Source
        if (-not $pwshExe) { $pwshExe = Join-Path $PSHOME "powershell.exe" }
        Start-Process -FilePath $pwshExe -ArgumentList @("-NoProfile", "-File", (Join-Path $tools "origin-test.ps1")) -WindowStyle Hidden
        Write-Host "    dummy origin (http) on 127.0.0.1:19099"
    }
    if (-not (Test-PortOpen 19101)) {
        $pwshExe = (Get-Command pwsh -ErrorAction SilentlyContinue).Source
        if (-not $pwshExe) { $pwshExe = Join-Path $PSHOME "powershell.exe" }
        Start-Process -FilePath $pwshExe -ArgumentList @("-NoProfile", "-File", (Join-Path $tools "tcp-echo.ps1")) -WindowStyle Hidden
        Write-Host "    tcp echo origin on 127.0.0.1:19101"
    }
}

Write-Host ""
Write-Host "EdgeCDN is up."
Write-Host "  admin console : http://127.0.0.1:$Port/admin   (admin / admin123)"
Write-Host "  tenant portal : http://127.0.0.1:$Port/portal  (created by admin)"
if ($Edge) { Write-Host "  edge proxy    : per edge/config.json listen_addr" }
Write-Host "Stop with: .\stop.ps1"
