// Command adrient-admin is the operator CLI for Adrient Workspace:
//
//	adrient-admin version
//	adrient-admin migrate status|up
//	adrient-admin doctor
//	adrient-admin backup  -o FILE [--config ENVFILE] [--no-objects]
//	adrient-admin restore FILE --replace
//	adrient-admin export-org ORG -o FILE
//
// It reads the same environment as the server (ADRIENT_POSTGRES_DSN,
// ADRIENT_RUSTFS_*), so run it with the server's env file loaded.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Adrient-tech/Adrient-Workspace/internal/buildinfo"
	"os"
	"strings"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/adminops"
	"github.com/Adrient-tech/Adrient-Workspace/internal/demoseed"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	version = "0.0.0-dev"
	commit  = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(context.Background(), os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "adrient-admin:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: adrient-admin <command> [flags]

  version                         print version and embedded migration count
  migrate status                  show applied/pending migrations (read-only)
  migrate up                      pre-flight, then apply pending migrations
  doctor [--json]                 check config, database, migrations, storage, sign-in
  backup -o FILE [--config ENV]   database + object storage (+ config) to one archive
  restore FILE --replace          replace the database and objects with a backup
  export-org ORG -o FILE          export one organisation's data and files (portability)
  seed-demo [--org SLUG] [--remove]  add realistic fictional demo data for screenshots (idempotent), or remove it

Environment: ADRIENT_POSTGRES_DSN, ADRIENT_RUSTFS_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET
`)
}

func openPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("ADRIENT_POSTGRES_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("ADRIENT_POSTGRES_DSN is not set")
	}
	return storage.NewPool(ctx, dsn)
}

func openStore(ctx context.Context) (adminops.ObjectStore, error) {
	ep := os.Getenv("ADRIENT_RUSTFS_ENDPOINT")
	if ep == "" {
		return nil, fmt.Errorf("ADRIENT_RUSTFS_ENDPOINT is not set")
	}
	b := os.Getenv("ADRIENT_RUSTFS_BUCKET")
	if b == "" {
		b = "adrient-default"
	}
	blobs, err := drive.NewBlobs(ctx, drive.BlobsConfig{Endpoint: ep, AccessKey: os.Getenv("ADRIENT_RUSTFS_ACCESS_KEY"),
		SecretKey: os.Getenv("ADRIENT_RUSTFS_SECRET_KEY"), Bucket: b, Region: "us-east-1"})
	if err != nil {
		return nil, err
	}
	return adminops.BlobStore{B: blobs}, nil
}

func run(ctx context.Context, cmd string, args []string) error {
	switch cmd {
	case "version":
		bi := buildinfo.Resolve(version, commit, "")
		fmt.Printf("Adrient Workspace %s\nadrient-admin %s (%s, built %s), %d embedded migrations, backup format %d\n", bi.Version, bi.Version, bi.Commit, bi.BuildDate, storage.EmbeddedMigrationCount(), adminops.BackupFormat)
		return nil
	case "migrate":
		return runMigrate(ctx, args)
	case "doctor":
		return runDoctor(ctx, args)
	case "backup":
		return runBackup(ctx, args)
	case "restore":
		return runRestore(ctx, args)
	case "export-org":
		return runExport(ctx, args)
	case "seed-demo":
		return runSeedDemo(ctx, args)
	case "help", "-h", "--help":
		usage()
		return nil
	}
	usage()
	return fmt.Errorf("unknown command %q", cmd)
}

func runMigrate(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: migrate status|up")
	}
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	switch args[0] {
	case "status":
		st, err := storage.GetMigrationStatus(ctx, pool)
		if err != nil {
			return err
		}
		for _, m := range st.Migrations {
			mark := "pending"
			if m.Applied {
				mark = "applied " + m.AppliedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%04d  %-34s %s\n", m.Version, m.Name, mark)
		}
		fmt.Printf("%d applied, %d pending\n", len(st.Migrations)-len(st.Pending()), len(st.Pending()))
		if len(st.Unknown) > 0 {
			fmt.Printf("WARNING: database has versions this binary does not know: %v\n", st.Unknown)
			os.Exit(3)
		}
		return nil
	case "up":
		st, err := storage.Preflight(ctx, pool)
		if err != nil {
			return fmt.Errorf("pre-flight failed, nothing applied: %w", err)
		}
		fmt.Printf("pre-flight ok: %d pending\n", len(st.Pending()))
		if len(st.Pending()) == 0 {
			return nil
		}
		fmt.Println("tip: take a backup first (adrient-admin backup) if this is production data")
		if err := storage.RunMigrations(ctx, pool); err != nil {
			return err
		}
		fmt.Println("migrations applied")
		return nil
	}
	return fmt.Errorf("unknown migrate subcommand %q", args[0])
}

func runDoctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	_ = fs.Parse(args)
	var pool *pgxpool.Pool
	if os.Getenv("ADRIENT_POSTGRES_DSN") != "" {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if p, err := storage.NewPool(cctx, os.Getenv("ADRIENT_POSTGRES_DSN")); err == nil {
			pool = p
			defer p.Close()
		}
	}
	store, _ := openStore(ctx)
	checks := adminops.Doctor(ctx, pool, store, adminops.DoctorOptions{Env: os.Getenv})
	failed := false
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(checks)
	}
	for _, c := range checks {
		if c.Status == "fail" {
			failed = true
		}
		if !*asJSON {
			fmt.Printf("[%-4s] %-28s %s\n", c.Status, c.Name, c.Detail)
		}
	}
	if failed {
		os.Exit(1)
	}
	return nil
}

func runBackup(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	out := fs.String("o", "", "output file (.tar.gz)")
	cfg := fs.String("config", "", "env/config file to include (contains secrets)")
	noObj := fs.Bool("no-objects", false, "database only, skip object storage")
	_ = fs.Parse(args)
	if *out == "" {
		return fmt.Errorf("-o FILE is required")
	}
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	var store adminops.ObjectStore
	if !*noObj {
		if store, err = openStore(ctx); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	m, err := adminops.Backup(ctx, pool, store, f, adminops.BackupOptions{Version: buildinfo.Resolve(version, commit, "").Version, ConfigFile: *cfg, SkipObjects: *noObj})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(*out)
		return err
	}
	var rows int64
	for _, n := range m.Tables {
		rows += n
	}
	fmt.Printf("backup written to %s: %d tables, %d rows, %d objects (%d bytes), schema %04d\n", *out, len(m.Tables), rows, m.ObjectCount, m.ObjectBytes, m.MaxMigration)
	if m.ConfigIncluded {
		fmt.Println("note: the archive contains your config file with secrets; store it encrypted")
	}
	return nil
}

// flagsFirst moves positional arguments after flags so "restore FILE --replace"
// and "restore --replace FILE" both work (Go's flag package stops at the first
// positional argument).
func flagsFirst(args []string) []string {
	var flags, pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			pos = append(pos, a)
		}
	}
	return append(flags, pos...)
}

func runRestore(ctx context.Context, args []string) error {
	args = flagsFirst(args)
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	replace := fs.Bool("replace", false, "confirm: replace ALL data in the target database")
	noObj := fs.Bool("no-objects", false, "skip object storage")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: restore FILE --replace")
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	a, err := adminops.OpenArchive(f)
	if err != nil {
		return err
	}
	defer a.Close()
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	var store adminops.ObjectStore
	if !*noObj {
		if store, err = openStore(ctx); err != nil {
			return err
		}
	}
	rep, err := adminops.Restore(ctx, pool, store, a, adminops.RestoreOptions{Replace: *replace})
	if rep != nil {
		var rows int64
		for _, n := range rep.Tables {
			rows += n
		}
		fmt.Printf("restored %d tables (%d rows) and %d objects\n", len(rep.Tables), rows, rep.Objects)
		for _, m := range rep.Mismatch {
			fmt.Println("MISMATCH", m)
		}
	}
	if err != nil {
		return err
	}
	fmt.Println("verified: every table's row count matches the backup")
	if a.Config != "" {
		fmt.Println("the archive also holds a config file; it was NOT applied. Extract it from the archive manually if needed.")
	}
	return nil
}

func runExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export-org", flag.ExitOnError)
	out := fs.String("o", "", "output file (.tar.gz)")
	// Allow the org before or after the flags.
	var org string
	rest := args
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		org, rest = args[0], args[1:]
	}
	_ = fs.Parse(rest)
	if org == "" && fs.NArg() == 1 {
		org = fs.Arg(0)
	}
	if org == "" || *out == "" {
		return fmt.Errorf("usage: export-org ORG(slug|id) -o FILE")
	}
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := openStore(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: object storage unavailable, exporting data without files:", err)
		store = nil
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	m, err := adminops.ExportOrg(ctx, pool, store, org, f, version)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(*out)
		return err
	}
	fmt.Printf("exported org %s to %s: %d tables, %d files (%d bytes)\n", m.OrgSlug, *out, len(m.Tables), m.Files, m.FileBytes)
	return nil
}

// runSeedDemo fills an organisation with fictional demo data, or removes it.
func runSeedDemo(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("seed-demo", flag.ContinueOnError)
	org := fs.String("org", "default", "organisation slug")
	remove := fs.Bool("remove", false, "remove the demo data this command added")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	from := fs.Int("from-version", 1, "with --remove: remove only seed steps at or after this version (2 = the wave-5 content)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pool, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	var rep demoseed.Report
	var opt demoseed.Options
	if ep := os.Getenv("ADRIENT_RUSTFS_ENDPOINT"); ep != "" {
		b := os.Getenv("ADRIENT_RUSTFS_BUCKET")
		if b == "" {
			b = "adrient-default"
		}
		if blobs, berr := drive.NewBlobs(ctx, drive.BlobsConfig{Endpoint: ep, AccessKey: os.Getenv("ADRIENT_RUSTFS_ACCESS_KEY"), SecretKey: os.Getenv("ADRIENT_RUSTFS_SECRET_KEY"), Bucket: b, Region: "us-east-1"}); berr == nil {
			opt.Blobs = blobs
		}
	}
	if *remove {
		rep, err = demoseed.RemoveWith(ctx, pool, *org, opt, *from)
	} else {
		rep, err = demoseed.SeedWith(ctx, pool, *org, opt)
	}
	if err != nil {
		return err
	}
	if *asJSON {
		b, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("%s: %s\n", rep.Org, strings.ReplaceAll(rep.Action, "_", " "))
	for k, v := range rep.Counts {
		fmt.Printf("  %-22s %d\n", k, v)
	}
	for _, w := range rep.Warnings {
		fmt.Println("  warning:", w)
	}
	for _, w := range rep.Skipped {
		fmt.Println("  note:", w)
	}
	return nil
}
