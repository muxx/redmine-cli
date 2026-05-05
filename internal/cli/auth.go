package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/muxx/redmine-cli/internal/config"
	"github.com/muxx/redmine-cli/internal/openapi"
	"github.com/muxx/redmine-cli/internal/redmine"
	"github.com/spf13/cobra"
)

func addAuthCommands(root *cobra.Command, opts *rootOptions) {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}
	root.AddCommand(auth)

	var login loginOptions
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Verify and save Redmine authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, opts, login)
		},
	}
	loginFlags := loginCmd.Flags()
	loginFlags.StringVar(&login.host, "host", "", "Redmine base URL")
	loginFlags.StringVar(&login.apiKey, "api-key", "", "Redmine API key")
	loginFlags.StringVar(&login.username, "username", "", "Basic auth username")
	loginFlags.StringVar(&login.password, "password", "", "Basic auth password")
	loginFlags.BoolVar(&login.stdin, "stdin", false, "Read API key from stdin")
	auth.AddCommand(loginCmd)

	auth.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check configured authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, opts)
		},
	})

	auth.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Remove saved authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Remove(opts.configPath); err != nil {
				return err
			}
			_, err := fmt.Fprintln(opts.out, "Logged out")
			return err
		},
	})
}

type loginOptions struct {
	host     string
	apiKey   string
	username string
	password string
	stdin    bool
}

func runLogin(cmd *cobra.Command, opts *rootOptions, login loginOptions) error {
	host := firstNonEmpty(login.host, opts.host)
	if host == "" {
		return fmt.Errorf("--host is required")
	}
	apiKey := firstNonEmpty(login.apiKey, opts.apiKey)
	if login.stdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		apiKey = strings.TrimSpace(string(data))
	}
	username := firstNonEmpty(login.username, opts.username)
	password := firstNonEmpty(login.password, opts.password)
	if apiKey == "" && username == "" && password == "" {
		return fmt.Errorf("provide --api-key or --username/--password")
	}

	cfg := config.Config{
		Host:     host,
		APIKey:   apiKey,
		Username: username,
		Password: password,
	}
	result, err := checkAuth(cmd, opts, resolvedFromConfig(cfg))
	if err != nil {
		return err
	}
	if err := config.Save(opts.configPath, cfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(opts.out, "Logged in to %s as %s\n", host, result.User)
	return err
}

func runStatus(cmd *cobra.Command, opts *rootOptions) error {
	path := opts.configPath
	if path == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return err
		}
		path = defaultPath
	}
	resolvedCfg, err := resolvedConfig(opts)
	if err != nil {
		return err
	}
	if resolvedCfg.Host == "" {
		_, err := fmt.Fprintf(opts.out, "No Redmine authentication configured at %s\n", path)
		return err
	}
	result, err := checkAuth(cmd, opts, resolvedCfg)
	if err != nil {
		return err
	}
	method := "none"
	switch {
	case resolvedCfg.APIKey != "":
		method = "api-key"
	case resolvedCfg.Username != "" || resolvedCfg.Password != "":
		method = "basic"
	}
	_, err = fmt.Fprintf(opts.out, "Host: %s\nAuth: %s\nUser: %s\nStatus: ok\nConfig: %s\n", resolvedCfg.Host, method, result.User, path)
	return err
}

type authCheckResult struct {
	User string
}

func checkAuth(cmd *cobra.Command, opts *rootOptions, cfg resolved) (authCheckResult, error) {
	if cfg.Host == "" {
		return authCheckResult{}, fmt.Errorf("redmine host is not configured")
	}
	if cfg.APIKey == "" && cfg.Username == "" && cfg.Password == "" {
		return authCheckResult{}, fmt.Errorf("redmine authentication is not configured")
	}

	httpClient := opts.httpClient
	if httpClient == nil {
		httpClient = redmine.NewHTTPClient(opts.timeout, opts.insecure)
	}
	client := redmine.Client{
		BaseURL:    cfg.Host,
		APIKey:     cfg.APIKey,
		Username:   cfg.Username,
		Password:   cfg.Password,
		SwitchUser: cfg.SwitchUser,
		HTTPClient: httpClient,
	}
	resp, err := client.Do(cmd.Context(), redmine.Request{
		Operation: currentUserOperation(),
	})
	if err != nil {
		return authCheckResult{}, fmt.Errorf("authentication check failed: %w", err)
	}
	return authCheckResult{User: currentUserLabel(resp.Body)}, nil
}

func currentUserOperation() openapi.Operation {
	for _, op := range openapi.Operations {
		if op.ID == "getCurrentUser" {
			return op
		}
	}
	return openapi.Operation{
		ID:     "getCurrentUser",
		Method: http.MethodGet,
		Path:   "/users/current.{format}",
	}
}

func currentUserLabel(data []byte) string {
	var payload struct {
		User struct {
			ID        int    `json:"id"`
			Login     string `json:"login"`
			Firstname string `json:"firstname"`
			Lastname  string `json:"lastname"`
			Mail      string `json:"mail"`
		} `json:"user"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "authenticated user"
	}
	user := payload.User
	switch {
	case user.Login != "":
		return user.Login
	case user.Firstname != "" || user.Lastname != "":
		return strings.TrimSpace(user.Firstname + " " + user.Lastname)
	case user.Mail != "":
		return user.Mail
	case user.ID > 0:
		return fmt.Sprintf("user #%d", user.ID)
	default:
		return "authenticated user"
	}
}
