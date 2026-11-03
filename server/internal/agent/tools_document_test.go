package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/agent"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

type documentReadFixture struct {
	db      *gorm.DB
	storage *testutil.MemStorage
	reader  tool.InvokableTool
	project store.UUID
	owner   store.UUID
	doc     store.Document
}

type documentReadResponse struct {
	ContentDigest   string `json:"content_digest"`
	VersionKind     string `json:"version_kind"`
	Unit            string `json:"unit"`
	TotalBytes      int    `json:"total_bytes"`
	MaxSegmentBytes int    `json:"max_segment_bytes"`
	Offset          int    `json:"offset"`
	NextOffset      int    `json:"next_offset"`
	Done            bool   `json:"done"`
	Text            string `json:"text"`
}

func newDocumentReadFixture(t *testing.T, text string) documentReadFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/read.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&store.User{}, &store.Project{}, &store.Document{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	owner, project := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: owner}, Username: "document-reader", PasswordHash: "test"}).Error)
	require.NoError(t, db.Create(&store.Project{BaseModel: store.BaseModel{ID: project}, Name: "Reader", OwnerID: owner}).Error)
	doc := store.Document{BaseModel: store.BaseModel{ID: store.MustNewUUID()}, Name: "large.txt", Type: "text/plain", Size: int64(len(text)), S3Key: "read-doc", ProjectID: project, OwnerID: owner}
	require.NoError(t, db.Create(&doc).Error)
	storage := testutil.NewMemStorage()
	require.NoError(t, storage.Put(context.Background(), doc.S3Key, []byte(text)))
	return documentReadFixture{db: db, storage: storage, reader: findDocumentReader(t, db, storage, project, owner), project: project, owner: owner, doc: doc}
}

func findDocumentReader(t *testing.T, db *gorm.DB, objectStorage storage.Storage, project, owner store.UUID) tool.InvokableTool {
	t.Helper()
	tools, err := agent.ProjectTools(db, objectStorage, project, owner, store.MustNewUUID(), 0)
	require.NoError(t, err)
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		require.NoError(t, err)
		if info.Name == "read_document" {
			reader, ok := candidate.(tool.InvokableTool)
			require.True(t, ok)
			return reader
		}
	}
	t.Fatal("read_document tool is missing")
	return nil
}

func readDocument(t *testing.T, reader tool.InvokableTool, arguments map[string]any) (documentReadResponse, error) {
	t.Helper()
	input, err := json.Marshal(arguments)
	require.NoError(t, err)
	output, err := reader.InvokableRun(context.Background(), string(input))
	if err != nil {
		require.Empty(t, output, "failed reads must not return partial content")
		return documentReadResponse{}, err
	}
	var response documentReadResponse
	require.NoError(t, json.Unmarshal([]byte(output), &response))
	return response, nil
}

func TestReadDocumentSupportsCompletePagedText(t *testing.T) {
	text := strings.Repeat("中文🙂abc\n", 6000) + "末尾证据"
	fixture := newDocumentReadFixture(t, text)
	metadata, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name})
	require.NoError(t, err)
	require.Empty(t, metadata.Text, "metadata discovery must not dump the whole document into model context")
	require.Equal(t, len(text), metadata.TotalBytes)
	require.Equal(t, "utf8-byte", metadata.Unit)
	require.Equal(t, "extracted-text-sha256", metadata.VersionKind)
	digest := sha256.Sum256([]byte(text))
	require.Equal(t, hex.EncodeToString(digest[:]), metadata.ContentDigest)

	var content strings.Builder
	for offset := 0; offset < metadata.TotalBytes; {
		segment, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "offset": offset, "max_bytes": 4096, "expected_digest": metadata.ContentDigest})
		require.NoError(t, err)
		require.Equal(t, offset, segment.Offset)
		require.Equal(t, metadata.ContentDigest, segment.ContentDigest)
		require.True(t, utf8.ValidString(segment.Text))
		require.LessOrEqual(t, len(segment.Text), 4096)
		require.Greater(t, segment.NextOffset, offset)
		require.Equal(t, segment.NextOffset == len(text), segment.Done)
		content.WriteString(segment.Text)
		offset = segment.NextOffset
	}
	require.Equal(t, text, content.String())
}

