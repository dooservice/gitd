// Package progress reads what git writes to stderr with `--progress` while it works.
//
// git rewrites one line with `\r` while a phase advances and ends it with `\n`, so the
// stream is cut at both. A line such as `Receiving objects:  45% (450/1000), 1.20 MiB`
// becomes a phase and a percentage; anything else is left to the error, if there is one.
package progress

import (
	"bytes"
	"strconv"
	"strings"
)

// Report is told each time a phase changes or its percentage moves.
type Report func(phase string, percent int)

// Parse reads one line and says whether it was progress: a phase, a colon, and a
// percentage before anything else.
func Parse(text string) (phase string, percent int, ok bool) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "remote: ")
	phase, rest, found := strings.Cut(text, ": ")
	number, _, isPercent := strings.Cut(strings.TrimSpace(rest), "%")
	percent, err := strconv.Atoi(number)
	if !found || !isPercent || err != nil || strings.Contains(phase, "/") {
		return "", 0, false
	}
	return phase, percent, true
}

// Writer cuts git's stderr into lines, hands progress to a report and keeps the last
// lines of the rest, as many as room, for the error.
type Writer struct {
	report  Report
	pending []byte
	phase   string
	percent int
	kept    []string
	room    int
}

// NewWriter returns a writer that reports to report, which may be nil.
func NewWriter(report Report) *Writer {
	return &Writer{report: report, percent: -1, room: 200}
}

func (writer *Writer) Write(data []byte) (int, error) {
	writer.pending = append(writer.pending, data...)
	for {
		end := bytes.IndexAny(writer.pending, "\r\n")
		if end < 0 {
			return len(data), nil
		}
		writer.line(string(writer.pending[:end]))
		writer.pending = writer.pending[end+1:]
	}
}

func (writer *Writer) line(text string) {
	if phase, percent, ok := Parse(text); ok {
		if writer.report != nil && (phase != writer.phase || percent != writer.percent) {
			writer.report(phase, percent)
		}
		writer.phase, writer.percent = phase, percent
		return
	}
	if text = strings.TrimSpace(text); text == "" {
		return
	}
	writer.kept = append(writer.kept, text)
	if len(writer.kept) > writer.room {
		writer.kept = writer.kept[len(writer.kept)-writer.room:]
	}
}

// Lines is what git said that was not progress, the last line included.
func (writer *Writer) Lines() []string {
	if rest := strings.TrimSpace(string(writer.pending)); rest != "" {
		writer.line(rest)
		writer.pending = nil
	}
	return writer.kept
}
