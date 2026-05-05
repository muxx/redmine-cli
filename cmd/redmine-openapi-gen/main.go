package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/muxx/redmine-cli/internal/openapi"
	"gopkg.in/yaml.v3"
)

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

func main() {
	specPath := flag.String("spec", "openapi/openapi.yaml", "OpenAPI YAML file")
	outPath := flag.String("out", "internal/openapi/generated.go", "generated Go output")
	docsPath := flag.String("docs", "docs/usage.md", "generated usage docs output")
	flag.Parse()

	if err := run(*specPath, *outPath, *docsPath); err != nil {
		fmt.Fprintf(os.Stderr, "redmine-openapi-gen: %v\n", err)
		os.Exit(1)
	}
}

func run(specPath, outPath, docsPath string) error {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return err
	}

	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.OpenAPI == "" {
		return errors.New("input does not look like an OpenAPI document")
	}

	ops, err := buildOperations(&doc)
	if err != nil {
		return err
	}

	if outPath != "" {
		source, err := renderGo(&doc, ops)
		if err != nil {
			return err
		}
		if err := writeFile(outPath, source); err != nil {
			return err
		}
	}

	if docsPath != "" {
		if err := writeFile(docsPath, renderDocs(&doc, ops)); err != nil {
			return err
		}
	}

	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

type document struct {
	OpenAPI    string              `yaml:"openapi"`
	Info       info                `yaml:"info"`
	Tags       []tag               `yaml:"tags"`
	Paths      map[string]pathItem `yaml:"paths"`
	Components components          `yaml:"components"`
}

type info struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

type tag struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type components struct {
	Parameters map[string]parameter `yaml:"parameters"`
}

type pathItem struct {
	Get    *operation `yaml:"get"`
	Post   *operation `yaml:"post"`
	Put    *operation `yaml:"put"`
	Patch  *operation `yaml:"patch"`
	Delete *operation `yaml:"delete"`
}

func (p pathItem) operations() []methodOperation {
	return []methodOperation{
		{method: "GET", operation: p.Get},
		{method: "POST", operation: p.Post},
		{method: "PUT", operation: p.Put},
		{method: "PATCH", operation: p.Patch},
		{method: "DELETE", operation: p.Delete},
	}
}

type methodOperation struct {
	method    string
	operation *operation
}

type operation struct {
	Tags        []string            `yaml:"tags"`
	Summary     string              `yaml:"summary"`
	Description string              `yaml:"description"`
	OperationID string              `yaml:"operationId"`
	Parameters  []parameter         `yaml:"parameters"`
	RequestBody requestBody         `yaml:"requestBody"`
	Responses   map[string]response `yaml:"responses"`
}

type parameter struct {
	Ref         string  `yaml:"$ref"`
	Name        string  `yaml:"name"`
	In          string  `yaml:"in"`
	Required    bool    `yaml:"required"`
	Description string  `yaml:"description"`
	Schema      *schema `yaml:"schema"`
}

type requestBody struct {
	Content map[string]mediaType `yaml:"content"`
}

type response struct {
	Ref     string               `yaml:"$ref"`
	Content map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema *schema `yaml:"schema"`
}

type schema struct {
	Ref         string             `yaml:"$ref"`
	Type        string             `yaml:"type"`
	Format      string             `yaml:"format"`
	Description string             `yaml:"description"`
	Nullable    bool               `yaml:"nullable"`
	Required    []string           `yaml:"required"`
	Properties  map[string]*schema `yaml:"properties"`
	Items       *schema            `yaml:"items"`
	OneOf       []*schema          `yaml:"oneOf"`
	AllOf       []*schema          `yaml:"allOf"`
	Enum        []any              `yaml:"enum"`
}

