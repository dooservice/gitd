// Package git runs the official git binary, and is the only thing in gitd that does.
//
// Every process gets an environment built here rather than gitd's own: inheriting it once
// made a private repository look public, because the editor's `GIT_ASKPASS` answered for
// it. Killing a call kills git's whole process group, because git starts children of its
// own that would otherwise outlive it.
package git

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dooservice/gitd/internal/progress"
)

// Runner starts git processes.
type Runner struct {
	binary      string
	environment []string
	allowed     map[string]bool
	running     atomic.Int64
}

// NewRunner returns a runner for the git at binary.
func NewRunner(binary string) *Runner {
	return &Runner{binary: binary, environment: closedEnvironment(), allowed: subcommands()}
}

// Running is how many git processes are alive right now.
func (runner *Runner) Running() int64 {
	return runner.running.Load()
}

// Cmd is one git invocation. Args starts with the subcommand; Env is added to the closed
// environment, as a request's credentials are; Local lets git read a repository on this
// disk, which only a pool is.
//
// Differences says that git ending with status 1 found differences — as diff and
// merge-tree do — and answered, rather than failed.
type Cmd struct {
	Dir         string
	Args        []string
	Env         []string
	Local       bool
	Progress    progress.Report
	Differences bool
}

// Run runs git and returns its stdout, or a failure with what git said.
func (runner *Runner) Run(ctx context.Context, command Cmd) (string, error) {
	if len(command.Args) == 0 || !runner.allowed[command.Args[0]] {
		return "", Fail(Failed, "git %v is not something gitd runs", command.Args)
	}
	process := exec.CommandContext(ctx, runner.binary, append(hardened(command.Local), command.Args...)...)
	process.Dir = command.Dir
	process.Env = append(append([]string{}, runner.environment...), command.Env...)
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	process.Cancel = func() error {
		return syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
	}
	process.WaitDelay = 5 * time.Second

	var stdout bytes.Buffer
	stderr := progress.NewWriter(command.Progress)
	process.Stdout = &stdout
	process.Stderr = stderr

	runner.running.Add(1)
	err := process.Run()
	runner.running.Add(-1)
	var exit *exec.ExitError
	if err != nil && command.Differences && errors.As(err, &exit) && exit.ExitCode() == 1 && ctx.Err() == nil {
		return stdout.String(), nil
	}
	if err != nil {
		return stdout.String(), failed(ctx, command.Args[0], err, stderr.Lines())
	}
	return stdout.String(), nil
}

// failed is what a git that did not succeed said, or what stopped it.
func failed(ctx context.Context, subcommand string, err error, lines []string) *Failure {
	failure := &Failure{Reason: classify(lines), GitSaid: lastWords(lines), ExitCode: -1}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		failure.ExitCode = exit.ExitCode()
	}
	if stopped := ctx.Err(); stopped != nil {
		failure.Reason = Timeout
		failure.GitSaid = "git " + subcommand + ": " + stopped.Error()
	} else if failure.GitSaid == "" {
		failure.GitSaid = "git " + subcommand + ": " + err.Error()
	}
	return failure
}
