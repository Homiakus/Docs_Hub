#requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateSet('Menu','Deploy','Configure','Build','Start','Status','Logs','Restart','Stop','Test','Doctor','Reset','Open')]
    [string]$Action = 'Menu'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Root = [IO.Path]::GetFullPath($PSScriptRoot)
$EnvFile = Join-Path $Root '.env.native'
$DataDir = Join-Path $Root 'data-native'
$BinDir = Join-Path $Root 'bin'
$Bin = Join-Path $BinDir 'docshub-native.exe'
$RunDir = Join-Path $Root '.run'
$PidFile = Join-Path $RunDir 'docshub-native.pid'
$LogDir = Join-Path $Root '.logs'
$OutLog = Join-Path $LogDir 'docshub-native.out.log'
$ErrLog = Join-Path $LogDir 'docshub-native.err.log'

function Write-Info([string]$Text) { Write-Host "→ $Text" -ForegroundColor Cyan }
function Write-Ok([string]$Text) { Write-Host "✓ $Text" -ForegroundColor Green }
function Write-Warn([string]$Text) { Write-Host "! $Text" -ForegroundColor Yellow }

function Ensure-Dirs {
    foreach ($p in @($DataDir,$BinDir,$RunDir,$LogDir)) {
        if (-not (Test-Path -LiteralPath $p)) { New-Item -ItemType Directory -Path $p -Force | Out-Null }
    }
}

function Get-RequiredGoVersion {
    $line = Get-Content -LiteralPath (Join-Path $Root 'go.mod') | Where-Object { $_ -match '^go\s+' } | Select-Object -First 1
    if (-not $line) { throw 'Cannot read Go version from go.mod.' }
    return (($line -split '\s+')[1]).Trim()
}

function Assert-Go {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $go) { throw "Go is not installed. Install Go $(Get-RequiredGoVersion)+ and retry." }
    $have = (& go env GOVERSION).Trim() -replace '^go',''
    $need = Get-RequiredGoVersion
    if ([version]$have -lt [version]$need) { throw "Go $need+ is required; detected Go $have." }
    Write-Ok "Go $have"
}

function New-HexSecret([int]$Bytes = 32) {
    $buf = New-Object byte[] $Bytes
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($buf) } finally { $rng.Dispose() }
    return -join ($buf | ForEach-Object { $_.ToString('x2') })
}

function Quote-Env([string]$Value) {
    return '"' + ($Value -replace '\\','\\' -replace '"','\"' -replace "`r|`n",' ') + '"'
}

function Read-EnvMap {
    $map = @{}
    if (-not (Test-Path -LiteralPath $EnvFile)) { return $map }
    foreach ($line in Get-Content -LiteralPath $EnvFile) {
        $t = $line.Trim()
        if (-not $t -or $t.StartsWith('#')) { continue }
        $i = $t.IndexOf('=')
        if ($i -lt 1) { continue }
        $k = $t.Substring(0,$i).Trim()
        $v = $t.Substring($i+1).Trim()
        if ($v.Length -ge 2 -and (($v[0] -eq '"' -and $v[$v.Length-1] -eq '"') -or ($v[0] -eq "'" -and $v[$v.Length-1] -eq "'"))) {
            $v = $v.Substring(1,$v.Length-2)
        }
        $v = $v -replace '\\"','"' -replace '\\\\','\'
        $map[$k] = $v
    }
    return $map
}

function Import-NativeEnv {
    $map = Read-EnvMap
    if ($map.Count -eq 0) { throw '.env.native is missing or empty. Run Configure first.' }
    foreach ($k in $map.Keys) { [Environment]::SetEnvironmentVariable($k,[string]$map[$k],'Process') }
}

function Get-Url {
    $map = Read-EnvMap
    $addr = if ($map.ContainsKey('ADDR')) { [string]$map.ADDR } else { '127.0.0.1:8080' }
    $parts = $addr.Split(':')
    $hostName = $parts[0]
    $port = $parts[$parts.Length-1]
    if ($hostName -eq '0.0.0.0') { $hostName = '127.0.0.1' }
    return "http://${hostName}:$port"
}