func buildOperations(doc *document) ([]openapi.Operation, error) {
	tagOrder := make(map[string]int, len(doc.Tags))
	for i, tag := range doc.Tags {
		tagOrder[tag.Name] = i
	}

	var paths []string
	for path := range doc.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	type indexedOperation struct {
		index int
		op    openapi.Operation
	}
	var indexed []indexedOperation
	index := 0
	seenCommands := map[string]map[string]int{}

	for _, path := range paths {
		for _, methodOp := range doc.Paths[path].operations() {
			src := methodOp.operation
			if src == nil {
				continue
			}
			tagName := firstTag(src.Tags)
			group, aliases := groupName(tagName)
			command := commandName(src.Summary, group, aliases)
			if command == "" {
				command = kebab(src.OperationID)
			}
			if _, ok := seenCommands[group]; !ok {
				seenCommands[group] = map[string]int{}
			}
			seenCommands[group][command]++
			if seenCommands[group][command] > 1 {
				command = command + "-" + kebab(src.OperationID)
			}

			params := resolveParameters(doc, src.Parameters)
			op := openapi.Operation{
				ID:             src.OperationID,
				Group:          group,
				GroupAlias:     aliases,
				Command:        command,
				Method:         methodOp.method,
				Path:           path,
				Summary:        clean(src.Summary),
				Description:    clean(src.Description),
				PathParams:     pathParameters(path, params),
				QueryParams:    queryParameters(params),
				HeaderParams:   headerParameters(params),
				Body:           body(src.RequestBody),
				ResponseBinary: responseBinary(src.Responses),
			}
			indexed = append(indexed, indexedOperation{
				index: sortIndex(tagOrder, tagName, index),
				op:    op,
			})
			index++
		}
	}

	sort.SliceStable(indexed, func(i, j int) bool {
		if indexed[i].index != indexed[j].index {
			return indexed[i].index < indexed[j].index
		}
		if indexed[i].op.Group != indexed[j].op.Group {
			return indexed[i].op.Group < indexed[j].op.Group
		}
		if indexed[i].op.Command != indexed[j].op.Command {
			return indexed[i].op.Command < indexed[j].op.Command
		}
		return indexed[i].op.ID < indexed[j].op.ID
	})

	ops := make([]openapi.Operation, 0, len(indexed))
	for _, entry := range indexed {
		ops = append(ops, entry.op)
	}
	return ops, nil
}

func sortIndex(tagOrder map[string]int, tag string, fallback int) int {
	if idx, ok := tagOrder[tag]; ok {
		return idx*1000 + fallback
	}
	return 100000 + fallback
}

func firstTag(tags []string) string {
	if len(tags) == 0 {
		return "API"
	}
	return tags[0]
}

func resolveParameters(doc *document, params []parameter) []parameter {
	resolved := make([]parameter, 0, len(params))
	for _, param := range params {
		if param.Ref != "" {
			name := strings.TrimPrefix(param.Ref, "#/components/parameters/")
			if component, ok := doc.Components.Parameters[name]; ok {
				resolved = append(resolved, component)
			}
			continue
		}
		resolved = append(resolved, param)
	}
	return resolved
}

func pathParameters(path string, params []parameter) []openapi.Parameter {
	byName := map[string]parameter{}
	for _, param := range params {
		if param.In == "path" {
			byName[param.Name] = param
		}
	}

	matches := pathParamPattern.FindAllStringSubmatch(path, -1)
	result := make([]openapi.Parameter, 0, len(matches))
	for _, match := range matches {
		name := match[1]
		if name == "format" {
			continue
		}
		param := byName[name]
		if param.Name == "" {
			param = parameter{Name: name, In: "path", Required: true}
		}
		result = append(result, convertParameter(param))
	}
	return result
}

func queryParameters(params []parameter) []openapi.Parameter {
	var result []openapi.Parameter
	for _, param := range params {
		if param.In != "query" {
			continue
		}
		result = append(result, convertParameter(param))
	}
	return result
}

func headerParameters(params []parameter) []openapi.Parameter {
	var result []openapi.Parameter
	for _, param := range params {
		if param.In != "header" {
			continue
		}
		if strings.EqualFold(param.Name, "X-Redmine-Switch-User") {
			continue
		}
		result = append(result, convertParameter(param))
	}
	return result
}

