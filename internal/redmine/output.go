package redmine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	OutputJSON = "json"
	OutputYAML = "yaml"
	OutputRaw  = "raw"
)

// WriteResponse renders an API response to the requested output format.
func WriteResponse(w io.Writer, resp Response, format string) error {
	if len(resp.Body) == 0 {
		return nil
	}
	if format == "" {
		format = OutputJSON
	}

	switch format {
	case OutputRaw:
		_, err := w.Write(resp.Body)
		return err
	case OutputYAML:
		return writeYAML(w, resp.Body)
	case OutputJSON:
		return writeJSON(w, resp.Body)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func writeJSON(w io.Writer, data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		_, writeErr := w.Write(data)
		return writeErr
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(encoded, '\n'))
	return err
}

func writeYAML(w io.Writer, data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		_, writeErr := w.Write(data)
		return writeErr
	}
	encoded, err := yaml.Marshal(normalizeYAML(value))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, bytes.NewReader(encoded))
	return err
}

func normalizeYAML(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, val := range typed {
			result[key] = normalizeYAML(val)
		}
		return result
	case []any:
		for i, val := range typed {
			typed[i] = normalizeYAML(val)
		}
		return typed
	case string:
		return strings.TrimRight(typed, "\n")
	default:
		return value
	}
}
