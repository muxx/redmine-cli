package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/muxx/redmine-cli/internal/config"
	"github.com/muxx/redmine-cli/internal/openapi"
	"github.com/muxx/redmine-cli/internal/redmine"
	"github.com/spf13/cobra"
)

// New returns the root CLI command.
func New(version string) *cobra.Command {
	return NewWithIO(version, os.Stdin, os.Stdout, os.Stderr, nil)
}

// NewWithIO returns the root CLI command with injectable streams and HTTP client.
func NewWithIO(version string, in io.Reader, out, errOut io.Writer, httpClient *http.Client) *cobra.Command {
	opts := &rootOptions{
		version:    version,
		in:         in,
		out:        out,
		errOut:     errOut,
		httpClient: httpClient,
		output:     redmine.OutputJSON,
		timeout:    30 * time.Second,
	}

	root := &cobra.Command{
		Use:           "redmine",
		Short:         "Work with Redmine from the command line",
		Long:          "Work with Redmine from the command line using commands generated from the Redmine OpenAPI specification.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)

	flags := root.PersistentFlags()
	flags.StringVar(&opts.configPath, "config", "", "Config file path")
	flags.StringVar(&opts.host, "host", "", "Redmine base URL")
	flags.StringVar(&opts.apiKey, "api-key", "", "Redmine API key")
	flags.StringVar(&opts.username, "username", "", "Basic auth username")
	flags.StringVar(&opts.password, "password", "", "Basic auth password")
	flags.StringVar(&opts.switchUser, "switch-user", "", "Send X-Redmine-Switch-User")
	flags.StringVarP(&opts.output, "output", "o", redmine.OutputJSON, "Output format: json, yaml, raw")
	flags.DurationVar(&opts.timeout, "timeout", 30*time.Second, "HTTP timeout")
	flags.BoolVar(&opts.insecure, "insecure", false, "Skip TLS certificate verification")

	addAuthCommands(root, opts)
	addGeneratedCommands(root, opts, openapi.Operations)
	return root
}

type rootOptions struct {
	version    string
	in         io.Reader
	out        io.Writer
	errOut     io.Writer
	httpClient *http.Client

	configPath string
	host       string
	apiKey     string
	username   string
	password   string
	switchUser string
	output     string
	timeout    time.Duration
	insecure   bool
}

func addGeneratedCommands(root *cobra.Command, opts *rootOptions, operations []openapi.Operation) {
	groups := map[string]*cobra.Command{}
	for _, op := range operations {
		group := groups[op.Group]
		if group == nil {
			group = &cobra.Command{
				Use:     op.Group,
				Aliases: op.GroupAlias,
				Short:   "Manage " + strings.ReplaceAll(op.Group, "-", " "),
			}
			root.AddCommand(group)
			groups[op.Group] = group
		}
		group.AddCommand(operationCommand(opts, op))
	}
}

func operationCommand(opts *rootOptions, op openapi.Operation) *cobra.Command {
	flags := &operationFlags{
		query:   map[string]*parameterValues{},
		headers: map[string]*parameterValues{},
		body:    map[string]*bodyValues{},
	}

	cmd := &cobra.Command{
		Use:   op.Command + usageArgs(op.PathParams),
		Short: op.Summary,
		Long:  operationLong(op),
		Args:  cobra.ExactArgs(len(op.PathParams)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOperation(cmd, opts, op, flags, args)
		},
	}

	cmdFlags := cmd.Flags()
	cmdFlags.StringArrayVar(&flags.extraQuery, "param", nil, "Additional query parameter as key=value")
	cmdFlags.StringArrayVar(&flags.extraHeaders, "header", nil, "Additional HTTP header as Name=value")
	for _, param := range op.QueryParams {
		if reservedFlag(param.Flag) {
			continue
		}
		values := &parameterValues{param: param}
		flags.query[param.Name] = values
		cmdFlags.StringArrayVar(&values.values, param.Flag, nil, flagDescription(param.Description, param.Type, param.Required, param.Enum))
	}
	for _, param := range op.HeaderParams {
		values := &parameterValues{param: param}
		flags.headers[param.Name] = values
		if cmdFlags.Lookup(param.Flag) != nil || reservedFlag(param.Flag) {
			continue
		}
		cmdFlags.StringArrayVar(&values.values, param.Flag, nil, flagDescription(param.Description, param.Type, param.Required, param.Enum))
	}
	if op.Body != nil {
		if op.Body.Binary {
			cmdFlags.StringVar(&flags.input, "input", "", "File to send as request body; use - for stdin")
		} else {
			cmdFlags.StringVar(&flags.rawBody, "body", "", "Raw JSON request body or @file")
			cmdFlags.StringArrayVar(&flags.extraFields, "field", nil, "Additional body field as key=value")
			for _, field := range op.Body.Fields {
				if cmdFlags.Lookup(field.Flag) != nil || reservedFlag(field.Flag) {
					continue
				}
				values := &bodyValues{field: field}
				flags.body[field.Name] = values
				cmdFlags.StringArrayVar(&values.values, field.Flag, nil, flagDescription(field.Description, field.Type, field.Required, field.Enum))
			}
		}
	}

	return cmd
}

func reservedFlag(name string) bool {
	switch name {
	case "api-key", "body", "config", "field", "header", "help", "host", "input", "insecure", "output", "param", "password", "switch-user", "timeout", "username", "version":
		return true
	default:
		return false
	}
}

type operationFlags struct {
	query        map[string]*parameterValues
	headers      map[string]*parameterValues
	body         map[string]*bodyValues
	extraQuery   []string
	extraHeaders []string
	extraFields  []string
	rawBody      string
	input        string
}

type parameterValues struct {
	param  openapi.Parameter
	values []string
}

type bodyValues struct {
	field  openapi.BodyField
	values []string
}

func runOperation(cmd *cobra.Command, opts *rootOptions, op openapi.Operation, flags *operationFlags, args []string) error {
	cfg, err := resolvedConfig(opts)
	if err != nil {
		return err
	}

	path := map[string]string{}
	for i, param := range op.PathParams {
		path[param.Name] = args[i]
	}
	query, err := collectParameters(flags.query, flags.extraQuery)
	if err != nil {
		return err
	}
	headers, err := collectHeaders(flags.headers, flags.extraHeaders)
	if err != nil {
		return err
	}
	body, err := buildBody(cmd, op, flags)
	if err != nil {
		return err
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
		Operation: op,
		Path:      path,
		Query:     query,
		Headers:   headers,
		Body:      body,
	})
	if err != nil {
		return err
	}
	return redmine.WriteResponse(opts.out, resp, opts.output)
}

