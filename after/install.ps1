param([switch]$Uninstall)

$ErrorActionPreference = "Stop"

$Repo = "parthyadav3105/claude-utils"
$Binary = "claudeafter"
$InstallDir = if ($env:CLAUDE_DIR) {
    $env:CLAUDE_DIR
} else {
    Join-Path $env:USERPROFILE ".claude"
}
# a CLAUDE_DIR written with a literal ~ ("~/.claude-max") is not expanded by PowerShell
if ($InstallDir -eq "~") {
    $InstallDir = $HOME
} elseif ($InstallDir.StartsWith("~/") -or $InstallDir.StartsWith("~\")) {
    $InstallDir = Join-Path $HOME $InstallDir.Substring(2)
}
$Settings = Join-Path $InstallDir "settings.json"
$Dest = Join-Path $InstallDir "${Binary}.exe"
$Command = $Dest -replace "\\", "/"

# Rewrites both hook entries from scratch: any entry already naming our binary
# is dropped before ours is added back, so re-running never stacks duplicates.
# With -Add:$false it stops after the dropping, which is the uninstall.
function Set-AfterHooks([bool]$Add) {
    if (Test-Path $Settings) {
        try { $json = Get-Content $Settings -Raw | ConvertFrom-Json } catch { $json = [PSCustomObject]@{} }
    } elseif (-not $Add) {
        return
    } else {
        $json = [PSCustomObject]@{}
    }
    if ($null -eq $json) { $json = [PSCustomObject]@{} }

    $hooks = if ($json.PSObject.Properties.Name -contains "hooks" -and $json.hooks) { $json.hooks } else { [PSCustomObject]@{} }

    foreach ($event in @("UserPromptSubmit", "Stop")) {
        $kept = @()
        if ($hooks.PSObject.Properties.Name -contains $event -and $hooks.$event) {
            $kept = @($hooks.$event | Where-Object {
                $cmds = @($_.hooks | ForEach-Object { $_.command })
                $cmds -notcontains $Command
            })
        }
        if ($Add) {
            $kept += [ordered]@{ hooks = @([ordered]@{ type = "command"; command = $Command }) }
        }
        if ($kept.Count -gt 0) {
            $hooks | Add-Member -MemberType NoteProperty -Name $event -Value $kept -Force
        } elseif ($hooks.PSObject.Properties.Name -contains $event) {
            $hooks.PSObject.Properties.Remove($event)
        }
    }

    if (@($hooks.PSObject.Properties).Count -gt 0) {
        $json | Add-Member -MemberType NoteProperty -Name "hooks" -Value $hooks -Force
    } elseif ($json.PSObject.Properties.Name -contains "hooks") {
        $json.PSObject.Properties.Remove("hooks")
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $json | ConvertTo-Json -Depth 10 | Set-Content $Settings -Encoding UTF8
}

if ($Uninstall) {
    Set-AfterHooks $false
    if (Test-Path $Dest) { Remove-Item $Dest }
    Write-Host "Removed $Dest and its hook entries."
    Write-Host "Queued messages were left in $(Join-Path $InstallDir 'after-queue.json')"
    exit 0
}

# detect arch
$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "Unsupported architecture"; exit 1
}

$Asset = "${Binary}-windows-${Arch}.exe"
$Url = "https://github.com/${Repo}/releases/download/after-latest/${Asset}"

Write-Host "Downloading $Asset..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest -Uri $Url -OutFile $Dest

Set-AfterHooks $true

Write-Host "Installed to $Dest"
Write-Host "Restart Claude Code, then type: after 5m <message>"
