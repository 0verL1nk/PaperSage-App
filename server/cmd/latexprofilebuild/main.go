// Command latexprofilebuild derives the inputs a TeX Live profile publish needs
// from one tlpdb snapshot: the filename index and the basenames that only
// kpsewhich can disambiguate.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/0verL1nk/GraduateProject/server/internal/latex"
)

func main() {
	tlpdbPath := flag.String("tlpdb", "", "path to a plain-text or .xz texlive.tlpdb")
	overridesPath := flag.String("license-overrides", "", "optional JSON license override file")
	listAmbiguous := flag.Bool("list-ambiguous", false, "print basenames claimed by more than one package")
	flag.Parse()
	if *tlpdbPath == "" {
		fmt.Fprintln(os.Stderr, "usage: latexprofilebuild -tlpdb <file> [-license-overrides <file>] [-list-ambiguous]")
		os.Exit(2)
	}
	overrides := map[string]latex.LicenseOverride{}
	if *overridesPath != "" {
		loaded, err := latex.LoadLicenseOverrides(*overridesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "latexprofilebuild: %v\n", err)
			os.Exit(1)
		}
		overrides = loaded
	}
	database, err := load(*tlpdbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "latexprofilebuild: %v\n", err)
		os.Exit(1)
	}
	index, excluded := latex.BuildTexliveIndex(database, overrides)
	ambiguous := latex.AmbiguousBasenames(index)
	fmt.Printf("packages=%d files=%d index_keys=%d ambiguous=%d excluded_packages=%d\n",
		len(database.Packages), len(database.Files), len(index), len(ambiguous), len(excluded))
	if *listAmbiguous {
		for _, name := range ambiguous {
			fmt.Println(name)
		}
	}
}

func load(path string) (*latex.TexliveDatabase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".xz") {
		return latex.ParseTlpdbXZ(data)
	}
	return latex.ParseTlpdb(bytes.NewReader(data))
}
