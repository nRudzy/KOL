param([ValidatePattern('^[a-zA-Z0-9._-]+$')][string]$Version = '0.1.0')
$ErrorActionPreference = 'Stop'
$kolRoot = Split-Path $PSScriptRoot -Parent
$kolPreviousGoOS = $env:GOOS
$kolPreviousGoArch = $env:GOARCH
$kolPreviousCGO = $env:CGO_ENABLED
Push-Location $kolRoot
try {
  New-Item -ItemType Directory -Force dist | Out-Null
  go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest assets/kol.manifest -ico assets/kol.ico -o cmd/kol/resource_windows_amd64.syso
  if ($LASTEXITCODE -ne 0) { throw 'Création des ressources Windows impossible.' }
  $env:GOOS = 'windows'
  $env:GOARCH = 'amd64'
  $env:CGO_ENABLED = '0'
  go build -trimpath -ldflags "-s -w -H=windowsgui -X=main.version=$Version" -o dist/KOL.exe ./cmd/kol
  if ($LASTEXITCODE -ne 0) { throw 'Compilation de KOL impossible.' }
  Write-Output "Exécutable créé : $kolRoot\dist\KOL.exe"
} finally {
  Remove-Item cmd/kol/resource_windows_amd64.syso -ErrorAction SilentlyContinue
  $env:GOOS = $kolPreviousGoOS
  $env:GOARCH = $kolPreviousGoArch
  $env:CGO_ENABLED = $kolPreviousCGO
  Pop-Location
}
