package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/client"
)

// tools are gitd's operations as an agent calls them.
type tools struct {
	gitd *client.Client
}

func yes() *bool { truth := true; return &truth }
func no() *bool  { falsehood := false; return &falsehood }

func (tools *tools) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "prepare_workspace",
		Title:       "Prepare a workspace",
		Description: "Make a workspace (a git checkout) of a repository at an absolute path under gitd's roots, at a branch, a commit or the default branch. Takes seconds after the first time. A workspace gitd already made at that path is moved instead.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: no(), OpenWorldHint: yes()},
	}, tools.prepare)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_workspace",
		Title:       "Update a workspace",
		Description: "Bring a workspace to the tip of its branch, or of the branch given. Discards uncommitted changes in it.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: yes(), OpenWorldHint: yes()},
	}, tools.update)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "checkout_commit",
		Title:       "Check out a commit",
		Description: "Move a workspace to a commit, detached, as a rollback does. Discards uncommitted changes in it.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: yes(), OpenWorldHint: yes()},
	}, tools.checkout)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "describe_workspace",
		Title:       "Describe a workspace",
		Description: "Say what a workspace is on disk: its repository, commit, branch, whether it has uncommitted changes, and its submodules. Reads nothing from the network.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: no()},
	}, tools.describe)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_changes",
		Title:       "List a workspace's changes",
		Description: "List each file added, modified or deleted in a workspace since gitd put it on its commit, committed or not, with its lines: review your work before publishing it. Reads nothing from the network.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: no()},
	}, tools.changes)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "show_diff",
		Title:       "Show a workspace's diff",
		Description: "Show the unified diff of a workspace's changes, of some files or all, cut short at a size. Reads nothing from the network.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: no()},
	}, tools.diff)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "publish_branch",
		Title:       "Publish a branch",
		Description: "Push what was committed in a workspace to a new branch of its repository, and open its pull request when gitd's settings say so or you ask. gitd pushes with the credential it keeps; the push is never forced, and gitd may refuse a publish that breaks its rules, such as changing the CI or deleting tests. Commit your changes in the workspace first.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: no(), OpenWorldHint: yes()},
	}, tools.publish)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "open_endpoint_access",
		Title:       "Open access to gitd's git endpoint",
		Description: "Give a sandbox that cannot mount gitd's workspaces — a microVM, a remote sandbox — what it clones a repository from over HTTP: an address and a secret of gitd's own, valid for one repository and one sandbox repository it may push to, for an hour unless told, never the git host's credential. Publish what it pushes with publish_branch and its sandbox.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: no(), OpenWorldHint: yes()},
	}, tools.openEndpoint)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_workspace",
		Title:       "Remove a workspace",
		Description: "Delete a workspace and everything in it, committed or not. Its repository's pool stays for the next workspace.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: yes(), OpenWorldHint: no()},
	}, tools.remove)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "probe_repository",
		Title:       "Probe a repository",
		Description: "Say whether a repository can be read with the credentials gitd keeps, its default branch, and its branches with the commit each points at.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: yes()},
	}, tools.probe)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_audit_log",
		Title:       "Read the audit log",
		Description: "List the publishes gitd made or refused, newest first: when, from which workspace, to which repository and branch, and what was pushed or why it was refused.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: no()},
	}, tools.audit)
}

// Inputs and outputs, as the agent sees them.

type prepareInput struct {
	URL         string `json:"url" jsonschema:"the repository, as https://host/owner/name, git@host:owner/name or ssh://"`
	Path        string `json:"path" jsonschema:"absolute path of the workspace, under one of gitd's roots"`
	Branch      string `json:"branch,omitempty" jsonschema:"the branch to check out; default: the repository's default branch"`
	Commit      string `json:"commit,omitempty" jsonschema:"a commit to check out instead of a branch"`
	Submodules  bool   `json:"submodules,omitempty" jsonschema:"check the submodules out too"`
	FullHistory bool   `json:"full_history,omitempty" jsonschema:"bring the whole history, for git log and git blame"`
}

type updateInput struct {
	Path       string `json:"path" jsonschema:"absolute path of the workspace"`
	Branch     string `json:"branch,omitempty" jsonschema:"the branch to follow; default: the one it follows"`
	Submodules bool   `json:"submodules,omitempty" jsonschema:"bring the submodules along"`
}

