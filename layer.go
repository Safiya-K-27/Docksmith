package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type snapEntry struct {
	Path     string
	Mode     fs.FileMode
	Type     byte
	Size     int64
	Hash     string
	Linkname string
}

func snapshotTree(root string) (map[string]snapEntry, error) {
	out := map[string]snapEntry{}
	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if full == root {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Lstat(full)
		if err != nil {
			return err
		}
		e := snapEntry{Path: rel, Mode: info.Mode(), Size: info.Size()}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			e.Type = tar.TypeSymlink
			ln, err := os.Readlink(full)
			if err != nil {
				return err
			}
			e.Linkname = ln
		case info.IsDir():
			e.Type = tar.TypeDir
		case info.Mode().IsRegular():
			e.Type = tar.TypeReg
			h, err := fileHash(full)
			if err != nil {
				return err
			}
			e.Hash = h
		default:
			return nil
		}
		out[rel] = e
		return nil
	})
	return out, err
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func changedPaths(before, after map[string]snapEntry) []string {
	var changed []string
	for p, a := range after {
		b, ok := before[p]
		if !ok || !equalEntry(b, a) {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	return changed
}

func equalEntry(a, b snapEntry) bool {
	return a.Type == b.Type && a.Mode.Perm() == b.Mode.Perm() && a.Size == b.Size && a.Hash == b.Hash && a.Linkname == b.Linkname
}

func buildDeltaTar(root string, changed []string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)
	zero := time.Unix(0, 0)

	for _, rel := range changed {
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, err
		}
		hdr.Name = rel
		hdr.ModTime = zero
		hdr.AccessTime = zero
		hdr.ChangeTime = zero
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Uname = ""
		hdr.Gname = ""

		if info.Mode()&os.ModeSymlink != 0 {
			ln, err := os.Readlink(full)
			if err != nil {
				return nil, err
			}
			hdr.Linkname = ln
		}
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(full)
			if err != nil {
				return nil, err
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return nil, err
			}
			f.Close()
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func storeLayer(root string, tarBytes []byte) (LayerDescriptor, error) {
	digest := sha256Bytes(tarBytes)
	path := digestFilePath(root, digest)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(path, tarBytes, 0o644); err != nil {
				return LayerDescriptor{}, err
			}
		} else {
			return LayerDescriptor{}, err
		}
	}
	return LayerDescriptor{Digest: digest, Size: int64(len(tarBytes))}, nil
}

func extractLayers(rootfs string, layerDigests []string, storeRoot string) error {
	for _, d := range layerDigests {
		layerPath := digestFilePath(storeRoot, d)
		if err := extractTarFile(layerPath, rootfs); err != nil {
			return fmt.Errorf("failed to extract layer %s: %w", d, err)
		}
	}
	return nil
}

func extractTarFile(tarPath, destRoot string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, filepath.FromSlash(hdr.Name))
		clean, err := ensureCleanAbs(destRoot, target)
		if err != nil {
			return err
		}
		target = clean
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, fs.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return nil
}