func convertParameter(param parameter) openapi.Parameter {
	typ, _ := schemaType(param.Schema)
	return openapi.Parameter{
		Name:        param.Name,
		Flag:        kebab(param.Name),
		Placeholder: strings.ToUpper(strings.ReplaceAll(param.Name, "-", "_")),
		Required:    param.Required,
		Type:        typ,
		Description: clean(param.Description),
		Enum:        enumStrings(param.Schema),
	}
}

func body(req requestBody) *openapi.Body {
	if req.Content == nil {
		return nil
	}
	if mt, ok := req.Content["application/octet-stream"]; ok {
		_ = mt
		return &openapi.Body{ContentType: "application/octet-stream", Binary: true}
	}

	mt, ok := req.Content["application/json"]
	if !ok || mt.Schema == nil {
		return nil
	}

	root := ""
	bodySchema := mt.Schema
	required := requiredSet(bodySchema.Required)
	if bodySchema.Type == "object" && len(bodySchema.Properties) == 1 {
		for name, child := range bodySchema.Properties {
			if required[name] && child != nil && (child.Type == "object" || len(child.Properties) > 0) {
				root = name
				bodySchema = child
			}
		}
	}

	fields := make([]openapi.BodyField, 0, len(bodySchema.Properties))
	required = requiredSet(bodySchema.Required)
	var names []string
	for name := range bodySchema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fieldSchema := bodySchema.Properties[name]
		typ, array := schemaType(fieldSchema)
		fields = append(fields, openapi.BodyField{
			Name:        name,
			Flag:        kebab(name),
			Required:    required[name],
			Type:        typ,
			Array:       array,
			Nullable:    fieldSchema != nil && fieldSchema.Nullable,
			Description: clean(schemaDescription(fieldSchema)),
			Enum:        enumStrings(fieldSchema),
		})
	}

	return &openapi.Body{
		ContentType: "application/json",
		Root:        root,
		Fields:      fields,
	}
}

func schemaType(s *schema) (string, bool) {
	if s == nil {
		return "string", false
	}
	if s.Type == "array" {
		itemType, _ := schemaType(s.Items)
		return itemType, true
	}
	if s.Type != "" {
		if s.Format != "" {
			return s.Type + ":" + s.Format, false
		}
		return s.Type, false
	}
	if len(s.OneOf) > 0 {
		types := make([]string, 0, len(s.OneOf))
		for _, child := range s.OneOf {
			typ, _ := schemaType(child)
			if typ != "" {
				types = append(types, typ)
			}
		}
		return strings.Join(types, "|"), false
	}
	if s.Ref != "" {
		return "object", false
	}
	return "string", false
}

func schemaDescription(s *schema) string {
	if s == nil {
		return ""
	}
	return s.Description
}

func enumStrings(s *schema) []string {
	if s == nil || len(s.Enum) == 0 {
		return nil
	}
	values := make([]string, 0, len(s.Enum))
	for _, value := range s.Enum {
		values = append(values, fmt.Sprint(value))
	}
	return values
}

func responseBinary(responses map[string]response) bool {
	for code, resp := range responses {
		if !strings.HasPrefix(code, "2") {
			continue
		}
		for contentType, mt := range resp.Content {
			if contentType != "application/json" {
				return true
			}
			if mt.Schema != nil && mt.Schema.Format == "binary" {
				return true
			}
		}
	}
	return false
}

func requiredSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func groupName(tag string) (string, []string) {
	plural := kebab(tag)
	parts := strings.Split(plural, "-")
	if len(parts) == 0 {
		return "api", nil
	}
	lastPlural := parts[len(parts)-1]
	lastSingular := singular(lastPlural)
	parts[len(parts)-1] = lastSingular
	group := strings.Join(parts, "-")
	aliases := uniqueNonEmpty(plural, lastSingular, lastPlural)
	aliases = removeValue(aliases, group)
	return group, aliases
}

