package extract_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/extract"
)

// 用 e2e fixtures 验证真实文件抽取
func TestExtractPDF(t *testing.T) {
	data, err := os.ReadFile("../../../frontend/tests/e2e/fixtures/extract.pdf")
	if err != nil {
		t.Skip("fixture 不存在")
	}
	text, err := extract.Text(context.Background(), data, "sample.pdf", "application/pdf", 10000)
	require.NoError(t, err)
	assert.NotEmpty(t, text, "PDF 应抽出文本")
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
}
