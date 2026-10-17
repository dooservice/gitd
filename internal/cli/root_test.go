package cli

import (
	"context"
	"errors"
	"slices"
	"testing"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func TestTheFlagsOfAUnitWrittenBeforeTheCommandLineStillRead(t *testing.T) {
	got := compatible([]string{"-config", "/etc/dooservice/gitd.toml", "-socket=/run/x.sock", "-version", "-h", "--json", "prepare", "-"})
	want := []string{"--config", "/etc/dooservice/gitd.toml", "--socket=/run/x.sock", "--version", "-h", "--json", "prepare", "-"}
	if !slices.Equal(got, want) {
		t.Errorf("compatible = %v; want %v", got, want)
	}
}

func TestAFailureReadsAsGitdsReasonAndWhatGitSaid(t *testing.T) {
	refused := connect.NewError(connect.CodeNotFound, errors.New("no branch"))
	detail, err := connect.NewErrorDetail(&gitdv1.GitFailure{Reason: gitdv1.GitFailure_REASON_NOT_FOUND, GitSaid: "couldn't find remote ref x"})
	if err != nil {
		t.Fatal(err)
	}
	refused.AddDetail(detail)
	for err, want := range map[error]string{
		refused:                               "NOT_FOUND: couldn't find remote ref x",
		context.Canceled:                      "cancelled",
		errors.New("dial unix: no such file"): "dial unix: no such file",
	} {
		if got := describe(err); got != want {
			t.Errorf("describe(%v) = %q; want %q", err, got, want)
		}
	}
}