function Configure-Native {
    Ensure-Dirs
    $old = Read-EnvMap
    Write-Host "`nAccess mode" -ForegroundColor White
    Write-Host '  1) This computer only — 127.0.0.1 (recommended)'
    Write-Host '  2) Trusted LAN — 0.0.0.0'
    $mode = Read-Host 'Mode [1]'
    $bind = if ($mode -eq '2') { '0.0.0.0' } else { '127.0.0.1' }

    $oldPort = '8080'
    if ($old.ContainsKey('ADDR')) { $oldPort = ([string]$old.ADDR).Split(':')[-1] }
    $port = Read-Host "Port [$oldPort]"
    if ([string]::IsNullOrWhiteSpace($port)) { $port = $oldPort }
    $portInt = 0
    if (-not [int]::TryParse($port,[ref]$portInt) -or $portInt -lt 1 -or $portInt -gt 65535) { throw 'Port must be 1..65535.' }

    $oldSite = if ($old.ContainsKey('SITE_NAME')) { [string]$old.SITE_NAME } else { 'Docs Hub Native' }
    $site = Read-Host "Site name [$oldSite]"; if (-not $site) { $site = $oldSite }
    $oldUser = if ($old.ContainsKey('ADMIN_USER')) { [string]$old.ADMIN_USER } else { 'admin' }
    $user = Read-Host "Admin user [$oldUser]"; if (-not $user) { $user = $oldUser }

    $password = ''
    if ($old.ContainsKey('ADMIN_PASSWORD')) {
        $keep = Read-Host 'Keep current native admin password? [Y/n]'
        if (-not $keep -or $keep -match '^[YyДд]') { $password = [string]$old.ADMIN_PASSWORD }
    }
    if (-not $password) {
        $secure = Read-Host 'Admin password (blank = generate)' -AsSecureString
        $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        try { $password = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr) }
        if (-not $password) {
            $password = New-HexSecret 18
            Write-Warn 'Generated admin password:'
            Write-Host $password -ForegroundColor White
            Write-Warn 'Store it now.'
        }
        if ($password.Length -lt 8) { throw 'Admin password must contain at least 8 characters.' }
    }

    $secret = New-HexSecret 32
    $lines = @(
        '# Generated by manage-native.ps1. Do not commit this file.',
        "ADDR=$(Quote-Env "$bind`:$port")",
        "DATA_DIR=$(Quote-Env $DataDir)",
        "SITE_NAME=$(Quote-Env $site)",
        "ADMIN_USER=$(Quote-Env $user)",
        "ADMIN_PASSWORD=$(Quote-Env $password)",
        "SESSION_SECRET=$(Quote-Env $secret)",
        'COOKIE_SECURE="0"',
        'LOG_LEVEL="info"',
        'RATE_LIMIT_ENABLED="true"',
        'RATE_LIMIT_RPM="120"',
        'RATE_LIMIT_BURST="20"',
        'TLS_ENABLED="0"',
        'TLS_CERT_FILE=""',
        'TLS_KEY_FILE=""'
    )
    [IO.File]::WriteAllLines($EnvFile,$lines,[Text.UTF8Encoding]::new($false))
    Write-Ok "Native configuration written: $EnvFile"
    Write-Host "URL: $(Get-Url)"
    if ($bind -eq '0.0.0.0') { Write-Warn 'LAN mode uses plain HTTP. Use only on a trusted network.' }
}

function Build-Native {
    Assert-Go
    Ensure-Dirs
    Write-Info 'Downloading/verifying Go modules...'
    Push-Location $Root
    try {
        & go mod download
        if ($LASTEXITCODE -ne 0) { throw 'go mod download failed.' }
        Write-Info 'Building native Docs Hub binary...'
        & go build -trimpath -ldflags '-s -w' -o $Bin ./cmd/docshub
        if ($LASTEXITCODE -ne 0) { throw 'go build failed.' }
    } finally { Pop-Location }
    Write-Ok "Built: $Bin"
    & $Bin version
}

function Get-NativeProcess {
    if (-not (Test-Path -LiteralPath $PidFile)) { return $null }
    $raw = (Get-Content -LiteralPath $PidFile -Raw).Trim()
    $pidValue = 0
    if (-not [int]::TryParse($raw,[ref]$pidValue)) { Remove-Item $PidFile -Force -ErrorAction SilentlyContinue; return $null }
    return Get-Process -Id $pidValue -ErrorAction SilentlyContinue
}

function Wait-Health {
    $url = Get-Url
    Write-Info 'Waiting for healthcheck...'
    for ($i=0; $i -lt 30; $i++) {
        try {
            $r = Invoke-WebRequest -UseBasicParsing -Uri "$url/healthz" -TimeoutSec 3
            if ($r.StatusCode -eq 200) { Write-Ok 'Healthcheck: healthy'; return }
        } catch {}
        Start-Sleep -Seconds 1
    }
    throw "Healthcheck failed. Check $OutLog and $ErrLog"
}

function Start-Native {
    if (-not (Test-Path $EnvFile)) { Configure-Native }
    if (-not (Test-Path $Bin)) { Build-Native }
    Ensure-Dirs
    $existing = Get-NativeProcess
    if ($null -ne $existing) { Write-Ok "Already running (PID $($existing.Id))."; Show-Status; return }
    Import-NativeEnv
    $p = Start-Process -FilePath $Bin -WorkingDirectory $Root -RedirectStandardOutput $OutLog -RedirectStandardError $ErrLog -PassThru -WindowStyle Hidden
    Set-Content -LiteralPath $PidFile -Value $p.Id -Encoding Ascii
    Start-Sleep -Milliseconds 300
    if ($p.HasExited) { throw "Process exited immediately. Check $ErrLog" }
    Wait-Health
    Show-Status
}

function Stop-Native {
    $p = Get-NativeProcess
    if ($null -eq $p) { Write-Warn 'Native process is not running.'; Remove-Item $PidFile -Force -ErrorAction SilentlyContinue; return }
    Write-Info "Stopping PID $($p.Id)..."
    Stop-Process -Id $p.Id
    $p.WaitForExit(10000) | Out-Null
    Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    Write-Ok 'Stopped.'
}

function Show-Status {
    Write-Host "`nNative status" -ForegroundColor White
    $p = Get-NativeProcess
    if ($null -eq $p) { Write-Host '  Process: stopped'; return }
    Write-Host "  Process: running (PID $($p.Id))"
    Write-Host "  URL:     $(Get-Url)"
    try { Invoke-WebRequest -UseBasicParsing -Uri "$(Get-Url)/healthz" -TimeoutSec 3 | Out-Null; Write-Ok 'Healthcheck: healthy' } catch { Write-Warn 'Healthcheck: failed' }
}

