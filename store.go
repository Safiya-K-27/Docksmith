package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func loadManifest(root string, ref ImageRef) (ImageManifest, error) {
	p := imageManifestFile(root, ref)
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ImageManifest{}, fmt.Errorf("image %s:%s not found", ref.Name, ref.Tag)
		}
		return ImageManifest{}, err
	}
	var m ImageManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return ImageManifest{}, fmt.Errorf("invalid manifest for %s:%s: %w", ref.Name, ref.Tag, err)
	}
	return m, nil
}

func manifestDigest(m ImageManifest) (string, error) {
	clone := m
	clone.Digest = ""
	b, err := stableJSON(clone)
	if err != nil {
		return "", err
	}
	return sha256Bytes(b), nil
}

func saveManifest(root string, m ImageManifest) error {
	d, err := manifestDigest(m)
	if err != nil {
		return err
	}
	m.Digest = d
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(imageManifestFile(root, ImageRef{Name: m.Name, Tag: m.Tag}), b, 0o644); err != nil {
		return err
	}
	return nil
}

func listManifests(root string) ([]ImageManifest, error) {
	var out []ImageManifest
	dir := filepath.Join(root, "images")
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var m ImageManifest
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Tag < out[j].Tag
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func loadCacheIndex(root string) (CacheIndex, error) {
	p := filepath.Join(root, "cache", "index.json")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CacheIndex{Entries: map[string]string{}}, nil
		}
		return CacheIndex{}, err
	}
	var idx CacheIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return CacheIndex{}, err
	}
	if idx.Entries == nil {
		idx.Entries = map[string]string{}
	}
	return idx, nil
}

func saveCacheIndex(root string, idx CacheIndex) error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(root, "cache", "index.json")
	return os.WriteFile(p, b, 0o644)
}

func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
