package redmine

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/muxx/redmine-cli/internal/openapi"
)

func TestClientDoBuildsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/redmine/issues/42.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query()["include"]; len(got) != 2 || got[0] != "journals" || got[1] != "watchers" {
			t.Fatalf("include query = %#v", got)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issue":{"id":42}}`))
	}))
	defer server.Close()

	client := Client{
		BaseURL:    server.URL + "/redmine",
		APIKey:     "secret",
		HTTPClient: server.Client(),
	}
	resp, err := client.Do(t.Context(), Request{
		Operation: openapi.Operation{
			ID:     "getIssue",
			Method: http.MethodGet,
			Path:   "/issues/{issue_id}.{format}",
			PathParams: []openapi.Parameter{{
				Name: "issue_id",
			}},
		},
		Path:  map[string]string{"issue_id": "42"},
		Query: map[string][]string{"include": {"journals", "watchers"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestClientEndpointEscapesPathParamsOnce(t *testing.T) {
	client := Client{BaseURL: "https://redmine.example/redmine"}
	operation := openapi.Operation{
		ID:     "getWikiPage",
		Method: http.MethodGet,
		Path:   "/projects/{project_id}/wiki/{wiki_page_title}.{format}",
		PathParams: []openapi.Parameter{
			{Name: "project_id"},
			{Name: "wiki_page_title"},
		},
	}

	tests := []struct {
		name  string
		title string
	}{
		{
			name:  "non-ASCII wiki page title",
			title: "AI-трансформация_RD",
		},
		{
			name:  "reserved path characters",
			title: "100% done/with space",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.endpoint(Request{
				Operation: operation,
				Path: map[string]string{
					"project_id":      "r-handbook",
					"wiki_page_title": tt.title,
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			want := "https://redmine.example/redmine/projects/r-handbook/wiki/" + url.PathEscape(tt.title) + ".json"
			if got != want {
				t.Fatalf("endpoint = %s, want %s", got, want)
			}
		})
	}
}