type checkoutInput struct {
	Path       string `json:"path" jsonschema:"absolute path of the workspace"`
	Commit     string `json:"commit" jsonschema:"the commit to move to"`
	Submodules bool   `json:"submodules,omitempty" jsonschema:"bring the submodules along"`
}

type pathInput struct {
	Path string `json:"path" jsonschema:"absolute path of the workspace"`
}

type changesInput struct {
	Path    string `json:"path" jsonschema:"absolute path of the workspace"`
	Ignored bool   `json:"include_ignored,omitempty" jsonschema:"include files the base commit's .gitignore ignores, such as node_modules"`
}

type diffInput struct {
	Path     string   `json:"path" jsonschema:"absolute path of the workspace"`
	Files    []string `json:"files,omitempty" jsonschema:"only these files, relative to the workspace; all the changes when empty"`
	MaxBytes int64    `json:"max_bytes,omitempty" jsonschema:"at most this many bytes of diff; 512 KiB when unset"`
}

type changedFile struct {
	Path    string `json:"path"`
	Kind    string `json:"kind" jsonschema:"added, modified or deleted"`
	Added   int32  `json:"added_lines"`
	Removed int32  `json:"removed_lines"`
	Binary  bool   `json:"binary,omitempty"`
}

type changesOutput struct {
	Base  string        `json:"base" jsonschema:"the commit gitd put the workspace on, which the changes are read against"`
	Files []changedFile `json:"files"`
}

type diffOutput struct {
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated,omitempty" jsonschema:"cut short at max_bytes: ask for fewer files"`
}

type publishInput struct {
	Path        string `json:"path,omitempty" jsonschema:"absolute path of the workspace; leave empty to publish from a sandbox instead"`
	Sandbox     string `json:"sandbox,omitempty" jsonschema:"the sandbox repository a sandbox pushed to through gitd's endpoint, instead of a workspace; branch is then the branch it pushed"`
	Branch      string `json:"branch" jsonschema:"the new branch to push to, which must not exist yet"`
	PullRequest bool   `json:"pull_request,omitempty" jsonschema:"open a pull request even when gitd opens them only when asked"`
	Title       string `json:"title,omitempty" jsonschema:"the pull request's title; default: the newest commit's subject"`
	Body        string `json:"body,omitempty" jsonschema:"what the pull request is about, before what gitd checked"`
	Draft       *bool  `json:"draft,omitempty" jsonschema:"open it as a draft; default: as gitd's settings say"`
}

type endpointInput struct {
	URL      string `json:"url" jsonschema:"the repository, as https://host/owner/name, git@host:owner/name or ssh://"`
	Branch   string `json:"branch,omitempty" jsonschema:"the branch the sandbox starts from; default: the repository's default branch"`
	Minutes  int64  `json:"minutes,omitempty" jsonschema:"how long the access lasts, in minutes; 60 when unset, 1440 at most"`
	ReadOnly bool   `json:"read_only,omitempty" jsonschema:"clone and fetch only, never push"`
}

type endpointOutput struct {
	CloneURL string `json:"clone_url" jsonschema:"where the sandbox clones from"`
	Secret   string `json:"secret" jsonschema:"the password to clone with: git clone https://x:<secret>@<host>/…; shown this once"`
	ID       string `json:"id" jsonschema:"what to revoke it by"`
	Sandbox  string `json:"sandbox" jsonschema:"the sandbox repository pushes land in, to publish from"`
	Expires  string `json:"expires"`
}

type probeInput struct {
	URL string `json:"url" jsonschema:"the repository, as https://host/owner/name, git@host:owner/name or ssh://"`
}

type auditInput struct {
	Since string `json:"since,omitempty" jsonschema:"only the publishes of this long ago or later, e.g. 24h or 30m; default: all"`
	URL   string `json:"url,omitempty" jsonschema:"only the publishes of this repository, by its https URL"`
	Limit int32  `json:"limit,omitempty" jsonschema:"at most this many; default 100, at most 1000"`
}

