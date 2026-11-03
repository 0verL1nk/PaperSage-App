package extract_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/extract"
)

func TestTextUnlimitedPreservesTail(t *testing.T) {
	want := strings.Repeat("a", 48*1024) + "结尾 evidence 😀"
	got, err := extract.Text(context.Background(), []byte(want), "paper.txt", "text/plain", 0)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.True(t, utf8.ValidString(got))
}

func TestTextExplicitLimit(t *testing.T) {
	const text = "a中文😀z"
	for _, limit := range []int{1, 2, len(text) - 1} {
		got, err := extract.Text(context.Background(), []byte(text), "paper.txt", "text/plain", limit)
		assert.ErrorIs(t, err, extract.ErrLimitExceeded, "limit %d", limit)
		assert.Empty(t, got)
	}
	got, err := extract.Text(context.Background(), []byte(text), "paper.txt", "text/plain", len(text))
	require.NoError(t, err)
	assert.Equal(t, text, got)
}

func TestTextCanceledBeforeExtraction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, filename := range []string{"file.txt", "invalid.pdf", "invalid.docx"} {
		got, err := extract.Text(ctx, []byte("not a document"), filename, "", 0)
		assert.True(t, errors.Is(err, context.Canceled), "%s: %v", filename, err)
		assert.Empty(t, got)
	}
}

func TestDOCXRejectsCorruptArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<document><p><t>evidence</t></p></document>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	data := buf.Bytes()
	central := bytes.LastIndex(data, []byte{'P', 'K', 1, 2})
	require.GreaterOrEqual(t, central, 0)
	data[central+16] ^= 0xff // corrupt the ZIP entry's central-directory checksum
	got, err := extract.Text(context.Background(), data, "paper.docx", "", 0)
	assert.Error(t, err)
	assert.Empty(t, got)
}

func TestReadSegmentUTF8RoundTrip(t *testing.T) {
	want := strings.Repeat("English 中文😀", 100000) + "末尾 evidence"
	reader, err := extract.NewTextReader(want)
	require.NoError(t, err)
	var chunks strings.Builder
	for offset := 0; ; {
		chunk, next, done, err := reader.ReadSegment(offset, 4097)
		require.NoError(t, err)
		require.True(t, utf8.ValidString(chunk))
		require.LessOrEqual(t, len(chunk), 4097)
		require.Equal(t, offset+len(chunk), next)
		chunks.WriteString(chunk)
		if done {
			require.Equal(t, len(want), next)
			break
		}
		require.Greater(t, next, offset)
		offset = next
	}
	assert.Equal(t, want, chunks.String())
}

func TestReadSegmentRejectsInvalidArguments(t *testing.T) {
	const text = "A中😀Z"
	reader, err := extract.NewTextReader(text)
	require.NoError(t, err)
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct{ offset, limit int }{{-1, 4}, {len(text) + 1, 4}, {maxInt, 4}, {2, 4}, {5, 4}, {0, 0}, {0, -1}, {1, 2}} {
		_, _, _, err := reader.ReadSegment(tc.offset, tc.limit)
		assert.Error(t, err, "offset=%d limit=%d", tc.offset, tc.limit)
	}
	_, err = extract.NewTextReader("\xff")
	assert.Error(t, err)
	chunk, next, done, err := reader.ReadSegment(len(text), maxInt)
	require.NoError(t, err)
	assert.Empty(t, chunk)
	assert.Equal(t, len(text), next)
	assert.True(t, done)
	chunk, next, done, err = reader.ReadSegment(1, maxInt)
	require.NoError(t, err)
	assert.Equal(t, text[1:], chunk)
	assert.Equal(t, len(text), next)
	assert.True(t, done)
}

func BenchmarkReadSegmentLargeText(b *testing.B) {
	text := strings.Repeat("English 中文😀", 100000)
	reader, err := extract.NewTextReader(text)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for offset := 0; ; {
			_, next, done, err := reader.ReadSegment(offset, 4096)
			if err != nil {
				b.Fatal(err)
			}
			if done {
				break
			}
			offset = next
		}
	}
}

// 用 e2e fixtures 验证真实文件抽取
func TestExtractPDF(t *testing.T) {
	data, err := os.ReadFile("../../../frontend/tests/e2e/fixtures/extract.pdf")
	if err != nil {
		t.Skip("fixture 不存在")
	}
	text, err := extract.Text(context.Background(), data, "sample.pdf", "application/pdf", 10000)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "PDF 应抽出文本")
	limited, err := extract.Text(context.Background(), data, "sample.pdf", "application/pdf", 1)
	assert.ErrorIs(t, err, extract.ErrLimitExceeded)
	assert.Empty(t, limited)
}

func TestExtractDOCX(t *testing.T) {
	data, err := os.ReadFile("../../../frontend/tests/e2e/fixtures/rich.docx")
	if err != nil {
		t.Skip("fixture 不存在")
	}
	text, err := extract.Text(context.Background(), data, "rich.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", 10000)
	require.NoError(t, err)
	assert.Contains(t, text, "深度学习", "DOCX 应抽出中文正文")
	limited, err := extract.Text(context.Background(), data, "rich.docx", "", 1)
	assert.ErrorIs(t, err, extract.ErrLimitExceeded)
	assert.Empty(t, limited)
}
