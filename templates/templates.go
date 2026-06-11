// Package templates provides the embedded template files for devd
// and helpers to parse them into html/template.Template values.
package templates

import (
	"embed"
	"html/template"
	"io/fs"
	"os"
	"strings"

	"github.com/dustin/go-humanize"
)

//go:embed *.html
var FS embed.FS

func bytes(size int64) string {
	return humanize.Bytes(uint64(size))
}

func fileType(f os.FileInfo) string {
	if f.IsDir() {
		return "dir"
	}
	if strings.HasPrefix(f.Name(), ".") {
		return "hidden"
	}
	return "file"
}

// MustMakeTemplates parses the embedded templates, panicking on error.
func MustMakeTemplates() *template.Template {
	t, err := MakeTemplates()
	if err != nil {
		panic(err)
	}
	return t
}

// MakeTemplates parses the embedded templates and returns an html.Template.
func MakeTemplates() (*template.Template, error) {
	tmpl := template.New("")

	funcMap := template.FuncMap{
		"bytes":    bytes,
		"reltime":  humanize.Time,
		"fileType": fileType,
	}
	tmpl.Funcs(funcMap)

	err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			data, err := fs.ReadFile(FS, path)
			if err != nil {
				return err
			}
			_, err = tmpl.New(path).Parse(string(data))
			if err != nil {
				return err
			}
		}
		return nil
	})
	return tmpl, err
}
