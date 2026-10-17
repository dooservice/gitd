// Package strategy makes a workspace's directory from a template without writing its
// files: an overlay layer on top of the template, or a reflink copy of it. Each is a Maker;
// when none can run, the workspace package makes the workspace by writing its files, as it
// always has (Copy).
package strategy

import (
	"context"
	"errors"
)

// Name names a strategy, as the configuration and the answers say it.
type Name string

const (
	// Copy writes every file of the workspace: no template, always possible.
	Copy Name = "copy"
	// Overlay puts a layer of the workspace's own on top of the template.
	Overlay Name = "overlay"
	// Reflink copies the template with block cloning: files share blocks until written.
	Reflink Name = "reflink"
)

// Maker makes workspaces from templates one way.
type Maker interface {
	Name() Name
	// Make makes dest, which must not exist, a directory showing the template's files and
	// keeping its own writes.
	Make(ctx context.Context, template, dest string) error
	// Unmake removes the workspace at dest, and whatever Make kept for it elsewhere.
	Unmake(ctx context.Context, dest string) error
}

// ErrUnsupported is a strategy this machine, or this path, cannot run.
var ErrUnsupported = errors.New("not supported here")
