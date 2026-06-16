$ErrorActionPreference = "Stop"

$Repo = "parthyadav3105/claude-utils"
$Binary = "claudeline"
$InstallDir = Join-Path $env:USERPROFILE ".claude"
$Settings = Join-Path $InstallDir "settings.json"

# detect arch
$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "Unsupported architecture"; exit 1
}

$Asset = "${Binary}-windows-${Arch}.exe"
$Url = "https://github.com/${Repo}/releases/download/statusline-latest/${Asset}"
$Dest = Join-Path $InstallDir "${Binary}.exe"

Write-Host "Downloading $Asset..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest -Uri $Url -OutFile $Dest

# patch settings.json — always sets both type and command (idempotent)
$Command = $Dest -replace "\\", "/"
$statusLine = [ordered]@{ type = "command"; command = $Command }

if (Test-Path $Settings) {
    try {
        $json = Get-Content $Settings -Raw | ConvertFrom-Json
    } catch {
        $json = [PSCustomObject]@{}
    }
    $json | Add-Member -MemberType NoteProperty -Name "statusLine" -Value $statusLine -Force
    $json | ConvertTo-Json -Depth 10 | Set-Content $Settings -Encoding UTF8
} else {
    [PSCustomObject]@{ statusLine = $statusLine } | ConvertTo-Json -Depth 10 | Set-Content $Settings -Encoding UTF8
}

Write-Host "Installed to $Dest"
Write-Host "Restart Claude Code to apply."
