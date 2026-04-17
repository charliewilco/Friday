package corpus

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type Frontmatter struct {
	Format string
	Values map[string]any
	Body   []byte
}

func ParseFrontmatter(input []byte) (Frontmatter, error) {
	if bytes.HasPrefix(input, []byte("---\n")) {
		end := bytes.Index(input[4:], []byte("\n---\n"))
		if end < 0 {
			return Frontmatter{Body: input}, nil
		}
		raw := input[4 : 4+end]
		body := input[4+end+5:]
		values := map[string]any{}
		if err := yaml.Unmarshal(raw, &values); err != nil {
			return Frontmatter{}, fmt.Errorf("invalid YAML frontmatter: %w", err)
		}
		return Frontmatter{Format: "yaml", Values: values, Body: body}, nil
	}

	if bytes.HasPrefix(input, []byte("+++\n")) {
		end := bytes.Index(input[4:], []byte("\n+++\n"))
		if end < 0 {
			return Frontmatter{Body: input}, nil
		}
		raw := input[4 : 4+end]
		body := input[4+end+5:]
		values := map[string]any{}
		if err := toml.Unmarshal(raw, &values); err != nil {
			return Frontmatter{}, fmt.Errorf("invalid TOML frontmatter: %w", err)
		}
		return Frontmatter{Format: "toml", Values: values, Body: body}, nil
	}

	return Frontmatter{Body: input}, nil
}

func BuildFrontmatterContext(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		lines = append(lines, fmt.Sprintf("%s: %s", key, stringifyFrontmatter(value)))
	}

	return strings.Join(lines, "\n")
}

func FrontmatterTitle(values map[string]any) string {
	for _, key := range []string{"title", "name"} {
		if value, ok := values[key]; ok {
			if text := strings.TrimSpace(stringifyFrontmatter(value)); text != "" {
				return text
			}
		}
	}
	return ""
}

func stringifyFrontmatter(value any) string {
	switch v := value.(type) {
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, stringifyFrontmatter(item))
		}
		return strings.Join(parts, ", ")
	case []string:
		return strings.Join(v, ", ")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, stringifyFrontmatter(v[key])))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}
