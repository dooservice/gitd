package client

import (
	"context"

	"connectrpc.com/connect"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// unary calls one method and answers its message.
func unary[Request, Response any](
	ctx context.Context, call func(context.Context, *connect.Request[Request]) (*connect.Response[Response], error), request *Request,
) (*Response, error) {
	answer, err := call(ctx, connect.NewRequest(request))
	if err != nil {
		return nil, err
	}
	return answer.Msg, nil
}

// ListConnections is the credentials gitd keeps, never a secret.
func (client *Client) ListConnections(ctx context.Context) (*gitdv1.ListConnectionsResponse, error) {
	return unary(ctx, client.Connections.ListConnections, &gitdv1.ListConnectionsRequest{})
}

// RemoveConnection forgets a connection made through the API.
func (client *Client) RemoveConnection(ctx context.Context, id string) error {
	_, err := unary(ctx, client.Connections.RemoveConnection, &gitdv1.RemoveConnectionRequest{Id: id})
	return err
}

// AddToken keeps a token for the repositories under a prefix.
func (client *Client) AddToken(ctx context.Context, request *gitdv1.AddTokenRequest) (*gitdv1.AddTokenResponse, error) {
	return unary(ctx, client.Connections.AddToken, request)
}

// AddGitHubApp keeps a GitHub App by its ID and private key.
func (client *Client) AddGitHubApp(ctx context.Context, request *gitdv1.AddGitHubAppRequest) (*gitdv1.AddGitHubAppResponse, error) {
	return unary(ctx, client.Connections.AddGitHubApp, request)
}

// BeginGitHubApp starts creating a GitHub App from a manifest.
func (client *Client) BeginGitHubApp(ctx context.Context, request *gitdv1.BeginGitHubAppRequest) (*gitdv1.BeginGitHubAppResponse, error) {
	return unary(ctx, client.Connections.BeginGitHubApp, request)
}

// CompleteGitHubApp finishes creating a GitHub App with the code GitHub sent back.
func (client *Client) CompleteGitHubApp(ctx context.Context, request *gitdv1.CompleteGitHubAppRequest) (*gitdv1.CompleteGitHubAppResponse, error) {
	return unary(ctx, client.Connections.CompleteGitHubApp, request)
}

// StartGitHubLogin asks for the code a person enters to sign in to GitHub.
func (client *Client) StartGitHubLogin(ctx context.Context, host string) (*gitdv1.StartGitHubLoginResponse, error) {
	return unary(ctx, client.Connections.StartGitHubLogin, &gitdv1.StartGitHubLoginRequest{Host: host})
}

// FinishGitHubLogin waits until the person entered the code, and keeps the sign-in.
func (client *Client) FinishGitHubLogin(ctx context.Context, loginID string) (*gitdv1.FinishGitHubLoginResponse, error) {
	return unary(ctx, client.Connections.FinishGitHubLogin, &gitdv1.FinishGitHubLoginRequest{LoginId: loginID})
}

// GetSettings is every publishing setting.
func (client *Client) GetSettings(ctx context.Context) (*gitdv1.GetSettingsResponse, error) {
	return unary(ctx, client.Publishing.GetSettings, &gitdv1.GetSettingsRequest{})
}

// SetSettings keeps the publishing settings of a prefix.
func (client *Client) SetSettings(ctx context.Context, settings *gitdv1.PublishSettings) (*gitdv1.SetSettingsResponse, error) {
	return unary(ctx, client.Publishing.SetSettings, &gitdv1.SetSettingsRequest{Settings: settings})
}

// RemoveSettings forgets the publishing settings of a prefix.
func (client *Client) RemoveSettings(ctx context.Context, prefix string) error {
	_, err := unary(ctx, client.Publishing.RemoveSettings, &gitdv1.RemoveSettingsRequest{UrlPrefix: prefix})
	return err
}