func uniqueNonEmpty(values ...string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func removeValue(values []string, remove string) []string {
	result := values[:0]
	for _, value := range values {
		if value == remove {
			continue
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func singular(value string) string {
	switch value {
	case "statuses":
		return "status"
	case "categories":
		return "category"
	case "queries":
		return "query"
	case "entries":
		return "entry"
	case "memberships":
		return "membership"
	case "repositories":
		return "repository"
	case "fields":
		return "field"
	case "news":
		return "news"
	}
	if strings.HasSuffix(value, "ies") {
		return strings.TrimSuffix(value, "ies") + "y"
	}
	if strings.HasSuffix(value, "ses") {
		return strings.TrimSuffix(value, "es")
	}
	if strings.HasSuffix(value, "s") && len(value) > 1 {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

func commandName(summary, group string, aliases []string) string {
	action := kebab(summary)
	if action == "" {
		return ""
	}
	allAliases := append([]string{group}, aliases...)
	for _, alias := range allAliases {
		if action == alias {
			return "run"
		}
	}

	parts := strings.SplitN(action, "-", 2)
	if len(parts) == 1 {
		return action
	}

	verb := parts[0]
	rest := parts[1]
	switch verb {
	case "list", "show", "create", "update", "delete", "add", "remove", "archive", "unarchive", "close", "reopen", "download", "upload":
	default:
		return action
	}

	for _, alias := range allAliases {
		if rest == alias {
			return verb
		}
		if strings.HasPrefix(rest, alias+"-") {
			return verb + "-" + strings.TrimPrefix(rest, alias+"-")
		}
		if strings.HasSuffix(rest, "-"+alias) {
			prefix := strings.TrimSuffix(rest, "-"+alias)
			if prefix != "" {
				return verb + "-" + prefix
			}
			return verb
		}
	}
	return action
}

func kebab(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func clean(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func renderGo(doc *document, ops []openapi.Operation) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("// Code generated by redmine-openapi-gen; DO NOT EDIT.\n\n")
	buf.WriteString("package openapi\n\n")
	fmt.Fprintf(&buf, "const SpecTitle = %q\n", doc.Info.Title)
	fmt.Fprintf(&buf, "const SpecVersion = %q\n\n", doc.Info.Version)
	buf.WriteString("var Operations = []Operation{\n")
	for _, op := range ops {
		writeOperation(&buf, op)
	}
	buf.WriteString("}\n")

	source, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, err
	}
	return source, nil
}

func writeOperation(buf *bytes.Buffer, op openapi.Operation) {
	buf.WriteString("{\n")
	fmt.Fprintf(buf, "ID:%q,\n", op.ID)
	fmt.Fprintf(buf, "Group:%q,\n", op.Group)
	fmt.Fprintf(buf, "GroupAlias:%s,\n", stringSlice(op.GroupAlias))
	fmt.Fprintf(buf, "Command:%q,\n", op.Command)
	fmt.Fprintf(buf, "Method:%q,\n", op.Method)
	fmt.Fprintf(buf, "Path:%q,\n", op.Path)
	fmt.Fprintf(buf, "Summary:%q,\n", op.Summary)
	fmt.Fprintf(buf, "Description:%q,\n", op.Description)
	fmt.Fprintf(buf, "PathParams:%s,\n", parameters(op.PathParams))
	fmt.Fprintf(buf, "QueryParams:%s,\n", parameters(op.QueryParams))
	fmt.Fprintf(buf, "HeaderParams:%s,\n", parameters(op.HeaderParams))
	fmt.Fprintf(buf, "Body:%s,\n", bodyLiteral(op.Body))
	fmt.Fprintf(buf, "ResponseBinary:%t,\n", op.ResponseBinary)
	buf.WriteString("},\n")
}

func stringSlice(values []string) string {
	if len(values) == 0 {
		return "nil"
	}
	var parts []string
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%q", value))
	}
	return "[]" + "string{" + strings.Join(parts, ",") + "}"
}

func parameters(params []openapi.Parameter) string {
	if len(params) == 0 {
		return "nil"
	}
	var buf strings.Builder
	buf.WriteString("[]Parameter{\n")
	for _, param := range params {
		fmt.Fprintf(&buf, "{Name:%q,Flag:%q,Placeholder:%q,Required:%t,Type:%q,Description:%q,Enum:%s},\n",
			param.Name,
			param.Flag,
			param.Placeholder,
			param.Required,
			param.Type,
			param.Description,
			stringSlice(param.Enum),
		)
	}
	buf.WriteString("}")
	return buf.String()
}

func bodyLiteral(body *openapi.Body) string {
	if body == nil {
		return "nil"
	}
	return fmt.Sprintf("&Body{ContentType:%q,Root:%q,Binary:%t,Fields:%s}", body.ContentType, body.Root, body.Binary, bodyFields(body.Fields))
}

func bodyFields(fields []openapi.BodyField) string {
	if len(fields) == 0 {
		return "nil"
	}
	var buf strings.Builder
	buf.WriteString("[]BodyField{\n")
	for _, field := range fields {
		fmt.Fprintf(&buf, "{Name:%q,Flag:%q,Required:%t,Type:%q,Array:%t,Nullable:%t,Description:%q,Enum:%s},\n",
			field.Name,
			field.Flag,
			field.Required,
			field.Type,
			field.Array,
			field.Nullable,
			field.Description,
			stringSlice(field.Enum),
		)
	}
	buf.WriteString("}")
	return buf.String()
}

func renderDocs(doc *document, ops []openapi.Operation) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# redmine CLI usage\n\n")
	fmt.Fprintf(&buf, "<!-- Code generated by redmine-openapi-gen from openapi/openapi.yaml; DO NOT EDIT. -->\n\n")
	fmt.Fprintf(&buf, "Generated from `%s` `%s`.\n\n", doc.Info.Title, doc.Info.Version)
	buf.WriteString("## Global usage\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("redmine [command] [subcommand] [flags]\n")
	buf.WriteString("```\n\n")
	buf.WriteString("Set `REDMINE_HOST` and `REDMINE_API_KEY`, or run `redmine auth login --host <url> --api-key <key>`. Login validates credentials with `GET /users/current.json` before saving them.\n\n")
	buf.WriteString("Common flags: `--host`, `--api-key`, `--username`, `--password`, `--switch-user`, `--output json|yaml|raw`, `--config`.\n\n")
	buf.WriteString("Request body commands accept generated body flags, repeated `--field key=value`, or `--body @file.json`.\n\n")
	renderAuthDocs(&buf)
	buf.WriteString("## Examples\n\n")
	buf.WriteString("```bash\n")
	buf.WriteString("redmine auth login --host https://redmine.example.com --api-key \"$REDMINE_API_KEY\"\n")
	buf.WriteString("redmine auth status\n")
	buf.WriteString("redmine issue list --limit 20\n")
	buf.WriteString("redmine issue show 123 --include journals\n")
	buf.WriteString("redmine issue create --project-id my-project --subject \"Fix checkout\"\n")
	buf.WriteString("redmine project list\n")
	buf.WriteString("```\n\n")

	grouped := map[string][]openapi.Operation{}
	var groups []string
	for _, op := range ops {
		if _, ok := grouped[op.Group]; !ok {
			groups = append(groups, op.Group)
		}
		grouped[op.Group] = append(grouped[op.Group], op)
	}

	buf.WriteString("## Commands\n\n")
	for _, group := range groups {
		fmt.Fprintf(&buf, "### `%s`\n\n", group)
		for _, op := range grouped[group] {
			fmt.Fprintf(&buf, "- `redmine %s %s%s` - %s\n", op.Group, op.Command, usageArgs(op.PathParams), op.Summary)
		}
		buf.WriteString("\n")
	}

	buf.WriteString("## Command reference\n\n")
	for _, group := range groups {
		for _, op := range grouped[group] {
			renderOperationDocs(&buf, op)
		}
	}
	return buf.Bytes()
}

func renderAuthDocs(buf *bytes.Buffer) {
	buf.WriteString("## Authentication\n\n")
	buf.WriteString("### `redmine auth login --host <url> --api-key <key>`\n\n")
	buf.WriteString("Checks the credentials with `GET /users/current.json` and saves them to the config file only after a successful response. The API key can also be read from stdin with `--stdin`.\n\n")
	buf.WriteString("### `redmine auth status`\n\n")
	buf.WriteString("Loads authentication from flags, environment, or config, calls `GET /users/current.json`, and prints the resolved host, auth method, authenticated user, and status.\n\n")
	buf.WriteString("### `redmine auth logout`\n\n")
	buf.WriteString("Removes the saved config file.\n\n")
}

func renderOperationDocs(buf *bytes.Buffer, op openapi.Operation) {
	fmt.Fprintf(buf, "### `redmine %s %s%s`\n\n", op.Group, op.Command, usageArgs(op.PathParams))
	fmt.Fprintf(buf, "%s\n\n", op.Summary)
	if op.Description != "" {
		fmt.Fprintf(buf, "%s\n\n", op.Description)
	}
	fmt.Fprintf(buf, "- Operation: `%s %s` (`%s`)\n", op.Method, op.Path, op.ID)
	if len(op.PathParams) > 0 {
		buf.WriteString("- Arguments:\n")
		for _, param := range op.PathParams {
			fmt.Fprintf(buf, "  - `%s`", strings.ToLower(param.Placeholder))
			if param.Description != "" {
				fmt.Fprintf(buf, ": %s", oneLine(param.Description))
			}
			buf.WriteString("\n")
		}
	}
	if len(op.QueryParams) > 0 || len(op.HeaderParams) > 0 {
		buf.WriteString("- Flags:\n")
		for _, param := range op.QueryParams {
			usage := fmt.Sprintf("--%s <%s>", param.Flag, flagType(param.Type))
			if reservedFlagName(param.Flag) {
				usage = fmt.Sprintf("--param %s=<%s>", param.Name, flagType(param.Type))
			}
			fmt.Fprintf(buf, "  - `%s`", usage)
			if param.Description != "" {
				fmt.Fprintf(buf, ": %s", oneLine(param.Description))
			}
			buf.WriteString("\n")
		}
		for _, param := range op.HeaderParams {
			usage := fmt.Sprintf("--%s <%s>", param.Flag, flagType(param.Type))
			if reservedFlagName(param.Flag) {
				usage = fmt.Sprintf("--header %s=<%s>", param.Name, flagType(param.Type))
			}
			fmt.Fprintf(buf, "  - `%s`", usage)
			if param.Description != "" {
				fmt.Fprintf(buf, ": %s", oneLine(param.Description))
			}
			buf.WriteString("\n")
		}
	}
	if op.Body != nil {
		if op.Body.Binary {
			buf.WriteString("- Body: `--input <file>` sends `application/octet-stream`.\n")
		} else {
			if op.Body.Root != "" {
				fmt.Fprintf(buf, "- Body root: `%s`\n", op.Body.Root)
			}
			if len(op.Body.Fields) > 0 {
				buf.WriteString("- Body flags:\n")
				for _, field := range op.Body.Fields {
					required := ""
					if field.Required {
						required = " required"
					}
					usage := fmt.Sprintf("--%s <%s>", field.Flag, flagType(field.Type))
					if reservedFlagName(field.Flag) {
						usage = fmt.Sprintf("--field %s=<%s>", field.Name, flagType(field.Type))
					}
					fmt.Fprintf(buf, "  - `%s`%s", usage, required)
					if field.Description != "" {
						fmt.Fprintf(buf, ": %s", oneLine(field.Description))
					}
					buf.WriteString("\n")
				}
			}
		}
	}
	buf.WriteString("\n")
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

func flagType(typ string) string {
	if typ == "" {
		return "value"
	}
	return typ
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func reservedFlagName(name string) bool {
	switch name {
	case "api-key", "body", "config", "field", "header", "help", "host", "input", "insecure", "output", "param", "password", "switch-user", "timeout", "username", "version":
		return true
	default:
		return false
	}
}
