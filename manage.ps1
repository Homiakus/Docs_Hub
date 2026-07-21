#requires -Version 5.1
<#
.SYNOPSIS
    Надёжная панель управления Docker-приложением Docs_Hub.

.DESCRIPTION
    Поддерживает интерактивное меню и автоматический CLI-режим: диагностику,
    сборку, деплой, health-check, просмотр логов, управление контейнером,
    безопасную очистку кэша и открытие приложения.

.EXAMPLE
    .\manage.ps1

.EXAMPLE
    .\manage.ps1 -Action BuildDeploy -Force

.EXAMPLE
    .\manage.ps1 -Action Build -NoCache

.EXAMPLE
    .\manage.ps1 -Action Logs -Follow -Tail 200
#>

[CmdletBinding()]
param(
    [ValidateSet(
        "Menu",
        "Doctor",
        "Build",
        "Deploy",
        "BuildDeploy",
        "Status",
        "Logs",
        "Start",
        "Stop",
        "Restart",
        "Open",
        "Remove",
        "Cleanup"
    )]
    [string]$Action = "Menu",

    [ValidateNotNullOrEmpty()]
    [string]$ImageName = "docshub",

    [ValidateNotNullOrEmpty()]
    [string]$ContainerName = "docshub-app",

    [ValidateNotNullOrEmpty()]
    [string]$Tag = "latest",

    [ValidateRange(0, 65535)]
    [int]$HostPort = 0,

    [string]$DataDir = "",

    [ValidateRange(1, 10000)]
    [int]$Tail = 100,

    [ValidateRange(1, 5)]
    [int]$BuildRetries = 2,

    [switch]$NoCache,
    [switch]$SkipPull,
    [switch]$Follow,
    [switch]$Force,
    [switch]$OpenBrowser,
    [switch]$NonInteractive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$script:OriginalLocation = (Get-Location).Path
$script:ProjectRoot = [System.IO.Path]::GetFullPath($PSScriptRoot)
$script:DockerfilePath = Join-Path $script:ProjectRoot "Dockerfile"
$script:EnvFilePath = Join-Path $script:ProjectRoot ".env"
$script:EnvExamplePath = Join-Path $script:ProjectRoot ".env.example"
$script:DockerIgnorePath = Join-Path $script:ProjectRoot ".dockerignore"
$script:LogDirectory = Join-Path $script:ProjectRoot ".docker-logs"
$script:IsWindowsPlatform = ($env:OS -eq "Windows_NT")
$script:IsInteractive = (-not $NonInteractive.IsPresent) -and [Environment]::UserInteractive
$script:ImageReference = "${ImageName}:${Tag}"
$script:ResolvedHostPort = 8080
$script:ResolvedDataDirectory = Join-Path $script:ProjectRoot "data"

function Initialize-Console {
    try {
        $utf8 = [System.Text.UTF8Encoding]::new($false)
        $global:OutputEncoding = $utf8
        [Console]::OutputEncoding = $utf8
        [Console]::InputEncoding = $utf8
    }
    catch {
        # Некоторые хосты PowerShell не позволяют менять кодировку консоли.
    }
}

function Initialize-LogDirectory {
    if (-not (Test-Path -LiteralPath $script:LogDirectory -PathType Container)) {
        New-Item -ItemType Directory -Path $script:LogDirectory -Force | Out-Null
    }
}

function Write-Event {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message,

        [ValidateSet("Info", "Success", "Warning", "Error", "Debug")]
        [string]$Level = "Info",

        [switch]$NoConsole
    )

    $color = switch ($Level) {
        "Success" { "Green" }
        "Warning" { "Yellow" }
        "Error"   { "Red" }
        "Debug"   { "DarkGray" }
        default   { "Gray" }
    }

    if (-not $NoConsole.IsPresent) {
        Write-Host $Message -ForegroundColor $color
    }

    try {
        Initialize-LogDirectory
        $line = "{0} [{1}] {2}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Level.ToUpperInvariant(), $Message
        Add-Content -LiteralPath (Join-Path $script:LogDirectory "manage.log") -Value $line -Encoding UTF8
    }
    catch {
        # Ошибка журналирования не должна ломать управление приложением.
    }
}

function Write-Section {
    param([Parameter(Mandatory = $true)][string]$Title)
    Write-Host "`n=== $Title ===" -ForegroundColor Cyan
    Write-Event -Message "Раздел: $Title" -Level Debug -NoConsole
}

function Pause-Console {
    if (-not $script:IsInteractive) {
        return
    }

    Write-Host "`nНажмите Enter для продолжения..." -ForegroundColor DarkGray
    [void](Read-Host)
}

function Confirm-Operation {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Prompt,
        [string]$RequiredText = "",
        [switch]$DefaultYes
    )

    if ($Force.IsPresent) {
        return $true
    }

    if (-not $script:IsInteractive) {
        throw "Операция требует подтверждения. Повторите команду с параметром -Force."
    }

    if (-not [string]::IsNullOrWhiteSpace($RequiredText)) {
        $answer = Read-Host "$Prompt Введите '$RequiredText'"
        return ($answer -ceq $RequiredText)
    }

    $suffix = if ($DefaultYes.IsPresent) { "[Y/n]" } else { "[y/N]" }
    $answer = (Read-Host "$Prompt $suffix").Trim().ToLowerInvariant()

    if ([string]::IsNullOrWhiteSpace($answer)) {
        return $DefaultYes.IsPresent
    }

    return ($answer -eq "y" -or $answer -eq "yes" -or $answer -eq "д" -or $answer -eq "да")
}

