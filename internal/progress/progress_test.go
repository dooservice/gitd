package progress

import (
	"slices"
	"testing"
)

func TestAProgressLineIsAPhaseAndAPercentage(t *testing.T) {
	cases := map[string]struct {
		phase   string
		percent int
	}{
		"Receiving objects:  45% (450/1000), 1.20 MiB | 2.00 MiB/s": {"Receiving objects", 45},
		"remote: Counting objects: 100% (12/12), done.":             {"Counting objects", 100},
		"Resolving deltas:   0% (0/3)":                              {"Resolving deltas", 0},
	}
	for line, want := range cases {
		phase, percent, ok := Parse(line)
		if !ok || phase != want.phase || percent != want.percent {
			t.Errorf("Parse(%q) = %q, %d, %v", line, phase, percent, ok)
		}
	}
}

func TestWhatIsNotProgressIsKeptForTheError(t *testing.T) {
	for _, line := range []string{
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
		"remote: Repository not found.",
		"Cloning into bare repository 'x.git'...",
	} {
		if _, _, ok := Parse(line); ok {
			t.Errorf("Parse(%q) took it for progress", line)
		}
	}
}

func TestTheWriterReportsEachChangeOnceAndKeepsTheRest(t *testing.T) {
	var reported []int
	writer := NewWriter(func(phase string, percent int) { reported = append(reported, percent) })
	_, _ = writer.Write([]byte("Receiving objects:  10% (1/10)\rReceiving objects:  10% (1/10)\rReceiving"))
	_, _ = writer.Write([]byte(" objects: 100% (10/10), done.\nfatal: something went wrong"))

	if !slices.Equal(reported, []int{10, 100}) {
		t.Errorf("reported %v", reported)
	}
	if lines := writer.Lines(); !slices.Equal(lines, []string{"fatal: something went wrong"}) {
		t.Errorf("kept %v", lines)
	}
}
