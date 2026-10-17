package githubapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// What every call to GitHub's API sends and is bounded by.
const (
	apiVersion     = "2022-11-28"
	apiTimeout     = 30 * time.Second
	maxAnswerBytes = 1 << 20
)

// API calls GitHub's REST API at one address: api.github.com, or a GitHub Enterprise
// Server's. A redirect is never followed, so a token is sent only where it was meant for.
type API struct {
	base      string
	userAgent string
	client    *http.Client
}

// APIError is an answer GitHub gave other than success: its status and its message.
type APIError struct {
	Status  int
	Message string
}

func (failure *APIError) Error() string {
	return fmt.Sprintf("GitHub answered %d: %s", failure.Status, failure.Message)
}

// NewAPI returns the API at base, calling as userAgent (gitd and its version).
func NewAPI(base, userAgent string) *API {
	return &API{base: base, userAgent: userAgent, client: &http.Client{
		Timeout: apiTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// APIBase is the address of the API of a GitHub host: api.github.com for github.com, and
// `/api/v3` on the host, on its HTTPS port, for a GitHub Enterprise Server.
func APIBase(host string, httpsPort int) string {
	if host == "" || host == "github.com" {
		return "https://api.github.com"
	}
	if httpsPort != 0 && httpsPort != 443 {
		host += ":" + strconv.Itoa(httpsPort)
	}
	return "https://" + host + "/api/v3"
}

// failureMessage is what GitHub said of a failure: its message, and the message or field
// and code of each error it lists, as a validation failure says what was invalid only there.
func failureMessage(status int, data []byte) string {
	var said struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(data, &said)
	message := said.Message
	if message == "" {
		message = http.StatusText(status)
	}
	var details []string
	for _, listed := range said.Errors {
		switch {
		case listed.Message != "":
			details = append(details, listed.Message)
		case listed.Field != "":
			details = append(details, listed.Field+" "+listed.Code)
		}
	}
	if len(details) > 0 {
		message += ": " + strings.Join(details, "; ")
	}
	return message
}

// call sends a request with an Authorization header (empty for none) and, when given, a
// JSON body, and reads the JSON answer into answer.
func (api *API) call(ctx context.Context, method, path, authorization string, body, answer any) error {
	var sent io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		sent = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, api.base+path, sent)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", api.userAgent)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := api.client.Do(request)
	if err != nil {
		return fmt.Errorf("calling GitHub: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	if err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &APIError{Status: response.StatusCode, Message: failureMessage(response.StatusCode, data)}
	}
	if answer == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, answer); err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	return nil
}