function Format-CommandArgument {
    param([Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value)

    if ($Value -match '[\s"]') {
        return '"{0}"' -f ($Value -replace '"', '\"')
    }

    return $Value
}

function Invoke-DockerCommand {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,

        [switch]$CaptureOutput,
        [switch]$AllowFailure,
        [string]$LogName = "docker"
    )

    Initialize-LogDirectory

    $safeCommand = "docker " + (($Arguments | ForEach-Object { Format-CommandArgument -Value $_ }) -join " ")
    $commandLog = Join-Path $script:LogDirectory ("{0}-{1}.log" -f $LogName, (Get-Date -Format "yyyyMMdd-HHmmss-fff"))
    $lines = New-Object 'System.Collections.Generic.List[string]'

    Write-Event -Message "Выполнение: $safeCommand" -Level Debug -NoConsole

    if ($CaptureOutput.IsPresent) {
        $rawOutput = @(& docker @Arguments 2>&1)
        $exitCode = $LASTEXITCODE

        foreach ($item in $rawOutput) {
            [void]$lines.Add([string]$item)
        }
    }
    else {
        & docker @Arguments 2>&1 | ForEach-Object {
            $text = [string]$_
            [void]$lines.Add($text)
            Write-Host $text
        }
        $exitCode = $LASTEXITCODE
    }

    try {
        @(
            "COMMAND: $safeCommand",
            "EXIT_CODE: $exitCode",
            "",
            ($lines -join [Environment]::NewLine)
        ) | Set-Content -LiteralPath $commandLog -Encoding UTF8
    }
    catch {
        # Журнал команды является вспомогательным.
    }

    $result = [pscustomobject]@{
        ExitCode = $exitCode
        Output   = $lines.ToArray()
        Command  = $safeCommand
        LogPath  = $commandLog
    }

    if ($exitCode -ne 0 -and -not $AllowFailure.IsPresent) {
        $message = "Команда Docker завершилась с кодом $exitCode. Журнал: $commandLog"
        throw $message
    }

    return $result
}

function Get-CommandOutputText {
    param([Parameter(Mandatory = $true)]$Result)
    return (($Result.Output | ForEach-Object { [string]$_ }) -join [Environment]::NewLine).Trim()
}

function Assert-DockerReady {
    $dockerCommand = Get-Command docker -ErrorAction SilentlyContinue
    if ($null -eq $dockerCommand) {
        throw "Docker CLI не найден. Установите Docker Desktop и добавьте docker в PATH."
    }

    $result = Invoke-DockerCommand -Arguments @("version", "--format", "{{.Server.Version}}") -CaptureOutput -AllowFailure -LogName "docker-version"
    if ($result.ExitCode -ne 0) {
        $details = Get-CommandOutputText -Result $result
        throw "Docker Engine недоступен. Запустите Docker Desktop.`n$details"
    }
}

function Test-DockerObjectExists {
    param(
        [Parameter(Mandatory = $true)][ValidateSet("container", "image")][string]$Type,
        [Parameter(Mandatory = $true)][string]$Name
    )

    $result = Invoke-DockerCommand -Arguments @($Type, "inspect", $Name) -CaptureOutput -AllowFailure -LogName "inspect-$Type"
    return ($result.ExitCode -eq 0)
}

function Get-DotEnvMap {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)

    $map = @{}
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return $map
    }

    foreach ($rawLine in Get-Content -LiteralPath $Path) {
        if ($null -eq $rawLine) {
            continue
        }

        $line = [string]$rawLine
        $trimmed = $line.Trim()
        if ([string]::IsNullOrWhiteSpace($trimmed) -or $trimmed.StartsWith("#")) {
            continue
        }

        if ($trimmed -match '^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            $key = $Matches[1]
            $value = $Matches[2]

            if ($value.Length -ge 2) {
                $first = $value.Substring(0, 1)
                $last = $value.Substring($value.Length - 1, 1)
                if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
                    $value = $value.Substring(1, $value.Length - 2)
                }
            }

            $map[$key] = $value
        }
    }

    return $map
}

function Set-DotEnvValue {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Key,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value
    )

    if ($Value.Contains("`r") -or $Value.Contains("`n")) {
        throw "Значение переменной $Key не должно содержать перевод строки."
    }

    $lines = New-Object 'System.Collections.Generic.List[string]'
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        foreach ($line in Get-Content -LiteralPath $Path) {
            [void]$lines.Add([string]$line)
        }
    }

    $pattern = '^\s*(?:export\s+)?' + [Regex]::Escape($Key) + '\s*='
    $replacement = "$Key=$Value"
    $updated = $false

    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match $pattern) {
            $lines[$i] = $replacement
            $updated = $true
            break
        }
    }

    if (-not $updated) {
        [void]$lines.Add($replacement)
    }

    $utf8NoBom = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllLines($Path, $lines.ToArray(), $utf8NoBom)
}

