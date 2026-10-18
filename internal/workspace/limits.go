package workspace

import (
	"github.com/dooservice/gitd/internal/git"
)

// Limits hold one tenant's part of gitd to its share: at most Workspaces workspaces, and no
// new one, nor a new sandbox, once Used — the disk its pools and templates take, measured
// each maintenance round — reaches Disk. Zero is no limit.
type Limits struct {
	Workspaces int
	Disk       int64
	Used       func() int64
}

// UseLimits holds new workspaces to limits.
func (workspaces *Workspaces) UseLimits(limits Limits) {
	workspaces.limits = limits
}

// WithinDisk says why nothing new may be made for lack of disk, or nothing when it may.
func (workspaces *Workspaces) WithinDisk() error {
	limits := workspaces.limits
	if limits.Disk > 0 && limits.Used != nil && limits.Used() >= limits.Disk {
		return git.Fail(git.LimitReached, "the disk limit of %d bytes is reached: remove workspaces or retire pools", limits.Disk)
	}
	return nil
}

// withinLimits says why one more workspace may not be made, or nothing when it may.
func (workspaces *Workspaces) withinLimits() error {
	if err := workspaces.WithinDisk(); err != nil {
		return err
	}
	if most := workspaces.limits.Workspaces; most > 0 && workspaces.pools.OwnedCount() >= most {
		return git.Fail(git.LimitReached, "the limit of %d workspaces is reached: remove one first", most)
	}
	return nil
}
