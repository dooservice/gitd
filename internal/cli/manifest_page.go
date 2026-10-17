package cli

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strconv"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// manifestWait is how long a person has to create the App, as GitHub gives.
const manifestWait = time.Hour

// postManifest is the page that posts a manifest to GitHub as soon as it opens.
var postManifest = template.Must(template.New("post").Parse(`<!doctype html>
<meta charset="utf-8"><title>gitd: create the GitHub App</title>
<form method="post" action="{{.Action}}"><input type="hidden" name="manifest" value="{{.Manifest}}">
<button type="submit">Create the GitHub App on GitHub</button></form>
<script>document.forms[0].submit()</script>`))

// done is the page a browser lands on once GitHub sends it back.
var done = template.Must(template.New("done").Parse(`<!doctype html>
<meta charset="utf-8"><title>gitd</title><p>{{.}}</p>`))

// manifestPage serves, on this machine only, the page that posts a manifest to GitHub, and
// waits for GitHub to send the browser back with a code.
type manifestPage struct {
	listener net.Listener
	server   *http.Server
	codes    chan string
	finished chan struct{}
	message  string
}

func newManifestPage(port int) (*manifestPage, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return &manifestPage{listener: listener, codes: make(chan string, 1), finished: make(chan struct{})}, nil
}

func (page *manifestPage) address() string {
	return "http://" + page.listener.Addr().String()
}

// redirectURL is where GitHub sends the browser back.
func (page *manifestPage) redirectURL() string {
	return page.address() + "/created"
}

// serve starts answering, and returns the address to open.
func (page *manifestPage) serve(begun *gitdv1.BeginGitHubAppResponse) string {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /{$}", func(writer http.ResponseWriter, _ *http.Request) {
		_ = postManifest.Execute(writer, map[string]string{"Action": begun.GetFormAction(), "Manifest": begun.GetManifest()})
	})
	routes.HandleFunc("GET /created", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("state") != begun.GetState() {
			http.Error(writer, "this is not the App gitd asked for", http.StatusBadRequest)
			return
		}
		select {
		case page.codes <- request.URL.Query().Get("code"):
		default:
		}
		select {
		case <-page.finished:
			_ = done.Execute(writer, page.message)
		case <-request.Context().Done():
		}
	})
	page.server = &http.Server{Handler: routes, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = page.server.Serve(page.listener) }()
	return page.address() + "/"
}

// waitForCode waits for GitHub to send the browser back with a code.
func (page *manifestPage) waitForCode(ctx context.Context) (string, error) {
	select {
	case code := <-page.codes:
		return code, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(manifestWait):
		return "", errors.New("the App was not created within an hour")
	}
}

// finish tells the browser how it went.
func (page *manifestPage) finish(completed *gitdv1.CompleteGitHubAppResponse, err error) {
	if err != nil {
		page.message = fmt.Sprintf("gitd could not keep the App: %v", err)
	} else {
		page.message = fmt.Sprintf("gitd keeps %s. Install it on the repositories it may reach: %s",
			completed.GetConnection().GetActsAs(), completed.GetInstallUrl())
	}
	close(page.finished)
}

func (page *manifestPage) close() {
	if page.server != nil {
		shutting, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = page.server.Shutdown(shutting)
		return
	}
	_ = page.listener.Close()
}