function New-CryptoSecret {
    param([ValidateRange(16, 256)][int]$ByteCount = 48)

    $bytes = [byte[]]::new($ByteCount)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    }
    finally {
        $rng.Dispose()
    }

    return ([Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_'))
}

function ConvertFrom-SecureValue {
    param([Parameter(Mandatory = $true)][Security.SecureString]$SecureValue)

    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureValue)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

function Protect-EnvFilePermissions {
    param([Parameter(Mandatory = $true)][string]$Path)

    if ($script:IsWindowsPlatform) {
        return
    }

    $chmod = Get-Command chmod -ErrorAction SilentlyContinue
    if ($null -ne $chmod) {
        & $chmod.Source 600 -- $Path 2>$null
    }
}

function Initialize-EnvFile {
    if (Test-Path -LiteralPath $script:EnvFilePath -PathType Leaf) {
        return
    }

    Write-Event -Message "Файл .env не найден." -Level Warning

    if (-not (Test-Path -LiteralPath $script:EnvExamplePath -PathType Leaf)) {
        throw "Не найден .env.example. Невозможно безопасно определить обязательные переменные приложения."
    }

    if (-not $script:IsInteractive) {
        throw "Для первого запуска создайте .env из .env.example вручную или запустите скрипт интерактивно."
    }

    Copy-Item -LiteralPath $script:EnvExamplePath -Destination $script:EnvFilePath -Force

    $securePassword = Read-Host "Введите новый пароль администратора (не отображается)" -AsSecureString
    $adminPassword = ConvertFrom-SecureValue -SecureValue $securePassword

    if ([string]::IsNullOrWhiteSpace($adminPassword)) {
        $adminPassword = New-CryptoSecret -ByteCount 18
        Write-Event -Message "Пароль не был введён. Сгенерирован случайный пароль администратора:" -Level Warning
        Write-Host $adminPassword -ForegroundColor White
        Write-Event -Message "Сохраните пароль сейчас: позднее скрипт его не покажет." -Level Warning
    }
    elseif ($adminPassword.Length -lt 8) {
        Write-Event -Message "Пароль должен содержать не менее 8 символов." -Level Error
        throw "ADMIN_PASSWORD слишком короткий. Приложение требует минимум 8 символов."
    }
    elseif ($adminPassword.Length -lt 12) {
        Write-Event -Message "Пароль короче 12 символов. Для production рекомендуется более длинный пароль." -Level Warning
    }

    $sessionSecret = New-CryptoSecret -ByteCount 48
    Set-DotEnvValue -Path $script:EnvFilePath -Key "ADMIN_PASSWORD" -Value $adminPassword
    Set-DotEnvValue -Path $script:EnvFilePath -Key "SESSION_SECRET" -Value $sessionSecret
    Protect-EnvFilePermissions -Path $script:EnvFilePath

    Write-Event -Message "Файл .env создан из шаблона и заполнен безопасными значениями." -Level Success
}

function Test-EnvConfiguration {
    [CmdletBinding()]
    param([switch]$ThrowOnError)

    $errors = New-Object 'System.Collections.Generic.List[string]'
    $warnings = New-Object 'System.Collections.Generic.List[string]'

    if (-not (Test-Path -LiteralPath $script:EnvFilePath -PathType Leaf)) {
        [void]$errors.Add("Файл .env отсутствует.")
    }
    else {
        $envMap = Get-DotEnvMap -Path $script:EnvFilePath

        if (-not $envMap.ContainsKey("ADMIN_PASSWORD") -or [string]::IsNullOrWhiteSpace([string]$envMap["ADMIN_PASSWORD"])) {
            [void]$errors.Add("ADMIN_PASSWORD не задан.")
        }
        else {
            $password = [string]$envMap["ADMIN_PASSWORD"]
            $knownWeakPasswords = @("change-me-now", "asdqwe123", "admin", "password", "12345678")
            if ($knownWeakPasswords -contains $password.ToLowerInvariant()) {
                [void]$errors.Add("ADMIN_PASSWORD содержит известное слабое или шаблонное значение.")
            }
            elseif ($password.Length -lt 8) {
                [void]$errors.Add("ADMIN_PASSWORD должен содержать не менее 8 символов.")
            }
            elseif ($password.Length -lt 12) {
                [void]$warnings.Add("ADMIN_PASSWORD короче 12 символов.")
            }
        }

        if (-not $envMap.ContainsKey("SESSION_SECRET") -or [string]::IsNullOrWhiteSpace([string]$envMap["SESSION_SECRET"])) {
            [void]$errors.Add("SESSION_SECRET не задан.")
        }
        else {
            $secret = [string]$envMap["SESSION_SECRET"]
            if ($secret -match 'replace|change-me|example|secret-here') {
                [void]$errors.Add("SESSION_SECRET содержит шаблонное значение.")
            }
            elseif ($secret.Length -lt 32) {
                [void]$errors.Add("SESSION_SECRET должен содержать не менее 32 символов.")
            }
        }
    }

    foreach ($warning in $warnings) {
        Write-Event -Message $warning -Level Warning
    }

    foreach ($errorItem in $errors) {
        Write-Event -Message $errorItem -Level Error
    }

    if ($ThrowOnError.IsPresent -and $errors.Count -gt 0) {
        throw "Конфигурация .env содержит ошибки. Исправьте их перед деплоем."
    }

    return [pscustomobject]@{
        IsValid  = ($errors.Count -eq 0)
        Errors   = $errors.ToArray()
        Warnings = $warnings.ToArray()
    }
}

function Resolve-RuntimeConfiguration {
    $envMap = Get-DotEnvMap -Path $script:EnvFilePath

    $port = $HostPort
    if ($port -eq 0 -and $envMap.ContainsKey("HOST_PORT")) {
        $candidatePort = 0
        if ([int]::TryParse([string]$envMap["HOST_PORT"], [ref]$candidatePort)) {
            $port = $candidatePort
        }
    }

    if ($port -eq 0) {
        $port = 8080
    }

    if ($port -lt 1 -or $port -gt 65535) {
        throw "Некорректный порт хоста: $port"
    }

    $dataCandidate = $DataDir
    if ([string]::IsNullOrWhiteSpace($dataCandidate) -and $envMap.ContainsKey("HOST_DATA_DIR")) {
        $dataCandidate = [string]$envMap["HOST_DATA_DIR"]
    }
    if ([string]::IsNullOrWhiteSpace($dataCandidate) -and $envMap.ContainsKey("DATA_DIR")) {
        $dataCandidate = [string]$envMap["DATA_DIR"]
    }
    if ([string]::IsNullOrWhiteSpace($dataCandidate) -or $dataCandidate -eq "/data" -or $dataCandidate -eq "\data") {
        $dataCandidate = "./data"
    }

    if ([System.IO.Path]::IsPathRooted($dataCandidate)) {
        $resolvedData = [System.IO.Path]::GetFullPath($dataCandidate)
    }
    else {
        $resolvedData = [System.IO.Path]::GetFullPath((Join-Path $script:ProjectRoot $dataCandidate))
    }

    $script:ResolvedHostPort = $port
    $script:ResolvedDataDirectory = $resolvedData
}

function Test-TcpPortAvailable {
    param([Parameter(Mandatory = $true)][ValidateRange(1, 65535)][int]$Port)

    $listener = $null
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
        $listener.Start()
        return $true
    }
    catch {
        return $false
    }
    finally {
        if ($null -ne $listener) {
            try { $listener.Stop() } catch { }
        }
    }
}

