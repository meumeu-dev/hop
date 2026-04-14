# Install hop — https://github.com/meumeu-dev/hop
#
# Usage:
#   iwr -useb meumeu.dev/hop/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Repo       = 'meumeu-dev/hop'
$InstallDir = Join-Path $env:LOCALAPPDATA 'hop'
$BinaryName = 'hop.exe'

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
if ($arch -ne 'X64') {
    Write-Error "Architecture non supportee sur Windows: $arch (amd64 uniquement)"
    exit 1
}

Write-Host "-> Detection de la derniere version..."
$latest = (Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest").tag_name
if (-not $latest) {
    Write-Error "Impossible de trouver la derniere release"
    exit 1
}

$asset = "hop-windows-amd64.exe"
$url   = "https://github.com/$Repo/releases/download/$latest/$asset"

Write-Host "-> Version: $latest"
Write-Host "-> Telechargement $asset..."

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$dest = Join-Path $InstallDir $BinaryName
Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing

# Ajout au PATH utilisateur si absent
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$InstallDir*") {
    Write-Host "-> Ajout de $InstallDir au PATH utilisateur..."
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
    $env:Path += ";$InstallDir"
}

Write-Host ""
Write-Host "-> hop $latest installe dans $dest"
Write-Host ""
Write-Host "Ouvre un NOUVEAU terminal PowerShell puis:"
Write-Host "  hop config"
