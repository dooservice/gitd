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
		Name:        "publish_branch",
		Title:       "Publish a branch",
		Description: "Push what was committed in a workspace to a new branch of its repository, so a pull request can be opened from it. gitd pushes with the credential it keeps; the push is never forced. Commit your changes in the workspace first.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: no(), OpenWorldHint: yes()},
	}, tools.publish)
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

type publishInput struct {
	Path   string `json:"path" jsonschema:"absolute path of the workspace"`
	Branch string `json:"branch" jsonschema:"the new branch to push to, which must not exist yet"`
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
	out := describeOutput{URL: answer.GetUrl(), Commit: answer.GetCommit(), Branch: answer.GetBranch(), Modified: answer.GetModified(), Strategy: made(answer.GetStrategy())}
	for _, state := range answer.GetSubmodules() {
		out.Submodules = append(out.Submodules, submodule{Path: state.GetPath(), URL: state.GetUrl(), Commit: state.GetCommit(), CheckedOut: state.GetCheckedOut()})
	}
	return nil, out, nil
}

func (tools *tools) publish(ctx context.Context, _ *mcp.CallToolRequest, in publishInput) (*mcp.CallToolResult, publishOutput, error) {
	answer, err := tools.gitd.Publish(ctx, &gitdv1.PublishRequest{Path: in.Path, Branch: in.Branch})
	if err != nil {
		return nil, publishOutput{}, refusal(err)
	}
	return nil, publishOutput{Branch: answer.GetBranch(), Repositories: publishedOf(answer.GetRepository(), answer.GetSubmodules())}, nil
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
	out := workspaceOutput{Path: path, Commit: result.GetCommit(), Branch: result.GetBranch(), Strategy: made(result.GetStrategy())}
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
