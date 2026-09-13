param([switch]$Uninstall)

$ErrorActionPreference = "Stop"

$Repo = "parthyadav3105/claude-utils"
$Binary = "task"
$InstallDir = if ($env:INSTALL_DIR) {
    $env:INSTALL_DIR
} else {
    Join-Path $env:USERPROFILE ".local\bin"
}
$Dest = Join-Path $InstallDir "${Binary}.exe"

if ((Test-Path $Dest) -and -not ((& $Dest --help 2>$null | Out-String) -match "Task Management Tool for AI Coding Agent")) {
    Write-Host "$Dest is a different program called task; leaving it alone. Set INSTALL_DIR to use another folder."
    exit 1
}

if ($Uninstall) {
    if (Test-Path $Dest) {
        & $Dest uninstall claude-code
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Could not remove the Claude Code setup, so $Dest was kept."
            exit 1
        }
        Remove-Item $Dest
    }
    Write-Host "Removed $Dest. Your tasks were kept."
    exit 0
}

$Arch = if ([System.Environment]::Is64BitOperatingSystem) {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
} else {
    Write-Error "Unsupported architecture"; exit 1
}
if ($Arch -ne "amd64") {
    Write-Error "Unsupported architecture: $Arch"; exit 1
}

$Asset = "${Binary}-windows-${Arch}.exe"
$Url = "https://github.com/${Repo}/releases/download/tasks-latest/${Asset}"

Write-Host "Downloading $Asset..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$Tmp = "$Dest.download"
Invoke-WebRequest -Uri $Url -OutFile $Tmp
Move-Item -Force $Tmp $Dest

$UserPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
if (($UserPath -split ";") -notcontains $InstallDir) {
    [System.Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your user PATH. Open a new terminal to use it."
}

Write-Host "Installed to $Dest"
Write-Host "To use it with Claude Code, run: task setup claude-code"
