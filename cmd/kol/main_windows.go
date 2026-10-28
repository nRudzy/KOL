//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nRudzy/KOL/internal/desktop"
	"github.com/nRudzy/KOL/internal/platform"
)

var version = "dev"

func main() {
	background := flag.Bool("background", false, "Démarrer dans la zone de notification")
	dir := flag.String("data-dir", "", "Dossier de données local (facultatif)")
	quit := flag.Bool("quit", false, "Quitter KOL proprement (installation et désinstallation)")
	startup := flag.String("configure-startup", "", "Configurer sans interface : enable ou disable")
	flag.Parse()
	if *quit {
		if err := desktop.StopExisting(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *startup != "" {
		var err error
		if *dir == "" {
			*dir, err = platform.DataDir()
		} else {
			*dir, err = filepath.Abs(*dir)
			if err == nil {
				err = os.MkdirAll(*dir, 0700)
			}
		}
		if err == nil {
			err = platform.ConfigureStartup(*dir, *startup)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := desktop.Run(version, *dir, *background); err != nil {
		desktop.ShowError(err)
		os.Exit(1)
	}
}
