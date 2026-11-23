// Command latexprofile inspects a TeX Live profile before it is embedded in the
// server binary. `verify` is the offline gate CI runs on every profile change;
// `list-ambiguous` produces the basenames that need a kpsewhich override.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
	"github.com/0verL1nk/GraduateProject/server/internal/latex/profile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "verify":
		verify(os.Args[2:])
	case "list-ambiguous":
		listAmbiguous(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: latexprofile verify -dir <profile dir> | list-ambiguous -tlpdb <file> [-license-overrides <file>]")
	os.Exit(2)
}

func verify(args []string) {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	dir := flags.String("dir", "", "profile directory")
	_ = flags.Parse(args)
	if *dir == "" {
		usage()
	}
	bundle, err := profile.Load(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "latexprofile: %v\n", err)
		os.Exit(1)
	}
	findings := bundle.Verify()
	for _, finding := range findings {
		fmt.Printf("FAIL %s\n", finding)
	}
	if len(findings) > 0 {
		fmt.Printf("%s: %d problem(s)\n", bundle.Profile.Profile, len(findings))
		os.Exit(1)
	}
	fmt.Printf("%s: ok, %d required resources resolve offline\n", bundle.Profile.Profile, len(profile.RequiredResources))
}

func listAmbiguous(args []string) {
	flags := flag.NewFlagSet("list-ambiguous", flag.ExitOnError)
	tlpdbPath := flags.String("tlpdb", "", "path to a plain-text or .xz texlive.tlpdb")
	overridesPath := flags.String("license-overrides", "", "optional JSON license override file")
	_ = flags.Parse(args)
	if *tlpdbPath == "" {
		usage()
	}
	overrides := map[string]latex.LicenseOverride{}
	if *overridesPath != "" {
		loaded, err := latex.LoadLicenseOverrides(*overridesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "latexprofile: %v\n", err)
			os.Exit(1)
		}
		overrides = loaded
	}
	data, err := os.ReadFile(*tlpdbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "latexprofile: %v\n", err)
		os.Exit(1)
	}
	var database *latex.TexliveDatabase
	if strings.HasSuffix(*tlpdbPath, ".xz") {
		database, err = latex.ParseTlpdbXZ(data)
	} else {
		database, err = latex.ParseTlpdb(bytes.NewReader(data))
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "latexprofile: %v\n", err)
		os.Exit(1)
	}
	index, excluded := latex.BuildTexliveIndex(database, overrides)
	ambiguous := latex.AmbiguousBasenames(index)
	fmt.Printf("packages=%d files=%d index_keys=%d ambiguous=%d excluded_packages=%d\n",
		len(database.Packages), len(database.Files), len(index), len(ambiguous), len(excluded))
	for _, name := range ambiguous {
		fmt.Println(name)
	}
}
