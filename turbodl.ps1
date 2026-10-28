# ==============================================================================
# TURBODL // CHRIS TITUS STYLE 1-CLICK POWERSHELL LAUNCHER
# Run via: powershell -ExecutionPolicy Bypass -File turbodl.ps1
# Or 1-line: irm https://raw.githubusercontent.com/Lethal-Aggression/An-extremly-fast-Download-manager/main/turbodl.ps1 | iex
# ==============================================================================

$Host.UI.RawUI.WindowTitle = "TURBODL // HARDWARE SPEED BONDING ENGINE"
$Host.UI.RawUI.BackgroundColor = "Black"
$Host.UI.RawUI.ForegroundColor = "White"
Clear-Host

Write-Host "┌──────────────────────────────────────────────────────────────────┐" -ForegroundColor White
Write-Host "│ TURBODL // MULTI-INTERFACE HARDWARE SPEED BONDING                │" -ForegroundColor White
Write-Host "│ [PC WI-FI + PHONE USB TETHERING SPEED AGGREGATOR]                │" -ForegroundColor White
Write-Host "└──────────────────────────────────────────────────────────────────┘" -ForegroundColor White

$CurrentDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ExePath = Join-Path $CurrentDir "turbodl.exe"

# If not found locally, check AppData or download latest binary
if (-not (Test-Path $ExePath)) {
    $AppDataDir = Join-Path $env:LOCALAPPDATA "TurboDL"
    if (-not (Test-Path $AppDataDir)) {
        New-Item -ItemType Directory -Path $AppDataDir -Force | Out-Null
    }
    $ExePath = Join-Path $AppDataDir "turbodl.exe"

    if (-not (Test-Path $ExePath)) {
        Write-Host "`n[*] Downloading native TurboDL high-performance binary..." -ForegroundColor Gray
        $DownloadUrl = "https://github.com/Lethal-Aggression/An-extremly-fast-Download-manager/releases/latest/download/turbodl.exe"
        try {
            [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
            Invoke-WebRequest -Uri $DownloadUrl -OutFile $ExePath -UseBasicParsing
            Write-Host "[OK] Binary verified and installed to $ExePath" -ForegroundColor White
        } catch {
            Write-Host "[!] Could not auto-download from releases. Checking local build..." -ForegroundColor Gray
        }
    }
}

if (Test-Path $ExePath) {
    & $ExePath $args
} else {
    Write-Host "`n[!] turbodl.exe not found. Please compile with 'go build -o turbodl.exe ./cmd/turbodl' or download it from GitHub releases." -ForegroundColor Red
}
