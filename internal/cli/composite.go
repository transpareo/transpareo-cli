package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/flows"
	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// addCompositeCommands attaches the hand-written flows to their
// groups: imports map and run, exports create with a download,
// events tail. They replace the generated commands of the same
// name, which handWritten lists.
func (a *App) addCompositeCommands(root *cobra.Command) {
	group := func(name string) *cobra.Command {
		for _, cmd := range root.Commands() {
			if cmd.Name() == name {
				return cmd
			}
		}
		cmd := &cobra.Command{Use: name, Short: name}
		root.AddCommand(cmd)
		return cmd
	}
	group("imports").AddCommand(a.importsMapCommand(), a.importsRunCommand())
	group("exports").AddCommand(a.exportsCreateCommand())
	group("events").AddCommand(a.eventsTailCommand())
}

// guess prints one column a similarity match mapped, so whoever
// reads the run sees what nobody chose.
func (a *App) guess() func(flows.Guess) {
	printer := a.Printer()
	return func(guess flows.Guess) {
		printer.Message("guessed %s as %s (%.0f%% alike)", guess.Header,
			guess.Target, guess.Similarity*100)
	}
}

// progress prints task polls to stderr.
func (a *App) progress() func(*transpareo.Task) {
	printer := a.Printer()
	return func(task *transpareo.Task) {
		printer.Message("%s %d%%", task.Status, task.Progress)
	}
}

func (a *App) importsMapCommand() *cobra.Command {
	var mappingsFile string
	var specs []string
	var accept, skipFuzzy, published, skipBackup bool
	cmd := &cobra.Command{
		Use:   "map <id>",
		Short: "Send the column mapping of a fresh import",
		Long: `Reads the import's preview and sends one action per column: from a
mapping file, from --map options, or from the preview's own
suggestions with --accept-suggestions, which takes every column
the platform recognised, a similarity match included, and prints
the guesses it took. A column it matched to nothing stops the run
with exit code 5 and the unresolved columns. --skip-fuzzy leaves
the similarity matches to you as well. It never creates a
property type on its own.

A --map target is a core attribute (name, gtin), property:<type
name> for an existing property type, new:<type name> to create
one, or skip.`,
		Example: `  transpareo imports map 12 --mappings mapping.json
  transpareo imports map 12 --map "Artikelname=name" \
      --map "Gewicht=property:Weight" --map "Farbe=new:Colour" \
      --map "Intern=skip"
  transpareo imports map 12 --accept-suggestions`,
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{annotationOperation: "map_import"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.importsMap(cmd.Context(), args[0], mappingsFile, specs,
				accept, skipFuzzy,
				runOptions(cmd, published, skipBackup, false))
		},
	}
	f := cmd.Flags()
	f.StringVar(&mappingsFile, "mappings", "",
		"mapping file (ImportMappingsInput)")
	f.StringArrayVar(&specs, "map", nil, "Column=target (repeatable)")
	f.BoolVar(&accept, "accept-suggestions", false,
		"take the preview's suggestion for every column it matched")
	f.BoolVar(&skipFuzzy, "skip-fuzzy", false,
		"with --accept-suggestions, leave a similarity match to a person")
	f.BoolVar(&published, "published", false,
		"publish the records the import creates")
	f.BoolVar(&skipBackup, "skip-backup", false,
		"execute without the backup a revert needs")
	return cmd
}

// runOptions builds the options only when a flag was given, so
// the platform's defaults stay in force otherwise. The flag says
// what to skip and the API field says what to take, so one is the
// negation of the other.
func runOptions(cmd *cobra.Command, published, skipBackup,
	auto bool) *flows.MappingOptions {
	var opts flows.MappingOptions
	set := false
	if cmd.Flags().Changed("published") {
		opts.Published = &published
		set = true
	}
	if cmd.Flags().Changed("skip-backup") {
		backup := !skipBackup
		opts.Backup = &backup
		set = true
	}
	if cmd.Flags().Changed("auto") {
		opts.Auto = &auto
		set = true
	}
	if !set {
		return nil
	}
	return &opts
}

