package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func mustStoreRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}
	root := filepath.Join(home, ".docksmith")
	for _, sub := range []string{"", "images", "layers", "cache"} {
		p := filepath.Join(root, sub)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return "", fmt.Errorf("failed to create state dir %s: %w", p, err)
		}
	}
	return root, nil
}

func parseImageRef(s string) (ImageRef, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ImageRef{}, fmt.Errorf("invalid image reference %q, expected name:tag", s)
	}
	return ImageRef{Name: parts[0], Tag: parts[1]}, nil
}

func imageManifestFile(root string, ref ImageRef) string {
	name := strings.ReplaceAll(ref.Name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	tag := strings.ReplaceAll(ref.Tag, "/", "_")
	return filepath.Join(root, "images", fmt.Sprintf("%s__%s.json", name, tag))
}

func digestFilePath(root, digest string) string {
	h := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(root, "layers", h+".tar")
}

func sha256Bytes(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func sortedEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func stableJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func ensureCleanAbs(base, candidate string) (string, error) {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, candAbs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return candAbs, nil
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes base directory")
	}
	return candAbs, nil
}

func toSlashClean(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
