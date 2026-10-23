package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/internal/trajectory"
)

const exportTrajectoryUsage = `Usage: cercano export trajectory --conversation <id> --out <path> [options]

Export a complete conversation as a Cercano trajectory bundle. If --out ends in
.zip, the bundle is packaged as a zip archive; otherwise a directory bundle is
written.

Options:
  --conversation <id>     Conversation id to export (required)
  --out <path>            Output directory or .zip path (required)
  --zip                   Force zip output even when --out does not end in .zip
  --redact default|none   Redaction mode (default: default)
  --include-logs          Reserved for future log artifact export
  --overwrite             Replace an existing output path
`

func runExport(args []string) {
	if len(args) == 0 || args[0] != "trajectory" {
		fmt.Fprint(os.Stderr, "usage: cercano export trajectory --conversation <id> --out <path>\n")
		os.Exit(2)
	}
	runExportTrajectory(args[1:])
}

type exportTrajectoryFlags struct {
	convID, out, redact                  *string
	zipOut, includeLogs, overwrite, help *bool
}

// newExportTrajectoryFlags defines `cercano export trajectory` flags; the
// startup check parses with the same definitions to recognize a help request.
func newExportTrajectoryFlags(output io.Writer) (*flag.FlagSet, exportTrajectoryFlags) {
	fs := flag.NewFlagSet("export trajectory", flag.ContinueOnError)
	fs.SetOutput(output)
	var f exportTrajectoryFlags
	f.convID = fs.String("conversation", "", "conversation id to export")
	f.out = fs.String("out", "", "output directory or .zip path")
	f.zipOut = fs.Bool("zip", false, "force zip output")
	f.redact = fs.String("redact", "default", "redaction mode: default or none")
	f.includeLogs = fs.Bool("include-logs", false, "include logs (reserved)")
	f.overwrite = fs.Bool("overwrite", false, "replace existing output")
	f.help = fs.Bool("help", false, "show help")
	return fs, f
}

func runExportTrajectory(args []string) {
	fs, f := newExportTrajectoryFlags(os.Stderr)
	convID, out, zipOut, redact, includeLogs, overwrite, help := f.convID, f.out, f.zipOut, f.redact, f.includeLogs, f.overwrite, f.help
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if *help {
		fmt.Print(exportTrajectoryUsage)
		return
	}
	if strings.TrimSpace(*convID) == "" || strings.TrimSpace(*out) == "" {
		fmt.Fprint(os.Stderr, exportTrajectoryUsage)
		os.Exit(2)
	}
	mode := trajectory.RedactionMode(*redact)
	if mode != trajectory.RedactDefault && mode != trajectory.RedactNone {
		fmt.Fprintf(os.Stderr, "invalid --redact %q (want default or none)\n", *redact)
		os.Exit(2)
	}
	format := trajectory.FormatInfer
	if *zipOut {
		format = trajectory.FormatZip
	}
	dbPath, err := conversation.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "conversation db:", err)
		os.Exit(1)
	}
	store, err := conversation.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open conversation db:", err)
		os.Exit(1)
	}
	defer store.Close()
	exp := trajectory.Exporter{Store: store}
	res, err := exp.Export(context.Background(), trajectory.Options{
		ConversationID: *convID,
		OutPath:        *out,
		Format:         format,
		Redaction:      mode,
		IncludeLogs:    *includeLogs,
		Overwrite:      *overwrite,
		Version:        version,
		Now:            time.Now(),
	}, func(phase, message string) {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", phase, message)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "export trajectory:", err)
		os.Exit(1)
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fmt.Println(res.OutputPath)
}
