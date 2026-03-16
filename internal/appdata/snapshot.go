package appdata

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const snapshotDir = ".snapshots"

// CreateSnapshot creates a zip archive of the storage directory (excluding .snapshots/).
// Returns the path to the created snapshot.
func CreateSnapshot(storageDir string) (string, error) {
	snapDir := filepath.Join(storageDir, snapshotDir)
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	name := time.Now().Format("2006-01-02T15-04-05") + ".zip"
	snapPath := filepath.Join(snapDir, name)

	f, err := os.Create(snapPath)
	if err != nil {
		return "", fmt.Errorf("create snapshot file: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := zip.NewWriter(f)
	defer func() { _ = w.Close() }()

	err = filepath.Walk(storageDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(storageDir, path)
		if err != nil {
			return err
		}

		// Skip the snapshots directory itself
		if strings.HasPrefix(rel, snapshotDir) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip the root directory entry
		if rel == "." {
			return nil
		}

		if info.IsDir() {
			_, err := w.Create(rel + "/")
			return err
		}

		fw, err := w.Create(rel)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()

		_, err = io.Copy(fw, src)
		return err
	})

	if err != nil {
		return "", fmt.Errorf("walk storage dir: %w", err)
	}

	return snapPath, nil
}

// PruneSnapshots keeps the most recent `keep` snapshots and removes older ones.
func PruneSnapshots(storageDir string, keep int) error {
	if keep <= 0 {
		return nil
	}

	snapDir := filepath.Join(storageDir, snapshotDir)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read snapshot dir: %w", err)
	}

	// Filter to .zip files only
	var zips []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".zip") {
			zips = append(zips, e)
		}
	}

	if len(zips) <= keep {
		return nil
	}

	// Sort by name (timestamp format ensures chronological order)
	sort.Slice(zips, func(i, j int) bool {
		return zips[i].Name() < zips[j].Name()
	})

	// Remove oldest
	for _, e := range zips[:len(zips)-keep] {
		path := filepath.Join(snapDir, e.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove snapshot %s: %w", e.Name(), err)
		}
	}

	return nil
}
