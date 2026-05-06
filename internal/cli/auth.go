package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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
		Use:   "list",
		Short: "List authentication profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(opts)
		},
	})

	auth.AddCommand(&cobra.Command{
		Use:   "use <profile>",
		Short: "Set the current authentication profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUse(opts, args[0])
		},
	})

	var logout logoutOptions
	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove saved authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogout(opts, logout)
		},
	}
	logoutCmd.Flags().BoolVar(&logout.all, "all", false, "Remove all saved profiles")
	auth.AddCommand(logoutCmd)
}

type logoutOptions struct {
	all bool
}

type loginOptions struct {
	host     string
	apiKey   string
	username string
	password string
	stdin    bool
}

func runLogin(cmd *cobra.Command, opts *rootOptions, login loginOptions) error {
	fileCfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	profileName := selectedProfileName(opts, fileCfg)
	if strings.TrimSpace(profileName) == "" {
		return fmt.Errorf("profile name is required")
	}

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

	profile := config.Profile{
		Host:     host,
		APIKey:   apiKey,
		Username: username,
		Password: password,
	}
	result, err := checkAuth(cmd, opts, resolvedFromProfile(profileName, profile))
	if err != nil {
		return err
	}
	fileCfg.SetProfile(profileName, profile)
	fileCfg.CurrentProfile = profileName
	if err := config.Save(opts.configPath, fileCfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(opts.out, "Logged in to %s as %s using profile %s\n", host, result.User, profileName)
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
		_, err := fmt.Fprintf(opts.out, "No Redmine authentication configured for profile %s at %s\n", resolvedCfg.Profile, path)
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
	_, err = fmt.Fprintf(opts.out, "Profile: %s\nHost: %s\nAuth: %s\nUser: %s\nStatus: ok\nConfig: %s\n", resolvedCfg.Profile, resolvedCfg.Host, method, result.User, path)
	return err
}

func runList(opts *rootOptions) error {
	path := opts.configPath
	if path == "" {
		defaultPath, err := config.DefaultPath()
		if err != nil {
			return err
		}
		path = defaultPath
	}
	fileCfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	if len(fileCfg.Profiles) == 0 {
		_, err := fmt.Fprintf(opts.out, "No Redmine profiles configured at %s\n", path)
		return err
	}
	current := firstNonEmpty(fileCfg.CurrentProfile, config.DefaultProfileName)
	_, _ = fmt.Fprintf(opts.out, "Current profile: %s\nProfiles:\n", current)
	for _, name := range sortedProfileNames(fileCfg.Profiles) {
		marker := " "
		if name == current {
			marker = "*"
		}
		_, _ = fmt.Fprintf(opts.out, "%s %s\t%s\n", marker, name, fileCfg.Profiles[name].Host)
	}
	return nil
}

func runUse(opts *rootOptions, profileName string) error {
	if strings.TrimSpace(profileName) == "" {
		return fmt.Errorf("profile name is required")
	}
	fileCfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	if _, ok := fileCfg.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %q is not configured", profileName)
	}
	fileCfg.CurrentProfile = profileName
	if err := config.Save(opts.configPath, fileCfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(opts.out, "Current profile set to %s\n", profileName)
	return err
}

func runLogout(opts *rootOptions, logout logoutOptions) error {
	if logout.all {
		if err := config.Remove(opts.configPath); err != nil {
			return err
		}
		_, err := fmt.Fprintln(opts.out, "Logged out from all profiles")
		return err
	}

	fileCfg, err := config.Load(opts.configPath)
	if err != nil {
		return err
	}
	profileName := selectedProfileName(opts, fileCfg)
	if _, ok := fileCfg.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %q is not configured", profileName)
	}
	fileCfg.DeleteProfile(profileName)
	if len(fileCfg.Profiles) == 0 {
		if err := config.Remove(opts.configPath); err != nil {
			return err
		}
		_, err := fmt.Fprintf(opts.out, "Logged out from profile %s\n", profileName)
		return err
	}
	if fileCfg.CurrentProfile == "" {
		fileCfg.CurrentProfile = sortedProfileNames(fileCfg.Profiles)[0]
	}
	if err := config.Save(opts.configPath, fileCfg); err != nil {
		return err
	}
	_, err = fmt.Fprintf(opts.out, "Logged out from profile %s\n", profileName)
	return err
}

func sortedProfileNames(profiles map[string]config.Profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
