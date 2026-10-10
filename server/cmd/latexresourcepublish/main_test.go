package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoredCatalogCompressionIsDeterministicAndRoundTrips(t *testing.T) {
	plain := []byte(strings.Repeat(`{"schema_version":3,"entry":"xetex/26/example.sty"}`+"\n", 1000))
	first, err := encodeStoredCatalog(plain)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encodeStoredCatalog(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("catalog compression is not deterministic")
	}
	if len(first) >= len(plain) {
		t.Fatalf("compressed catalog is not smaller: %d >= %d", len(first), len(plain))
	}
	decoded, err := decodeStoredCatalog(first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatal("compressed catalog did not round trip")
	}
	legacy, err := decodeStoredCatalog(plain)
	if err != nil || !bytes.Equal(legacy, plain) {
		t.Fatalf("legacy plain catalog did not remain readable: %v", err)
	}
}

func validTestCatalogWithDerivedResource() catalog {
	generated := []byte("generated runtime config\n")
	digest := sha256.Sum256(generated)
	hash := hex.EncodeToString(digest[:])
	var lookup struct {
		Kind   string `json:"kind"`
		Format string `json:"format"`
		Name   string `json:"name"`
	}
	lookup.Kind = "dvipdfmx"
	lookup.Format = "11"
	lookup.Name = "generated.map"
	packageLookup := lookup
	packageLookup.Kind = "xetex"
	packageLookup.Format = "26"
	packageLookup.Name = "example.sty"
	return catalog{
		SchemaVersion: 3,
		Profile:       "test-profile",
		Revision:      "test-profile",
		Objects: []catalogObject{{
			SHA256: hash, Size: int64(len(generated)), MediaType: "text/plain",
			Provenance: []catalogProvenance{{
				SourcePath: "texmf-dist/tex/latex/example/example.sty", Package: "example", PackageRevision: "12345",
				ContainerSize: 100, ContainerSHA512: strings.Repeat("a", 128), License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/example",
			}},
		}},
		Entries: []catalogEntry{{Lookup: packageLookup, SHA256: hash}},
		DerivedResources: []derivedResource{{
			Lookup: lookup, SHA256: hash, Size: int64(len(generated)), MediaType: "text/plain", ObjectKey: "derived/" + hash,
			Generator: derivedGenerator{Name: "updmap-sys", Version: "TeX Live 2026 r80534", ExecutableSHA256: strings.Repeat("b", 64)},
			Inputs: []derivedInput{{
				SourcePath: "texmf-dist/web2c/updmap.cfg", SHA256: strings.Repeat("c", 64), Package: "texlive-config", PackageRevision: "12345",
				ContainerSize: 100, ContainerSHA512: strings.Repeat("d", 128), License: "LPPL-1.3c", LicenseSource: "https://ctan.org/pkg/texlive-config",
			}},
		}},
	}
}

func TestValidateCatalogChecksDerivedResourceProvenance(t *testing.T) {
	document := validTestCatalogWithDerivedResource()
	if _, err := validateCatalog(document); err != nil {
		t.Fatalf("valid derived resource catalog rejected: %v", err)
	}

	document.DerivedResources[0].Inputs[0].SHA256 = "invalid"
	if _, err := validateCatalog(document); err == nil {
		t.Fatal("invalid derived input hash was accepted")
	}
}

func TestValidateDerivedFilesVerifiesContentAddressedArtifact(t *testing.T) {
	root := t.TempDir()
	document := validTestCatalogWithDerivedResource()
	resource := document.DerivedResources[0]
	data := []byte("generated runtime config\n")
	derivedDir := filepath.Join(root, "derived")
	if err := os.MkdirAll(derivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(derivedDir, resource.SHA256), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateDerivedFiles(root, document.DerivedResources); err != nil {
		t.Fatalf("valid generated artifact rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(derivedDir, resource.SHA256), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateDerivedFiles(root, document.DerivedResources); err == nil {
		t.Fatal("tampered generated artifact was accepted")
	}
}