function Assert-ProjectFiles {
    $requiredFiles = @($script:DockerfilePath, (Join-Path $script:ProjectRoot "go.mod"))
    foreach ($file in $requiredFiles) {
        if (-not (Test-Path -LiteralPath $file -PathType Leaf)) {
            throw "Не найден обязательный файл: $file"
        }
    }

    if (-not (Test-Path -LiteralPath $script:DockerIgnorePath -PathType Leaf)) {
        Write-Event -Message ".dockerignore отсутствует: контекст сборки может быть большим и содержать лишние файлы." -Level Warning
    }
}

function Test-TransientBuildFailure {
    param([Parameter(Mandatory = $true)][string]$Text)

    return ($Text -match '(?i)unexpected EOF|short read|TLS handshake timeout|connection reset|i/o timeout|temporary failure|context deadline exceeded|failed to copy|failed to fetch|429 Too Many Requests|502 Bad Gateway|503 Service Unavailable')
}

function Show-BuildFailureAdvice {
    param([Parameter(Mandatory = $true)][string]$OutputText)

    if ($OutputText -match '(?i)unexpected EOF|short read') {
        Write-Event -Message "Docker получил неполный слой образа. Это обычно повреждённый кэш или нестабильное registry-зеркало." -Level Error
        Write-Event -Message "Запустите пункт 'Очистка' -> кэш сборщика, затем повторите сборку с официальным базовым образом." -Level Warning
    }
    elseif ($OutputText -match '(?i)no space left on device') {
        Write-Event -Message "На диске Docker закончилось место. Проверьте docker system df и выполните безопасную очистку." -Level Error
    }
    elseif ($OutputText -match '(?i)failed to solve|failed to compute cache key') {
        Write-Event -Message "BuildKit не смог сформировать слой. Проверьте контекст, .dockerignore и целостность локального кэша." -Level Error
    }
}

function Build-Image {
    Write-Section "Сборка Docker-образа"
    Assert-DockerReady
    Assert-ProjectFiles

    $env:DOCKER_BUILDKIT = "1"

    $arguments = New-Object 'System.Collections.Generic.List[string]'
    [void]$arguments.Add("build")
    [void]$arguments.Add("--file")
    [void]$arguments.Add($script:DockerfilePath)
    [void]$arguments.Add("--tag")
    [void]$arguments.Add($script:ImageReference)
    [void]$arguments.Add("--progress=plain")

    if (-not $SkipPull.IsPresent) {
        [void]$arguments.Add("--pull")
    }
    if ($NoCache.IsPresent) {
        [void]$arguments.Add("--no-cache")
    }

    [void]$arguments.Add($script:ProjectRoot)

    Write-Event -Message "Образ: $($script:ImageReference)" -Level Info
    Write-Event -Message "Контекст: $($script:ProjectRoot)" -Level Info

    $lastResult = $null
    for ($attempt = 1; $attempt -le $BuildRetries; $attempt++) {
        if ($attempt -gt 1) {
            Write-Event -Message "Повторная попытка сборки $attempt из $BuildRetries после сетевой ошибки." -Level Warning
            Start-Sleep -Seconds 2
        }

        $lastResult = Invoke-DockerCommand -Arguments $arguments.ToArray() -AllowFailure -LogName "build"
        if ($lastResult.ExitCode -eq 0) {
            Write-Event -Message "Образ $($script:ImageReference) успешно собран." -Level Success
            return $true
        }

        $outputText = Get-CommandOutputText -Result $lastResult
        Show-BuildFailureAdvice -OutputText $outputText

        if (-not (Test-TransientBuildFailure -Text $outputText)) {
            break
        }
    }

    if ($null -ne $lastResult) {
        throw "Сборка завершилась ошибкой. Полный журнал: $($lastResult.LogPath)"
    }

    throw "Сборка завершилась ошибкой."
}

