package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheKeyChangesWithEnvAndWorkdir(t *testing.T) {
	ctx := t.TempDir()
	if err := os.WriteFile(filepath.Join(ctx, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := buildState{
		baseManifest: ImageManifest{Digest: "sha256:base"},
		config: ImageConfig{
			Env:        map[string]string{"A": "1"},
			WorkingDir: "/app",
		},
	}
	ins := Instruction{Op: "COPY", Raw: "COPY x.txt /app/x.txt", Args: "x.txt /app/x.txt"}

	k1, err := computeCacheKey(ctx, st, ins, "")
	if err != nil {
		t.Fatal(err)
	}
	st.config.Env["A"] = "2"
	k2, err := computeCacheKey(ctx, st, ins, "")
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k2 {
		t.Fatal("env change must change cache key")
	}

	st.config.Env["A"] = "1"
	st.config.WorkingDir = "/other"
	k3, err := computeCacheKey(ctx, st, ins, "")
	if err != nil {
		t.Fatal(err)
	}
	if k1 == k3 {
		t.Fatal("workdir change must change cache key")
	}
}