func (a *App) importsMap(ctx context.Context, id, mappingsFile string,
	specs []string,
	accept, skipFuzzy bool, options *flows.MappingOptions) error {
	reg, err := a.Registry()
	if err != nil {
		return err
	}
	if err := a.refuseUnlessAllowed(reg.Find("map_import"), "PUT"); err != nil {
		return err
	}
	explicit, err := a.explicitMappings(mappingsFile, specs)
	if err != nil {
		return err
	}
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	imp, err := flows.Get(ctx, client, id)
	if err != nil {
		return err
	}
	mappings, guesses, err := flows.ResolveMappings(imp, explicit, accept,
		skipFuzzy)
	report := a.guess()
	for _, guess := range guesses {
		report(guess)
	}
	if err != nil {
		return a.mappingError(err)
	}
	updated, err := flows.SendMappings(ctx, client, id, mappings, options)
	if err != nil {
		return err
	}
	return a.Printer().Print(updated.Body)
}

// explicitMappings reads --mappings and --map into one map.
func (a *App) explicitMappings(mappingsFile string,
	specs []string) (map[string]flows.Mapping, error) {
	explicit := map[string]flows.Mapping{}
	if mappingsFile != "" {
		data, err := a.readBody("@" + mappingsFile)
		if err != nil {
			return nil, err
		}
		var input struct {
			Mappings map[string]flows.Mapping `json:"mappings"`
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return nil, output.Exit(output.ExitUsage,
				fmt.Errorf("%s: %w", mappingsFile, err))
		}
		if input.Mappings == nil {
			if err := json.Unmarshal(data, &explicit); err != nil {
				return nil, output.Exit(output.ExitUsage,
					fmt.Errorf("%s: %w", mappingsFile, err))
			}
		} else {
			explicit = input.Mappings
		}
	}
	for _, spec := range specs {
		column, m, err := flows.ParseMapSpec(spec)
		if err != nil {
			return nil, output.Exit(output.ExitUsage, err)
		}
		explicit[column] = m
	}
	return explicit, nil
}

// mappingError prints the unresolved columns and exits with 5;
// any other error passes through.
func (a *App) mappingError(err error) error {
	var required *flows.ErrMappingRequired
	if !errors.As(err, &required) {
		return err
	}
	printer := a.Printer()
	if err := printer.Print(map[string]any{
		"importId":   required.Import.ID,
		"unresolved": required.Unresolved,
		"coreAttributes": func() []string {
			if required.Import.Preview != nil {
				return required.Import.Preview.CoreAttributes
			}
			return nil
		}(),
	}); err != nil {
		return err
	}
	return output.Exit(output.ExitMapping, nil)
}

