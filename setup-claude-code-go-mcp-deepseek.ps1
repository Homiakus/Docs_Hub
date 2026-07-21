#Requires -Version 7.0
<#
.SYNOPSIS
  Interactive Claude Code + Go MCP + DeepSeek setup for PowerShell 7+.

.DESCRIPTION
  Installs/configures a practical Claude Code MCP stack for Go development:
  - gopls MCP
  - Serena MCP
  - Repomix MCP
  - Context7 MCP
  Optional:
  - Playwright MCP
  - Semgrep MCP
  - GitHub MCP

  Also configures Claude Code to use DeepSeek through Anthropic-compatible
  environment variables.

  Advanced tool pack (install + combined usage):
  - Spec Kit (GitHub): spec-driven development with /speckit.* commands
  - ECC (Enhanced Claude Code): 261+ skills, 64 agents, hooks, orchestration
  - Supermemory: persistent AI memory via MCP server

  All three combine into a powerful workflow:
      /speckit.specify → /speckit.plan → /speckit.tasks → ECC skills execute → Supermemory remembers context

.NOTES
  Run from PowerShell 7+:
    pwsh -ExecutionPolicy Bypass -File .\setup-claude-code-go-mcp-deepseek.ps1

  Dry-run:
    pwsh -ExecutionPolicy Bypass -File .\setup-claude-code-go-mcp-deepseek.ps1 -DryRun
#>

[CmdletBinding()]
param(
    [switch]$DryRun,
    [ValidateSet('User', 'Project')]
    [string]$DefaultMcpScope = 'User',
    [switch]$NoProfileWrite
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Script:DeepSeekKeys = @(
    'ANTHROPIC_BASE_URL',
    'ANTHROPIC_AUTH_TOKEN',
    'ANTHROPIC_MODEL',
    'ANTHROPIC_DEFAULT_OPUS_MODEL',
    'ANTHROPIC_DEFAULT_SONNET_MODEL',
    'ANTHROPIC_DEFAULT_HAIKU_MODEL',
    'CLAUDE_CODE_SUBAGENT_MODEL',
    'CLAUDE_CODE_EFFORT_LEVEL'
)

$Script:ProfileBegin = '# >>> claude-code-deepseek >>>'
$Script:ProfileEnd   = '# <<< claude-code-deepseek <<<'

function Write-Title
{
    param([string]$Text)
    Write-Host ''
    Write-Host ('=' * 78) -ForegroundColor DarkCyan
    Write-Host $Text -ForegroundColor Cyan
    Write-Host ('=' * 78) -ForegroundColor DarkCyan
}

function Write-Info
{ param([string]$Text) Write-Host "[i] $Text" -ForegroundColor Gray
}
function Write-Ok
{ param([string]$Text) Write-Host "[OK] $Text" -ForegroundColor Green
}
function Write-Warn
{ param([string]$Text) Write-Host "[!] $Text" -ForegroundColor Yellow
}
function Write-Err
{ param([string]$Text) Write-Host "[x] $Text" -ForegroundColor Red
}

function Test-Cmd
{
    param([Parameter(Mandatory)][string]$Name)
    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Confirm-YesNo
{
    param(
        [Parameter(Mandatory)][string]$Question,
        [bool]$DefaultYes = $true
    )
    $suffix = if ($DefaultYes)
    { 'Y/n'
    } else
    { 'y/N'
    }
    while ($true)
    {
        $answer = Read-Host "$Question [$suffix]"
        if ([string]::IsNullOrWhiteSpace($answer))
        { return $DefaultYes
        }
        switch -Regex ($answer.Trim())
        {
            '^(y|yes|д|да)$'
            { return $true
            }
            '^(n|no|н|нет)$'
            { return $false
            }
            default
            { Write-Warn 'Ответь y/n или да/нет.'
            }
        }
    }
}

function Read-MenuChoice
{
    param(
        [Parameter(Mandatory)][string]$Prompt,
        [Parameter(Mandatory)][string[]]$Allowed,
        [string]$Default = ''
    )
    while ($true)
    {
        $suffix = if ($Default)
        { " [$Default]"
        } else
        { ''
        }
        $value = Read-Host "$Prompt$suffix"
        if ([string]::IsNullOrWhiteSpace($value) -and $Default)
        { $value = $Default
        }
        if ($Allowed -contains $value)
        { return $value
        }
        Write-Warn "Выбери: $($Allowed -join ', ')"
    }
}

function Invoke-Step
{
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter()][string[]]$ArgumentList = @(),
        [Parameter(Mandatory)][string]$Description,
        [switch]$AllowFailure
    )

    Write-Host ''
    Write-Info $Description
    Write-Host "  > $FilePath $($ArgumentList -join ' ')" -ForegroundColor DarkGray

    if ($DryRun)
    {
        Write-Warn 'DRY RUN: команда не выполнена.'
        return $true
    }

    & $FilePath @ArgumentList
    $exitCode = if ($null -eq $LASTEXITCODE)
    { 0
    } else
    { $LASTEXITCODE
    }
    if ($exitCode -ne 0)
    {
        $msg = "Команда завершилась с кодом ${exitCode}: $Description"
        if ($AllowFailure)
        {
            Write-Warn $msg
            return $false
        }
        throw $msg
    }
    return $true
}

function Refresh-SessionPath
{
    $paths = New-Object System.Collections.Generic.List[string]

    if ($IsWindows)
    {
        $candidates = @(
            "$env:APPDATA\npm",
            "$env:USERPROFILE\go\bin",
            "$env:USERPROFILE\.local\bin",
            "$env:USERPROFILE\.cargo\bin"
        )
    } else
    {
        $homeDir = $HOME
        $candidates = @(
            "$homeDir/go/bin",
            "$homeDir/.local/bin",
            "$homeDir/.cargo/bin"
        )
    }

    foreach ($p in $candidates)
    {
        if ($p -and (Test-Path $p) -and (($env:PATH -split [IO.Path]::PathSeparator) -notcontains $p))
        {
            $paths.Add($p) | Out-Null
        }
    }

    if ($paths.Count -gt 0)
    {
        $env:PATH = $env:PATH + [IO.Path]::PathSeparator + ($paths -join [IO.Path]::PathSeparator)
        Write-Info "PATH обновлён для текущей сессии: $($paths -join ', ')"
    }
}

function Ensure-Node
{
    if ((Test-Cmd node) -and (Test-Cmd npm) -and (Test-Cmd npx))
    {
        Write-Ok "Node/npm/npx найдены: $(node --version)"
        return
    }

    Write-Warn 'Node.js/npm/npx не найдены. Они нужны для Claude Code, Repomix, Context7 и Playwright MCP.'

    if ($IsWindows -and (Test-Cmd winget) -and (Confirm-YesNo 'Установить Node.js LTS через winget?' $true))
    {
        Invoke-Step winget @('install', '-e', '--id', 'OpenJS.NodeJS.LTS') 'Установка Node.js LTS через winget'
        Refresh-SessionPath
        return
    }

    if ($IsMacOS -and (Test-Cmd brew) -and (Confirm-YesNo 'Установить Node.js через Homebrew?' $true))
    {
        Invoke-Step brew @('install', 'node') 'Установка Node.js через Homebrew'
        Refresh-SessionPath
        return
    }

    Write-Warn 'Установи Node.js 18+ вручную, затем запусти скрипт снова.'
}

