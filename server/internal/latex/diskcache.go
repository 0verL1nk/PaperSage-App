package latex

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// TeX Live resource bytes are immutable per profile, so the on-disk cache is the
// only persistence the resource service needs: no object storage, and an
// upgrade that swaps the binary keeps the cache directory untouched.
const (
	diskCacheMagic        = "PSLR1"
	diskCacheMaxHeader    = 64 << 10
	diskCacheEvictRatio   = 0.8
	diskCacheTmpMaxAge    = time.Hour
	diskCacheMaxEntrySize = 64 << 20
)

// DiskCacheMeta is the self-describing header of one cache entry.
type DiskCacheMeta struct {
	Profile    string `json:"profile"`
	SourcePath string `json:"source_path"`
	Package    string `json:"package"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	MediaType  string `json:"media_type"`
}

type DiskCacheEntry struct {
	Meta DiskCacheMeta
	Data []byte
}

type diskCacheFlight struct {
	done  chan struct{}
	entry DiskCacheEntry
	err   error
}

// DiskCache is a bounded, self-healing local cache of verified resource bytes.
// Entries are written through a temp file and renamed, so a crash can never
// leave a half-written entry visible.
type DiskCache struct {
	root       string
	entriesDir string
	tmpDir     string
	maxBytes   int64

	mu      sync.Mutex
	sizes   map[string]int64
	lru     *list.List
	items   map[string]*list.Element
	bytes   int64
	flights map[string]*diskCacheFlight
}

// ValidateCacheDir rejects locations that would lose the cache on restart or
// corrupt a release: /tmp is wiped (and PrivateTmp hides it) and release
// directories are replaced on every deploy.
func ValidateCacheDir(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cleaned := filepath.Clean(absolute)
	temp, err := filepath.Abs(os.TempDir())
	if err == nil && (cleaned == temp || strings.HasPrefix(cleaned, temp+string(os.PathSeparator))) {
		return fmt.Errorf("LaTeX resource cache directory %q must not live under %q", path, temp)
	}
	if strings.HasPrefix(cleaned, "/opt/papersage/releases") {
		return fmt.Errorf("LaTeX resource cache directory %q must not live under /opt/papersage/releases", path)
	}
	return nil
}

// NewDiskCache prepares the cache directory, probes that it is writable and
// rebuilds the size accounting from disk. A misconfigured directory fails
// startup instead of silently running without a cache.
func NewDiskCache(root string, maxBytes int64) (*DiskCache, error) {
	if maxBytes <= 0 {
		return nil, errors.New("LaTeX resource cache size limit must be positive")
	}
	cache := &DiskCache{
		root:       root,
		entriesDir: filepath.Join(root, "entries"),
		tmpDir:     filepath.Join(root, "tmp"),
		maxBytes:   maxBytes,
		sizes:      make(map[string]int64),
		lru:        list.New(),
		items:      make(map[string]*list.Element),
		flights:    make(map[string]*diskCacheFlight),
	}
	for _, dir := range []string{root, cache.entriesDir, cache.tmpDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create LaTeX resource cache directory: %w", err)
		}
	}
	probe, err := os.CreateTemp(cache.tmpDir, "probe-*")
	if err != nil {
		return nil, fmt.Errorf("LaTeX resource cache directory is not writable: %w", err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	cache.removeStaleTempFiles()
	if err := cache.scan(); err != nil {
		return nil, err
	}
	cache.evict()
	return cache, nil
}

func (c *DiskCache) removeStaleTempFiles() {
	entries, err := os.ReadDir(c.tmpDir)
	if err != nil {
		return
	}
	deadline := time.Now().Add(-diskCacheTmpMaxAge)
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(deadline) {
			_ = os.Remove(filepath.Join(c.tmpDir, entry.Name()))
		}
	}
}

func (c *DiskCache) scan() error {
	type scanned struct {
		key     string
		size    int64
		modTime time.Time
	}
	var found []scanned
	err := filepath.WalkDir(c.entriesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			return err
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		found = append(found, scanned{key: strings.TrimSuffix(entry.Name(), ".bin"), size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return fmt.Errorf("scan LaTeX resource cache: %w", err)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].modTime.Before(found[j].modTime) })
	for _, item := range found {
		c.sizes[item.key] = item.size
		c.items[item.key] = c.lru.PushBack(item.key)
		c.bytes += item.size
	}
	return nil
}

func (c *DiskCache) path(key string) string {
	return filepath.Join(c.entriesDir, key[:2], key+".bin")
}

// LoadOrFill returns the cached entry, otherwise runs fill once per key and
// caches the result. A cache write failure never fails the fill: the bytes were
// already verified upstream, and losing a cache write only costs a future fetch.
func (c *DiskCache) LoadOrFill(key string, fill func() (DiskCacheEntry, error)) (DiskCacheEntry, error) {
	if entry, ok := c.Get(key); ok {
		return entry, nil
	}
	c.mu.Lock()
	if flight, running := c.flights[key]; running {
		c.mu.Unlock()
		<-flight.done
		return flight.entry, flight.err
	}
	flight := &diskCacheFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	entry, err := fill()
	if err == nil {
		_ = c.Put(key, entry)
	}
	c.mu.Lock()
	flight.entry, flight.err = entry, err
	delete(c.flights, key)
	close(flight.done)
	c.mu.Unlock()
	return entry, err
}

// Get returns a verified entry. A damaged entry is deleted and reported as a
// miss so the caller refetches instead of serving bad bytes.
func (c *DiskCache) Get(key string) (DiskCacheEntry, bool) {
	entry, err := c.read(key)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(c.path(key))
			c.forget(key)
		}
		return DiskCacheEntry{}, false
	}
	c.touch(key)
	return entry, true
}

func (c *DiskCache) read(key string) (DiskCacheEntry, error) {
	file, err := os.Open(c.path(key))
	if err != nil {
		return DiskCacheEntry{}, err
	}
	defer func() { _ = file.Close() }()
	magic := make([]byte, len(diskCacheMagic))
	if _, err := io.ReadFull(file, magic); err != nil || string(magic) != diskCacheMagic {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry has an invalid header")
	}
	var headerLength uint32
	if err := binary.Read(file, binary.BigEndian, &headerLength); err != nil || headerLength == 0 || headerLength > diskCacheMaxHeader {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry has an invalid header length")
	}
	header := make([]byte, headerLength)
	if _, err := io.ReadFull(file, header); err != nil {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry is truncated")
	}
	var meta DiskCacheMeta
	if err := json.Unmarshal(header, &meta); err != nil {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry has an invalid header")
	}
	if meta.Size < 0 || meta.Size > diskCacheMaxEntrySize {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry has an invalid size")
	}
	data, err := io.ReadAll(io.LimitReader(file, meta.Size+1))
	if err != nil || int64(len(data)) != meta.Size {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry has an invalid body")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != meta.SHA256 {
		return DiskCacheEntry{}, errors.New("LaTeX resource cache entry failed its hash check")
	}
	return DiskCacheEntry{Meta: meta, Data: data}, nil
}

// Put writes one entry atomically. The caller decides whether a failure is
// worth logging; it never changes the response.
func (c *DiskCache) Put(key string, entry DiskCacheEntry) error {
	header, err := json.Marshal(entry.Meta)
	if err != nil {
		return err
	}
	if len(header) > diskCacheMaxHeader {
		return errors.New("LaTeX resource cache header is too large")
	}
	target := c.path(key)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(c.tmpDir, "entry-*")
	if err != nil {
		return err
	}
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
	}
	if _, err := temporary.WriteString(diskCacheMagic); err != nil {
		cleanup()
		return err
	}
	if err := binary.Write(temporary, binary.BigEndian, uint32(len(header))); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(header); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(entry.Data); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := os.Rename(temporary.Name(), target); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if dir, err := os.Open(filepath.Dir(target)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	c.record(key, int64(len(header))+int64(len(entry.Data))+int64(len(diskCacheMagic))+4)
	c.evict()
	return nil
}

func (c *DiskCache) record(key string, size int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous, ok := c.sizes[key]; ok {
		c.bytes -= previous
		if element, ok := c.items[key]; ok {
			c.lru.Remove(element)
		}
	}
	c.sizes[key] = size
	c.items[key] = c.lru.PushFront(key)
	c.bytes += size
}

func (c *DiskCache) forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if size, ok := c.sizes[key]; ok {
		c.bytes -= size
		delete(c.sizes, key)
	}
	if element, ok := c.items[key]; ok {
		c.lru.Remove(element)
		delete(c.items, key)
	}
}

func (c *DiskCache) touch(key string) {
	c.mu.Lock()
	if element, ok := c.items[key]; ok {
		c.lru.MoveToFront(element)
	}
	c.mu.Unlock()
	// Keep the on-disk order close to the in-memory LRU so a restart evicts the
	// right entries; a failed timestamp update only costs accuracy.
	now := time.Now()
	_ = os.Chtimes(c.path(key), now, now)
}

func (c *DiskCache) evict() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bytes <= c.maxBytes {
		return
	}
	target := int64(float64(c.maxBytes) * diskCacheEvictRatio)
	for c.bytes > target {
		oldest := c.lru.Back()
		if oldest == nil {
			return
		}
		key := oldest.Value.(string)
		c.lru.Remove(oldest)
		delete(c.items, key)
		c.bytes -= c.sizes[key]
		delete(c.sizes, key)
		_ = os.Remove(c.path(key))
	}
}

// Bytes reports the current on-disk footprint, for logs and tests.
func (c *DiskCache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// diskCacheKey is the cache identity of one exact lookup; a new profile
// invalidates every entry without any explicit flush.
func diskCacheKey(profile, kind, format, name string) string {
	digest := sha256.Sum256([]byte(profile + "\x00" + kind + "\x00" + format + "\x00" + name))
	return hex.EncodeToString(digest[:])
}
