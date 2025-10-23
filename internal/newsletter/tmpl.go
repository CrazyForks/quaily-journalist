package newsletter

import (
	"bytes"
	_ "embed"
	"text/template"
)

type Item struct {
	Title       string
	URL         string
	NodeName    string
	NodeURL     string
	Description string
	Replies     int
	Created     string
}

type Data struct {
	Title      string
	Slug       string
	Datetime   string
	Preface    string
	Postscript string
	Items      []Item
}

//go:embed newsletter.tmpl
var newsletterTpl string

var compiled = template.Must(template.New("newsletter").Parse(newsletterTpl))

func Render(d Data) (string, error) {
	var buf bytes.Buffer
	if err := compiled.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}
