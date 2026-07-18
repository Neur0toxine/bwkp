package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/Neur0toxine/bwkp/internal/app"
	"github.com/Neur0toxine/bwkp/internal/buildinfo"
	"github.com/Neur0toxine/bwkp/internal/native"
	"github.com/Neur0toxine/bwkp/internal/progress"
	"github.com/Neur0toxine/bwkp/internal/prompt"
	"github.com/Neur0toxine/bwkp/internal/security"
	"github.com/Neur0toxine/bwkp/pkg/bwapi"
	"github.com/Neur0toxine/bwkp/pkg/convert"
	"github.com/Neur0toxine/bwkp/pkg/kpdb"
)

type CLI struct{ stdout, stderr io.Writer }

func New(stdout, stderr io.Writer) *CLI { return &CLI{stdout: stdout, stderr: stderr} }

func (c *CLI) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		c.usage()
		return errors.New("a command is required")
	}
	switch args[0] {
	case "export":
		return c.export(ctx, args[1:])
	case "version":
		_, err := fmt.Fprintf(c.stdout,
			"bwkp %s (%s, %s, %s/%s)\nKeePassXC: %s\nBitwarden SDK: %s\n",
			buildinfo.Version, buildinfo.Commit, buildinfo.Date, runtime.GOOS, runtime.GOARCH,
			native.KeePassXCVersion(), native.BitwardenSDKVersion())
		return err
	case "help", "-h", "--help":
		c.usage()
		return nil
	default:
		c.usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (c *CLI) export(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("bwkp export", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	var region, server, apiURL, identityURL, caCert, email, output string
	var masterPasswordFile, totpFile, databasePasswordFile, keyFile string
	var force, keyFileOnly, noProgress, appendSource, allowLossy bool
	var cipher, compression string
	var memory uint64
	var iterations uint64
	var parallelism uint
	var target time.Duration
	flags.StringVar(&region, "region", "", "Bitwarden cloud region: us or eu")
	flags.StringVar(&server, "server", "", "self-hosted Vaultwarden base URL")
	flags.StringVar(&apiURL, "api-url", "", "advanced API endpoint override")
	flags.StringVar(&identityURL, "identity-url", "", "advanced identity endpoint override")
	flags.StringVar(&caCert, "ca-cert", "", "PEM certificate authority for a self-hosted server")
	flags.StringVar(&email, "email", "", "Bitwarden account email")
	flags.StringVar(&output, "output", "", "target .kdbx path (required)")
	flags.BoolVar(&force, "force", false, "atomically replace an existing target")
	flags.StringVar(&masterPasswordFile, "master-password-file", "", "read the master password from a file")
	flags.StringVar(&totpFile, "totp-file", "", "read authenticator TOTP from a file")
	flags.StringVar(&databasePasswordFile, "database-password-file", "", "read the database password from a file")
	flags.StringVar(&keyFile, "key-file", "", "use an existing KeePass key file")
	flags.BoolVar(&keyFileOnly, "key-file-only", false, "do not add a database password")
	flags.BoolVar(&noProgress, "no-progress", false, "disable interactive progress bars")
	flags.BoolVar(&appendSource, "append-source", false, "append complete protected Bitwarden source metadata")
	flags.BoolVar(&allowLossy, "allow-lossy", false, "skip items that cannot be converted and show warnings")
	flags.StringVar(&cipher, "cipher", string(kpdb.CipherAES256), "KDBX cipher: aes256 or chacha20")
	flags.StringVar(&compression, "compression", string(kpdb.CompressionGZip), "KDBX compression: gzip or none")
	flags.Uint64Var(&memory, "kdf-memory-kib", 64*1024, "Argon2id memory in KiB")
	flags.Uint64Var(&iterations, "kdf-iterations", 0, "fixed Argon2id iterations; disables calibration")
	flags.UintVar(&parallelism, "kdf-parallelism", uint(min(runtime.NumCPU(), 4)), "Argon2id parallel lanes")
	flags.DurationVar(&target, "kdf-target", time.Second, "calibrated KDF duration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if email == "" {
		return errors.New("--email is required")
	}
	if output == "" {
		return errors.New("--output is required")
	}

	caCertPEM, err := readOptional(caCert)
	if err != nil {
		return fmt.Errorf("read CA certificate: %w", err)
	}
	endpoints, err := bwapi.ResolveEndpoints(bwapi.Region(region), server, apiURL, identityURL, caCertPEM)
	if err != nil {
		return err
	}
	masterPassword, err := prompt.Secret("Master password", masterPasswordFile, false)
	if err != nil {
		return err
	}
	defer security.Clear(masterPassword)
	credentials := kpdb.Credentials{}
	if !keyFileOnly {
		credentials.Password, err = prompt.Secret("Database password", databasePasswordFile, true)
		if err != nil {
			return err
		}
		defer security.Clear(credentials.Password)
	}
	credentials.KeyFile, err = readOptional(keyFile)
	if err != nil {
		return fmt.Errorf("read key file: %w", err)
	}
	defer security.Clear(credentials.KeyFile)
	options := kpdb.DefaultOptions()
	options.Cipher, options.Compression = kpdb.Cipher(cipher), kpdb.Compression(compression)
	options.MemoryKiB, options.Parallelism = memory, uint32(parallelism)
	if iterations > 0 {
		options.Iterations, options.TargetTime = iterations, 0
	} else {
		options.TargetTime = target
	}

	progressRenderer := progress.NewTerminal(c.stderr, !noProgress)
	defer progressRenderer.Close()
	exporter := app.New(bwapi.NewNativeClient(), convert.NewWithOptions(convert.Options{
		AppendSource: appendSource,
		AllowLossy:   allowLossy,
	}), kpdb.NewNativeWriter())
	report, err := exporter.Export(ctx, app.Request{
		Login:  bwapi.LoginRequest{Endpoints: endpoints, Email: email, MasterPassword: masterPassword},
		TOTP:   func(context.Context) (string, error) { return prompt.Code("Authenticator code", totpFile) },
		Output: output, Force: force, Credentials: credentials, Options: options, Progress: progressRenderer,
	})
	if err != nil {
		return err
	}
	progressRenderer.Close()
	if err := writeWarnings(c.stderr, report.Warnings); err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.stdout, "Exported %d items as %d entries with %d attachments to %s\n", report.Items, report.Entries, report.Attachments, output)
	return err
}

func writeWarnings(writer io.Writer, warnings []convert.Warning) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(writer, "Warning: skipped item %q (%s): %s\n", warning.ItemName, warning.ItemID, warning.Message); err != nil {
			return err
		}
	}
	return nil
}

func readOptional(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	return os.ReadFile(path)
}

func (c *CLI) usage() {
	_, _ = fmt.Fprintln(c.stderr, "Usage:\n  bwkp export --server URL --email EMAIL --output FILE [options]\n  bwkp export --region us|eu --email EMAIL --output FILE [options]\n  bwkp version")
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if _, ok := errors.AsType[*strconv.NumError](err); ok {
		return 2
	}
	return 1
}