function Remove-ExistingContainerForDeploy {
    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        return
    }

    Write-Event -Message "Контейнер '$ContainerName' уже существует." -Level Warning
    if (-not (Confirm-Operation -Prompt "Пересоздать контейнер? Данные в примонтированной папке сохранятся." -DefaultYes)) {
        throw "Деплой отменён пользователем."
    }

    $result = Invoke-DockerCommand -Arguments @("rm", "--force", $ContainerName) -AllowFailure -LogName "remove-before-deploy"
    if ($result.ExitCode -ne 0) {
        throw "Не удалось удалить существующий контейнер '$ContainerName'."
    }
}

function Wait-ContainerReady {
    [CmdletBinding()]
    param([ValidateRange(5, 300)][int]$TimeoutSeconds = 60)

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $lastHealth = "unknown"

    while ((Get-Date) -lt $deadline) {
        $result = Invoke-DockerCommand -Arguments @(
            "inspect",
            "--format",
            "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}",
            $ContainerName
        ) -CaptureOutput -AllowFailure -LogName "health"

        if ($result.ExitCode -ne 0) {
            Start-Sleep -Seconds 2
            continue
        }

        $statusLine = Get-CommandOutputText -Result $result
        $parts = $statusLine.Split('|')
        $state = if ($parts.Length -ge 1) { $parts[0] } else { "unknown" }
        $health = if ($parts.Length -ge 2) { $parts[1] } else { "none" }
        $lastHealth = $health

        if ($state -eq "exited" -or $state -eq "dead") {
            Write-Event -Message "Контейнер завершился до готовности. Последние логи:" -Level Error
            [void](Invoke-DockerCommand -Arguments @("logs", "--tail", "100", $ContainerName) -AllowFailure -LogName "startup-failure")
            return $false
        }

        if ($state -eq "running" -and $health -eq "healthy") {
            return $true
        }

        if ($state -eq "running" -and $health -eq "none") {
            try {
                $uri = "http://127.0.0.1:$($script:ResolvedHostPort)/healthz"
                $response = Invoke-WebRequest -Uri $uri -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
                if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) {
                    return $true
                }
            }
            catch {
                # Приложение ещё запускается либо endpoint healthz отсутствует.
            }
        }

        Start-Sleep -Seconds 2
    }

    if ($lastHealth -eq "unhealthy") {
        Write-Event -Message "Контейнер имеет статус unhealthy. Последние логи:" -Level Error
        [void](Invoke-DockerCommand -Arguments @("logs", "--tail", "100", $ContainerName) -AllowFailure -LogName "unhealthy")
        return $false
    }

    Write-Event -Message "Контейнер запущен, но готовность не подтверждена за отведённое время." -Level Warning
    return $true
}

function Deploy-Container {
    Write-Section "Деплой контейнера"
    Assert-DockerReady
    Initialize-EnvFile
    [void](Test-EnvConfiguration -ThrowOnError)
    Resolve-RuntimeConfiguration

    if (-not (Test-DockerObjectExists -Type image -Name $script:ImageReference)) {
        Write-Event -Message "Локальный образ $($script:ImageReference) отсутствует." -Level Warning
        if ($script:IsInteractive) {
            if (Confirm-Operation -Prompt "Собрать образ сейчас?" -DefaultYes) {
                [void](Build-Image)
            }
            else {
                throw "Деплой невозможен без локального образа."
            }
        }
        else {
            throw "Локальный образ отсутствует. Сначала выполните -Action Build или -Action BuildDeploy."
        }
    }

    Remove-ExistingContainerForDeploy

    if (-not (Test-TcpPortAvailable -Port $script:ResolvedHostPort)) {
        throw "Порт $($script:ResolvedHostPort) уже занят другим процессом. Укажите другой -HostPort или HOST_PORT в .env."
    }

    if (-not (Test-Path -LiteralPath $script:ResolvedDataDirectory -PathType Container)) {
        New-Item -ItemType Directory -Path $script:ResolvedDataDirectory -Force | Out-Null
    }

    $arguments = New-Object 'System.Collections.Generic.List[string]'
    foreach ($value in @(
        "run", "--detach",
        "--name", $ContainerName,
        "--env-file", $script:EnvFilePath,
        "--env", "DATA_DIR=/data",
        "--volume", "$($script:ResolvedDataDirectory):/data",
        "--publish", "$($script:ResolvedHostPort):8080",
        "--restart", "unless-stopped",
        "--init",
        "--stop-timeout", "30",
        "--security-opt", "no-new-privileges:true",
        "--cap-drop", "ALL",
        "--log-opt", "max-size=10m",
        "--log-opt", "max-file=3",
        "--label", "com.docshub.managed-by=manage.ps1",
        "--label", "com.docshub.project-root=$($script:ProjectRoot)",
        $script:ImageReference
    )) {
        [void]$arguments.Add([string]$value)
    }

    Write-Event -Message "Контейнер: $ContainerName" -Level Info
    Write-Event -Message "Адрес: http://localhost:$($script:ResolvedHostPort)" -Level Info
    Write-Event -Message "Данные: $($script:ResolvedDataDirectory)" -Level Info

    $runResult = Invoke-DockerCommand -Arguments $arguments.ToArray() -AllowFailure -LogName "deploy"
    if ($runResult.ExitCode -ne 0) {
        throw "Docker не смог запустить контейнер. Журнал: $($runResult.LogPath)"
    }

    if (-not (Wait-ContainerReady -TimeoutSeconds 60)) {
        throw "Контейнер создан, но приложение не прошло проверку готовности."
    }

    Write-Event -Message "Контейнер успешно развёрнут и запущен." -Level Success
    Write-Host "http://localhost:$($script:ResolvedHostPort)" -ForegroundColor White

    if ($OpenBrowser.IsPresent) {
        Open-Application
    }
}

