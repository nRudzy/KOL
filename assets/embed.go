// Package assets contains KOL's production brand assets.
package assets

import _ "embed"

//go:embed mark.png
var Logo []byte

//go:embed kol.ico
var Icon []byte
