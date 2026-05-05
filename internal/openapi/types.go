package openapi

// Operation describes one Redmine API operation as command metadata.
type Operation struct {
	ID          string
	Group       string
	GroupAlias  []string
	Command     string
	Method      string
	Path        string
	Summary     string
	Description string

	PathParams   []Parameter
	QueryParams  []Parameter
	HeaderParams []Parameter
	Body         *Body

	ResponseBinary bool
}

// Parameter describes a generated command flag or positional path argument.
type Parameter struct {
	Name        string
	Flag        string
	Placeholder string
	Required    bool
	Type        string
	Description string
	Enum        []string
}

// Body describes an operation request body.
type Body struct {
	ContentType string
	Root        string
	Binary      bool
	Fields      []BodyField
}

// BodyField describes a generated body flag.
type BodyField struct {
	Name        string
	Flag        string
	Required    bool
	Type        string
	Array       bool
	Nullable    bool
	Description string
	Enum        []string
}