type pending struct {
	Path   string `json:"path"`
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

type workspaceOutput struct {
	Path     string    `json:"path"`
	Commit   string    `json:"commit"`
	Strategy string    `json:"strategy" jsonschema:"how it was made: overlay (milliseconds, a shared template), reflink or copy"`
	Branch   string    `json:"branch,omitempty" jsonschema:"the branch it follows; empty when detached"`
	Pending  []pending `json:"pending_submodules,omitempty" jsonschema:"submodules that could not be checked out, and why"`
	LFS      []string  `json:"lfs_patterns,omitempty" jsonschema:"files the repository keeps in git LFS: they are pointers here, as gitd does not download them"`
}

type submodule struct {
	Path       string `json:"path"`
	URL        string `json:"url"`
	Commit     string `json:"commit"`
	CheckedOut bool   `json:"checked_out"`
}

type describeOutput struct {
	URL        string      `json:"url"`
	Strategy   string      `json:"strategy" jsonschema:"how it was made: overlay, reflink or copy"`
	Commit     string      `json:"commit"`
	Branch     string      `json:"branch,omitempty"`
	Modified   bool        `json:"modified" jsonschema:"whether there are uncommitted changes"`
	Submodules []submodule `json:"submodules,omitempty"`
	LFS        []string    `json:"lfs_patterns,omitempty" jsonschema:"files the repository keeps in git LFS: they are pointers here"`
}

type published struct {
	URL      string   `json:"url"`
	Path     string   `json:"path,omitempty" jsonschema:"the submodule's path; empty for the workspace's own repository"`
	Commit   string   `json:"commit"`
	Subjects []string `json:"subjects" jsonschema:"the subjects of the new commits, newest first"`
}

type publishOutput struct {
	Branch       string      `json:"branch"`
	Repositories []published `json:"repositories" jsonschema:"the workspace's repository first, then each submodule pushed"`
	PullRequest  string      `json:"pull_request,omitempty" jsonschema:"the pull request opened, when one was"`
	NoPull       string      `json:"no_pull_request,omitempty" jsonschema:"why the pull request could not be opened; the branch is published all the same"`
}

type removeOutput struct {
	Removed string `json:"removed"`
}

type probeOutput struct {
	Access        string            `json:"access" jsonschema:"public, reachable (with a credential gitd keeps) or denied"`
	DefaultBranch string            `json:"default_branch,omitempty"`
	Branches      map[string]string `json:"branches,omitempty" jsonschema:"each branch and the commit it points at"`
	Denied        string            `json:"denied,omitempty" jsonschema:"why it cannot be read"`
}

type auditEntry struct {
	Time         string      `json:"time"`
	Workspace    string      `json:"workspace"`
	URL          string      `json:"url,omitempty"`
	Branch       string      `json:"branch"`
	Repositories []published `json:"repositories,omitempty" jsonschema:"what was pushed, when it was published"`
	Refused      string      `json:"refused,omitempty" jsonschema:"why it was refused, when it was"`
}

type auditOutput struct {
	Entries []auditEntry `json:"entries" jsonschema:"newest first"`
}

// Handlers.

func (tools *tools) prepare(ctx context.Context, _ *mcp.CallToolRequest, in prepareInput) (*mcp.CallToolResult, workspaceOutput, error) {
	request := &gitdv1.PrepareRequest{Url: in.URL, Path: in.Path, WithSubmodules: in.Submodules, FullHistory: in.FullHistory}
	switch {
	case in.Branch != "" && in.Commit != "":
		return nil, workspaceOutput{}, errors.New("give a branch or a commit, not both")
	case in.Branch != "":
		request.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Branch{Branch: in.Branch}}
	case in.Commit != "":
		request.Revision = &gitdv1.Revision{Target: &gitdv1.Revision_Commit{Commit: in.Commit}}
	}
	result, err := tools.gitd.Prepare(ctx, request, nil)
	return nil, workspace(in.Path, result), refusal(err)
}

func (tools *tools) update(ctx context.Context, _ *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, workspaceOutput, error) {
	result, err := tools.gitd.Update(ctx, &gitdv1.UpdateRequest{Path: in.Path, Branch: in.Branch, WithSubmodules: in.Submodules}, nil)
	return nil, workspace(in.Path, result), refusal(err)
}

func (tools *tools) checkout(ctx context.Context, _ *mcp.CallToolRequest, in checkoutInput) (*mcp.CallToolResult, workspaceOutput, error) {
	result, err := tools.gitd.Checkout(ctx, &gitdv1.CheckoutRequest{Path: in.Path, Commit: in.Commit, WithSubmodules: in.Submodules}, nil)
	return nil, workspace(in.Path, result), refusal(err)
}

