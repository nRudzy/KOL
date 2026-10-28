//go:build windows

package main

import (
	"flag"
	"os"

	"github.com/nRudzy/KOL/internal/desktop"
)

var version = "dev"

func main() {
	background := flag.Bool("background", false, "Démarrer dans la zone de notification")
	dir := flag.String("data-dir", "", "Dossier de données local (facultatif)")
	flag.Parse()
	if err := desktop.Run(version, *dir, *background); err != nil {
		desktop.ShowError(err)
		os.Exit(1)
	}
}
