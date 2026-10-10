package latex

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ulikunitz/xz"
)

const (
	maxResourceArchiveBytes     = int64(128 << 20)
	maxExpandedArchiveBytes     = int64(512 << 20)
	maxCachedArchiveBytes       = int64(64 << 20)
	maxCachedArchiveEntryBytes  = int64(8 << 20)
	maxConcurrentArchiveFetches = 4
)

var resourceArchiveHashPattern = regexp.MustCompile(`^[a-f0-9]{128}$`)
var resourcePackagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

type packageArchiveCache struct {
	mu        sync.Mutex
	archives  map[string][]byte
	flights   map[string]*archiveFetchFlight
	order     []string
	bytes     int64
	maxBytes  int64
	semaphore chan struct{}
	client    *http.Client
}

type archiveFetchFlight struct {
	done chan struct{}
	data []byte
	err  error
}

func newPackageArchiveCache(maxBytes int64) *packageArchiveCache {
	return &packageArchiveCache{
		archives:  make(map[string][]byte),
		flights:   make(map[string]*archiveFetchFlight),
		maxBytes:  maxBytes,
		semaphore: make(chan struct{}, maxConcurrentArchiveFetches),
		client: &http.Client{
			Timeout: 90 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 || !safeExternalHTTPS(request.URL) {
					return errors.New("TeX Live repository redirected to an invalid address")
				}
				return nil
			},
		},
	}
}

func validateResourceRepository(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil || !safeExternalHTTPS(u) || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(u.Path, "/archive") {
		return errors.New("LaTeX resource repository must be a valid external HTTPS TeX Live repository URL")
	}
	return nil
}

func safeExternalHTTPS(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
	}
	return true
}

func validResourcePackage(name string) bool {
	return resourcePackagePattern.MatchString(name) && name != "." && name != ".."
}

func (c *packageArchiveCache) resource(ctx context.Context, repository string, provenance resourceCatalogProvenance, object resourceCatalogObject) ([]byte, error) {
	archive, err := c.archive(ctx, repository, provenance)
	if err != nil {
		return nil, err
	}
	return extractCataloguedResource(archive, provenance.SourcePath, object)
}

func (c *packageArchiveCache) packageFile(ctx context.Context, repository string, provenance resourceCatalogProvenance) ([]byte, error) {
	archive, err := c.archive(ctx, repository, provenance)
	if err != nil {
		return nil, err
	}
	return extractPackageResource(archive, provenance.SourcePath)
}

func (c *packageArchiveCache) archive(ctx context.Context, repository string, provenance resourceCatalogProvenance) ([]byte, error) {
	key := repository + "\x00" + provenance.Package + "\x00" + provenance.PackageRevision + "\x00" + provenance.ContainerSHA512
	c.mu.Lock()
	if archive, ok := c.archives[key]; ok {
		c.mu.Unlock()
		return archive, nil
	}
	if flight, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return flight.data, flight.err
		}
	}
	flight := &archiveFetchFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	archive, err := c.fetch(ctx, repository, provenance)
	c.mu.Lock()
	flight.data, flight.err = archive, err
	if err == nil && int64(len(archive)) <= maxCachedArchiveEntryBytes && int64(len(archive)) <= c.maxBytes {
		for c.bytes+int64(len(archive)) > c.maxBytes && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			c.bytes -= int64(len(c.archives[oldest]))
			delete(c.archives, oldest)
		}
		c.archives[key] = archive
		c.order = append(c.order, key)
		c.bytes += int64(len(archive))
	}
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return archive, err
}

func (c *packageArchiveCache) fetch(ctx context.Context, repository string, provenance resourceCatalogProvenance) ([]byte, error) {
	if !validResourcePackage(provenance.Package) || provenance.ContainerSize <= 0 || provenance.ContainerSize > maxResourceArchiveBytes || !resourceArchiveHashPattern.MatchString(provenance.ContainerSHA512) {
		return nil, errors.New("catalogued TeX Live package archive metadata is invalid")
	}
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	archiveURL := strings.TrimRight(repository, "/") + "/archive/" + url.PathEscape(provenance.Package) + ".tar.xz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create TeX Live archive request: %w", err)
	}
	req.Header.Set("Accept", "application/x-xz, application/octet-stream")
	response, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch TeX Live package archive %q: %w", provenance.Package, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TeX Live package archive %q returned HTTP %d", provenance.Package, response.StatusCode)
	}
	if !safeExternalHTTPS(response.Request.URL) {
		return nil, errors.New("TeX Live package archive resolved to an invalid address")
	}
	if response.ContentLength >= 0 && response.ContentLength != provenance.ContainerSize {
		return nil, fmt.Errorf("TeX Live package archive %q has an unexpected size", provenance.Package)
	}
	limited := io.LimitReader(response.Body, maxResourceArchiveBytes+1)
	archive, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read TeX Live package archive %q: %w", provenance.Package, err)
	}
	if int64(len(archive)) != provenance.ContainerSize {
		return nil, fmt.Errorf("TeX Live package archive %q size does not match its pinned revision", provenance.Package)
	}
	digest := sha512.Sum512(archive)
	if hex.EncodeToString(digest[:]) != provenance.ContainerSHA512 {
		return nil, fmt.Errorf("TeX Live package archive %q checksum does not match its pinned revision", provenance.Package)
	}
	return archive, nil
}

func extractCataloguedResource(archive []byte, sourcePath string, object resourceCatalogObject) ([]byte, error) {
	data, err := extractPackageResource(archive, sourcePath)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != object.Size {
		return nil, errors.New("extracted TeX Live resource size does not match its catalog")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != object.SHA256 {
		return nil, errors.New("extracted TeX Live resource checksum does not match its catalog")
	}
	return data, nil
}

func extractPackageResource(archive []byte, sourcePath string) ([]byte, error) {
	if !validCatalogSourcePath(sourcePath) || len(archive) == 0 {
		return nil, errors.New("TeX Live resource path is invalid")
	}
	xzReader, err := (xz.ReaderConfig{DictCap: 32 << 20, SingleStream: true}).NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open TeX Live package archive: %w", err)
	}
	limited := &io.LimitedReader{R: xzReader, N: maxExpandedArchiveBytes + 1}
	tarReader := tar.NewReader(limited)
	target := strings.TrimPrefix(sourcePath, "texmf-dist/")
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			if limited.N <= 0 {
				return nil, errors.New("TeX Live package archive exceeds the expanded size limit")
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read TeX Live package archive: %w", err)
		}
		member := strings.TrimPrefix(path.Clean(header.Name), "./")
		if !safeArchiveMember(member) {
			return nil, errors.New("TeX Live package archive contains an unsafe member path")
		}
		if member != target && member != "texmf-dist/"+target && member != "RELOC/"+target && member != "RELOC/texmf-dist/"+target {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, errors.New("catalogued TeX Live resource is not a regular archive file")
		}
		if header.Size <= 0 || header.Size > maxResourceBytes {
			return nil, errors.New("TeX Live resource size is outside supported bounds")
		}
		data, err := io.ReadAll(io.LimitReader(tarReader, maxResourceBytes+1))
		if err != nil {
			return nil, fmt.Errorf("extract TeX Live resource: %w", err)
		}
		if int64(len(data)) != header.Size {
			return nil, errors.New("extracted TeX Live resource size does not match its archive header")
		}
		return data, nil
	}
	return nil, errors.New("file is absent from its TeX Live package archive")
}

func safeArchiveMember(name string) bool {
	return name != "" && !path.IsAbs(name) && path.Clean(name) == name && name != "." && !strings.HasPrefix(name, "../") && !strings.Contains(name, `\`)
}
