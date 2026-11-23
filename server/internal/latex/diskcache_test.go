package latex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testDiskEntry(name string, size int) DiskCacheEntry {
	pattern := []byte(name)
	data := bytes.Repeat(pattern, size/len(pattern)+1)[:size]
	digest := sha256.Sum256(data)
	return DiskCacheEntry{
		Meta: DiskCacheMeta{
			Profile:    "tl-test",
			SourcePath: "texmf-dist/tex/latex/" + name + "/" + name + ".sty",
			Package:    name,
			SHA256:     hex.EncodeToString(digest[:]),
			Size:       int64(len(data)),
			MediaType:  "application/octet-stream",
		},
		Data: data,
	}
}

func TestDiskCacheRoundTrip(t *testing.T) {
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	key := diskCacheKey("tl-test", "xetex", "26", "kvoptions.sty")
	entry := testDiskEntry("kvoptions", 128)
	require.NoError(t, cache.Put(key, entry))

	loaded, ok := cache.Get(key)
	require.True(t, ok)
	require.Equal(t, entry.Meta, loaded.Meta)
	require.Equal(t, entry.Data, loaded.Data)

	entries, err := os.ReadDir(cache.tmpDir)
	require.NoError(t, err)
	require.Empty(t, entries, "a completed write must not leave temp files behind")
}

func TestDiskCacheMissAndSelfHealing(t *testing.T) {
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	key := diskCacheKey("tl-test", "xetex", "26", "geometry.sty")
	_, ok := cache.Get(key)
	require.False(t, ok)

	require.NoError(t, cache.Put(key, testDiskEntry("geometry", 64)))
	target := cache.path(key)
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xff
	require.NoError(t, os.WriteFile(target, raw, 0o600))
	_, ok = cache.Get(key)
	require.False(t, ok, "a corrupted entry must be a miss")
	_, err = os.Stat(target)
	require.True(t, os.IsNotExist(err), "a corrupted entry must be deleted so the next request refetches")
}

func TestDiskCacheRejectsTruncatedEntries(t *testing.T) {
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	key := diskCacheKey("tl-test", "xetex", "26", "layouts.sty")
	require.NoError(t, cache.Put(key, testDiskEntry("layouts", 256)))
	raw, err := os.ReadFile(cache.path(key))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cache.path(key), raw[:len(raw)/2], 0o600))
	_, ok := cache.Get(key)
	require.False(t, ok)
}

func TestDiskCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache, err := NewDiskCache(t.TempDir(), 1000)
	require.NoError(t, err)
	keys := make([]string, 0, 6)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		key := diskCacheKey("tl-test", "xetex", "26", name+".sty")
		keys = append(keys, key)
		require.NoError(t, cache.Put(key, testDiskEntry(name, 200)))
	}
	require.LessOrEqual(t, cache.Bytes(), int64(1000))
	_, first := cache.Get(keys[0])
	_, last := cache.Get(keys[len(keys)-1])
	require.False(t, first, "the oldest entry must be evicted first")
	require.True(t, last, "the newest entry must survive eviction")
}

func TestDiskCacheRebuildsAccountingOnRestart(t *testing.T) {
	root := t.TempDir()
	cache, err := NewDiskCache(root, 1<<20)
	require.NoError(t, err)
	key := diskCacheKey("tl-test", "xetex", "26", "ccaption.sty")
	require.NoError(t, cache.Put(key, testDiskEntry("ccaption", 300)))
	footprint := cache.Bytes()
	require.Positive(t, footprint)

	reopened, err := NewDiskCache(root, 1<<20)
	require.NoError(t, err)
	require.Equal(t, footprint, reopened.Bytes())
	entry, ok := reopened.Get(key)
	require.True(t, ok)
	require.Len(t, entry.Data, 300)
}

func TestDiskCacheRemovesStaleTempFilesOnly(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tmp"), 0o700))
	stale := filepath.Join(root, "tmp", "entry-stale")
	recent := filepath.Join(root, "tmp", "entry-recent")
	require.NoError(t, os.WriteFile(stale, []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(recent, []byte("x"), 0o600))
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))

	_, err := NewDiskCache(root, 1<<20)
	require.NoError(t, err)
	_, err = os.Stat(stale)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(recent)
	require.NoError(t, err)
}

func TestDiskCacheLoadOrFillSharesOneFetch(t *testing.T) {
	cache, err := NewDiskCache(t.TempDir(), 1<<20)
	require.NoError(t, err)
	key := diskCacheKey("tl-test", "xetex", "26", "gbt7714.sty")
	release := make(chan struct{})
	var calls atomic.Int32
	fill := func() (DiskCacheEntry, error) {
		calls.Add(1)
		<-release
		return testDiskEntry("gbt7714", 64), nil
	}

	var wait sync.WaitGroup
	results := make([]DiskCacheEntry, 8)
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			entry, err := cache.LoadOrFill(key, fill)
			require.NoError(t, err)
			results[index] = entry
		}(index)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wait.Wait()
	require.Equal(t, int32(1), calls.Load(), "concurrent requests for one lookup must share a single fetch")
	for _, entry := range results {
		require.Equal(t, results[0].Data, entry.Data)
	}
}

func TestSlashPathDropsWindowsDrive(t *testing.T) {
	// Windows resolves the guard's deploy paths to a drive-qualified form; the
	// comparison must then still match. Kept platform-neutral so both CI legs
	// exercise the same rule.
	require.Equal(t, "/opt/papersage/releases/v1/data", slashPath("C:/opt/papersage/releases/v1/data"))
	require.Equal(t, "/data/latex-resources", slashPath("D:/data/latex-resources"))
}

func TestValidateCacheDirRejectsVolatileLocations(t *testing.T) {
	require.Error(t, ValidateCacheDir(filepath.Join(os.TempDir(), "latex-resources")))
	require.Error(t, ValidateCacheDir("/opt/papersage/releases/v1/data"))
	require.NoError(t, ValidateCacheDir("./data/latex-resources"))
}
