#requires -Version 5.1
[CmdletBinding()]
param(
    [ValidateSet('Menu','Docker','Native')]
    [string]$Mode = 'Menu',
    [string]$Action = 'Menu'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$Root = [IO.Path]::GetFullPath($PSScriptRoot)

function Run-Docker {
    & (Join-Path $Root 'manage.ps1') -Action $Action
    exit $LASTEXITCODE
}

function Run-Native {
    $nativeActions = @('Menu','Deploy','Configure','Build','Start','Status','Logs','Restart','Stop','Test','Doctor','Reset','Open')
    if ($nativeActions -notcontains $Action) { throw "Unsupported native action: $Action" }
    & (Join-Path $Root 'manage-native.ps1') -Action $Action
    exit $LASTEXITCODE
}

switch ($Mode) {
    'Docker' { Run-Docker }
    'Native' { Run-Native }
    default {
        Clear-Host
        Write-Host 'Docs Hub deployment mode' -ForegroundColor Cyan
        Write-Host ''
        Write-Host '  1) Local machine — Docker Desktop'
        Write-Host '  2) Native — WITHOUT Docker (Go binary)'
        Write-Host '  0) Exit'
        Write-Host ''
        $choice = Read-Host 'Select mode'
        switch ($choice) {
            '1' { $Action = 'Menu'; Run-Docker }
            '2' { $Action = 'Menu'; Run-Native }
            '0' { exit 0 }
            default { throw 'Unknown mode.' }
        }
    }
}
