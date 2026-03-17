package commands

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/expr-lang/expr/vm"
	"github.com/hay-kot/mmdot/internal/appdata"
	"github.com/hay-kot/mmdot/internal/core"
	"github.com/hay-kot/mmdot/pkgs/printer"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type AppDataCmd struct {
	flags  *core.Flags
	dryRun bool
}

func NewAppDataCmd(flags *core.Flags) *AppDataCmd {
	return &AppDataCmd{flags: flags}
}

func (ad *AppDataCmd) Register(app *cli.Command) *cli.Command {
	cmd := &cli.Command{
		Name:    "appdata",
		Aliases: []string{"ad"},
		Usage:   "Backup and restore application config files",
		Commands: []*cli.Command{
			{
				Name:      "backup",
				Usage:     "Copy application config files to storage",
				ArgsUsage: "[expression]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "dry-run",
						Usage:       "show what would be copied without copying",
						Destination: &ad.dryRun,
					},
				},
				Action: ad.backup,
			},
			{
				Name:      "restore",
				Usage:     "Copy application config files from storage to home",
				ArgsUsage: "[expression]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "dry-run",
						Usage:       "show what would be copied without copying",
						Destination: &ad.dryRun,
					},
				},
				Action: ad.restore,
			},
			{
				Name:      "list",
				Usage:     "List known or configured applications",
				ArgsUsage: "[expression]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "configured",
						Usage: "only show apps that are configured",
					},
				},
				Action: ad.list,
			},
		},
	}

	app.Commands = append(app.Commands, cmd)
	return app
}

// resolveApps builds the app DB and resolves all apps to absolute paths.
func (ad *AppDataCmd) resolveApps(cfg core.ConfigFile) ([]appdata.ResolvedApp, error) {
	db, err := appdata.BuildAppDB(cfg.AppData.Apps, cfg.AppData.Ignore, cfg.AppData.Custom)
	if err != nil {
		return nil, fmt.Errorf("build app db: %w", err)
	}

	resolved := make([]appdata.ResolvedApp, 0, len(db))
	for id, app := range db {
		resolved = append(resolved, appdata.ResolveApp(id, app, cfg.AppData.Storage))
	}

	return resolved, nil
}

// filterApps compiles the expression and filters resolved apps.
// Returns all apps if expression is empty.
func filterApps(apps []appdata.ResolvedApp, expression string, macros map[string]string) ([]appdata.ResolvedApp, error) {
	program, err := compileExpr(expression, macros, true)
	if err != nil {
		return nil, fmt.Errorf("compile expression: %w", err)
	}

	return matchApps(apps, program)
}

func matchApps(apps []appdata.ResolvedApp, program *vm.Program) ([]appdata.ResolvedApp, error) {
	var matched []appdata.ResolvedApp
	for _, app := range apps {
		env := map[string]any{
			"tags": app.Tags,
			"name": app.Name,
			"id":   app.ID,
		}
		ok, err := evalCompiledExpr(program, env)
		if err != nil {
			return nil, fmt.Errorf("eval expression for %s: %w", app.ID, err)
		}
		if ok {
			matched = append(matched, app)
		}
	}
	return matched, nil
}

func (ad *AppDataCmd) backup(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ad.flags.ConfigFilePath)
	if err != nil {
		return err
	}

	if cfg.AppData.Storage == "" {
		return fmt.Errorf("appdata.storage is required in config")
	}

	apps, err := ad.resolveApps(cfg)
	if err != nil {
		return err
	}

	expression := strings.Join(cmd.Args().Slice(), " ")
	apps, err = filterApps(apps, expression, cfg.Macros)
	if err != nil {
		return err
	}

	if ad.dryRun {
		log.Info().Msg("dry-run: showing what would be backed up")
	}

	results := appdata.BackupAll(apps, ad.dryRun)

	p := printer.New(os.Stdout)
	p.LineBreak()

	var errs []string
	var items []printer.StatusListItem

	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
			items = append(items, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s: %v", r.App.ID, r.Err),
			})
			continue
		}

		items = append(items, printer.StatusListItem{
			Ok:     true,
			Status: fmt.Sprintf("%s: %d copied, %d skipped", r.App.ID, r.Copied, r.Skipped),
		})
	}

	title := "Backup Results:"
	if ad.dryRun {
		title = "Backup (dry-run):"
	}
	p.StatusList(title, items)

	if len(errs) > 0 {
		return fmt.Errorf("%d app(s) failed", len(errs))
	}

	return nil
}

