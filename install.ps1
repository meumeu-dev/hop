# Install hop — https://github.com/meumeu-dev/hop
#
# Usage:
#   iwr -useb meumeu.dev/hop/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$Repo       = 'meumeu-dev/hop'
$InstallDir = Join-Path $env:LOCALAPPDATA 'hop'
$BinaryName = 'hop.exe'

$arch = $env:PROCESSOR_ARCHITECTURE
# Under 32-bit PowerShell on 64-bit Windows, PROCESSOR_ARCHITECTURE is x86
# but PROCESSOR_ARCHITEW6432 reveals the real arch
if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }
if ($arch -ne 'AMD64') {
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

# Telechargement dans un fichier temporaire : on ne remplace le binaire
# installe qu'apres verification, sinon un telechargement corrompu ecrasait
# directement une installation qui fonctionnait.
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "hop-download-$([guid]::NewGuid()).exe"
Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing

# Verification d'integrite SHA256 — meme garantie que `hop update`.
Write-Host "-> Verification de l'integrite..."
try {
    $expected = ((Invoke-WebRequest -Uri "$url.sha256" -UseBasicParsing).Content -split '\s+')[0]
} catch {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    Write-Error "Somme de controle introuvable pour $asset — installation annulee."
    exit 1
}

$actual = (Get-FileHash -Path $tmp -Algorithm SHA256).Hash
if ($expected -and ($actual -ieq $expected)) {
    Write-Host "-> Integrite verifiee (SHA256)"
} else {
    Remove-Item $tmp -Force -ErrorAction SilentlyContinue
    Write-Error "SOMME DE CONTROLE INVALIDE — binaire corrompu ou altere.`n  Attendu: $expected`n  Obtenu:  $actual"
    exit 1
}

Move-Item -Path $tmp -Destination $dest -Force

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
