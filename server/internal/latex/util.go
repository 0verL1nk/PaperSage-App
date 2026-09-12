package latex

import (
	"io"
	"mime/multipart"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

func nowUTC() *time.Time {
	t := time.Now().UTC()
	return &t
}

func readAll(c *gin.Context, fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func queryEscape(s string) string { return url.QueryEscape(s) }