function Show-ContainerStatus {
    Write-Section "Статус контейнера"
    Assert-DockerReady
    Resolve-RuntimeConfiguration

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        Write-Event -Message "Контейнер '$ContainerName' не найден." -Level Warning
        return
    }

    [void](Invoke-DockerCommand -Arguments @(
        "ps", "--all",
        "--filter", "name=^/$ContainerName$",
        "--format", "table {{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"
    ) -LogName "status")

    $details = Invoke-DockerCommand -Arguments @(
        "inspect",
        "--format",
        "State={{.State.Status}} | Health={{if .State.Health}}{{.State.Health.Status}}{{else}}N/A{{end}} | Restarts={{.RestartCount}} | Started={{.State.StartedAt}}",
        $ContainerName
    ) -CaptureOutput -AllowFailure -LogName "status-details"

    if ($details.ExitCode -eq 0) {
        Write-Host "`n$(Get-CommandOutputText -Result $details)" -ForegroundColor Gray
    }

    Write-Host "URL: http://localhost:$($script:ResolvedHostPort)" -ForegroundColor Green
}

function Show-ContainerLogs {
    Write-Section "Логи контейнера"
    Assert-DockerReady

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        Write-Event -Message "Контейнер '$ContainerName' не найден." -Level Warning
        return
    }

    $arguments = New-Object 'System.Collections.Generic.List[string]'
    [void]$arguments.Add("logs")
    [void]$arguments.Add("--tail")
    [void]$arguments.Add([string]$Tail)
    [void]$arguments.Add("--timestamps")
    if ($Follow.IsPresent) {
        [void]$arguments.Add("--follow")
        Write-Event -Message "Режим слежения включён. Для выхода нажмите Ctrl+C." -Level Warning
    }
    [void]$arguments.Add($ContainerName)

    try {
        [void](Invoke-DockerCommand -Arguments $arguments.ToArray() -AllowFailure -LogName "logs")
    }
    catch [System.Management.Automation.PipelineStoppedException] {
        Write-Event -Message "Просмотр логов остановлен." -Level Info
    }
}

function Start-ManagedContainer {
    Write-Section "Запуск контейнера"
    Assert-DockerReady

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        throw "Контейнер '$ContainerName' не найден. Выполните деплой."
    }

    $result = Invoke-DockerCommand -Arguments @("start", $ContainerName) -AllowFailure -LogName "start"
    if ($result.ExitCode -ne 0) {
        throw "Не удалось запустить контейнер '$ContainerName'."
    }

    Write-Event -Message "Контейнер запущен." -Level Success
}

function Stop-ManagedContainer {
    Write-Section "Остановка контейнера"
    Assert-DockerReady

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        Write-Event -Message "Контейнер '$ContainerName' не найден." -Level Warning
        return
    }

    $result = Invoke-DockerCommand -Arguments @("stop", "--time", "30", $ContainerName) -AllowFailure -LogName "stop"
    if ($result.ExitCode -ne 0) {
        throw "Не удалось остановить контейнер '$ContainerName'."
    }

    Write-Event -Message "Контейнер остановлен." -Level Success
}

function Restart-ManagedContainer {
    Write-Section "Перезапуск контейнера"
    Assert-DockerReady

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        throw "Контейнер '$ContainerName' не найден."
    }

    $result = Invoke-DockerCommand -Arguments @("restart", "--time", "30", $ContainerName) -AllowFailure -LogName "restart"
    if ($result.ExitCode -ne 0) {
        throw "Не удалось перезапустить контейнер '$ContainerName'."
    }

    Write-Event -Message "Контейнер перезапущен." -Level Success
}

function Remove-ManagedContainer {
    Write-Section "Удаление контейнера"
    Assert-DockerReady

    if (-not (Test-DockerObjectExists -Type container -Name $ContainerName)) {
        Write-Event -Message "Контейнер '$ContainerName' не найден." -Level Warning
        return
    }

    if (-not (Confirm-Operation -Prompt "Удалить контейнер '$ContainerName'? Примонтированные данные останутся.")) {
        Write-Event -Message "Удаление отменено." -Level Warning
        return
    }

    $result = Invoke-DockerCommand -Arguments @("rm", "--force", $ContainerName) -AllowFailure -LogName "remove"
    if ($result.ExitCode -ne 0) {
        throw "Не удалось удалить контейнер '$ContainerName'."
    }

    Write-Event -Message "Контейнер удалён. Каталог данных не затронут." -Level Success
}