function Ensure-Go
{
    if (Test-Cmd go)
    {
        Write-Ok "Go найден: $(go version)"
        return
    }

    Write-Warn 'Go не найден. Он нужен для gopls MCP.'

    if ($IsWindows -and (Test-Cmd winget) -and (Confirm-YesNo 'Установить Go через winget?' $true))
    {
        Invoke-Step winget @('install', '-e', '--id', 'GoLang.Go') 'Установка Go через winget'
        Refresh-SessionPath
        return
    }

    if ($IsMacOS -and (Test-Cmd brew) -and (Confirm-YesNo 'Установить Go через Homebrew?' $true))
    {
        Invoke-Step brew @('install', 'go') 'Установка Go через Homebrew'
        Refresh-SessionPath
        return
    }

    Write-Warn 'Установи Go вручную, затем запусти скрипт снова.'
}

function Ensure-Uv
{
    Refresh-SessionPath
    if (Test-Cmd uv)
    {
        Write-Ok "uv найден: $(uv --version)"
        return
    }

    Write-Warn 'uv не найден. Он нужен для Serena и Semgrep MCP через uvx.'
    if (-not (Confirm-YesNo 'Установить uv официальным install-скриптом Astral?' $true))
    { return
    }

    if ($IsWindows)
    {
        if ($DryRun)
        {
            Write-Warn 'DRY RUN: irm https://astral.sh/uv/install.ps1 | iex'
        } else
        {
            Write-Info 'Установка uv через https://astral.sh/uv/install.ps1'
            irm https://astral.sh/uv/install.ps1 | iex
        }
    } else
    {
        if (-not (Test-Cmd curl))
        { throw 'curl не найден; установи curl или uv вручную.'
        }
        Invoke-Step sh @('-c', 'curl -LsSf https://astral.sh/uv/install.sh | sh') 'Установка uv через официальный shell installer'
    }
    Refresh-SessionPath
}

function Ensure-ClaudeCode
{
    Refresh-SessionPath
    if (Test-Cmd claude)
    {
        Write-Ok "Claude Code найден: $(claude --version 2>$null)"
        return
    }

    Ensure-Node
    if (-not (Test-Cmd npm))
    {
        Write-Warn 'npm всё ещё не найден. Claude Code не установлен.'
        return
    }

    if (Confirm-YesNo 'Установить Claude Code глобально через npm?' $true)
    {
        Invoke-Step npm @('install', '-g', '@anthropic-ai/claude-code') 'Установка Claude Code CLI'
        Refresh-SessionPath
    }
}

function Install-GoplsTool
{
    Ensure-Go
    if (-not (Test-Cmd go))
    { return
    }
    Invoke-Step go @('install', 'golang.org/x/tools/gopls@latest') 'Установка/обновление gopls'
    Refresh-SessionPath
    if (Test-Cmd gopls)
    { Write-Ok "gopls готов: $(gopls version 2>$null | Select-Object -First 1)"
    }
}

function Install-SerenaTool
{
    Ensure-Uv
    if (-not (Test-Cmd uv))
    { return
    }
    Invoke-Step uv @('tool', 'install', '-p', '3.13', 'serena-agent') 'Установка/обновление Serena через uv tool'
    Refresh-SessionPath
    if (Test-Cmd serena)
    {
        Invoke-Step serena @('init') 'Инициализация Serena' -AllowFailure
    } else
    {
        Write-Warn 'Команда serena не найдена после установки. Проверь PATH: ~/.local/bin или %USERPROFILE%\.local\bin.'
    }
}

function Get-McpScopeArgs
{
    param([ValidateSet('User', 'Project')][string]$Scope = $DefaultMcpScope)
    if ($Scope -eq 'User')
    { return @('--scope', 'user')
    }
    return @()
}

function Add-ClaudeMcpServer
{
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$Command,
        [Parameter()][string[]]$ServerArgs = @(),
        [ValidateSet('User', 'Project')][string]$Scope = $DefaultMcpScope
    )

    Ensure-ClaudeCode
    if (-not (Test-Cmd claude))
    {
        Write-Warn "Claude Code не найден; MCP '$Name' не добавлен."
        return
    }

    $args = @('mcp', 'add') + (Get-McpScopeArgs -Scope $Scope) + @($Name, '--', $Command) + $ServerArgs
    Invoke-Step claude $args "Добавление MCP '$Name' в Claude Code ($Scope scope)" -AllowFailure
}

function Install-McpGopls
{
    Install-GoplsTool
    Add-ClaudeMcpServer -Name 'gopls' -Command 'gopls' -ServerArgs @('mcp')
}

function Install-McpSerena
{
    Install-SerenaTool
    if (-not (Test-Cmd serena))
    { return
    }

    # Official setup command is convenient and may configure client-specific extras.
    if (Confirm-YesNo 'Запустить официальный serena setup claude-code?' $true)
    {
        Invoke-Step serena @('setup', 'claude-code') 'Официальная настройка Serena для Claude Code' -AllowFailure
    }

    # Also register explicitly as MCP for this scope if the user wants a predictable Claude MCP entry.
    if (Confirm-YesNo "Добавить Serena как явный MCP server через claude mcp add ($DefaultMcpScope scope)?" $true)
    {
        Add-ClaudeMcpServer -Name 'serena' -Command 'serena' -ServerArgs @('start-mcp-server', '--context', 'claude-code', '--project-from-cwd')
    }
}

function Install-McpRepomix
{
    Ensure-Node
    Add-ClaudeMcpServer -Name 'repomix' -Command 'npx' -ServerArgs @('-y', 'repomix', '--mcp')
}

function Install-McpContext7
{
    Ensure-Node
    Write-Host ''
    Write-Info 'Context7 API key необязателен, но даёт более высокие лимиты.'
    $apiKey = Read-Host 'Context7 API key, Enter чтобы пропустить'
    $args = @('-y', '@upstash/context7-mcp@latest')
    if (-not [string]::IsNullOrWhiteSpace($apiKey))
    {
        Write-Warn 'API key будет сохранён в конфигурации MCP как аргумент запуска. Не коммить .mcp.json.'
        $args += @('--api-key', $apiKey.Trim())
    }
    Add-ClaudeMcpServer -Name 'context7' -Command 'npx' -ServerArgs $args
}

function Install-McpPlaywright
{
    Ensure-Node
    Add-ClaudeMcpServer -Name 'playwright' -Command 'npx' -ServerArgs @('@playwright/mcp@latest')
}

