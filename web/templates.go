package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html templates/partials/*.html
var templatesFS embed.FS

// LoadTemplates parses all pages and partials. The daemon template func lets
// pages hide desktop-only controls when running as a server.
func LoadTemplates(daemon bool) (*template.Template, error) {
	funcs := template.FuncMap{
		"daemon": func() bool { return daemon },
	}

	tmpl, err := template.New("").Funcs(funcs).
		ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	partials, err := template.New("").Funcs(funcs).
		ParseFS(templatesFS, "templates/partials/*.html")
	if err != nil {
		return nil, err
	}

	for _, t := range partials.Templates() {
		tmpl.AddParseTree(t.Name(), t.Tree)
	}

	return tmpl, nil
}
