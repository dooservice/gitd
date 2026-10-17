// Package strategy makes a workspace's directory from a template without writing its
// files: an overlay layer on top of the template, or a reflink copy of it. Each is a Maker;
// when none can run, the workspace package makes the workspace by writing its files, as it
// always has (Copy).
package strategy

import (
	"context"
	"errors"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
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

// Proto is the name as the contract carries it.
func (name Name) Proto() gitdv1.Strategy {
	switch name {
	case Overlay:
		return gitdv1.Strategy_STRATEGY_OVERLAY
	case Reflink:
		return gitdv1.Strategy_STRATEGY_REFLINK
	case Copy:
		return gitdv1.Strategy_STRATEGY_COPY
	}
	return gitdv1.Strategy_STRATEGY_UNSPECIFIED
}

// FromProto is the name a request's strategy asks for; unspecified is "".
func FromProto(strategy gitdv1.Strategy) Name {
	switch strategy {
	case gitdv1.Strategy_STRATEGY_OVERLAY:
		return Overlay
	case gitdv1.Strategy_STRATEGY_REFLINK:
		return Reflink
	case gitdv1.Strategy_STRATEGY_COPY:
		return Copy
	}
	return ""
}
