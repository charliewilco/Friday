package corpus

import "testing"

func TestParseYAMLFrontmatter(t *testing.T) {
	input := []byte("---\ntitle: Friday\ntags:\n  - go\n  - sqlite\n---\n# Body\n")
	fm, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("ParseFrontmatter() error = %v", err)
	}

	if got := FrontmatterTitle(fm.Values); got != "Friday" {
		t.Fatalf("expected title Friday, got %q", got)
	}

	context := BuildFrontmatterContext(fm.Values)
	if context == "" {
		t.Fatal("expected non-empty frontmatter context")
	}
}

func TestParseTOMLFrontmatter(t *testing.T) {
	input := []byte("+++\ntitle = \"Friday\"\ntags = [\"go\", \"cli\"]\n+++\nhello\n")
	fm, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("ParseFrontmatter() error = %v", err)
	}

	if got := fm.Format; got != "toml" {
		t.Fatalf("expected toml format, got %q", got)
	}
}
