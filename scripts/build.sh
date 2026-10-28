#!/usr/bin/env sh
set -eu
kol_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$kol_root"
kol_version=${1:-0.1.0}
case "$kol_version" in *[!a-zA-Z0-9._-]*|'') printf '%s\n' 'Version invalide.' >&2; exit 1;; esac
mkdir -p dist
go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest assets/kol.manifest -ico assets/kol.ico -o cmd/kol/resource_windows_amd64.syso
trap 'rm -f cmd/kol/resource_windows_amd64.syso' EXIT HUP INT TERM
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H=windowsgui -X=main.version=$kol_version" -o dist/KOL.exe ./cmd/kol
printf '%s\n' "Exécutable créé : $kol_root/dist/KOL.exe"