function Install-McpSemgrep
{
    Ensure-Uv
    if (-not (Test-Cmd uvx))
    {
        Write-Warn 'uvx не найден; попробую через uv tool run, но лучше обновить uv.'
    }
    Write-Info 'Semgrep MCP можно использовать без токена. Токен Semgrep AppSec Platform опционален.'
    $token = Read-Host 'SEMGREP_APP_TOKEN, Enter чтобы пропустить'

    if ([string]::IsNullOrWhiteSpace($token))
    {
        Add-ClaudeMcpServer -Name 'semgrep' -Command 'uvx' -ServerArgs @('semgrep-mcp')
    } else
    {
        Write-Warn 'Токен будет сохранён в Claude MCP конфигурации. Для приватных проектов лучше использовать user scope.'
        # Claude mcp add currently has limited env handling through simple CLI; use add-json for env.
        $json = @{
            command = 'uvx'
            args = @('semgrep-mcp')
            env = @{ SEMGREP_APP_TOKEN = $token.Trim() }
        } | ConvertTo-Json -Compress -Depth 8
        Invoke-Step claude @('mcp', 'add-json', 'semgrep', $json) 'Добавление Semgrep MCP через add-json' -AllowFailure
    }

    Write-Info 'Альтернатива: в Claude Code запусти /plugin → Discover → Semgrep → Install → /setup-semgrep-plugin.'
}

function Install-McpGitHub
{
    Ensure-ClaudeCode
    Write-Warn 'GitHub MCP требует токен. Минимизируй scopes: repo read/PR/issues только если нужны.'
    $token = Read-Host 'GitHub PAT, Enter чтобы пропустить'
    if ([string]::IsNullOrWhiteSpace($token))
    {
        Write-Warn 'GitHub MCP пропущен.'
        return
    }

    $json = @{
        type = 'http'
        url = 'https://api.githubcopilot.com/mcp'
        headers = @{ Authorization = "Bearer $($token.Trim())" }
    } | ConvertTo-Json -Compress -Depth 8

    Write-Warn 'Токен будет сохранён в конфигурации Claude Code MCP. Не коммить project .mcp.json.'
    Invoke-Step claude @('mcp', 'add-json', 'github', $json) 'Добавление GitHub MCP remote server через add-json' -AllowFailure
}

# ── Spec Kit (GitHub) ──────────────────────────────────────────────
function Get-LatestSpecKitVersion
{
    param([int]$MaxRetries = 2)
    $tag = $null
    for ($attempt = 1; $attempt -le $MaxRetries; $attempt++)
    {
        try
        {
            $releases = Invoke-RestMethod -Uri 'https://api.github.com/repos/github/spec-kit/releases?per_page=5' -TimeoutSec 10 -ErrorAction Stop
            foreach ($r in $releases)
            {
                if (-not $r.prerelease -and $r.tag_name -match '^v')
                {
                    $tag = $r.tag_name
                    break
                }
            }
            if ($tag)
            { break
            }
        } catch
        {
            if ($attempt -eq $MaxRetries)
            { Write-Warn 'Не удалось получить список релизов Spec Kit с GitHub API.'
            }
        }
    }
    if (-not $tag)
    { $tag = 'v0.0.99'
    } # fallback
    return $tag
}

function Install-SpecKit
{
    Write-Title 'Spec Kit (GitHub) — Spec-Driven Development'
    Write-Info 'Spec Kit — open-source toolkit для spec-driven development (SDD).'
    Write-Info 'Даёт /speckit.specify, /speckit.plan, /speckit.tasks, /speckit.implement и др.'
    Write-Info 'Репозиторий: https://github.com/github/spec-kit'

    Ensure-Uv
    if (-not (Test-Cmd uv))
    {
        Write-Warn 'uv не найден. Spec Kit требует uv для установки.'
        return
    }

    if (Test-Cmd specify)
    {
        Write-Ok "Specify CLI уже установлен: $(specify --version 2>$null)"
        if (-not (Confirm-YesNo 'Обновить specify-cli до последней версии?' $false))
        { return
        }
    }

    $latestTag = Get-LatestSpecKitVersion
    Write-Info "Установка specify-cli из git (тег: $latestTag)..."

    Invoke-Step uv @('tool', 'install', 'specify-cli', '--from', "git+https://github.com/github/spec-kit.git@$latestTag") `
        'Установка specify-cli (Spec Kit CLI)' -AllowFailure
    Refresh-SessionPath

    if (-not (Test-Cmd specify))
    {
        Write-Warn 'specify не найден после установки. Проверь PATH: ~/.local/bin или %USERPROFILE%\.local\bin.'
        return
    }

    Write-Ok "Specify CLI готов: $(specify --version 2>$null)"

    # Initialize for current project with Claude Code integration
    if (Confirm-YesNo "Инициализировать Spec Kit в текущей директории ($(Get-Location)) для Claude Code?" $true)
    {
        # Check if .specify already exists
        if ((Test-Path '.specify') -and -not (Confirm-YesNo '.specify уже существует. Перезаписать?' $false))
        {
            Write-Warn 'Инициализация пропущена.'
        } else
        {
            Invoke-Step specify @('init', '.', '--integration', 'claude', '--force') `
                'Инициализация Spec Kit для Claude Code' -AllowFailure
        }
    }

    Write-Host ''
    Write-Info 'Команды Spec Kit (после инициализации в Claude Code):'
    Write-Host '  /speckit.constitution  — принципы проекта' -ForegroundColor DarkGray
    Write-Host '  /speckit.specify       — что строим' -ForegroundColor DarkGray
    Write-Host '  /speckit.plan          — технический план' -ForegroundColor DarkGray
    Write-Host '  /speckit.tasks         — список задач' -ForegroundColor DarkGray
    Write-Host '  /speckit.implement      — выполнить реализацию' -ForegroundColor DarkGray
    Write-Host '  /speckit.clarify        — уточнить требования' -ForegroundColor DarkGray
    Write-Host '  /speckit.analyze        — кросс-артефакт анализ' -ForegroundColor DarkGray
    Write-Host '  /speckit.checklist      — чеклист качества' -ForegroundColor DarkGray
}

