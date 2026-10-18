package server

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/endpoint"
	"github.com/dooservice/gitd/internal/git"
	"github.com/dooservice/gitd/internal/pool"
	"github.com/dooservice/gitd/internal/repo"
)

// Endpoint is what the endpoint's calls need: its tokens and sandbox repositories, and the
// address sandboxes reach it at, e.g. `https://gitd.internal:9420`.
type Endpoint struct {
	Tokens    *endpoint.Tokens
	Sandboxes *endpoint.Sandboxes
	URL       string
	// Limits, when set, refuse a new sandbox repository once a tenant's disk is full.
	Limits interface{ WithinDisk() error }
}

// endpointHandlers answer EndpointService.
type endpointHandlers struct {
	server *Server
}

// IssueToken syncs the repository's pool, makes the sandbox repository when none is named,
// and issues a token for it.
func (handlers *endpointHandlers) IssueToken(
	ctx context.Context, request *connect.Request[gitdv1.IssueTokenRequest],
) (*connect.Response[gitdv1.IssueTokenResponse], error) {
	server := handlers.server
	asked := request.Msg
	repository, ok := repo.Of(asked.GetUrl())
	if !ok {
		return nil, server.refuse(git.Unnamed(asked.GetUrl()))
	}
	life := time.Duration(asked.GetLifetimeSeconds()) * time.Second
	if asked.GetLifetimeSeconds() < 0 || life > endpoint.MaxLife {
		return nil, server.refuse(git.Fail(git.BadAddress, "a token lives a day at most"))
	}
	sandboxes := server.Endpoint.Sandboxes
	var sandbox endpoint.Sandbox
	var err error
	if asked.GetSandbox() != "" {
		sandbox, err = sandboxes.Get(ctx, asked.GetSandbox())
		if err == nil && sandbox.Repository.HTTPS() != repository.HTTPS() {
			err = git.Fail(git.PathRefused, "sandbox %s is of %s, not of %s", sandbox.ID, sandbox.Repository.HTTPS(), repository.HTTPS())
		}
	} else {
		if limits := server.Endpoint.Limits; limits != nil {
			if err := limits.WithinDisk(); err != nil {
				return nil, server.refuse(err)
			}
		}
		err = server.Pools.Sync(ctx, repository, nil, pool.OnlyBranch(asked.GetBranch()), nil)
		if err == nil {
			sandbox, err = sandboxes.Make(ctx, repository, asked.GetBranch())
		}
	}
	if err != nil {
		return nil, server.refuse(err)
	}
	secret, token, err := server.Endpoint.Tokens.Issue(repository, sandbox.ID, !asked.GetReadOnly(), life)
	if err != nil {
		return nil, server.refuse(err)
	}
	server.recordChange(gitdv1.ConfigurationChange_ACTION_TOKEN_ISSUED, token.ID, repository.HTTPS())
	return connect.NewResponse(&gitdv1.IssueTokenResponse{
		Token: secret, Info: tokenInfo(token), CloneUrl: cloneURL(server.Endpoint.URL, repository),
	}), nil
}

// RevokeToken ends a token, and removes its sandbox repository when asked.
func (handlers *endpointHandlers) RevokeToken(
	ctx context.Context, request *connect.Request[gitdv1.RevokeTokenRequest],
) (*connect.Response[gitdv1.RevokeTokenResponse], error) {
	server := handlers.server
	token, ok := server.Endpoint.Tokens.Revoke(request.Msg.GetId())
	if !ok {
		return nil, server.refuse(git.Fail(git.NotFound, "there is no token %s alive", request.Msg.GetId()))
	}
	server.recordChange(gitdv1.ConfigurationChange_ACTION_TOKEN_REVOKED, token.ID, token.Repository.HTTPS())
	if request.Msg.GetRemoveSandbox() {
		server.Endpoint.Tokens.RevokeSandbox(token.Sandbox)
		if err := server.Endpoint.Sandboxes.Remove(ctx, token.Sandbox); err != nil {
			return nil, server.refuse(err)
		}
	}
	return connect.NewResponse(&gitdv1.RevokeTokenResponse{}), nil
}

// ListTokens lists the tokens alive.
func (handlers *endpointHandlers) ListTokens(
	context.Context, *connect.Request[gitdv1.ListTokensRequest],
) (*connect.Response[gitdv1.ListTokensResponse], error) {
	answer := &gitdv1.ListTokensResponse{}
	for _, token := range handlers.server.Endpoint.Tokens.Alive() {
		answer.Tokens = append(answer.Tokens, tokenInfo(token))
	}
	return connect.NewResponse(answer), nil
}

// publishPushed publishes what a sandbox pushed to its sandbox repository.
func (server *Server) publishPushed(
	ctx context.Context, request *gitdv1.PublishRequest,
) (*connect.Response[gitdv1.PublishResponse], error) {
	if server.Endpoint == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this gitd serves no endpoint"))
	}
	if request.GetPath() != "" {
		return nil, server.refuse(git.Fail(git.BadAddress, "a publish is of a workspace or of a sandbox, not both"))
	}
	sandbox, err := server.Endpoint.Sandboxes.Get(ctx, request.GetSandbox())
	if err != nil {
		return nil, server.refuse(err)
	}
	published, err := server.Workspaces.PublishPushed(ctx, request, sandbox.Repository, server.Endpoint.Sandboxes.Path(sandbox.ID))
	if err != nil {
		return nil, server.refuse(err)
	}
	return connect.NewResponse(published), nil
}

func tokenInfo(token endpoint.Token) *gitdv1.TokenInfo {
	return &gitdv1.TokenInfo{
		Id: token.ID, Url: token.Repository.HTTPS(), Sandbox: token.Sandbox, Writable: token.Writable,
		IssuedAt: timestamppb.New(token.Issued), ExpiresAt: timestamppb.New(token.Expires),
	}
}

// cloneURL is where a sandbox clones repository from, at the endpoint's address.
func cloneURL(base string, repository repo.Repo) string {
	parsed, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil || base == "" {
		return ""
	}
	parsed.Path += endpoint.Prefix + repository.Host + "/" + repository.Path + ".git"
	return parsed.String()
}
