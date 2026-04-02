package main

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

func expandCopySources(contextDir, pattern string) ([]string, error) {
	contextAbs, err := filepath.Abs(contextDir)
	if err != nil {
		return nil, err
	}
	contextReal, err := filepath.EvalSymlinks(contextAbs)
	if err != nil {
		contextReal = contextAbs
	}

	pattern = filepath.ToSlash(pattern)
	if strings.HasPrefix(pattern, "/") {
		pattern = strings.TrimPrefix(pattern, "/")
	}
	if pattern == "" {
		return nil, fmt.Errorf("empty copy source pattern")
	}

	var out []string
	if strings.Contains(pattern, "**") {
		err := filepath.WalkDir(contextDir, func(full string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := assertPathInContext(contextReal, full); err != nil {
				return err
			}
			rel, err := filepath.Rel(contextDir, full)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			ok, err := doublestarMatch(pattern, rel)
			if err != nil {
				return err
			}
			if ok {
				out = append(out, full)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		matches, err := filepath.Glob(filepath.Join(contextDir, filepath.FromSlash(pattern)))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if err := assertPathInContext(contextReal, m); err != nil {
				return nil, err
			}
		}
		out = append(out, matches...)
	}

	sort.Strings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("COPY source pattern %q matched no files", pattern)
	}
	return out, nil
}

func assertPathInContext(contextReal, candidate string) error {
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return err
	}
	candReal, err := filepath.EvalSymlinks(candAbs)
	if err != nil {
		candReal = candAbs
	}
	rel, err := filepath.Rel(contextReal, candReal)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("COPY source %q escapes build context", candidate)
	}
	return nil
}

func doublestarMatch(pattern, candidate string) (bool, error) {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	cc := strings.Split(strings.Trim(candidate, "/"), "/")
	return matchParts(pp, cc)
}

func matchParts(patternParts, candParts []string) (bool, error) {
	if len(patternParts) == 0 {
		return len(candParts) == 0, nil
	}
	if patternParts[0] == "**" {
		for i := 0; i <= len(candParts); i++ {
			ok, err := matchParts(patternParts[1:], candParts[i:])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if len(candParts) == 0 {
		return false, nil
	}
	ok, err := path.Match(patternParts[0], candParts[0])
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return matchParts(patternParts[1:], candParts[1:])
}
