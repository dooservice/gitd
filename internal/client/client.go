// Package client calls a gitd on its Unix socket, for the command line and the MCP server.
//
// It speaks the Connect protocol over HTTP/1.1, which the server answers besides gRPC, so
// a stream of progress needs nothing more than the standard library.
package client

import (
	"context"
	"errors"
	"net"
	"net/http"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/gen/go/gitd/v1/gitdv1connect"
)

// Client is a gitd on one socket.
type Client struct {
	Workspaces   gitdv1connect.WorkspaceServiceClient
	Repositories gitdv1connect.RepositoryServiceClient
	Server       gitdv1connect.ServerServiceClient
	Connections  gitdv1connect.ConnectionServiceClient
	Publishing   gitdv1connect.PublishingServiceClient
}

// New returns the gitd that answers on socket. Nothing is dialled until the first call.
func New(socket string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	httpClient := &http.Client{Transport: transport}
	// The host is never looked up: every connection goes to the socket.
	const base = "http://gitd"
	return &Client{
		Workspaces:   gitdv1connect.NewWorkspaceServiceClient(httpClient, base),
		Repositories: gitdv1connect.NewRepositoryServiceClient(httpClient, base),
		Server:       gitdv1connect.NewServerServiceClient(httpClient, base),
		Connections:  gitdv1connect.NewConnectionServiceClient(httpClient, base),
		Publishing:   gitdv1connect.NewPublishingServiceClient(httpClient, base),
	}
}

// Progress is told what git is doing while a long operation runs.
type Progress func(*gitdv1.Progress)

// message is a message of one of the streams: progress, or the result that ends it.
type message[M any] interface {
	*M
	GetProgress() *gitdv1.Progress
}

// drain reads a stream to its end, telling progress what comes before the result and
// returning what result picks out of the last message.
func drain[M any, PM message[M], Result any](
	stream *connect.ServerStreamForClient[M], progress Progress, result func(PM) Result,
) (Result, error) {
	var zero, last Result
	got := false
	for stream.Receive() {
		received := PM(stream.Msg())
		if step := received.GetProgress(); step != nil {
			if progress != nil {
				progress(step)
			}
			continue
		}
		last, got = result(received), true
	}
	if err := stream.Err(); err != nil {
		return zero, err
	}
	if !got {
		return zero, errors.New("gitd ended the call without a result")
	}
	return last, nil
}

// Prepare makes a workspace, or moves the one gitd already made there.
func (client *Client) Prepare(
	ctx context.Context, request *gitdv1.PrepareRequest, progress Progress,
) (*gitdv1.WorkspaceResult, error) {
	stream, err := client.Workspaces.Prepare(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return drain(stream, progress, (*gitdv1.PrepareResponse).GetResult)
}

// Update brings a workspace to the tip of a branch.
func (client *Client) Update(
	ctx context.Context, request *gitdv1.UpdateRequest, progress Progress,
) (*gitdv1.WorkspaceResult, error) {
	stream, err := client.Workspaces.Update(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return drain(stream, progress, (*gitdv1.UpdateResponse).GetResult)
}

// Checkout moves a workspace to a commit.
func (client *Client) Checkout(
	ctx context.Context, request *gitdv1.CheckoutRequest, progress Progress,
) (*gitdv1.WorkspaceResult, error) {
	stream, err := client.Workspaces.Checkout(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return drain(stream, progress, (*gitdv1.CheckoutResponse).GetResult)
}

// SyncSubmodules checks out a workspace's submodules.
func (client *Client) SyncSubmodules(
	ctx context.Context, request *gitdv1.SyncSubmodulesRequest, progress Progress,
) (*gitdv1.SyncSubmodulesResult, error) {
	stream, err := client.Workspaces.SyncSubmodules(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return drain(stream, progress, (*gitdv1.SyncSubmodulesResponse).GetResult)
}

// Adopt takes an existing clone in.
func (client *Client) Adopt(
	ctx context.Context, request *gitdv1.AdoptRequest, progress Progress,
) (*gitdv1.AdoptResult, error) {
	stream, err := client.Workspaces.Adopt(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return drain(stream, progress, (*gitdv1.AdoptResponse).GetResult)
}

// Describe says what a workspace is on disk.
func (client *Client) Describe(ctx context.Context, path string) (*gitdv1.DescribeResponse, error) {
	answer, err := client.Workspaces.Describe(ctx, connect.NewRequest(&gitdv1.DescribeRequest{Path: path}))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Remove deletes a workspace.
func (client *Client) Remove(ctx context.Context, path string) error {
	_, err := client.Workspaces.Remove(ctx, connect.NewRequest(&gitdv1.RemoveRequest{Path: path}))
	return err
}

// Publish pushes what was committed in a workspace to a new branch.
func (client *Client) Publish(ctx context.Context, request *gitdv1.PublishRequest) (*gitdv1.PublishResponse, error) {
	answer, err := client.Workspaces.Publish(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Probe asks a repository whether it can be read, and for its branches.
func (client *Client) Probe(ctx context.Context, request *gitdv1.ProbeRequest) (*gitdv1.ProbeResponse, error) {
	answer, err := client.Repositories.Probe(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Info says which gitd answers.
func (client *Client) Info(ctx context.Context) (*gitdv1.InfoResponse, error) {
	answer, err := client.Server.Info(ctx, connect.NewRequest(&gitdv1.InfoRequest{}))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Retire removes now the pools nobody used for a while.
func (client *Client) Retire(ctx context.Context, unusedForSeconds int64) (*gitdv1.RetireResponse, error) {
	answer, err := client.Server.Retire(ctx, connect.NewRequest(&gitdv1.RetireRequest{UnusedForSeconds: unusedForSeconds}))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Audit reads the audit log: every publish, done or refused, newest first.
func (client *Client) Audit(ctx context.Context, request *gitdv1.AuditRequest) (*gitdv1.AuditResponse, error) {
	answer, err := client.Server.Audit(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// Reason is why gitd refused a call, as the contract names it, and what git said, read
// from the error's typed detail; empty when the error is not one of gitd's refusals.
func Reason(err error) (reason, said string) {
	var refused *connect.Error
	if !errors.As(err, &refused) {
		return "", ""
	}
	for _, detail := range refused.Details() {
		value, err := detail.Value()
		if err != nil {
			continue
		}
		if failure, ok := value.(*gitdv1.GitFailure); ok {
			return failure.GetReason().String(), failure.GetGitSaid()
		}
	}
	return "", refused.Message()
}