func (tools *tools) describe(ctx context.Context, _ *mcp.CallToolRequest, in pathInput) (*mcp.CallToolResult, describeOutput, error) {
	answer, err := tools.gitd.Describe(ctx, in.Path)
	if err != nil {
		return nil, describeOutput{}, refusal(err)
	}
	out := describeOutput{
		URL: answer.GetUrl(), Commit: answer.GetCommit(), Branch: answer.GetBranch(), Modified: answer.GetModified(),
		Strategy: made(answer.GetStrategy()), LFS: answer.GetLfsPatterns(),
	}
	for _, state := range answer.GetSubmodules() {
		out.Submodules = append(out.Submodules, submodule{Path: state.GetPath(), URL: state.GetUrl(), Commit: state.GetCommit(), CheckedOut: state.GetCheckedOut()})
	}
	return nil, out, nil
}

func (tools *tools) changes(ctx context.Context, _ *mcp.CallToolRequest, in changesInput) (*mcp.CallToolResult, changesOutput, error) {
	answer, err := tools.gitd.Changes(ctx, &gitdv1.ChangesRequest{Path: in.Path, IncludeIgnored: in.Ignored})
	if err != nil {
		return nil, changesOutput{}, refusal(err)
	}
	out := changesOutput{Base: answer.GetBase(), Files: []changedFile{}}
	for _, file := range answer.GetFiles() {
		out.Files = append(out.Files, changedFile{
			Path: file.GetPath(), Kind: strings.ToLower(strings.TrimPrefix(file.GetKind().String(), "KIND_")),
			Added: file.GetAddedLines(), Removed: file.GetRemovedLines(), Binary: file.GetBinary(),
		})
	}
	return nil, out, nil
}

func (tools *tools) diff(ctx context.Context, _ *mcp.CallToolRequest, in diffInput) (*mcp.CallToolResult, diffOutput, error) {
	answer, err := tools.gitd.Diff(ctx, &gitdv1.DiffRequest{Path: in.Path, Files: in.Files, MaxBytes: in.MaxBytes})
	if err != nil {
		return nil, diffOutput{}, refusal(err)
	}
	return nil, diffOutput{Diff: answer.GetDiff(), Truncated: answer.GetTruncated()}, nil
}

func (tools *tools) publish(ctx context.Context, _ *mcp.CallToolRequest, in publishInput) (*mcp.CallToolResult, publishOutput, error) {
	request := &gitdv1.PublishRequest{Path: in.Path, Branch: in.Branch, Sandbox: in.Sandbox}
	if in.PullRequest || in.Title != "" || in.Body != "" || in.Draft != nil {
		request.PullRequest = &gitdv1.PullRequestOptions{Open: in.PullRequest, Title: in.Title, Body: in.Body, Draft: in.Draft}
	}
	answer, err := tools.gitd.Publish(ctx, request)
	if err != nil {
		return nil, publishOutput{}, refusal(err)
	}
	out := publishOutput{Branch: answer.GetBranch(), Repositories: publishedOf(answer.GetRepository(), answer.GetSubmodules())}
	out.PullRequest = answer.GetPullRequest().GetUrl()
	if failure := answer.GetPullRequestFailure(); failure != nil {
		out.NoPull = strings.TrimPrefix(failure.GetReason().String(), "REASON_") + ": " + failure.GetGitSaid()
	}
	return nil, out, nil
}

func (tools *tools) openEndpoint(ctx context.Context, _ *mcp.CallToolRequest, in endpointInput) (*mcp.CallToolResult, endpointOutput, error) {
	answer, err := tools.gitd.IssueToken(ctx, &gitdv1.IssueTokenRequest{
		Url: in.URL, Branch: in.Branch, LifetimeSeconds: in.Minutes * 60, ReadOnly: in.ReadOnly,
	})
	if err != nil {
		return nil, endpointOutput{}, refusal(err)
	}
	info := answer.GetInfo()
	return nil, endpointOutput{
		CloneURL: answer.GetCloneUrl(), Secret: answer.GetToken(), ID: info.GetId(), Sandbox: info.GetSandbox(),
		Expires: info.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
	}, nil
}

func (tools *tools) remove(ctx context.Context, _ *mcp.CallToolRequest, in pathInput) (*mcp.CallToolResult, removeOutput, error) {
	if err := tools.gitd.Remove(ctx, in.Path); err != nil {
		return nil, removeOutput{}, refusal(err)
	}
	return nil, removeOutput{Removed: in.Path}, nil
}

