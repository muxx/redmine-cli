package redmine

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/muxx/redmine-cli/internal/openapi"
)

const defaultTimeout = 30 * time.Second

// Client executes generated Redmine API operations.
type Client struct {
	BaseURL    string
	APIKey     string
	Username   string
	Password   string
	SwitchUser string
	HTTPClient *http.Client
}

// Request contains runtime values for an OpenAPI operation.
type Request struct {
	Operation openapi.Operation
	Path      map[string]string
	Query     map[string][]string
	Headers   map[string]string
	Body      []byte
}

// Response is an HTTP response snapshot.
type Response struct {
	StatusCode  int
	Header      http.Header
	Body        []byte
	ContentType string
}

// HTTPError describes a non-2xx API response.
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	body := strings.TrimSpace(string(e.Body))
	if body == "" {
		return fmt.Sprintf("redmine API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("redmine API returned HTTP %d: %s", e.StatusCode, body)
}

// NewHTTPClient returns an HTTP client suitable for CLI use.
func NewHTTPClient(timeout time.Duration, insecure bool) *http.Client {
	if timeout == 0 {
		timeout = defaultTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		// Controlled by the explicit --insecure CLI flag.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// Do executes a generated operation.
func (c Client) Do(ctx context.Context, req Request) (Response, error) {
	if c.BaseURL == "" {
		return Response{}, fmt.Errorf("redmine host is not configured; set REDMINE_HOST or run `redmine auth login --profile <name>`")
	}
	endpoint, err := c.endpoint(req)
	if err != nil {
		return Response{}, err
	}

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Operation.Method, endpoint, body)
	if err != nil {
		return Response{}, err
	}
	if req.Operation.ResponseBinary {
		httpReq.Header.Set("Accept", "*/*")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	if len(req.Body) > 0 && req.Operation.Body != nil && req.Operation.Body.ContentType != "" {
		httpReq.Header.Set("Content-Type", req.Operation.Body.ContentType)
	}
	for name, value := range req.Headers {
		if value == "" {
			continue
		}
		httpReq.Header.Set(name, value)
	}
	if c.SwitchUser != "" {
		httpReq.Header.Set("X-Redmine-Switch-User", c.SwitchUser)
	}
	if c.APIKey != "" {
		httpReq.Header.Set("X-Redmine-API-Key", c.APIKey)
	}
	if c.Username != "" || c.Password != "" {
		httpReq.SetBasicAuth(c.Username, c.Password)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = NewHTTPClient(defaultTimeout, false)
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	data, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}
	resp := Response{
		StatusCode:  httpResp.StatusCode,
		Header:      httpResp.Header.Clone(),
		Body:        data,
		ContentType: httpResp.Header.Get("Content-Type"),
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		return resp, &HTTPError{StatusCode: httpResp.StatusCode, Body: data}
	}
	return resp, nil
}

func (c Client) endpoint(req Request) (string, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("redmine host must include scheme and host, got %q", c.BaseURL)
	}

	basePath := strings.TrimRight(base.Path, "/")
	baseEscapedPath := strings.TrimRight(base.EscapedPath(), "/")

	decodedPath := req.Operation.Path
	escapedPath := req.Operation.Path
	decodedPath = strings.ReplaceAll(decodedPath, "{format}", "json")
	escapedPath = strings.ReplaceAll(escapedPath, "{format}", "json")
	for _, param := range req.Operation.PathParams {
		value, ok := req.Path[param.Name]
		if !ok || value == "" {
			return "", fmt.Errorf("missing path argument %s", param.Name)
		}
		placeholder := "{" + param.Name + "}"
		decodedPath = strings.ReplaceAll(decodedPath, placeholder, value)
		escapedPath = strings.ReplaceAll(escapedPath, placeholder, url.PathEscape(value))
	}

	base.Path = basePath + decodedPath
	base.RawPath = baseEscapedPath + escapedPath
	query := base.Query()
	for name, values := range req.Query {
		for _, value := range values {
			query.Add(name, value)
		}
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}