func (a *App) importsRunCommand() *cobra.Command {
	var file, dataType, mappingsFile, separator string
	var specs []string
	var accept, skipFuzzy, execute, published, skipBackup, auto bool
	cmd := &cobra.Command{
		Use:   "run --file <path> --type <components|products|dpps>",
		Short: "Upload, map, validate and, with --execute, run an import",
		Long: `Uploads the file, maps its columns when the platform could not
resolve them on its own, waits for the validation and prints its
findings. Exit code 5 means columns need a mapping (they are in
the output), 3 that the validation found failing rows. Records
are written only with --execute and only when the validation
passed. A template file or a JSON file with canonical keys needs
no mapping option.

--auto leaves the whole run to the platform: the upload takes the
mapping the mapping form would prefill, every column that matches
no property type becomes one under its own heading, and a clean
validation carries on into the write without --execute. What it
writes is published, which --published=false holds back. It
changes the schema of the workspace, so use it on a sheet whose
headings are already the ones you want, and send an uncertain one
through the preview instead.`,
		Example: `  transpareo imports run --file catalogue.xlsx --type products
  transpareo imports run --file catalogue.xlsx --type products \
      --accept-suggestions --map "Farbe=new:Colour" --execute
  transpareo imports run --file catalogue.xlsx --type products --auto`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{annotationOperation: "execute_import"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return output.Exit(output.ExitUsage,
					errors.New("--file is required"))
			}
			explicit, err := a.explicitMappings(mappingsFile, specs)
			if err != nil {
				return err
			}
			return a.importsRun(cmd.Context(), flows.RunOptions{
				Path: file, DataType: dataType, ValueSeparator: separator,
				Mappings: explicit, AcceptSuggestions: accept,
				ExactOnly: skipFuzzy, Execute: execute,
				Options:  runOptions(cmd, published, skipBackup, auto),
				Progress: a.progress(), OnGuess: a.guess(),
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&file, "file", "", "the spreadsheet or JSON file to import")
	f.StringVar(&dataType, "type", "", "components, products or dpps")
	f.StringVar(&separator, "value-separator", "",
		"what separates several values in a cell")
	f.StringVar(&mappingsFile, "mappings", "",
		"mapping file (ImportMappingsInput)")
	f.StringArrayVar(&specs, "map", nil, "Column=target (repeatable)")
	f.BoolVar(&accept, "accept-suggestions", false,
		"take the preview's suggestion for every column it matched")
	f.BoolVar(&skipFuzzy, "skip-fuzzy", false,
		"with --accept-suggestions, leave a similarity match to a person")
	f.BoolVar(&execute, "execute", false,
		"write the records when the validation passes")
	f.BoolVar(&auto, "auto", false,
		"let the platform map, write and publish in one call, making "+
			"a property type for every column that matches none")
	f.BoolVar(&published, "published", false,
		"publish the records the import creates")
	f.BoolVar(&skipBackup, "skip-backup", false,
		"execute without the backup a revert needs")
	return cmd
}

func (a *App) importsRun(ctx context.Context, opts flows.RunOptions) error {
	reg, err := a.Registry()
	if err != nil {
		return err
	}
	// An automatic run writes the rows from the upload, so the
	// guard has to weigh it as the write it is.
	op := reg.Find("validate_import")
	if opts.Execute || opts.Automatic() {
		op = reg.Find("execute_import")
	}
	if err := a.refuseUnlessAllowed(op, "POST"); err != nil {
		return err
	}
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	imp, err := flows.Run(ctx, client, opts)
	var failed *flows.ErrValidationFailed
	switch {
	case err == nil:
		return a.Printer().Print(imp.Body)
	case flows.IsMappingRequired(err):
		return a.mappingError(err)
	case errors.As(err, &failed):
		if err := a.Printer().Print(failed.Import.Body); err != nil {
			return err
		}
		return output.Exit(output.ExitValidation, nil)
	default:
		return err
	}
}

func (a *App) exportsCreateCommand() *cobra.Command {
	var format, download string
	var normalize, includeMedia, wait bool
	cmd := &cobra.Command{
		Use: "create [--format jsonld|csv|xlsx|sql] [--wait] [--download " +
			"<path>]",
		Short: "Start a passport export, wait for it and download the archive",
		Long: `Starts an export of the passport catalogue. With --wait the command
polls until the archive is ready and prints the finished export;
with --download it also writes the archive to the path. One
export runs at a time per consumer.`,
		Example: `  transpareo exports create --format jsonld --wait \
      --download catalogue.tar.gz
  transpareo exports create --format csv --normalize`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{annotationOperation: "create_export"},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := flows.ExportOptions{Format: format, Wait: wait ||
				download != "",
				Progress: a.progress()}
			if cmd.Flags().Changed("normalize") {
				opts.Normalize = &normalize
			}
			if cmd.Flags().Changed("include-media") {
				opts.IncludeMedia = &includeMedia
			}
			return a.exportsCreate(cmd.Context(), opts, download)
		},
	}
	f := cmd.Flags()
	f.StringVar(&format, "format", "", "jsonld (default), csv, xlsx or sql")
	f.BoolVar(&normalize, "normalize", false,
		"resolve references into the rows")
	f.BoolVar(&includeMedia, "include-media", false,
		"copy media files into the archive")
	f.BoolVar(&wait, "wait", false, "poll until the export is done")
	f.StringVar(&download, "download", "",
		"write the finished archive to this path")
	return cmd
}

func (a *App) exportsCreate(ctx context.Context, opts flows.ExportOptions,
	download string) error {
	reg, err := a.Registry()
	if err != nil {
		return err
	}
	if err := a.refuseUnlessAllowed(reg.Find("create_export"),
		"POST"); err != nil {
		return err
	}
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	exp, err := flows.StartExport(ctx, client, opts)
	if err != nil {
		return err
	}
	if download != "" {
		file, err := os.Create(download)
		if err != nil {
			return err
		}
		defer file.Close()
		n, err := flows.Download(ctx, client, exp, file)
		if err != nil {
			return err
		}
		a.Printer().Message("Wrote %d bytes to %s.", n, download)
	}
	return a.Printer().Print(exp.Body)
}

func (a *App) eventsTailCommand() *cobra.Command {
	var opts flows.FeedOptions
	var follow bool
	var interval time.Duration
	cmd := &cobra.Command{
		Use: "tail [--since <cursor>] [--follow]",
		Short: "Read the workspace's passport events, optionally as a live " +
			"stream",
		Long: `Prints the events since a cursor and the next cursor to keep. With
--follow it keeps polling and prints every new event as one JSON
line; the cursor of the last event printed goes to standard error
when the stream stops, so the next run can continue from it.`,
		Example: `  transpareo events tail --since 2026-09-01T00:00:00Z
  transpareo events tail --follow --types published,voided`,
		Args:        cobra.NoArgs,
		Annotations: map[string]string{annotationOperation: "list_events"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.eventsTail(cmd.Context(), opts, follow, interval)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.Since, "since", "",
		"cursor of the previous answer, or an ISO 8601 time")
	f.StringVar(&opts.Types, "types", "",
		"event types to keep, comma separated")
	f.StringVar(&opts.DppCode, "dpp-code", "",
		"confine the feed to one passport")
	f.IntVar(&opts.Limit, "limit", 0,
		"events per answer (default 100, max 500)")
	f.BoolVar(&follow, "follow", false,
		"keep polling and print new events as they arrive")
	f.DurationVar(&interval, "interval", 5*time.Second,
		"wait between polls with --follow")
	return cmd
}

func (a *App) eventsTail(ctx context.Context, opts flows.FeedOptions,
	follow bool,
	interval time.Duration) error {
	client, _, err := a.Client()
	if err != nil {
		return err
	}
	printer := a.Printer()
	if !follow {
		feed, err := flows.ReadFeed(ctx, client, opts)
		if err != nil {
			return err
		}
		if printer.JSONMode() && !printer.JSONL && !printer.Quiet {
			return printer.Print(feed)
		}
		printer.Message("cursor: %s", feed.NextCursor)
		return printer.Print(feed.Events)
	}
	cursor := opts.Since
	err = flows.Follow(ctx, client, opts, interval, nil,
		func(feed *flows.Feed) error {
			if feed.NextCursor != "" {
				cursor = feed.NextCursor
			}
			for _, event := range feed.Events {
				if err := printer.PrintRaw(compact(event)); err != nil {
					return err
				}
			}
			return nil
		})
	printer.Message("cursor: %s", cursor)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func compact(raw json.RawMessage) []byte {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	data, _ := json.Marshal(v)
	return data
}

// compositeOperations are the operations the composite commands
// replace, so the generator skips them.
var compositeOperations = map[string]bool{
	"map_import":    true,
	"create_export": true,
}

func init() {
	for id := range compositeOperations {
		handWritten[id] = true
	}
}