# ── ECC (Enhanced Claude Code) ─────────────────────────────────────
function Install-ECC
{
    Write-Title 'ECC (Enhanced Claude Code) — Agent Harness Operating System'
    Write-Info 'ECC — система навыков, агентов, хуков и MCP-конвенций для Claude Code.'
    Write-Info '261+ навыков, 64 агента, 84 команды, кросс-харнесс оркестрация.'
    Write-Info 'Репозиторий: https://github.com/affaan-m/ECC'

    Ensure-Node
    if (-not (Test-Cmd npx))
    {
        Write-Warn 'npx не найден. ECC требует Node.js/npm.'
        return
    }

    Write-Host ''
    Write-Host 'Выбери способ установки ECC:' -ForegroundColor Cyan
    Write-Host '  1) Claude Code plugin (рекомендуемый): /plugin marketplace add + /plugin install'
    Write-Host '  2) npx быстрая установка: npx ecc-install --profile core --target claude'
    Write-Host '  3) npx минимальная установка (без хуков): npx ecc-install --profile minimal --target claude'
    Write-Host '  4) Git clone + полный контроль'
    $choice = Read-MenuChoice 'Выбор' @('1','2','3','4') '1'

    switch ($choice)
    {
        '1'
        {
            Write-Info 'Вариант 1: Plugin install (выполни эти команды внутри Claude Code):'
            Write-Host ''
            Write-Host '  /plugin marketplace add https://github.com/affaan-m/ECC' -ForegroundColor Yellow
            Write-Host '  /plugin install ecc@ecc' -ForegroundColor Yellow
            Write-Host ''
            Write-Info 'После установки плагина, скопируй нужные rules вручную:'
            Write-Host '  mkdir -p ~/.claude/rules/ecc' -ForegroundColor DarkGray
            Write-Host '  cp -R <ECC_clone>/rules/common ~/.claude/rules/ecc/' -ForegroundColor DarkGray
            Write-Host '  cp -R <ECC_clone>/rules/golang ~/.claude/rules/ecc/' -ForegroundColor DarkGray

            # Try the plugin install via Claude Code CLI if available
            if ((Test-Cmd claude) -and (Confirm-YesNo 'Попробовать добавить marketplace и plugin через Claude Code CLI сейчас?' $true))
            {
                Invoke-Step claude @('plugin', 'marketplace', 'add', 'https://github.com/affaan-m/ECC') `
                    'Добавление ECC marketplace' -AllowFailure
                Invoke-Step claude @('plugin', 'install', 'ecc@ecc') `
                    'Установка ECC plugin' -AllowFailure
            }
        }
        '2'
        {
            Invoke-Step npx @('ecc-install', '--profile', 'core', '--target', 'claude') `
                'npx ecc-install --profile core --target claude' -AllowFailure
        }
        '3'
        {
            Invoke-Step npx @('ecc-install', '--profile', 'minimal', '--target', 'claude') `
                'npx ecc-install --profile minimal --target claude (без хуков)' -AllowFailure
        }
        '4'
        {
            $cloneDir = Read-Host 'Директория для клонирования ECC (Enter = ./.ecc)'
            if ([string]::IsNullOrWhiteSpace($cloneDir))
            { $cloneDir = '.ecc'
            }
            $cloneDir = Join-Path (Get-Location) $cloneDir
            if (Test-Path $cloneDir)
            {
                Write-Warn "$cloneDir уже существует."
                if (Confirm-YesNo 'Сделать git pull?' $true)
                {
                    Push-Location $cloneDir
                    Invoke-Step git @('pull') 'git pull ECC' -AllowFailure
                    Pop-Location
                }
            } else
            {
                Invoke-Step git @('clone', 'https://github.com/affaan-m/ECC.git', $cloneDir) 'Клонирование ECC'
                Push-Location $cloneDir
                Invoke-Step npm @('install', '--no-audit', '--no-fund', '--loglevel=error') 'Установка зависимостей ECC' -AllowFailure
                Pop-Location
            }
            if (Confirm-YesNo 'Запустить полную установку (install.ps1 --profile full)?' $false)
            {
                $installScript = Join-Path $cloneDir 'install.ps1'
                if (Test-Path $installScript)
                {
                    Invoke-Step pwsh @('-ExecutionPolicy', 'Bypass', '-File', $installScript, '--profile', 'full') `
                        'ECC install.ps1 --profile full' -AllowFailure
                }
            }
        }
    }

    Write-Host ''
    Write-Info 'ECC готов к использованию. Проверь в Claude Code:'
    Write-Host '  /ecc:plan "Your feature description"' -ForegroundColor DarkGray
    Write-Host '  npx ecc consult "your query" --target claude' -ForegroundColor DarkGray
    Write-Host '  npx ecc status --markdown --write status.md' -ForegroundColor DarkGray
}

# ── Supermemory ─────────────────────────────────────────────────────
function Install-Supermemory
{
    Write-Title 'Supermemory — Persistent AI Memory'
    Write-Info 'Supermemory — memory engine для AI. Даёт персистентную память через MCP server.'
    Write-Info '#1 на LongMemEval, LoCoMo, ConvoMem — трёх главных бенчмарках AI-памяти.'
    Write-Info 'Репозиторий: https://github.com/supermemoryai/supermemory'
    Write-Info 'Документация: https://supermemory.ai/docs'

    Ensure-Node
    if (-not (Test-Cmd npx))
    {
        Write-Warn 'npx не найден. Supermemory MCP требует Node.js/npm.'
        return
    }

    Write-Host ''
    Write-Host 'Выбери способ установки Supermemory:' -ForegroundColor Cyan
    Write-Host '  1) MCP quick install (рекомендуемый): auto-configure для Claude Code'
    Write-Host '  2) MCP manual config: добавить URL в MCP конфигурацию'
    Write-Host '  3) OAuth-based MCP (без API ключа)'
    Write-Host '  4) Claude Code plugin: https://github.com/supermemoryai/claude-supermemory'
    $choice = Read-MenuChoice 'Выбор' @('1','2','3','4') '1'

    switch ($choice)
    {
        '1'
        {
            Write-Info 'Быстрая установка MCP с OAuth...'
            Invoke-Step npx @('-y', 'install-mcp@latest', 'https://mcp.supermemory.ai/mcp', '--client', 'claude', '--oauth=yes') `
                'Supermemory MCP quick install (OAuth)' -AllowFailure
        }
        '2'
        {
            $apiKey = Read-Host 'Supermemory API key (sm_...), Enter чтобы пропустить'
            if (-not [string]::IsNullOrWhiteSpace($apiKey))
            {
                $json = @{
                    url = 'https://mcp.supermemory.ai/mcp'
                    headers = @{ Authorization = "Bearer $($apiKey.Trim())" }
                } | ConvertTo-Json -Compress -Depth 8
                Write-Warn 'API key будет сохранён в конфигурации MCP.'
                Invoke-Step claude @('mcp', 'add-json', 'supermemory', $json) `
                    'Добавление Supermemory MCP через add-json (API key)' -AllowFailure
            } else
            {
                Write-Info 'Добавление Supermemory MCP без API ключа (потребуется OAuth при первом вызове).'
                $json = @{
                    url = 'https://mcp.supermemory.ai/mcp'
                } | ConvertTo-Json -Compress -Depth 8
                Invoke-Step claude @('mcp', 'add-json', 'supermemory', $json) `
                    'Добавление Supermemory MCP через add-json (no auth)' -AllowFailure
            }
        }
        '3'
        {
            Write-Info 'Установка через OAuth (рекомендуется для личного использования)...'
            $json = @{ url = 'https://mcp.supermemory.ai/mcp' } | ConvertTo-Json -Compress -Depth 8
            Invoke-Step claude @('mcp', 'add-json', 'supermemory', $json) `
                'Добавление Supermemory MCP (OAuth)' -AllowFailure
            Write-Info 'При первом обращении к MCP откроется окно браузера для OAuth-авторизации.'
        }
        '4'
        {
            Write-Info 'Claude Code plugin для Supermemory:'
            Write-Host '  Репозиторий: https://github.com/supermemoryai/claude-supermemory' -ForegroundColor DarkGray
            Write-Host '  Внутри Claude Code:' -ForegroundColor DarkGray
            Write-Host '    /plugin install supermemory@supermemory' -ForegroundColor Yellow
            Write-Host '  Или клонируй и установи вручную.' -ForegroundColor DarkGray
        }
    }

    Write-Host ''
    Write-Info 'Supermemory установлен. В Claude Code доступны инструменты:'
    Write-Host '  memory  — сохранить/забыть информацию' -ForegroundColor DarkGray
    Write-Host '  recall  — поиск по памяти' -ForegroundColor DarkGray
    Write-Host '  context — инжектировать профиль пользователя в контекст' -ForegroundColor DarkGray
    Write-Host '  /context — в интерактивном режиме Claude Code' -ForegroundColor DarkGray

    # Optionally install the Python/Node SDK for programmatic use
    if (Confirm-YesNo 'Установить Supermemory SDK (npm supermemory) для кода?' $false)
    {
        Ensure-Node
        Invoke-Step npm @('install', '-g', 'supermemory') 'Установка Supermemory npm SDK' -AllowFailure
    }
}

# ── Advanced Tools Pack (все три) ──────────────────────────────────
function Install-AdvancedToolsPack
{
    Write-Title 'Установка ADVANCED TOOLS PACK'
    Write-Host ''
    Write-Host '  Этот набор объединяет три мощных инструмента:' -ForegroundColor Cyan
    Write-Host ''
    Write-Host '  1) Spec Kit   — structured spec → plan → tasks → implement' -ForegroundColor DarkCyan
    Write-Host '  2) ECC        — 261+ навыков, 64 агента, хуки, оркестрация' -ForegroundColor DarkCyan
    Write-Host '  3) Supermemory — персистентная AI-память через MCP' -ForegroundColor DarkCyan
    Write-Host ''
    Write-Host '  Сильное сочетание (мощный воркфлоу):' -ForegroundColor Green
    Write-Host '  ┌─ /speckit.specify   ← Что строим (Spec Kit)' -ForegroundColor DarkGray
    Write-Host '  ├─ /speckit.clarify   ← Уточняем требования (Spec Kit)' -ForegroundColor DarkGray
    Write-Host '  ├─ /speckit.plan      ← Архитектура + стек (Spec Kit)' -ForegroundColor DarkGray
    Write-Host '  ├─ /speckit.tasks     ← Разбивка на задачи (Spec Kit)' -ForegroundColor DarkGray
    Write-Host '  ├─ /speckit.checklist  ← Чеклист качества (Spec Kit)' -ForegroundColor DarkGray
    Write-Host '  ├─ ECC skills          ← Исполнение: coding, TDD, review, security' -ForegroundColor DarkGray
    Write-Host '  ├─ ECC hooks           ← Авто-запуск проверок при save/commit' -ForegroundColor DarkGray
    Write-Host '  └─ Supermemory         ← Весь контекст помнится между сессиями' -ForegroundColor DarkGray
    Write-Host ''

    Write-Info 'Устанавливаем все три инструмента последовательно...'

    # 1. Spec Kit
    if (Confirm-YesNo 'Установить Spec Kit (spec-driven development)?' $true)
    { Install-SpecKit
    }

    # 2. ECC
    if (Confirm-YesNo 'Установить ECC (Enhanced Claude Code)?' $true)
    { Install-ECC
    }

    # 3. Supermemory
    if (Confirm-YesNo 'Установить Supermemory (AI memory)?' $true)
    { Install-Supermemory
    }

    Write-Title 'ADVANCED TOOLS PACK — ГОТОВО'
    Show-CombinedWorkflowGuide
}

function Show-CombinedWorkflowGuide
{
    Write-Host ''
    Write-Host '════════════════════════════════════════════════════════════' -ForegroundColor Green
    Write-Host '  МОЩНЫЙ КОМБИНИРОВАННЫЙ ВОРКФЛОУ' -ForegroundColor Green
    Write-Host '════════════════════════════════════════════════════════════' -ForegroundColor Green
    Write-Host ''

    Write-Host '  Этап 1: Спецификация (Spec Kit)' -ForegroundColor Cyan
    Write-Host '  ─────────────────────────────────' -ForegroundColor DarkCyan
    Write-Host '  /speckit.constitution \' -ForegroundColor Yellow
    Write-Host '      "Принципы: код-ревью на каждый PR, TDD, покрытие > 80%' -ForegroundColor DarkGray
    Write-Host '  /speckit.specify \' -ForegroundColor Yellow
    Write-Host '      "Описание фичи: что и зачем строим"' -ForegroundColor DarkGray
    Write-Host '  /speckit.clarify' -ForegroundColor Yellow
    Write-Host ''

    Write-Host '  Этап 2: Планирование (Spec Kit)' -ForegroundColor Cyan
    Write-Host '  ─────────────────────────────────' -ForegroundColor DarkCyan
    Write-Host '  /speckit.plan \' -ForegroundColor Yellow
    Write-Host '      "Go 1.24, SQLite, HTMX, Docker. Минимум зависимостей."' -ForegroundColor DarkGray
    Write-Host '  /speckit.tasks' -ForegroundColor Yellow
    Write-Host '  /speckit.checklist' -ForegroundColor Yellow
    Write-Host ''

    Write-Host '  Этап 3: Исполнение (ECC skills)' -ForegroundColor Cyan
    Write-Host '  ─────────────────────────────────' -ForegroundColor DarkCyan
    Write-Host '  /ecc:plan "Создать модель User с валидацией"' -ForegroundColor Yellow
    Write-Host '  # ECC авто-запускает TDD, review, security scan через хуки' -ForegroundColor DarkGray
    Write-Host '  # npx ecc status --markdown --write status.md  (мониторинг)' -ForegroundColor DarkGray
    Write-Host '  /speckit.implement   # Или запустить всё сразу из Spec Kit' -ForegroundColor Yellow
    Write-Host ''

    Write-Host '  Этап 4: Память (Supermemory)' -ForegroundColor Cyan
    Write-Host '  ─────────────────────────────────' -ForegroundColor DarkCyan
    Write-Host '  # Supermemory автоматически помнит:' -ForegroundColor DarkGray
    Write-Host '  #   - Принятые архитектурные решения' -ForegroundColor DarkGray
    Write-Host '  #   - Предпочтения по стилю кода' -ForegroundColor DarkGray
    Write-Host '  #   - Контекст проекта между сессиями' -ForegroundColor DarkGray
    Write-Host '  # Используй /context чтобы загрузить профиль в новой сессии' -ForegroundColor Yellow
    Write-Host ''

    Write-Host '  Быстрые команды для мониторинга:' -ForegroundColor Cyan
    Write-Host '  ─────────────────────────────────' -ForegroundColor DarkCyan
    Write-Host '  npx ecc status              — общий статус ECC' -ForegroundColor DarkGray
    Write-Host '  npx ecc doctor              — диагностика ECC' -ForegroundColor DarkGray
    Write-Host '  npx ecc list-installed      — что установлено' -ForegroundColor DarkGray
    Write-Host '  npx ecc consult "..."       — поиск навыков под задачу' -ForegroundColor DarkGray
    Write-Host '  specify self check          — проверить обновления Spec Kit' -ForegroundColor DarkGray
    Write-Host '  claude mcp list             — список MCP серверов (вкл. Supermemory)' -ForegroundColor DarkGray
    Write-Host ''

    Write-Host '  Совет: используй Supermemory project IDs для разных проектов,' -ForegroundColor Gray
    Write-Host '  чтобы разделять контекст между работа/личное/разные репозитории.' -ForegroundColor Gray
    Write-Host ''
    Write-Host '════════════════════════════════════════════════════════════' -ForegroundColor Green
}

function Install-Prerequisites
{
    Write-Title 'Проверка и установка базовых зависимостей'
    Ensure-Node
    Ensure-Go
    Ensure-Uv
    Ensure-ClaudeCode
    Refresh-SessionPath
}

function Install-CoreMcpPack
{
    Write-Title 'Установка CORE MCP pack для Go'
    Install-McpGopls
    Install-McpSerena
    Install-McpRepomix
    Install-McpContext7
}

function Install-FullMcpPack
{
    Write-Title 'Установка FULL MCP pack'
    Install-CoreMcpPack
    if (Confirm-YesNo 'Добавить Playwright MCP для браузерной автоматизации/e2e?' $false)
    { Install-McpPlaywright
    }
    if (Confirm-YesNo 'Добавить Semgrep MCP для security scanning?' $true)
    { Install-McpSemgrep
    }
    if (Confirm-YesNo 'Добавить GitHub MCP для PR/issues/actions?' $false)
    { Install-McpGitHub
    }
}

function Convert-SecureStringToPlainText
{
    param([Parameter(Mandatory)][securestring]$SecureString)
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureString)
    try
    { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    } finally
    { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
}

function Get-DeepSeekEnvMap
{
    Write-Title 'Настройка DeepSeek для Claude Code'
    Write-Info 'DeepSeek использует Anthropic-compatible endpoint для Claude Code.'
    Write-Warn 'API key при сохранении в env/profile будет храниться в открытом виде. Это нормально для dev-машины, но не для shared/CI.'

    Write-Host ''
    Write-Host 'Профиль модели:' -ForegroundColor Cyan
    Write-Host '  1) balanced/pro: deepseek-v4-pro[1m] + subagents flash'
    Write-Host '  2) cheap/fast:    deepseek-v4-flash везде'
    Write-Host '  3) custom:        ввести модель вручную'
    $choice = Read-MenuChoice 'Выбор' @('1','2','3') '1'

    switch ($choice)
    {
        '1'
        {
            $mainModel = 'deepseek-v4-pro[1m]'
            $fastModel = 'deepseek-v4-flash'
            $effort = 'max'
        }
        '2'
        {
            $mainModel = 'deepseek-v4-flash'
            $fastModel = 'deepseek-v4-flash'
            $effort = 'medium'
        }
        '3'
        {
            $mainModel = Read-Host 'ANTHROPIC_MODEL, например deepseek-v4-pro[1m]'
            if ([string]::IsNullOrWhiteSpace($mainModel))
            { $mainModel = 'deepseek-v4-pro[1m]'
            }
            $fastModel = Read-Host 'CLAUDE_CODE_SUBAGENT_MODEL, например deepseek-v4-flash'
            if ([string]::IsNullOrWhiteSpace($fastModel))
            { $fastModel = 'deepseek-v4-flash'
            }
            $effort = Read-Host 'CLAUDE_CODE_EFFORT_LEVEL, например max/medium/low'
            if ([string]::IsNullOrWhiteSpace($effort))
            { $effort = 'max'
            }
        }
    }

    $existing = $env:ANTHROPIC_AUTH_TOKEN
    if ($existing)
    {
        if (Confirm-YesNo 'Использовать текущий ANTHROPIC_AUTH_TOKEN из окружения?' $true)
        {
            $apiKey = $existing
        } else
        {
            $secure = Read-Host 'DeepSeek API key' -AsSecureString
            $apiKey = Convert-SecureStringToPlainText $secure
        }
    } else
    {
        $secure = Read-Host 'DeepSeek API key' -AsSecureString
        $apiKey = Convert-SecureStringToPlainText $secure
    }

    if ([string]::IsNullOrWhiteSpace($apiKey))
    { throw 'DeepSeek API key пустой.'
    }

    return [ordered]@{
        ANTHROPIC_BASE_URL = 'https://api.deepseek.com/anthropic'
        ANTHROPIC_AUTH_TOKEN = $apiKey.Trim()
        ANTHROPIC_MODEL = $mainModel.Trim()
        ANTHROPIC_DEFAULT_OPUS_MODEL = $mainModel.Trim()
        ANTHROPIC_DEFAULT_SONNET_MODEL = $mainModel.Trim()
        ANTHROPIC_DEFAULT_HAIKU_MODEL = $fastModel.Trim()
        CLAUDE_CODE_SUBAGENT_MODEL = $fastModel.Trim()
        CLAUDE_CODE_EFFORT_LEVEL = $effort.Trim()
    }
}

function Set-CurrentProcessEnv
{
    param([Parameter(Mandatory)][hashtable]$EnvMap)
    foreach ($key in $EnvMap.Keys)
    {
        Set-Item -Path "Env:$key" -Value ([string]$EnvMap[$key])
    }
    Write-Ok 'DeepSeek env vars установлены для текущей PowerShell-сессии.'
}

function Escape-PowerShellSingleQuotedString
{
    param([string]$Value)
    if ($null -eq $Value)
    { return ''
    }
    return ($Value -replace "'", "''")
}

function Set-ProfileDeepSeekBlock
{
    param([Parameter(Mandatory)][hashtable]$EnvMap)

    if ($NoProfileWrite)
    {
        Write-Warn 'NoProfileWrite включён: профиль PowerShell не изменён.'
        return
    }

    $profilePath = $PROFILE.CurrentUserAllHosts
    $profileDir = Split-Path -Parent $profilePath
    if (-not (Test-Path $profileDir))
    {
        if (-not $DryRun)
        { New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
        }
    }
    if (-not (Test-Path $profilePath))
    {
        if (-not $DryRun)
        { New-Item -ItemType File -Force -Path $profilePath | Out-Null
        }
    }

    $content = ''
    if ((Test-Path $profilePath) -and -not $DryRun)
    { $content = Get-Content -Raw -Path $profilePath
    }
    if ($null -eq $content)
    { $content = ''
    }
    $pattern = [regex]::Escape($Script:ProfileBegin) + '(?s).*?' + [regex]::Escape($Script:ProfileEnd)
    $content = [regex]::Replace($content, $pattern, '').TrimEnd()

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add($Script:ProfileBegin) | Out-Null
    $lines.Add('# Generated by setup-claude-code-go-mcp-deepseek.ps1') | Out-Null
    foreach ($key in $EnvMap.Keys)
    {
        $v = Escape-PowerShellSingleQuotedString ([string]$EnvMap[$key])
        $lines.Add("`$env:$key = '$v'") | Out-Null
    }
    $lines.Add('') | Out-Null
    $lines.Add('function claude-deepseek {') | Out-Null
    $lines.Add('  claude @args') | Out-Null
    $lines.Add('}') | Out-Null
    $lines.Add('') | Out-Null
    $lines.Add('function claude-anthropic {') | Out-Null
    foreach ($key in $Script:DeepSeekKeys)
    {
        $lines.Add("  Remove-Item Env:\$key -ErrorAction SilentlyContinue") | Out-Null
    }
    $lines.Add('  claude @args') | Out-Null
    $lines.Add('}') | Out-Null
    $lines.Add($Script:ProfileEnd) | Out-Null
    $block = $lines -join [Environment]::NewLine

    $newContent = if ([string]::IsNullOrWhiteSpace($content))
    { $block
    } else
    { $content + [Environment]::NewLine + [Environment]::NewLine + $block
    }
    if ($DryRun)
    {
        Write-Warn "DRY RUN: профиль не изменён: $profilePath"
        Write-Host $block -ForegroundColor DarkGray
    } else
    {
        Set-Content -Path $profilePath -Value $newContent -Encoding UTF8
        Write-Ok "DeepSeek block добавлен в PowerShell profile: $profilePath"
    }
}

function Set-WindowsUserEnv
{
    param([Parameter(Mandatory)][hashtable]$EnvMap)
    if (-not $IsWindows)
    {
        Write-Warn 'Windows User env доступен только на Windows. Используй PowerShell profile.'
        return
    }
    foreach ($key in $EnvMap.Keys)
    {
        if ($DryRun)
        {
            Write-Warn "DRY RUN: set user env $key"
        } else
        {
            [Environment]::SetEnvironmentVariable($key, [string]$EnvMap[$key], 'User')
        }
    }
    Write-Ok 'DeepSeek env vars сохранены в Windows User environment. Открой новый терминал, чтобы они применились.'
}

function Remove-ProfileDeepSeekBlock
{
    $profilePath = $PROFILE.CurrentUserAllHosts
    if (-not (Test-Path $profilePath))
    { return
    }
    $content = Get-Content -Raw -Path $profilePath
    if ($null -eq $content)
    { $content = ''
    }
    $pattern = [regex]::Escape($Script:ProfileBegin) + '(?s).*?' + [regex]::Escape($Script:ProfileEnd)
    $newContent = [regex]::Replace($content, $pattern, '').TrimEnd() + [Environment]::NewLine
    if ($DryRun)
    {
        Write-Warn "DRY RUN: удалить DeepSeek block из $profilePath"
    } else
    {
        Set-Content -Path $profilePath -Value $newContent -Encoding UTF8
        Write-Ok "DeepSeek block удалён из PowerShell profile: $profilePath"
    }
}

function Disable-DeepSeekEnv
{
    Write-Title 'Отключение DeepSeek env vars'
    foreach ($key in $Script:DeepSeekKeys)
    {
        Remove-Item "Env:\$key" -ErrorAction SilentlyContinue
        if ($IsWindows -and -not $DryRun)
        {
            [Environment]::SetEnvironmentVariable($key, $null, 'User')
        }
    }
    Remove-ProfileDeepSeekBlock
    Write-Ok 'DeepSeek env vars отключены для текущей сессии; профиль/Windows env очищены где возможно.'
}

function Configure-DeepSeek
{
    $envMap = Get-DeepSeekEnvMap
    Set-CurrentProcessEnv $envMap

    Write-Host ''
    Write-Host 'Как сохранить DeepSeek настройки?' -ForegroundColor Cyan
    Write-Host '  1) Только текущая PowerShell-сессия'
    Write-Host '  2) PowerShell profile: автозагрузка env vars + функции claude-deepseek/claude-anthropic'
    if ($IsWindows)
    { Write-Host '  3) Windows User environment: env vars для новых терминалов'
    }

    $allowed = if ($IsWindows)
    { @('1','2','3')
    } else
    { @('1','2')
    }
    $choice = Read-MenuChoice 'Выбор' $allowed '2'
    switch ($choice)
    {
        '1'
        { Write-Ok 'Оставлено только в текущей сессии.'
        }
        '2'
        { Set-ProfileDeepSeekBlock $envMap
        }
        '3'
        { Set-WindowsUserEnv $envMap
        }
    }

    Write-Host ''
    Write-Ok 'DeepSeek настроен. Проверка: claude /status внутри Claude Code.'
    Write-Info 'Для возврата к обычному Anthropic в текущей сессии: claude-anthropic или меню Disable DeepSeek.'
}

function Get-ClaudeSettingsObject
{
    return [ordered]@{
        permissions = [ordered]@{
            allow = @(
                'Bash(go test ./...)',
                'Bash(go test *)',
                'Bash(go vet ./...)',
                'Bash(go fmt ./...)',
                'Bash(gofmt -w *)',
                'Bash(go mod tidy)',
                'Bash(git status *)',
                'Bash(git diff *)',
                'Bash(git log *)'
            )
            ask = @(
                'Bash(go get *)',
                'Bash(git commit *)',
                'Bash(git push *)',
                'Bash(docker *)',
                'Bash(kubectl *)',
                'Bash(terraform *)',
                'Bash(semgrep *)'
            )
            deny = @(
                'Read(./.env)',
                'Read(./.env.*)',
                'Read(./secrets/**)',
                'Read(./config/credentials.json)',
                'Read(./.aws/**)',
                'Read(./.kube/**)',
                'Bash(sudo *)',
                'Bash(rm -rf *)',
                'Bash(curl * | sh *)',
                'Bash(wget * | sh *)'
            )
        }
    }
}

function Merge-JsonObjectShallow
{
    param(
        [Parameter(Mandatory)]$Base,
        [Parameter(Mandatory)]$Patch
    )
    foreach ($prop in $Patch.PSObject.Properties)
    {
        $Base | Add-Member -NotePropertyName $prop.Name -NotePropertyValue $prop.Value -Force
    }
    return $Base
}

function Write-ClaudeSettings
{
    Write-Title 'Создание безопасных settings.json для Claude Code'
    Write-Host 'Куда записать settings.json?' -ForegroundColor Cyan
    Write-Host '  1) Текущий проект: ./.claude/settings.json'
    Write-Host '  2) User settings: ~/.claude/settings.json'
    $choice = Read-MenuChoice 'Выбор' @('1','2') '1'

    $path = if ($choice -eq '1')
    {
        Join-Path (Get-Location) '.claude/settings.json'
    } else
    {
        Join-Path $HOME '.claude/settings.json'
    }

    $dir = Split-Path -Parent $path
    if (-not (Test-Path $dir) -and -not $DryRun)
    { New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }

    $new = Get-ClaudeSettingsObject
    if (Test-Path $path)
    {
        if (-not (Confirm-YesNo "Файл уже существует: $path. Перезаписать permissions?" $false))
        {
            Write-Warn 'settings.json не изменён.'
            return
        }
    }

    $json = $new | ConvertTo-Json -Depth 12
    if ($DryRun)
    {
        Write-Warn "DRY RUN: settings.json не записан: $path"
        Write-Host $json -ForegroundColor DarkGray
    } else
    {
        Set-Content -Path $path -Value $json -Encoding UTF8
        Write-Ok "Claude Code settings записаны: $path"
    }
}

function Write-ClaudeMd
{
    Write-Title 'Создание CLAUDE.md для Go-проекта'
    $path = Join-Path (Get-Location) 'CLAUDE.md'
    if ((Test-Path $path) -and -not (Confirm-YesNo 'CLAUDE.md уже существует. Перезаписать?' $false))
    {
        Write-Warn 'CLAUDE.md не изменён.'
        return
    }

    $content = @'
# Project rules for Claude Code

## Language and style
- This is a Go project.
- Prefer simple, explicit Go code.
- Keep public APIs small and documented.
- Do not introduce new dependencies without asking.
- Do not change generated files manually.

## Workflow
- Before editing, understand the relevant package structure.
- Prefer gopls and Serena semantic tools before broad grep/read exploration.
- For large context questions, use Repomix first to create a codebase snapshot.
- For external library/API usage, use Context7 before writing code.

## Go commands
- Format with `gofmt` or `go fmt ./...`.
- Run `go test ./...` after meaningful changes.
- Run `go vet ./...` for larger refactors.
- Use `go mod tidy` only when dependencies or imports changed.

## Safety
- Never read `.env`, credentials, private keys, AWS/Kube configs, or secrets.
- Never run destructive commands without explicit confirmation.
- Never push, commit, deploy, or run migrations without explicit confirmation.

## Output
- Explain what changed.
- Mention tests run and their result.
- If tests fail, summarize the failure and the suspected cause.
'@

    if ($DryRun)
    {
        Write-Warn "DRY RUN: CLAUDE.md не записан: $path"
        Write-Host $content -ForegroundColor DarkGray
    } else
    {
        Set-Content -Path $path -Value $content -Encoding UTF8
        Write-Ok "CLAUDE.md записан: $path"
    }
}

function Show-Verification
{
    Write-Title 'Проверка установки'
    foreach ($cmd in @('node', 'npm', 'npx', 'go', 'gopls', 'uv', 'uvx', 'serena', 'claude', 'specify'))
    {
        if (Test-Cmd $cmd)
        {
            Write-Ok "$cmd найден: $((Get-Command $cmd).Source)"
        } else
        {
            Write-Warn "$cmd не найден"
        }
    }

    if (Test-Cmd claude)
    {
        Invoke-Step claude @('mcp', 'list') 'claude mcp list' -AllowFailure
    }

    Write-Host ''
    Write-Info 'DeepSeek env в текущей сессии:'
    foreach ($key in $Script:DeepSeekKeys)
    {
        $value = [Environment]::GetEnvironmentVariable($key, 'Process')
        if ($key -eq 'ANTHROPIC_AUTH_TOKEN' -and $value)
        { $value = '<set, hidden>'
        }
        if ($value)
        { Write-Host "  $key = $value" -ForegroundColor DarkGray
        }
    }

    Write-Host ''
    Write-Info 'В Claude Code проверь: /status и /mcp'
}

function Show-ManualNotes
{
    Write-Title 'Что нужно сделать вручную после скрипта'
    $notes = @'
1) Перезапусти терминал, если ставил Node/Go/uv/Claude Code.
2) Запусти `claude`, авторизуйся или проверь DeepSeek через `/status`.
3) Внутри Claude Code выполни `/mcp`, чтобы увидеть подключенные серверы.
4) Для Semgrep Guardian plugin-варианта:
   - claude
   - /plugin
   - Discover → Semgrep → Install
   - /setup-semgrep-plugin
