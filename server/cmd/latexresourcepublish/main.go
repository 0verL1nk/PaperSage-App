package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	awsconfig "github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/joho/godotenv"
)

const maxCatalogBytes = 64 * 1024 * 1024
const maxPackageIndexBytes = 128 * 1024 * 1024

var (
	profilePattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)
	hashPattern      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	archiveHash      = regexp.MustCompile(`^[a-f0-9]{128}$`)
	packagePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	revisionPattern  = regexp.MustCompile(`^[0-9]+$`)
	generatorPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	formatPattern    = regexp.MustCompile(`^[A-Za-z0-9._+-]+(?:/[A-Za-z0-9._+-]+)*$`)
	fontVariant      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
)

type catalog struct {
	SchemaVersion       int               `json:"schema_version"`
	Profile             string            `json:"profile"`
	Revision            string            `json:"revision"`
	Objects             []catalogObject   `json:"objects"`
	Entries             []catalogEntry    `json:"entries"`
	DerivedResources    []derivedResource `json:"derived_resources"`
	PackageIndexGzip    string            `json:"package_index_gzip,omitempty"`
	PackageIndexSHA256  string            `json:"package_index_sha256,omitempty"`
	PackageIndexEntries int               `json:"package_index_entries,omitempty"`
}

type catalogObject struct {
	SHA256     string              `json:"sha256"`
	Size       int64               `json:"size"`
	MediaType  string              `json:"media_type"`
	Provenance []catalogProvenance `json:"provenance"`
}

type catalogProvenance struct {
	SourcePath      string `json:"source_path"`
	Package         string `json:"package"`
	PackageRevision string `json:"package_revision"`
	ContainerSize   int64  `json:"container_size"`
	ContainerSHA512 string `json:"container_checksum"`
	License         string `json:"license"`
	LicenseSource   string `json:"license_source"`
}

type catalogEntry struct {
	Lookup struct {
		Kind   string `json:"kind"`
		Format string `json:"format"`
		Name   string `json:"name"`
	} `json:"lookup"`
	SHA256 string `json:"sha256"`
}

type derivedGenerator struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	ExecutableSHA256 string `json:"executable_sha256"`
}

type derivedInput struct {
	SourcePath      string `json:"source_path"`
	SHA256          string `json:"sha256"`
	Package         string `json:"package"`
	PackageRevision string `json:"package_revision"`
	ContainerSize   int64  `json:"container_size"`
	ContainerSHA512 string `json:"container_checksum"`
	License         string `json:"license"`
	LicenseSource   string `json:"license_source"`
}

type derivedResource struct {
	Lookup struct {
		Kind   string `json:"kind"`
		Format string `json:"format"`
		Name   string `json:"name"`
	} `json:"lookup"`
	SHA256    string           `json:"sha256"`
	Size      int64            `json:"size"`
	MediaType string           `json:"media_type"`
	ObjectKey string           `json:"object_key"`
	Generator derivedGenerator `json:"generator"`
	Inputs    []derivedInput   `json:"inputs"`
}