func TestReadDocumentRejectsChangedContinuation(t *testing.T) {
	fixture := newDocumentReadFixture(t, "first document text")
	first, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 5})
	require.NoError(t, err)
	require.NoError(t, fixture.storage.Put(context.Background(), "changed-doc", []byte("replacement text")))
	require.NoError(t, fixture.db.Model(&store.Document{}).Where("id = ?", fixture.doc.ID).Update("s3_key", "changed-doc").Error)
	_, err = readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "offset": first.NextOffset, "max_bytes": 5, "expected_digest": first.ContentDigest})
	require.ErrorContains(t, err, "changed")
}

func TestReadDocumentRejectsInvalidCursorAndRechecksAccess(t *testing.T) {
	fixture := newDocumentReadFixture(t, "甲乙丙")
	metadata, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name})
	require.NoError(t, err)
	for _, arguments := range []map[string]any{
		{"name": fixture.doc.Name, "max_bytes": -1},
		{"name": fixture.doc.Name, "offset": 1, "max_bytes": 3, "expected_digest": metadata.ContentDigest},
		{"name": fixture.doc.Name, "offset": 3, "max_bytes": 3},
		{"name": fixture.doc.Name, "offset": metadata.TotalBytes + 1, "max_bytes": 3, "expected_digest": metadata.ContentDigest},
	} {
		_, err := readDocument(t, fixture.reader, arguments)
		require.Error(t, err)
	}
	foreign := findDocumentReader(t, fixture.db, fixture.storage, fixture.project, store.MustNewUUID())
	_, err = readDocument(t, foreign, map[string]any{"name": fixture.doc.Name})
	require.Error(t, err)
	require.NoError(t, fixture.db.Delete(&fixture.doc).Error)
	_, err = readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 3, "expected_digest": metadata.ContentDigest})
	require.Error(t, err, "cached content must not bypass deleted-source checks")
}

type blockingDocumentStorage struct {
	storage.Storage
	started chan struct{}
	resume  chan struct{}
}

func (s *blockingDocumentStorage) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := s.Storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	close(s.started)
	select {
	case <-s.resume:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestReadDocumentDefaultHasNoSegmentLimit(t *testing.T) {
	text := strings.Repeat("中", (64*1024-1)/3) + "x"
	require.Len(t, text, 64*1024)
	fixture := newDocumentReadFixture(t, text)
	metadata, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name})
	require.NoError(t, err)
	require.Empty(t, metadata.Text)
	require.Equal(t, len(text), metadata.TotalBytes)
	require.Zero(t, metadata.MaxSegmentBytes, "zero means no configured segment limit")
	zero, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 0})
	require.NoError(t, err)
	require.Empty(t, zero.Text)
	require.Equal(t, len(text), zero.TotalBytes)

	segment, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 64 * 1024})
	require.NoError(t, err)
	require.Equal(t, text, segment.Text)
	require.True(t, utf8.ValidString(segment.Text))
	require.True(t, segment.Done)
}

func TestReadDocumentRejectsOversizedSegmentWithoutTruncatingSource(t *testing.T) {
	text := strings.Repeat("中文x", 2000)
	fixture := newDocumentReadFixture(t, text)
	tools, err := agent.ProjectTools(fixture.db, fixture.storage, fixture.project, fixture.owner, store.MustNewUUID(), 8192)
	require.NoError(t, err)
	for _, candidate := range tools {
		info, infoErr := candidate.Info(context.Background())
		require.NoError(t, infoErr)
		if info.Name != "read_document" {
			continue
		}
		reader, ok := candidate.(tool.InvokableTool)
		require.True(t, ok)
		_, err = readDocument(t, reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 8193})
		require.ErrorContains(t, err, "8192 bytes")
		metadata, err := readDocument(t, reader, map[string]any{"name": fixture.doc.Name})
		require.NoError(t, err)
		require.Equal(t, len(text), metadata.TotalBytes)
		require.Empty(t, metadata.Text)
		require.Equal(t, 8192, metadata.MaxSegmentBytes)
		var content strings.Builder
		for offset := 0; offset < metadata.TotalBytes; {
			segment, err := readDocument(t, reader, map[string]any{"name": fixture.doc.Name, "offset": offset, "max_bytes": 8192, "expected_digest": metadata.ContentDigest})
			require.NoError(t, err)
			require.True(t, utf8.ValidString(segment.Text))
			require.Greater(t, segment.NextOffset, offset)
			content.WriteString(segment.Text)
			offset = segment.NextOffset
		}
		require.Equal(t, text, content.String())
		return
	}
	t.Fatal("read_document tool is missing")
}