function Test-Native {
    Assert-Go
    Push-Location $Root
    try { & go test ./...; if ($LASTEXITCODE -ne 0) { throw 'go test failed.' } }
    finally { Pop-Location }
    Write-Ok 'Tests passed.'
}

function Reset-Native {
    Write-Warn "This deletes ONLY native data: $DataDir"
    $answer = Read-Host 'Type RESET to continue'
    if ($answer -cne 'RESET') { return }
    Stop-Native
    Remove-Item $DataDir -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
    Write-Ok 'Native data reset. Docker and production data were not touched.'
}

function Doctor {
    Write-Host "`nNative preflight (no Docker)" -ForegroundColor White
    Assert-Go
    Write-Host "  Project: $Root"
    Write-Host "  Data:    $DataDir"
    Write-Host "  Binary:  $Bin"
    if (Test-Path $EnvFile) { Write-Host "  URL:     $(Get-Url)" }
}

function Deploy-Native { Doctor; if (-not (Test-Path $EnvFile)) { Configure-Native }; Build-Native; Start-Native }
function Restart-Native { Stop-Native; Start-Native }
function Open-Native { Start-Process (Get-Url) }
function Show-Logs {
    Ensure-Dirs
    if (-not (Test-Path $OutLog)) { New-Item -ItemType File -Path $OutLog | Out-Null }
    Get-Content -LiteralPath $OutLog -Tail 200 -Wait
}

function Show-Menu {
    while ($true) {
        Clear-Host
        Write-Host 'Docs Hub — Native / БЕЗ Docker' -ForegroundColor Cyan
        Write-Host "Project: $Root"
        if (Test-Path $EnvFile) { Write-Host "URL: $(Get-Url)" }
        Write-Host ''
        Write-Host ' 1) Полное развёртывание без Docker'
        Write-Host ' 2) Настроить адрес, порт и администратора'
        Write-Host ' 3) Собрать Go-бинарник'
        Write-Host ' 4) Запустить'
        Write-Host ' 5) Статус и healthcheck'
        Write-Host ' 6) Открыть сайт в браузере'
        Write-Host ' 7) Живые логи'
        Write-Host ' 8) Перезапустить'
        Write-Host ' 9) Остановить'
        Write-Host '10) Запустить Go-тесты'
        Write-Host '11) Диагностика'
        Write-Host '12) Сбросить ТОЛЬКО native-данные'
        Write-Host ' 0) Выход'
        $choice = Read-Host 'Выбор'
        switch ($choice) {
            '1' { Deploy-Native }
            '2' { Configure-Native }
            '3' { Build-Native }
            '4' { Start-Native }
            '5' { Show-Status }
            '6' { Open-Native }
            '7' { Show-Logs }
            '8' { Restart-Native }
            '9' { Stop-Native }
            '10' { Test-Native }
            '11' { Doctor }
            '12' { Reset-Native }
            '0' { return }
            default { Write-Warn 'Unknown menu item.' }
        }
        if ($choice -ne '7') { Read-Host 'Press Enter to continue' | Out-Null }
    }
}

switch ($Action) {
    'Deploy' { Deploy-Native }
    'Configure' { Configure-Native }
    'Build' { Build-Native }
    'Start' { Start-Native }
    'Status' { Show-Status }
    'Logs' { Show-Logs }
    'Restart' { Restart-Native }
    'Stop' { Stop-Native }
    'Test' { Test-Native }
    'Doctor' { Doctor }
    'Reset' { Reset-Native }
    'Open' { Open-Native }
    default { Show-Menu }
}