func main() {
	catalogDir := flag.String("catalog-dir", "", "generated catalog directory containing catalog.json")
	envFile := flag.String("env-file", "", "path to the server .env file; credentials are never printed")
	dryRun := flag.Bool("dry-run", false, "validate the catalog without connecting to object storage")
	flag.Parse()
	if err := run(*catalogDir, *envFile, *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(catalogDir, envFile string, dryRun bool) error {
	if catalogDir == "" {
		return errors.New("catalog-dir is required")
	}
	root, err := filepath.Abs(catalogDir)
	if err != nil {
		return fmt.Errorf("resolve catalog directory: %w", err)
	}
	catalogBytes, err := os.ReadFile(filepath.Join(root, "catalog.json"))
	if err != nil {
		return fmt.Errorf("read catalog: %w", err)
	}
	if len(catalogBytes) == 0 || len(catalogBytes) > maxCatalogBytes {
		return errors.New("catalog size is outside the supported limit")
	}
	var document catalog
	if err := json.Unmarshal(catalogBytes, &document); err != nil {
		return fmt.Errorf("decode catalog: %w", err)
	}
	resourceBytes, err := validateCatalog(document)
	if err != nil {
		return err
	}
	derivedBytes, err := validateDerivedFiles(root, document.DerivedResources)
	if err != nil {
		return err
	}
	resourceBytes += derivedBytes
	log.Printf("validated profile=%s package_lookups=%d package_resources=%d derived_resources=%d resource_bytes=%d catalog_bytes=%d",
		document.Profile, len(document.Entries), len(document.Objects), len(document.DerivedResources), resourceBytes, len(catalogBytes))
	if dryRun {
		return nil
	}
	if envFile != "" {
		values, err := godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("read configured env file: %w", err)
		}
		for key, value := range values {
			if _, exists := os.LookupEnv(key); !exists {
				if err := os.Setenv(key, value); err != nil {
					return fmt.Errorf("load configured environment: %w", err)
				}
			}
		}
	}
	settings, err := awsconfig.Load()
	if err != nil {
		return fmt.Errorf("load server settings: %w", err)
	}
	if settings.S3Endpoint == "" || settings.S3AccessKey == "" || settings.S3SecretKey == "" || settings.S3Bucket == "" {
		return errors.New("S3 endpoint, bucket, access key, and secret key must be configured")
	}
	if settings.LatexResourceProfile != document.Profile {
		return fmt.Errorf("catalog profile %q does not match server LATEX_RESOURCE_PROFILE %q", document.Profile, settings.LatexResourceProfile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	backend, err := storage.NewS3(ctx, settings)
	if err != nil {
		return fmt.Errorf("connect to configured object storage: %w", err)
	}
	key := "latex/texlive/" + document.Profile + "/catalog.json"
	if backend.Exists(ctx, key) {
		remote, err := backend.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("read existing immutable catalog: %w", err)
		}
		decodedRemote, decodeErr := decodeStoredCatalog(remote)
		if decodeErr != nil || !bytes.Equal(decodedRemote, catalogBytes) {
			var existing catalog
			if decodeErr != nil || json.Unmarshal(decodedRemote, &existing) != nil {
				return fmt.Errorf("immutable catalog key is occupied by unreadable data (%d bytes); choose a new profile revision", len(remote))
			}
			return fmt.Errorf("immutable catalog key is occupied: remote profile=%q schema=%d revision=%q (%d bytes), local schema=%d (%d bytes); choose a new profile revision", existing.Profile, existing.SchemaVersion, existing.Revision, len(remote), document.SchemaVersion, len(catalogBytes))
		}
		if err := publishDerivedResources(ctx, backend, root, document); err != nil {
			return err
		}
		log.Printf("catalog and %d immutable generated resources already published and verified", len(document.DerivedResources))
		return nil
	}
	if err := publishDerivedResources(ctx, backend, root, document); err != nil {
		return err
	}
	storedCatalog, err := encodeStoredCatalog(catalogBytes)
	if err != nil {
		return fmt.Errorf("compress resource catalog: %w", err)
	}
	if err := backend.Put(ctx, key, storedCatalog); err != nil {
		return fmt.Errorf("publish resource catalog: %w", err)
	}
	remote, err := backend.Get(ctx, key)
	decodedRemote, decodeErr := decodeStoredCatalog(remote)
	if err != nil || decodeErr != nil || !bytes.Equal(decodedRemote, catalogBytes) {
		return errors.New("published catalog could not be verified")
	}
	log.Printf("published catalog (%d bytes compressed from %d) and %d generated runtime resources; package resources remain on demand", len(storedCatalog), len(catalogBytes), len(document.DerivedResources))
	return nil
}

func encodeStoredCatalog(data []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer.ModTime = time.Time{}
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func decodeStoredCatalog(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		if len(data) == 0 || len(data) > maxCatalogBytes {
			return nil, errors.New("catalog size is outside supported limits")
		}
		return data, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, maxCatalogBytes+1))
	closeErr := reader.Close()
	if err != nil || len(decoded) == 0 || len(decoded) > maxCatalogBytes {
		return nil, errors.New("compressed catalog is invalid or exceeds supported limits")
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close compressed resource catalog: %w", closeErr)
	}
	return decoded, nil
}

func validateCatalog(document catalog) (int64, error) {
	if document.SchemaVersion != 3 || !profilePattern.MatchString(document.Profile) || document.Revision != document.Profile || len(document.Entries) == 0 || len(document.Objects) == 0 {
		return 0, errors.New("catalog profile or schema is invalid")
	}
	objects := make(map[string]catalogObject, len(document.Objects))
	var totalBytes int64
	for _, object := range document.Objects {
		if !hashPattern.MatchString(object.SHA256) || object.Size <= 0 || len(object.Provenance) == 0 {
			return 0, fmt.Errorf("catalog contains invalid object metadata for %q", object.SHA256)
		}
		if _, _, err := mime.ParseMediaType(object.MediaType); err != nil {
			return 0, fmt.Errorf("catalog contains invalid media type for %q", object.SHA256)
		}
		for _, provenance := range object.Provenance {
			if !validSourcePath(provenance.SourcePath) || !packagePattern.MatchString(provenance.Package) || !revisionPattern.MatchString(provenance.PackageRevision) || provenance.ContainerSize <= 0 || provenance.ContainerSize > 128<<20 || !archiveHash.MatchString(provenance.ContainerSHA512) || strings.TrimSpace(provenance.License) == "" || !strings.HasPrefix(provenance.LicenseSource, "https://") {
				return 0, fmt.Errorf("catalog contains incomplete package provenance for %q", object.SHA256)
			}
		}
		if _, exists := objects[object.SHA256]; exists {
			return 0, fmt.Errorf("catalog contains duplicate object %q", object.SHA256)
		}
		objects[object.SHA256] = object
		totalBytes += object.Size
	}
	lookups := make(map[string]struct{}, len(document.Entries))
	for _, entry := range document.Entries {
		if _, exists := objects[entry.SHA256]; !exists || entry.Lookup.Kind == "" || entry.Lookup.Format == "" || entry.Lookup.Name == "" || strings.ContainsAny(entry.Lookup.Name, `/\\`) {
			return 0, errors.New("catalog contains an invalid lookup or object reference")
		}
		key := entry.Lookup.Kind + "\x00" + entry.Lookup.Format + "\x00" + entry.Lookup.Name
		if _, exists := lookups[key]; exists {
			return 0, fmt.Errorf("catalog contains duplicate lookup %q", entry.Lookup.Name)
		}
		lookups[key] = struct{}{}
	}
	for _, resource := range document.DerivedResources {
		if !hashPattern.MatchString(resource.SHA256) || resource.Size <= 0 || resource.Size > 128<<20 || resource.ObjectKey != "derived/"+resource.SHA256 {
			return 0, errors.New("catalog contains an invalid generated resource object")
		}
		if _, _, err := mime.ParseMediaType(resource.MediaType); err != nil {
			return 0, fmt.Errorf("catalog contains an invalid generated resource media type: %w", err)
		}
		lookup := resource.Lookup
		formatValid := formatPattern.MatchString(lookup.Format)
		if lookup.Kind == "font" {
			formatValid = fontVariant.MatchString(lookup.Format)
		}
		for _, component := range strings.Split(lookup.Format, "/") {
			if component == "." || component == ".." {
				formatValid = false
			}
		}
		if (lookup.Kind != "xetex" && lookup.Kind != "pdftex" && lookup.Kind != "dvipdfmx" && lookup.Kind != "font") || !formatValid || len(lookup.Format) > 96 || lookup.Name == "" || len(lookup.Name) > 255 || strings.ContainsAny(lookup.Name, `/\\`) || lookup.Name == "." || lookup.Name == ".." {
			return 0, errors.New("catalog contains an invalid generated resource lookup")
		}
		key := lookup.Kind + "\x00" + lookup.Format + "\x00" + lookup.Name
		if _, exists := lookups[key]; exists {
			return 0, fmt.Errorf("catalog contains a duplicate or conflicting generated lookup %q", lookup.Name)
		}
		lookups[key] = struct{}{}
		if !generatorPattern.MatchString(resource.Generator.Name) || strings.TrimSpace(resource.Generator.Version) == "" || len(resource.Generator.Version) > 256 || !hashPattern.MatchString(resource.Generator.ExecutableSHA256) || len(resource.Inputs) == 0 {
			return 0, errors.New("catalog contains invalid generated resource provenance")
		}
		for _, input := range resource.Inputs {
			if !validSourcePath(input.SourcePath) || !strings.HasPrefix(input.SourcePath, "texmf-dist/") || !hashPattern.MatchString(input.SHA256) || !packagePattern.MatchString(input.Package) || !revisionPattern.MatchString(input.PackageRevision) || input.ContainerSize <= 0 || input.ContainerSize > 128<<20 || !archiveHash.MatchString(input.ContainerSHA512) || strings.TrimSpace(input.License) == "" || !strings.HasPrefix(input.LicenseSource, "https://") {
				return 0, errors.New("catalog contains invalid generated input provenance")
			}
		}
		totalBytes += resource.Size
	}
	if document.PackageIndexGzip != "" {
		compressed, err := base64.StdEncoding.DecodeString(document.PackageIndexGzip)
		if err != nil || len(compressed) == 0 || len(compressed) > 8<<20 || document.PackageIndexEntries <= 0 || !hashPattern.MatchString(document.PackageIndexSHA256) {
			return 0, errors.New("catalog contains invalid package filename index metadata")
		}
		digest := sha256.Sum256(compressed)
		if hex.EncodeToString(digest[:]) != document.PackageIndexSHA256 {
			return 0, errors.New("catalog package filename index checksum does not match")
		}
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return 0, errors.New("catalog package filename index is not valid gzip data")
		}
		inflated, readErr := io.ReadAll(io.LimitReader(reader, maxPackageIndexBytes+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || len(inflated) == 0 || len(inflated) > maxPackageIndexBytes {
			return 0, errors.New("catalog package filename index exceeds the supported uncompressed size")
		}
		var index map[string][]json.RawMessage
		if err := json.Unmarshal(inflated, &index); err != nil {
			return 0, errors.New("catalog package filename index is not valid JSON")
		}
		count := 0
		for _, records := range index {
			count += len(records)
		}
		if count != document.PackageIndexEntries {
			return 0, errors.New("catalog package filename index entry count does not match its contents")
		}
	} else if document.PackageIndexSHA256 != "" || document.PackageIndexEntries != 0 {
		return 0, errors.New("catalog package filename index metadata is incomplete")
	}
	return totalBytes, nil
}

func validateDerivedFiles(root string, resources []derivedResource) (int64, error) {
	var total int64
	for _, resource := range resources {
		file := filepath.Join(root, "derived", resource.SHA256)
		resolved, err := filepath.EvalSymlinks(file)
		if err != nil {
			return 0, fmt.Errorf("resolve generated resource %q: %w", resource.SHA256, err)
		}
		relative, err := filepath.Rel(filepath.Join(root, "derived"), resolved)
		if err != nil || relative != resource.SHA256 {
			return 0, fmt.Errorf("generated resource path escapes catalog output: %q", resource.SHA256)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return 0, fmt.Errorf("read generated resource %q: %w", resource.SHA256, err)
		}
		if int64(len(data)) != resource.Size {
			return 0, fmt.Errorf("generated resource %q size does not match catalog", resource.SHA256)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != resource.SHA256 {
			return 0, fmt.Errorf("generated resource %q hash does not match catalog", resource.SHA256)
		}
		total += int64(len(data))
	}
	return total, nil
}

func publishDerivedResources(ctx context.Context, backend storage.Storage, root string, document catalog) error {
	for _, resource := range document.DerivedResources {
		key := "latex/texlive/" + document.Profile + "/" + resource.ObjectKey
		file := filepath.Join(root, "derived", resource.SHA256)
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read generated resource %q: %w", resource.SHA256, err)
		}
		if backend.Exists(ctx, key) {
			remote, err := backend.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("read existing immutable generated resource %q: %w", resource.SHA256, err)
			}
			if !bytes.Equal(remote, data) {
				return fmt.Errorf("immutable generated resource key %q contains different bytes", key)
			}
			continue
		}
		if err := backend.Put(ctx, key, data); err != nil {
			return fmt.Errorf("publish generated resource %q: %w", resource.SHA256, err)
		}
		remote, err := backend.Get(ctx, key)
		if err != nil || !bytes.Equal(remote, data) {
			return fmt.Errorf("published generated resource %q could not be verified", resource.SHA256)
		}
	}
	return nil
}

func validSourcePath(sourcePath string) bool {
	return sourcePath != "" && len(sourcePath) <= 1024 && !path.IsAbs(sourcePath) && path.Clean(sourcePath) == sourcePath && !strings.Contains(sourcePath, `\\`)
}
