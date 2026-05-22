package webui

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed static/tap_viewer.html
var tapViewerTemplate string

//go:embed static/viewer_i18n.json
var viewerI18nJSON string

//go:embed static/index.html
var legacyIndexHTML string

const tapViewerScriptAnchor = "<script>\nconst $ = s =>"

func buildTapIndexHTML() string {
	html := tapViewerTemplate
	if !strings.Contains(html, tapViewerScriptAnchor) {
		return legacyIndexHTML
	}
	i18nScript := "const __CLAUDE_TAP_I18N__ = " + compactJSON(viewerI18nJSON) + ";\n"
	cfgScript := "const LIVE_MODE = true;\nconst CG_GATEWAY = true;\nconst __TRACE_JSONL_PATH__ = '';\nconst __TRACE_HTML_PATH__ = '';\n"
	injected := "<script>\n" + i18nScript + cfgScript + "</script>\n" + tapViewerScriptAnchor
	return strings.Replace(html, tapViewerScriptAnchor, injected, 1)
}

func compactJSON(raw string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return string(b)
}

var tapIndexHTML = buildTapIndexHTML()