function Open-Application {
    Resolve-RuntimeConfiguration
    $url = "http://localhost:$($script:ResolvedHostPort)"

    try {
        Start-Process $url | Out-Null
        Write-Event -Message "Открыт адрес $url" -Level Success
    }
    catch {
        Write-Event -Message "Не удалось открыть браузер автоматически. Откройте вручную: $url" -Level Warning
    }
}

function Invoke-Doctor {
    Write-Section "Диагностика окружения"

    $checks = New-Object 'System.Collections.Generic.List[object]'

    function Add-Check {
        param([string]$Name, [bool]$Ok, [string]$Details)
        [void]$checks.Add([pscustomobject]@{
            Check   = $Name
            Result  = if ($Ok) { "OK" } else { "FAIL" }
            Details = $Details
        })
    }

    Add-Check -Name "PowerShell" -Ok ($PSVersionTable.PSVersion.Major -ge 5) -Details ([string]$PSVersionTable.PSVersion)
    Add-Check -Name "Project root" -Ok (Test-Path -LiteralPath $script:ProjectRoot -PathType Container) -Details $script:ProjectRoot
    Add-Check -Name "Dockerfile" -Ok (Test-Path -LiteralPath $script:DockerfilePath -PathType Leaf) -Details $script:DockerfilePath
    Add-Check -Name "go.mod" -Ok (Test-Path -LiteralPath (Join-Path $script:ProjectRoot "go.mod") -PathType Leaf) -Details "Required for Go build"
    Add-Check -Name ".dockerignore" -Ok (Test-Path -LiteralPath $script:DockerIgnorePath -PathType Leaf) -Details $script:DockerIgnorePath
    Add-Check -Name ".env" -Ok (Test-Path -LiteralPath $script:EnvFilePath -PathType Leaf) -Details $script:EnvFilePath

    $dockerCli = Get-Command docker -ErrorAction SilentlyContinue
    Add-Check -Name "Docker CLI" -Ok ($null -ne $dockerCli) -Details $(if ($null -ne $dockerCli) { $dockerCli.Source } else { "Not found" })

    if ($null -ne $dockerCli) {
        $server = Invoke-DockerCommand -Arguments @("version", "--format", "{{.Server.Version}}") -CaptureOutput -AllowFailure -LogName "doctor-version"
        Add-Check -Name "Docker Engine" -Ok ($server.ExitCode -eq 0) -Details $(if ($server.ExitCode -eq 0) { Get-CommandOutputText -Result $server } else { "Unavailable" })

        $buildx = Invoke-DockerCommand -Arguments @("buildx", "version") -CaptureOutput -AllowFailure -LogName "doctor-buildx"
        Add-Check -Name "Docker Buildx" -Ok ($buildx.ExitCode -eq 0) -Details $(if ($buildx.ExitCode -eq 0) { Get-CommandOutputText -Result $buildx } else { "Unavailable" })

        $imageExists = Test-DockerObjectExists -Type image -Name $script:ImageReference
        Add-Check -Name "Application image" -Ok $imageExists -Details $script:ImageReference

        $containerExists = Test-DockerObjectExists -Type container -Name $ContainerName
        Add-Check -Name "Application container" -Ok $containerExists -Details $ContainerName
    }

    if (Test-Path -LiteralPath $script:EnvFilePath -PathType Leaf) {
        $envCheck = Test-EnvConfiguration
        Add-Check -Name "Environment config" -Ok $envCheck.IsValid -Details $(if ($envCheck.IsValid) { "Valid" } else { $envCheck.Errors -join "; " })
    }

    try {
        Resolve-RuntimeConfiguration
        Add-Check -Name "Host port" -Ok $true -Details ([string]$script:ResolvedHostPort)
        Add-Check -Name "Data directory" -Ok $true -Details $script:ResolvedDataDirectory
    }
    catch {
        Add-Check -Name "Runtime config" -Ok $false -Details $_.Exception.Message
    }

    $checks | Format-Table -AutoSize | Out-Host

    $failed = @($checks | Where-Object { $_.Result -eq "FAIL" })
    if ($failed.Count -eq 0) {
        Write-Event -Message "Критических проблем не обнаружено." -Level Success
    }
    else {
        Write-Event -Message "Обнаружено проблем: $($failed.Count)." -Level Warning
    }

    if ($null -ne $dockerCli) {
        Write-Host "`nИспользование диска Docker:" -ForegroundColor Cyan
        [void](Invoke-DockerCommand -Arguments @("system", "df") -AllowFailure -LogName "doctor-disk")
    }
}

