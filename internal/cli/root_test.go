package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/muxx/redmine-cli/internal/config"
)

func TestGeneratedCreateCommandSendsBody(t *testing.T) {
	t.Setenv("REDMINE_HOST", "")
	t.Setenv("REDMINE_API_KEY", "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/issues.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "secret" {
			t.Fatalf("api key header = %q", got)
		}
		var body map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		issue := body["issue"]
		if issue["project_id"] != "demo" {
			t.Fatalf("project_id = %#v", issue["project_id"])
		}
		if issue["subject"] != "Fix checkout" {
			t.Fatalf("subject = %#v", issue["subject"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issue":{"id":100}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{
		"--config", t.TempDir() + "/missing.yml",
		"--host", server.URL,
		"--api-key", "secret",
		"issue", "create",
		"--project-id", "demo",
		"--subject", "Fix checkout",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"id": 100`)) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestGeneratedShowCommandUsesPathAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/issues/42.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("include"); got != "journals" {
			t.Fatalf("include = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issue":{"id":42}}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{
		"--config", t.TempDir() + "/missing.yml",
		"--host", server.URL,
		"issue", "show", "42",
		"--include", "journals",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"id": 42`)) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestAuthLoginChecksBeforeSaving(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/current.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":7,"login":"jsmith"}}`))
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{
		"--config", configPath,
		"auth", "login",
		"--host", server.URL,
		"--api-key", "secret",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Logged in to "+server.URL+" as jsmith") {
		t.Fatalf("output = %s", out.String())
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.Profiles[config.DefaultProfileName]
	if cfg.CurrentProfile != config.DefaultProfileName || profile.Host != server.URL || profile.APIKey != "secret" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestAuthLoginWithProfileSetsCurrentProfile(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/current.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"login":"profile-user"}}`))
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{
		"--config", configPath,
		"auth", "login",
		"--profile", "work",
		"--host", server.URL,
		"--api-key", "secret",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "using profile work") {
		t.Fatalf("output = %s", out.String())
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProfile != "work" {
		t.Fatalf("current profile = %q", cfg.CurrentProfile)
	}
	if got := cfg.Profiles["work"].Host; got != server.URL {
		t.Fatalf("profile host = %q", got)
	}
}

func TestAuthLoginDoesNotSaveWhenCheckFails(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{
		"--config", configPath,
		"auth", "login",
		"--host", server.URL,
		"--api-key", "bad",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected auth check error")
	}
	if !strings.Contains(err.Error(), "authentication check failed") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file exists after failed login: %v", statErr)
	}
}

func TestAuthStatusChecksSavedConfig(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/current.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"id":7,"login":"jsmith"}}`))
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "work",
		Profiles: map[string]config.Profile{
			"work": {Host: server.URL, APIKey: "secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{"--config", configPath, "auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	for _, want := range []string{"Profile: work", "Auth: api-key", "User: jsmith", "Status: ok"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

func TestGeneratedCommandUsesSelectedProfile(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/issues/42.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "work-secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issue":{"id":42}}`))
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Host: "https://wrong.example", APIKey: "wrong"},
			"work":    {Host: server.URL, APIKey: "work-secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{"--config", configPath, "--profile", "work", "issue", "show", "42"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"id": 42`)) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestEnvProfileSelectsSavedProfile(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Redmine-API-Key"); got != "env-profile-secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issue":{"id":42}}`))
	}))
	defer server.Close()

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Host: "https://wrong.example", APIKey: "wrong"},
			"ci":      {Host: server.URL, APIKey: "env-profile-secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDMINE_PROFILE", "ci")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{"--config", configPath, "issue", "show", "42"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
}

func TestAuthUseSwitchesCurrentProfile(t *testing.T) {
	clearRedmineEnv(t)

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {Host: "https://default.example", APIKey: "default-secret"},
			"work":    {Host: "https://work.example", APIKey: "work-secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, nil)
	cmd.SetArgs([]string{"--config", configPath, "auth", "use", "work"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProfile != "work" {
		t.Fatalf("current profile = %q", cfg.CurrentProfile)
	}
}

func TestAuthListMarksCurrentProfile(t *testing.T) {
	clearRedmineEnv(t)

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "work",
		Profiles: map[string]config.Profile{
			"default": {Host: "https://default.example", APIKey: "default-secret"},
			"work":    {Host: "https://work.example", APIKey: "work-secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, nil)
	cmd.SetArgs([]string{"--config", configPath, "auth", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	for _, want := range []string{"Current profile: work", "* work\thttps://work.example", "  default\thttps://default.example"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

func TestAuthLogoutRemovesSelectedProfileAndKeepsAnotherCurrent(t *testing.T) {
	clearRedmineEnv(t)

	configPath := t.TempDir() + "/config.yml"
	if err := config.Save(configPath, config.Config{
		CurrentProfile: "work",
		Profiles: map[string]config.Profile{
			"default": {Host: "https://default.example", APIKey: "default-secret"},
			"work":    {Host: "https://work.example", APIKey: "work-secret"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, nil)
	cmd.SetArgs([]string{"--config", configPath, "auth", "logout"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profiles["work"]; ok {
		t.Fatalf("work profile still exists: %#v", cfg)
	}
	if cfg.CurrentProfile != "default" {
		t.Fatalf("current profile = %q", cfg.CurrentProfile)
	}
}

func TestAuthStatusChecksEnvConfig(t *testing.T) {
	clearRedmineEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/current.json" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("X-Redmine-API-Key"); got != "env-secret" {
			t.Fatalf("api key header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"login":"env-user"}}`))
	}))
	defer server.Close()

	t.Setenv("REDMINE_HOST", server.URL)
	t.Setenv("REDMINE_API_KEY", "env-secret")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewWithIO("test", bytes.NewReader(nil), &out, &errOut, server.Client())
	cmd.SetArgs([]string{"--config", t.TempDir() + "/missing.yml", "auth", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "User: env-user") {
		t.Fatalf("output = %s", out.String())
	}
}

func clearRedmineEnv(t *testing.T) {
	t.Helper()
	t.Setenv("REDMINE_HOST", "")
	t.Setenv("REDMINE_API_KEY", "")
	t.Setenv("REDMINE_USERNAME", "")
	t.Setenv("REDMINE_PASSWORD", "")
	t.Setenv("REDMINE_SWITCH_USER", "")
	t.Setenv("REDMINE_PROFILE", "")
}
