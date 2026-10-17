// Package connectpage writes the pages `gitd connect github-app` serves on this machine
// while a person creates their organisation's GitHub App: what will be created and may do,
// that it is connected and where to install it, or why it was not kept. They look as
// gitd.dev does.
package connectpage

import (
	"embed"
	"html/template"
	"io"
)

//go:embed page.css templates/*.html
var files embed.FS

// The three steps of connecting, as each page shows them.
var (
	creating   = [3]string{"now", "next", "next"}
	installing = [3]string{"done", "now", "next"}
	stopped    = [3]string{"now", "next", "next"}
)

// pages are the templates, each with the layout and the steps.
var pages = map[string]*template.Template{
	"create":    parse("create"),
	"connected": parse("connected"),
	"refused":   parse("refused"),
}

func parse(name string) *template.Template {
	return template.Must(template.ParseFS(files, "templates/layout.html", "templates/"+name+".html"))
}

// style is the stylesheet every page carries inline, so the page needs nothing but fonts.
var style = func() template.CSS {
	data, err := files.ReadFile("page.css")
	if err != nil {
		panic(err)
	}
	return template.CSS(data)
}()

// App is the App being created: its name, its owner (an organisation, or empty for the
// person creating it) and its host.
type App struct {
	Name         string
	Organization string
	Host         string
}

// owner is how the page names who owns the App.
func (app App) owner() string {
	if app.Organization == "" {
		return "your account"
	}
	return app.Organization
}

func (app App) host() string {
	if app.Host == "" {
		return "github.com"
	}
	return app.Host
}

// frame is what every page shows around its content.
type frame struct {
	Title  string
	Served string
	Style  template.CSS
	Steps  [3]string
}

// Create writes the page that shows the App to create and posts its manifest to GitHub.
func Create(out io.Writer, served string, app App, formAction, manifest string) error {
	return pages["create"].ExecuteTemplate(out, "layout", struct {
		frame
		Name, Owner, Host, FormAction, Manifest string
	}{
		frame:      frame{Title: "Create your GitHub App", Served: served, Style: style, Steps: creating},
		Name:       app.Name,
		Owner:      app.owner(),
		Host:       app.host(),
		FormAction: formAction,
		Manifest:   manifest,
	})
}

// Connected is the App gitd now keeps.
type Connected struct {
	ID         string
	ActsAs     string
	Prefix     string
	InstallURL string
	ManageURL  string
}

// WriteConnected writes the page that says the App is kept, and where to install it.
func WriteConnected(out io.Writer, served string, connected Connected) error {
	return pages["connected"].ExecuteTemplate(out, "layout", struct {
		frame
		Connected
	}{frame{Title: "Connected", Served: served, Style: style, Steps: installing}, connected})
}

// Refused writes the page that says why the App was not kept, and how to try again.
func Refused(out io.Writer, served string, app App, lead, message string) error {
	return pages["refused"].ExecuteTemplate(out, "layout", struct {
		frame
		Name, Organization, Lead, Message string
	}{
		frame: frame{Title: "Not connected", Served: served, Style: style, Steps: stopped},
		Name:  app.Name, Organization: app.Organization, Lead: lead, Message: message,
	})
}