func (tools *tools) probe(ctx context.Context, _ *mcp.CallToolRequest, in probeInput) (*mcp.CallToolResult, probeOutput, error) {
	answer, err := tools.gitd.Probe(ctx, &gitdv1.ProbeRequest{Url: in.URL})
	if err != nil {
		return nil, probeOutput{}, refusal(err)
	}
	out := probeOutput{Access: access(answer.GetAccess()), DefaultBranch: answer.GetDefaultBranch(), Branches: answer.GetCommits()}
	if failure := answer.GetFailure(); failure != nil {
		out.Denied = failure.GetGitSaid()
	}
	return nil, out, nil
}

func (tools *tools) audit(ctx context.Context, _ *mcp.CallToolRequest, in auditInput) (*mcp.CallToolResult, auditOutput, error) {
	request := &gitdv1.AuditRequest{Url: in.URL, Limit: in.Limit}
	if in.Since != "" {
		ago, err := time.ParseDuration(in.Since)
		if err != nil {
			return nil, auditOutput{}, fmt.Errorf("since %q is not a duration such as 24h", in.Since)
		}
		request.Since = timestamppb.New(time.Now().Add(-ago))
	}
	answer, err := tools.gitd.Audit(ctx, request)
	if err != nil {
		return nil, auditOutput{}, refusal(err)
	}
	out := auditOutput{Entries: []auditEntry{}}
	for _, entry := range answer.GetEntries() {
		if entry.GetChange() != nil {
			continue // changes to connections and settings are the platform's, not the agent's
		}
		recorded := auditEntry{
			Time: entry.GetTime().AsTime().UTC().Format(time.RFC3339), Workspace: entry.GetWorkspace(),
			URL: entry.GetUrl(), Branch: entry.GetBranch(),
		}
		if failure := entry.GetRefusal(); failure != nil {
			recorded.Refused = strings.TrimPrefix(failure.GetReason().String(), "REASON_") + ": " + failure.GetGitSaid()
		} else {
			recorded.Repositories = publishedOf(entry.GetRepository(), entry.GetSubmodules())
		}
		out.Entries = append(out.Entries, recorded)
	}
	return nil, out, nil
}

// publishedOf is what a publish pushed, as the agent reads it: the workspace's repository
// first, then each submodule.
func publishedOf(repository *gitdv1.PublishedRepository, submodules []*gitdv1.PublishedRepository) []published {
	var out []published
	for _, pushed := range append([]*gitdv1.PublishedRepository{repository}, submodules...) {
		out = append(out, published{URL: pushed.GetUrl(), Path: pushed.GetPath(), Commit: pushed.GetCommit(), Subjects: pushed.GetSubjects()})
	}
	return out
}

// workspace is where a workspace ended up, as the agent reads it.
func workspace(path string, result *gitdv1.WorkspaceResult) workspaceOutput {
	out := workspaceOutput{
		Path: path, Commit: result.GetCommit(), Branch: result.GetBranch(), Strategy: made(result.GetStrategy()),
		LFS: result.GetLfsPatterns(),
	}
	for _, left := range result.GetPendingSubmodules() {
		out.Pending = append(out.Pending, pending{Path: left.GetPath(), URL: left.GetUrl(), Reason: left.GetFailure().GetGitSaid()})
	}
	return out
}

// refusal is a failure as the agent reads it: gitd's reason and what git said.
func refusal(err error) error {
	if err == nil {
		return nil
	}
	if reason, said := client.Reason(err); reason != "" {
		return errors.New(strings.TrimPrefix(reason, "REASON_") + ": " + said)
	}
	return err
}

func access(value gitdv1.ProbeResponse_Access) string {
	switch value {
	case gitdv1.ProbeResponse_ACCESS_PUBLIC:
		return "public"
	case gitdv1.ProbeResponse_ACCESS_REACHABLE:
		return "reachable"
	case gitdv1.ProbeResponse_ACCESS_DENIED:
		return "denied"
	}
	return "unknown"
}

func made(strategy gitdv1.Strategy) string {
	switch strategy {
	case gitdv1.Strategy_STRATEGY_OVERLAY:
		return "overlay"
	case gitdv1.Strategy_STRATEGY_REFLINK:
		return "reflink"
	}
	return "copy"
}
