package ui

import (
	"embed"
	"encoding/base64"
	"strings"

	"github.com/nRudzy/KOL/assets"
)

//go:embed index.html styles.css app.js
var files embed.FS

func HTML() string {
	html, _ := files.ReadFile("index.html")
	css, _ := files.ReadFile("styles.css")
	js, _ := files.ReadFile("app.js")
	return strings.NewReplacer("/*__KOL_STYLE__*/", string(css), "/*__KOL_SCRIPT__*/", string(js), "__KOL_LOGO__", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(assets.Logo)).Replace(string(html))
}