5) Если выбрал Project scope, не коммить .mcp.json с токенами.
6) Если выбрал User scope, конфиги лежат в user-level Claude config.

ADVANCED TOOLS (после установки):
7) Spec Kit: `specify self check` — проверка обновлений
8) ECC:
   - `/plugin list ecc@ecc` — список команд ECC
   - `npx ecc status` — общий статус
   - `npx ecc doctor` — диагностика
   - `npx ecc consult "golang testing" --target claude` — поиск навыков
9) Supermemory:
   - `/context` в Claude Code — загрузить профиль пользователя
   - memory + recall инструменты доступны через MCP
   - https://supermemory.ai/docs — документация
10) Воркфлоу: /speckit.specify → /speckit.plan → /speckit.tasks →
    ECC skills для исполнения → Supermemory помнит контекст
'@
    Write-Host $notes -ForegroundColor Gray
}

function Show-MainMenu
{
    Write-Host ''
    Write-Host 'Claude Code + Go MCP + DeepSeek + Advanced Tools setup' -ForegroundColor Cyan
    Write-Host '  1) Install prerequisites: Node, Go, uv, Claude Code'
    Write-Host '  2) Install CORE MCP pack: gopls, Serena, Repomix, Context7'
    Write-Host '  3) Install FULL MCP pack: CORE + optional Playwright/Semgrep/GitHub'
    Write-Host '  4) Configure DeepSeek env vars for Claude Code'
    Write-Host '  5) Disable DeepSeek env vars / return to Anthropic'
    Write-Host '  6) Write Claude Code safe settings.json'
    Write-Host '  7) Write CLAUDE.md for current Go project'
    Write-Host '  8) Verify installation'
    Write-Host '  9) Manual notes'
    Write-Host '  ── ADVANCED TOOLS ─────────────────────────────'
    Write-Host '  A) Install Spec Kit (GitHub SDD)'
    Write-Host '  B) Install ECC (Enhanced Claude Code)'
    Write-Host '  C) Install Supermemory (AI Memory MCP)'
    Write-Host '  D) Install ALL Advanced Tools Pack + workflow guide'
    Write-Host '  0) Exit'
}