type resolved struct {
	Host       string
	APIKey     string
	Username   string
	Password   string
	SwitchUser string
}

func resolvedFromConfig(cfg config.Config) resolved {
	return resolved{
		Host:     cfg.Host,
		APIKey:   cfg.APIKey,
		Username: cfg.Username,
		Password: cfg.Password,
	}
}

func resolvedConfig(opts *rootOptions) (resolved, error) {
	fileCfg, err := config.Load(opts.configPath)
	if err != nil {
		return resolved{}, err
	}
	return resolved{
		Host:       firstNonEmpty(opts.host, os.Getenv("REDMINE_HOST"), fileCfg.Host),
		APIKey:     firstNonEmpty(opts.apiKey, os.Getenv("REDMINE_API_KEY"), fileCfg.APIKey),
		Username:   firstNonEmpty(opts.username, os.Getenv("REDMINE_USERNAME"), fileCfg.Username),
		Password:   firstNonEmpty(opts.password, os.Getenv("REDMINE_PASSWORD"), fileCfg.Password),
		SwitchUser: firstNonEmpty(opts.switchUser, os.Getenv("REDMINE_SWITCH_USER")),
	}, nil
}

func collectParameters(generated map[string]*parameterValues, extra []string) (map[string][]string, error) {
	result := map[string][]string{}
	for _, item := range generated {
		if len(item.values) == 0 {
			continue
		}
		result[item.param.Name] = append(result[item.param.Name], item.values...)
	}
	for _, value := range extra {
		key, val, err := splitPair(value)
		if err != nil {
			return nil, err
		}
		result[key] = append(result[key], val)
	}
	return result, nil
}

func collectHeaders(generated map[string]*parameterValues, extra []string) (map[string]string, error) {
	result := map[string]string{}
	for _, item := range generated {
		if len(item.values) == 0 {
			continue
		}
		result[item.param.Name] = item.values[len(item.values)-1]
	}
	for _, value := range extra {
		key, val, err := splitPair(value)
		if err != nil {
			return nil, err
		}
		result[key] = val
	}
	return result, nil
}