function Invoke-Cleanup {
    Write-Section "Безопасная очистка Docker"
    Assert-DockerReady

    Write-Host "1. Удалить кэш сборщика старше 7 дней" -ForegroundColor Yellow
    Write-Host "2. Удалить только локальный образ $($script:ImageReference)" -ForegroundColor Yellow
    Write-Host "3. Выполнить docker system prune без удаления volumes" -ForegroundColor Yellow
    Write-Host "0. Отмена" -ForegroundColor Gray

    if (-not $script:IsInteractive) {
        if (-not $Force.IsPresent) {
            throw "В неинтерактивном режиме Cleanup требует -Force и выполняет только очистку кэша сборщика."
        }
        $choice = "1"
    }
    else {
        $choice = (Read-Host "Выберите действие [0-3]").Trim()
    }

    switch ($choice) {
        "1" {
            if (Confirm-Operation -Prompt "Удалить неиспользуемый кэш сборщика старше 7 дней?") {
                [void](Invoke-DockerCommand -Arguments @("builder", "prune", "--force", "--filter", "until=168h") -LogName "builder-prune")
                Write-Event -Message "Кэш сборщика очищен." -Level Success
            }
        }
        "2" {
            if (-not (Test-DockerObjectExists -Type image -Name $script:ImageReference)) {
                Write-Event -Message "Образ $($script:ImageReference) не найден." -Level Warning
                return
            }
            if (Confirm-Operation -Prompt "Удалить образ $($script:ImageReference)?") {
                [void](Invoke-DockerCommand -Arguments @("image", "rm", $script:ImageReference) -LogName "image-remove")
                Write-Event -Message "Образ удалён." -Level Success
            }
        }
        "3" {
            if (Confirm-Operation -Prompt "Удалить все неиспользуемые контейнеры, сети, образы и build-cache? Volumes сохранятся." -RequiredText "PRUNE") {
                [void](Invoke-DockerCommand -Arguments @("system", "prune", "--force") -LogName "system-prune")
                Write-Event -Message "Неиспользуемые ресурсы Docker очищены." -Level Success
            }
        }
        default {
            Write-Event -Message "Очистка отменена." -Level Info
        }
    }
}

function Invoke-SelectedAction {
    param([Parameter(Mandatory = $true)][string]$SelectedAction)

    switch ($SelectedAction) {
        "Doctor"      { Invoke-Doctor }
        "Build"       { [void](Build-Image) }
        "Deploy"      { Deploy-Container }
        "BuildDeploy" { [void](Build-Image); Deploy-Container }
        "Status"      { Show-ContainerStatus }
        "Logs"        { Show-ContainerLogs }
        "Start"       { Start-ManagedContainer }
        "Stop"        { Stop-ManagedContainer }
        "Restart"     { Restart-ManagedContainer }
        "Open"        { Open-Application }
        "Remove"      { Remove-ManagedContainer }
        "Cleanup"     { Invoke-Cleanup }
        default        { throw "Неизвестное действие: $SelectedAction" }
    }
}

function Show-MainMenu {
    while ($true) {
        Clear-Host
        Write-Host "==================================================" -ForegroundColor Cyan
        Write-Host "              Docs_Hub Docker Manager             " -ForegroundColor Cyan
        Write-Host "==================================================" -ForegroundColor Cyan
        Write-Host "  1. Диагностика окружения" -ForegroundColor Yellow
        Write-Host "  2. Собрать образ" -ForegroundColor Yellow
        Write-Host "  3. Собрать и развернуть" -ForegroundColor Yellow
        Write-Host "  4. Развернуть существующий образ" -ForegroundColor Yellow
        Write-Host "  5. Статус контейнера" -ForegroundColor Yellow
        Write-Host "  6. Последние логи" -ForegroundColor Yellow
        Write-Host "  7. Запустить контейнер" -ForegroundColor Yellow
        Write-Host "  8. Остановить контейнер" -ForegroundColor Yellow
        Write-Host "  9. Перезапустить контейнер" -ForegroundColor Yellow
        Write-Host " 10. Открыть приложение" -ForegroundColor Yellow
        Write-Host " 11. Удалить контейнер" -ForegroundColor Yellow
        Write-Host " 12. Очистка Docker" -ForegroundColor Yellow
        Write-Host "  0. Выход" -ForegroundColor Red
        Write-Host "==================================================" -ForegroundColor Cyan
        Write-Host "Образ: $($script:ImageReference) | Контейнер: $ContainerName" -ForegroundColor DarkGray

        $choice = (Read-Host "Выберите действие [0-12]").Trim()
        $selectedAction = switch ($choice) {
            "1"  { "Doctor" }
            "2"  { "Build" }
            "3"  { "BuildDeploy" }
            "4"  { "Deploy" }
            "5"  { "Status" }
            "6"  { "Logs" }
            "7"  { "Start" }
            "8"  { "Stop" }
            "9"  { "Restart" }
            "10" { "Open" }
            "11" { "Remove" }
            "12" { "Cleanup" }
            "0"  { "Exit" }
            default { "Invalid" }
        }

        if ($selectedAction -eq "Exit") {
            Write-Event -Message "Выход из панели управления." -Level Success
            return
        }

        if ($selectedAction -eq "Invalid") {
            Write-Event -Message "Неверный пункт меню." -Level Error
            Start-Sleep -Seconds 1
            continue
        }

        try {
            Invoke-SelectedAction -SelectedAction $selectedAction
        }
        catch {
            Write-Event -Message $_.Exception.Message -Level Error
            Write-Event -Message "Подробности сохранены в $($script:LogDirectory)" -Level Warning
        }

        Pause-Console
    }
}

Initialize-Console
Set-Location -LiteralPath $script:ProjectRoot

try {
    if ($Action -eq "Menu") {
        if (-not $script:IsInteractive) {
            throw "Action=Menu недоступен в неинтерактивном режиме. Укажите конкретное -Action."
        }
        Show-MainMenu
    }
    else {
        Invoke-SelectedAction -SelectedAction $Action
    }
}
catch {
    Write-Event -Message $_.Exception.Message -Level Error
    Write-Event -Message "Журналы: $($script:LogDirectory)" -Level Warning
    exit 1
}
finally {
    if (Test-Path -LiteralPath $script:OriginalLocation -PathType Container) {
        Set-Location -LiteralPath $script:OriginalLocation
    }
}

