package githubapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

// homepage is the address every App gitd creates names as its own.
const homepage = "https://gitd.dev"

// manifestPermissions are what an App gitd creates may do, and no more: push branches and
// open pull requests.
var manifestPermissions = map[string]string{"contents": "write", "pull_requests": "write", "metadata": "read"}

// Manifest is the manifest of an App named name, which GitHub sends back to redirectURL
// with a code: private, no webhook, only the permissions publishing takes.
func Manifest(name, redirectURL string) (string, error) {
	manifest := map[string]any{
		"name":                name,
		"url":                 homepage,
		"description":         "Publishes the work of AI agents through gitd: branches and pull requests, never a token in a sandbox.",
		"redirect_url":        redirectURL,
		"public":              false,
		"default_permissions": manifestPermissions,
		"hook_attributes":     map[string]any{"url": homepage, "active": false},
	}
	data, err := json.Marshal(manifest)
	return string(data), err
}

// WebBase is the address of a GitHub host's pages.
func WebBase(host string, httpsPort int) string {
	if httpsPort != 0 && httpsPort != 443 {
		host += ":" + strconv.Itoa(httpsPort)
	}
	return "https://" + host
}

// FormAction is where a browser posts a manifest to create an App owned by organization,
// or by the person signed in when organization is empty, carrying state back.
func FormAction(webBase, organization, state string) string {
	path := "/settings/apps/new"
	if organization != "" {
		path = "/organizations/" + url.PathEscape(organization) + "/settings/apps/new"
	}
	return webBase + path + "?state=" + url.QueryEscape(state)
}

// Created is an App GitHub created from a manifest: its ID, its name in addresses, its
// page, who owns it, and its private key.
type Created struct {
	ID      int64  `json:"id"`
	Slug    string `json:"slug"`
	HTMLURL string `json:"html_url"`
	PEM     string `json:"pem"`
	Owner   struct {
		Login string `json:"login"`
	} `json:"owner"`
}

// Convert exchanges the code GitHub sent back from a manifest for the App it created. The
// code alone is the proof: the call carries no other credential.
func (api *API) Convert(ctx context.Context, code string) (Created, error) {
	var created Created
	err := api.call(ctx, http.MethodPost, "/app-manifests/"+url.PathEscape(code)+"/conversions", "", nil, &created)
	return created, err
}
