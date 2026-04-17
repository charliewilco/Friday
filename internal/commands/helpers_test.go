package commands

import "testing"

func TestKebabCase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trims and lowercases",
			input: "  Friday Notes  ",
			want:  "friday-notes",
		},
		{
			name:  "collapses punctuation into one dash",
			input: "Swift, Rust & Go",
			want:  "swift-rust-go",
		},
		{
			name:  "drops leading and trailing separators",
			input: "---Already-Kebab---",
			want:  "already-kebab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kebabCase(tt.input); got != tt.want {
				t.Fatalf("kebabCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
