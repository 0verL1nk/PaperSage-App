package web

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 构建产物填充前 assets/ 只有占位文件；填充后此处应包含 index.html
func TestDistContainsPlaceholder(t *testing.T) {
	dist, err := Dist()
	require.NoError(t, err)

	entries, err := fs.ReadDir(dist, ".")
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "assets 目录为空会导致 go:embed 编译失败")
}
