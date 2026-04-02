package main

import "testing"

func TestManifestDigestDeterministic(t *testing.T) {
	m := ImageManifest{
		Name:    "a",
		Tag:     "b",
		Digest:  "",
		Created: "2026-01-01T00:00:00Z",
		Config: ImageConfig{
			Env:        map[string]string{"A": "1"},
			Cmd:        []string{"/bin/sh"},
			WorkingDir: "/",
		},
		Layers: []LayerDescriptor{{Digest: "sha256:abc", Size: 1, CreatedBy: "COPY x y"}},
	}
	d1, err := manifestDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := manifestDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("digest should be stable: %s != %s", d1, d2)
	}
}
