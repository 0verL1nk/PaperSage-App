package agent

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamWriterEmitsSequenceOnlyForEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	sw, ok := newStreamWriter(c)
	require.True(t, ok)

	sw.seq = 7
	sw.TextDelta("t1", "hello")
	sw.seq = 0
	sw.ping()
	sw.Finish()

	body := rec.Body.String()
	assert.Contains(t, body, "id: 7\ndata: ", "持久化事件必须带 SSE id")
	assert.NotContains(t, body, "id: 0", "start/finish/心跳不得带序号")
	assert.Contains(t, body, ": ping", "心跳仍是注释行")
}