func TestReadDocumentRejectsDeletionDuringStorageRead(t *testing.T) {
	fixture := newDocumentReadFixture(t, "sensitive original")
	blocked := &blockingDocumentStorage{Storage: fixture.storage, started: make(chan struct{}), resume: make(chan struct{})}
	reader := findDocumentReader(t, fixture.db, blocked, fixture.project, fixture.owner)
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		text, err := reader.InvokableRun(context.Background(), `{"name":"large.txt","max_bytes":100}`)
		done <- result{text: text, err: err}
	}()
	select {
	case <-blocked.started:
	case <-time.After(5 * time.Second):
		close(blocked.resume)
		t.Fatal("read did not reach storage")
	}
	require.NoError(t, fixture.db.Delete(&fixture.doc).Error)
	close(blocked.resume)
	select {
	case got := <-done:
		require.Error(t, got.err)
		require.Empty(t, got.text, "deleted content must not be returned after the read")
	case <-time.After(5 * time.Second):
		t.Fatal("read did not finish")
	}
}

func TestReadDocumentDisambiguatesUploadedFiles(t *testing.T) {
	fixture := newDocumentReadFixture(t, "first document")
	second := store.Document{BaseModel: store.BaseModel{ID: store.MustNewUUID()}, Name: fixture.doc.Name, Type: "text/plain", Status: "uploaded", S3Key: "second-doc", ProjectID: fixture.project, OwnerID: fixture.owner}
	require.NoError(t, fixture.db.Create(&second).Error)
	require.NoError(t, fixture.storage.Put(context.Background(), second.S3Key, []byte("second document")))
	staged := store.Document{BaseModel: store.BaseModel{ID: store.MustNewUUID()}, Name: "pending.txt", Type: "text/plain", Status: "staged", S3Key: "staged-doc", ProjectID: fixture.project, OwnerID: fixture.owner}
	require.NoError(t, fixture.db.Create(&staged).Error)
	require.NoError(t, fixture.storage.Put(context.Background(), staged.S3Key, []byte("pending content")))

	_, err := readDocument(t, fixture.reader, map[string]any{"name": fixture.doc.Name, "max_bytes": 50})
	require.ErrorContains(t, err, "multiple")
	result, err := readDocument(t, fixture.reader, map[string]any{"document_id": second.ID.String(), "max_bytes": 50})
	require.NoError(t, err)
	require.Equal(t, "second document", result.Text)
	_, err = readDocument(t, fixture.reader, map[string]any{"document_id": staged.ID.String(), "max_bytes": 50})
	require.Error(t, err, "staged uploads are not available as confirmed sources")
}

func TestListDocumentsReportsEveryUploadedSource(t *testing.T) {
	fixture := newDocumentReadFixture(t, "text")
	const extra = 120
	rows := make([]store.Document, 0, extra)
	for i := 0; i < extra; i++ {
		rows = append(rows, store.Document{
			BaseModel: store.BaseModel{ID: store.MustNewUUID()},
			Name:      fmt.Sprintf("doc-%03d.txt", i), Type: "text/plain",
			Status: "uploaded", S3Key: fmt.Sprintf("extra-%d", i),
			ProjectID: fixture.project, OwnerID: fixture.owner,
		})
	}
	require.NoError(t, fixture.db.Create(&rows).Error)

	tools, err := agent.ProjectTools(fixture.db, fixture.storage, fixture.project, fixture.owner, store.MustNewUUID(), 0)
	require.NoError(t, err)
	var list tool.InvokableTool
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		require.NoError(t, err)
		if info.Name == "list_documents" {
			var ok bool
			list, ok = candidate.(tool.InvokableTool)
			require.True(t, ok)
		}
	}
	require.NotNil(t, list)
	out, err := list.InvokableRun(context.Background(), "{}")
	require.NoError(t, err)
	require.Contains(t, out, fmt.Sprintf("共 %d 篇", extra+1))
	require.Contains(t, out, "doc-119.txt")
}
