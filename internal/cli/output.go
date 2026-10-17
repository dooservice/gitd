package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/client"
)

// print writes an answer as JSON when asked, and as text otherwise.
func (app *app) print(answer proto.Message, text func(io.Writer)) {
	app.clearProgress()
	if app.asJSON {
		fmt.Fprintln(app.stdout, jsonOf(answer))
		return
	}
	text(app.stdout)
}

// jsonOf is an answer as JSON, indented the same way every time: protojson varies its
// spacing on purpose, so it is written compact and indented here.
func jsonOf(answer proto.Message) string {
	compact, err := protojson.Marshal(answer)
	if err != nil {
		return "{}"
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact, "", "  "); err != nil {
		return string(compact)
	}
	return indented.String()
}

// progress draws what git is doing on one line, on a terminal only, and not for JSON.
func (app *app) progress() client.Progress {
	if app.asJSON || !app.interactive {
		return nil
	}
	return func(step *gitdv1.Progress) {
		app.drawn = true
		if step.GetPercent() >= 0 {
			fmt.Fprintf(app.stderr, "\r\033[K%s: %s %d%%", step.GetRepository(), step.GetPhase(), step.GetPercent())
		} else {
			fmt.Fprintf(app.stderr, "\r\033[K%s: %s", step.GetRepository(), step.GetPhase())
		}
	}
}

// clearProgress takes the progress line away, if one was drawn.
func (app *app) clearProgress() {
	if app.drawn {
		fmt.Fprint(app.stderr, "\r\033[K")
		app.drawn = false
	}
}

// terminal is whether out is a terminal rather than a file, a pipe or a log.
func terminal(out *os.File) bool {
	info, err := out.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
