package cli

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/connectpage"
)

// manifestWait is how long a person has to create the App, as GitHub gives.
const manifestWait = time.Hour

// manifestPage serves, on this machine only, the page that shows the App to create and
// posts its manifest to GitHub, and waits for GitHub to send the browser back with a code.
type manifestPage struct {
	listener net.Listener
	server   *http.Server
	app      connectpage.App
	codes    chan string
	finished chan struct{}
	answer   []byte
}

func newManifestPage(port int, app connectpage.App) (*manifestPage, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return &manifestPage{listener: listener, app: app, codes: make(chan string, 1), finished: make(chan struct{})}, nil
}

// served is where the page is served, as it names itself.
func (page *manifestPage) served() string {
	return page.listener.Addr().String()
}

func (page *manifestPage) address() string {
	return "http://" + page.served()
}

// redirectURL is where GitHub sends the browser back.
func (page *manifestPage) redirectURL() string {
	return page.address() + "/created"
}

// serve starts answering, and returns the address to open.
func (page *manifestPage) serve(begun *gitdv1.BeginGitHubAppResponse) string {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /{$}", func(writer http.ResponseWriter, _ *http.Request) {
		page.write(writer, http.StatusOK, func(out *bytes.Buffer) error {
			return connectpage.Create(out, page.served(), page.app, begun.GetFormAction(), begun.GetManifest())
		})
	})
	routes.HandleFunc("GET /created", func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("state") != begun.GetState() {
			page.write(writer, http.StatusBadRequest, func(out *bytes.Buffer) error {
				return connectpage.Refused(out, page.served(), page.app, "This page was not opened for the App gitd asked GitHub to create.",
					"the state GitHub sent back is not the one gitd gave")
			})
			return
		}
		select {
		case page.codes <- request.URL.Query().Get("code"):
		default:
		}
		select {
		case <-page.finished:
			page.write(writer, http.StatusOK, func(out *bytes.Buffer) error {
				_, err := out.Write(page.answer)
				return err
			})
		case <-request.Context().Done():
		}
	})
	page.server = &http.Server{Handler: routes, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = page.server.Serve(page.listener) }()
	return page.address() + "/"
}

// write answers a page whole, or a plain error if it could not be written.
func (page *manifestPage) write(writer http.ResponseWriter, status int, render func(*bytes.Buffer) error) {
	var out bytes.Buffer
	if err := render(&out); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.WriteHeader(status)
	_, _ = writer.Write(out.Bytes())
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

// finish writes the page the browser lands on: the App kept, or why it was not.
func (page *manifestPage) finish(completed *gitdv1.CompleteGitHubAppResponse, err error) {
	var out bytes.Buffer
	if err != nil {
		_ = connectpage.Refused(&out, page.served(), page.app, "GitHub created the App, but gitd could not keep it.", err.Error())
	} else {
		connection := completed.GetConnection()
		_ = connectpage.WriteConnected(&out, page.served(), connectpage.Connected{
			ID: connection.GetId(), ActsAs: connection.GetActsAs(), Prefix: connection.GetUrlPrefix(),
			InstallURL: completed.GetInstallUrl(), ManageURL: connection.GetManageUrl(),
		})
	}
	page.answer = out.Bytes()
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