function Start-InteractiveMenu
{
    Write-Title 'Claude Code + Go MCP + DeepSeek PowerShell setup'
    Write-Info "Default MCP scope: $DefaultMcpScope"
    if ($DryRun)
    { Write-Warn 'DRY RUN включён: команды не будут выполняться.'
    }

    while ($true)
    {
        Show-MainMenu
        $choice = Read-MenuChoice 'Выбор' @('0','1','2','3','4','5','6','7','8','9','A','B','C','D') '2'
        try
        {
            switch ($choice)
            {
                '1'
                { Install-Prerequisites
                }
                '2'
                { Install-CoreMcpPack
                }
                '3'
                { Install-FullMcpPack
                }
                '4'
                { Configure-DeepSeek
                }
                '5'
                { Disable-DeepSeekEnv
                }
                '6'
                { Write-ClaudeSettings
                }
                '7'
                { Write-ClaudeMd
                }
                '8'
                { Show-Verification
                }
                '9'
                { Show-ManualNotes
                }
                'A'
                { Install-SpecKit
                }
                'B'
                { Install-ECC
                }
                'C'
                { Install-Supermemory
                }
                'D'
                { Install-AdvancedToolsPack
                }
                '0'
                { Write-Ok 'Готово.'; return
                }
            }
        } catch
        {
            Write-Err $_.Exception.Message
            if (-not (Confirm-YesNo 'Продолжить меню?' $true))
            { throw
            }
        }
    }
}

Start-InteractiveMenu