func buildBody(cmd *cobra.Command, op openapi.Operation, flags *operationFlags) ([]byte, error) {
	if op.Body == nil {
		if flags.rawBody != "" || len(flags.extraFields) > 0 || flags.input != "" {
			return nil, fmt.Errorf("%s does not accept a request body", op.ID)
		}
		return nil, nil
	}
	if op.Body.Binary {
		if flags.input == "" {
			return nil, fmt.Errorf("%s requires --input", op.ID)
		}
		return readValue(cmd, flags.input)
	}
	if flags.rawBody != "" {
		if len(flags.extraFields) > 0 || anyBodyFlags(flags.body) {
			return nil, fmt.Errorf("--body cannot be combined with generated body flags or --field")
		}
		return readValue(cmd, flags.rawBody)
	}

	root := map[string]any{}
	target := root
	if op.Body.Root != "" {
		child := map[string]any{}
		root[op.Body.Root] = child
		target = child
	}

	for _, values := range flags.body {
		if len(values.values) == 0 {
			continue
		}
		target[values.field.Name] = parseValues(values.values, values.field.Type, values.field.Array)
	}
	for _, rawField := range flags.extraFields {
		key, value, err := splitPair(rawField)
		if err != nil {
			return nil, err
		}
		setNested(target, key, parseLooseValue(value))
	}
	if emptyBody(root, op.Body.Root) {
		return nil, nil
	}
	return json.Marshal(root)
}

func anyBodyFlags(values map[string]*bodyValues) bool {
	for _, value := range values {
		if len(value.values) > 0 {
			return true
		}
	}
	return false
}

func emptyBody(root map[string]any, rootName string) bool {
	if rootName == "" {
		return len(root) == 0
	}
	child, ok := root[rootName].(map[string]any)
	return !ok || len(child) == 0
}

func readValue(cmd *cobra.Command, value string) ([]byte, error) {
	switch {
	case value == "-":
		return io.ReadAll(cmd.InOrStdin())
	case strings.HasPrefix(value, "@"):
		return os.ReadFile(strings.TrimPrefix(value, "@"))
	default:
		return []byte(value), nil
	}
}

func parseValues(values []string, typ string, array bool) any {
	if array {
		result := make([]any, 0, len(values))
		for _, value := range values {
			result = append(result, parseTypedValue(value, typ))
		}
		return result
	}
	return parseTypedValue(values[len(values)-1], typ)
}

func parseTypedValue(value, typ string) any {
	if value == "null" {
		return nil
	}
	switch strings.Split(typ, ":")[0] {
	case "boolean":
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	case "integer":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return parsed
		}
	case "number":
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	case "object":
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed
		}
	}
	return parseLooseValue(value)
}

func parseLooseValue(value string) any {
	if value == "null" {
		return nil
	}
	if value == "true" || value == "false" {
		parsed, _ := strconv.ParseBool(value)
		return parsed
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") || strings.HasPrefix(value, `"`) {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err == nil {
			return parsed
		}
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	return value
}

func setNested(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func splitPair(value string) (string, string, error) {
	key, val, ok := strings.Cut(value, "=")
	if !ok || key == "" {
		return "", "", fmt.Errorf("expected key=value, got %q", value)
	}
	return key, val, nil
}

func usageArgs(params []openapi.Parameter) string {
	if len(params) == 0 {
		return ""
	}
	var args []string
	for _, param := range params {
		args = append(args, "<"+strings.ToLower(param.Placeholder)+">")
	}
	return " " + strings.Join(args, " ")
}

func operationLong(op openapi.Operation) string {
	var b strings.Builder
	if op.Summary != "" {
		b.WriteString(op.Summary)
	}
	if op.Description != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(op.Description)
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Operation: %s %s (%s)", op.Method, op.Path, op.ID)
	return b.String()
}

func flagDescription(description, typ string, required bool, enum []string) string {
	var parts []string
	if description != "" {
		description = strings.ReplaceAll(description, "`", "'")
		parts = append(parts, strings.Join(strings.Fields(description), " "))
	}
	if typ != "" {
		parts = append(parts, "type: "+typ)
	}
	if len(enum) > 0 {
		parts = append(parts, "values: "+strings.Join(enum, ", "))
	}
	if required {
		parts = append(parts, "required")
	}
	return strings.Join(parts, "; ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
