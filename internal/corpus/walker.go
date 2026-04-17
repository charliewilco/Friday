package corpus

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charliewilco/friday/internal/config"
)

func WalkMarkdownFiles(cfg config.Loaded) ([]string, error) {
	seen := map[string]struct{}{}
	var files []string

	for _, relPath := range cfg.Config.Content.Paths {
		root := cfg.ResolveContentPath(relPath)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			projectRelative, err := filepath.Rel(cfg.ProjectRoot, path)
			if err != nil {
				return err
			}
			projectRelative = filepath.ToSlash(projectRelative)

			if shouldIgnore(projectRelative, cfg.Config.Content.Ignore) {
				return nil
			}
			if !slices.Contains(cfg.Config.Content.Extensions, strings.ToLower(filepath.Ext(path))) {
				return nil
			}
			if _, ok := seen[projectRelative]; ok {
				return nil
			}
			seen[projectRelative] = struct{}{}
			files = append(files, projectRelative)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	slices.Sort(files)
	return files, nil
}

func shouldIgnore(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		ok, err := filepath.Match(filepath.ToSlash(pattern), relPath)
		if err == nil && ok {
			return true
		}
		if strings.HasSuffix(pattern, "/**") {
			prefix := strings.TrimSuffix(filepath.ToSlash(pattern), "/**")
			if strings.HasPrefix(relPath, prefix+"/") {
				return true
			}
		}
	}
	return false
}
