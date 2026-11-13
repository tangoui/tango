// Command tan creates, runs, builds and ships Tango applications.
//
//	tan init [dir]          new project from a template (asks the questions when dir is omitted)
//	tan dev                 run the app against the front-end dev server, rebuilding on Go changes
//	tan build               production build for one or more platforms (desktop, iOS, Android)
//	tan run                 build, install and run on the iOS Simulator or an Android device
//	tan package             installers and archives from a build
//	tan sign                macOS code signing and notarization
//	tan keygen              Ed25519 key pair for signing updates
//	tan release-manifest    update files, patches and the signed manifest for a release
//	tan sign-update         sign an update manifest with an Ed25519 key
//	tan applinks            apple-app-site-association and assetlinks.json for tan.json "appLinks"
//	tan generate bindings   TypeScript clients for bind.Register services
//	tan doctor              check the toolchain
//	tan version             print the CLI version
//
// Every command reads tan.json from the current directory (or the one
// given by -C) except init, keygen, doctor and version.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "0.1.0-dev"

type command struct {
	name    string
	summary string
	run     func(args []string) error
}

func commands() []command {
	return []command{
		{"init", "create a project from a template", runInit},
		{"dev", "run the app in development mode", runDev},
		{"build", "build the app for one or more platforms", runBuild},
		{"run", "build and run the app on the iOS Simulator or Android", runRun},
		{"package", "build, then make installers and archives", runPackage},
		{"sign", "sign a macOS app or disk image (and notarize), or a Windows exe/msi/msix", runSign},
		{"keygen", "create an Ed25519 key pair for update signing", runKeygen},
		{"release-manifest", "make the signed update manifest and patches for a release", runReleaseManifest},
		{"sign-update", "sign an update manifest with an Ed25519 key", runSignUpdate},
		{"applinks", "write the Universal Links / App Links files for /.well-known/", runAppLinks},
		{"generate", "generate code (bindings)", runGenerate},
		{"doctor", "check the toolchain and platform dependencies", runDoctor},
		{"version", "print the version", runVersion},
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "tan:", err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stdout)
		return nil
	}
	if args[0] == "-v" || args[0] == "--version" {
		return runVersion(nil)
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(args[1:])
		}
	}
	usage(os.Stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: tan <command> [flags] [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-12s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, `Run "tan <command> -h" for a command's flags.`)
}

// newFlags returns a flag set for a subcommand whose usage line is
// "tan <name> <synopsis>".
func newFlags(name, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet("tan "+name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: tan %s %s\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// projectFlag adds -C, the project directory.
func projectFlag(fs *flag.FlagSet) *string {
	return fs.String("C", ".", "project `dir` (where tan.json is)")
}

func runVersion(args []string) error {
	v := version
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		v = strings.TrimPrefix(bi.Main.Version, "v")
	}
	fmt.Printf("tan %s\n", v)
	return nil
}

func logf(format string, a ...any)  { fmt.Fprintf(os.Stderr, "• "+format+"\n", a...) }
func warnf(format string, a ...any) { fmt.Fprintf(os.Stderr, "! "+format+"\n", a...) }