func (ad *AppDataCmd) restore(ctx context.Context, cmd *cli.Command) error {
	cfg, err := core.SetupEnv(ad.flags.ConfigFilePath)
	if err != nil {
		return err
	}

	if cfg.AppData.Storage == "" {
		return fmt.Errorf("appdata.storage is required in config")
	}

	apps, err := ad.resolveApps(cfg)
	if err != nil {
		return err
	}

	expression := strings.Join(cmd.Args().Slice(), " ")
	apps, err = filterApps(apps, expression, cfg.Macros)
	if err != nil {
		return err
	}

	if !ad.dryRun {
		snapPath, snapErr := appdata.SnapshotHomeFiles(cfg.AppData.Storage, apps)
		if snapErr != nil {
			log.Warn().Err(snapErr).Msg("failed to create pre-restore snapshot")
		} else {
			log.Info().Str("path", snapPath).Msg("pre-restore snapshot created")
		}

		retention := cfg.AppData.SnapshotRetention
		if retention == 0 {
			retention = 3
		}
		if pruneErr := appdata.PruneSnapshots(cfg.AppData.Storage, retention); pruneErr != nil {
			log.Warn().Err(pruneErr).Msg("failed to prune snapshots")
		}
	} else {
		log.Info().Msg("dry-run: showing what would be restored")
	}

	results := appdata.RestoreAll(apps, ad.dryRun)

	p := printer.New(os.Stdout)
	p.LineBreak()

	var errs []string
	var items []printer.StatusListItem

	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err.Error())
			items = append(items, printer.StatusListItem{
				Ok:     false,
				Status: fmt.Sprintf("%s: %v", r.App.ID, r.Err),
			})
			continue
		}

		items = append(items, printer.StatusListItem{
			Ok:     true,
			Status: fmt.Sprintf("%s: %d copied, %d skipped", r.App.ID, r.Copied, r.Skipped),
		})
	}

	title := "Restore Results:"
	if ad.dryRun {
		title = "Restore (dry-run):"
	}
	p.StatusList(title, items)

	if len(errs) > 0 {
		return fmt.Errorf("%d app(s) failed", len(errs))
	}

	return nil
}

func (ad *AppDataCmd) list(ctx context.Context, cmd *cli.Command) error {
	configured := cmd.Bool("configured")
	expression := strings.Join(cmd.Args().Slice(), " ")

	if configured {
		cfg, err := core.SetupEnv(ad.flags.ConfigFilePath)
		if err != nil {
			return err
		}

		db, err := appdata.BuildAppDB(cfg.AppData.Apps, cfg.AppData.Ignore, cfg.AppData.Custom)
		if err != nil {
			return err
		}

		ids := slices.Sorted(maps.Keys(db))

		// Build list items, then filter by expression if provided
		var listItems []ListItem
		for _, id := range ids {
			listItems = append(listItems, ListItem{
				Name: fmt.Sprintf("%s (%s)", id, db[id].Name),
				Tags: db[id].Tags,
			})
		}

		if expression != "" {
			program, compErr := compileExpr(expression, cfg.Macros, true)
			if compErr != nil {
				return fmt.Errorf("compile expression: %w", compErr)
			}

			var filtered []ListItem
			for i, id := range ids {
				env := map[string]any{
					"tags": db[id].Tags,
					"name": db[id].Name,
					"id":   id,
				}
				ok, evalErr := evalCompiledExpr(program, env)
				if evalErr != nil {
					return fmt.Errorf("eval expression for %s: %w", id, evalErr)
				}
				if ok {
					filtered = append(filtered, listItems[i])
				}
			}
			listItems = filtered
		}

		printList(fmt.Sprintf("Configured Apps (%d):", len(listItems)), listItems)
		return nil
	}

	db, err := appdata.LoadAppDB()
	if err != nil {
		return err
	}

	ids := slices.Sorted(maps.Keys(db))

	p := printer.New(os.Stdout)
	p.LineBreak()

	items := make([]string, len(ids))
	for i, id := range ids {
		items[i] = fmt.Sprintf("%s (%s)", id, db[id].Name)
	}

	p.List(fmt.Sprintf("Known Apps (%d):", len(ids)), items)
	return nil
}
