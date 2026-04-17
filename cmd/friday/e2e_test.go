package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/commands"
	"github.com/charliewilco/friday/internal/testutil/ollamatest"
)

func TestCLIInitEmbedAsk(t *testing.T) {
	server := ollamatest.New(t, ollamatest.Options{
		ChatResponse: "You wrote about Swift migration.",
		EmbeddingDim: 2,
	})

	projectDir := setupCLIProject(t, server)

	withWorkingDirectory(t, projectDir, func() {
		initOutput := runCLI(t, nil, "init", "--yes")
		if !strings.Contains(initOutput, "Run 'friday embed'") {
			t.Fatalf("unexpected init output: %s", initOutput)
		}

		embedOutput := runCLI(t, nil, "embed")
		if !strings.Contains(embedOutput, "embed complete") {
			t.Fatalf("unexpected embed output: %s", embedOutput)
		}

		askOutput := runCLI(t, nil, "ask", "what did i write about swift")
		if !strings.Contains(askOutput, "You wrote about Swift migration.") {
			t.Fatalf("unexpected ask output: %s", askOutput)
		}
		if !strings.Contains(askOutput, "Sources:") {
			t.Fatalf("expected citations in ask output: %s", askOutput)
		}
		if !strings.Contains(askOutput, "content/post.md") {
			t.Fatalf("expected specific citation path in ask output: %s", askOutput)
		}

		runOutput := runCLI(t, strings.NewReader(":stats\n:sources swift\n:q\n"), "run")
		if !strings.Contains(runOutput, "Friday ready.") {
			t.Fatalf("expected run welcome output: %s", runOutput)
		}
		if !strings.Contains(runOutput, "content/post.md") {
			t.Fatalf("expected sources output from run command: %s", runOutput)
		}
	})
}

func TestCLIEmbedTracksUnchangedRemovedAndReset(t *testing.T) {
	server := ollamatest.New(t, ollamatest.Options{EmbeddingDim: 2})
	projectDir := setupCLIProject(t, server)

	withWorkingDirectory(t, projectDir, func() {
		runCLI(t, nil, "init", "--yes")

		first := runCLI(t, nil, "embed")
		if !strings.Contains(first, "indexed: content/post.md") || !strings.Contains(first, "indexed: content/other.md") {
			t.Fatalf("expected indexed output, got %s", first)
		}

		second := runCLI(t, nil, "embed")
		if !strings.Contains(second, "unchanged: content/post.md") || !strings.Contains(second, "unchanged: content/other.md") {
			t.Fatalf("expected unchanged output, got %s", second)
		}

		if err := os.Remove(filepath.Join(projectDir, "content", "other.md")); err != nil {
			t.Fatal(err)
		}
		third := runCLI(t, nil, "embed")
		if !strings.Contains(third, "removed: content/other.md") {
			t.Fatalf("expected removed output, got %s", third)
		}

		resetOutput := runCLI(t, nil, "reset", "--yes")
		if !strings.Contains(resetOutput, "removed ") {
			t.Fatalf("expected reset output, got %s", resetOutput)
		}
		stateDir := filepath.Join(os.Getenv("HOME"), ".friday", "project")
		if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
			t.Fatalf("expected state dir %s to be removed, err=%v", stateDir, err)
		}
	})
}

func TestCLIInteractiveInitAcceptsDetectedContentPaths(t *testing.T) {
	server := ollamatest.New(t, ollamatest.Options{})
	projectDir := setupCLIProject(t, server)

	withWorkingDirectory(t, projectDir, func() {
		input := strings.NewReader("\n\n\n\n")
		output := runCLI(t, input, "init")
		if !strings.Contains(output, "Detected content paths:") {
			t.Fatalf("expected interactive content detection output, got %s", output)
		}

		configBytes, err := os.ReadFile(filepath.Join(projectDir, ".friday.toml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(configBytes), `paths = ["content"]`) {
			t.Fatalf("expected detected content path in config, got %s", string(configBytes))
		}
	})
}

func setupCLIProject(t *testing.T, server *ollamatest.Server) string {
	t.Helper()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("FRIDAY_OLLAMA_HOST", server.URLString())

	contentDir := filepath.Join(root, "project", "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "post.md"), []byte("# Swift\nI wrote about Swift migration.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "other.md"), []byte("# Other\nUnrelated note.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	return filepath.Join(root, "project")
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(previousWD)
	}()

	fn()
}

func runCLI(t *testing.T, input *strings.Reader, args ...string) string {
	t.Helper()

	cmd := commands.NewRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	if input != nil {
		cmd.SetIn(input)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, stdout.String())
	}
	return stdout.String()
}
