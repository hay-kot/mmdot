package appdata

import (
	"fmt"
	"os"
	"runtime"

	"github.com/sourcegraph/conc/pool"
)

// BackupResult holds the result of backing up a single app.
type BackupResult struct {
	App     ResolvedApp
	Copied  int
	Skipped int
	Err     error
}

// backupApp copies files from home to storage for a single app.
// If dryRun is true, it only counts what would be copied.
func backupApp(app ResolvedApp, dryRun bool) BackupResult {
	result := BackupResult{App: app}

	for _, entry := range app.Entries {
		_, err := os.Stat(entry.HomePath)
		if err != nil {
			result.Skipped++
			continue
		}

		if dryRun {
			result.Copied++
			continue
		}

		if err := CopyPath(entry.HomePath, entry.StoragePath); err != nil {
			result.Err = fmt.Errorf("backup %s: %w", app.ID, err)
			return result
		}
		result.Copied++
	}

	return result
}

// BackupAll backs up all resolved apps concurrently.
func BackupAll(apps []ResolvedApp, dryRun bool) []BackupResult {
	p := pool.NewWithResults[BackupResult]().WithMaxGoroutines(runtime.NumCPU())

	for _, app := range apps {
		p.Go(func() BackupResult {
			return backupApp(app, dryRun)
		})
	}

	return p.Wait()
}
