package latex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// 契约来源：backend/services/latex/clsi_client.py
// CLSI 状态常量
const (
	StatusSuccess  = "success"
	StatusFailure  = "failure"
	StatusConflict = "conflict"
	StatusTimedout = "timedout"
	StatusError    = "error"
)

// Resource 是 CLSI 编译请求中的单个资源（content/url 二选一，exclude_none 语义）。
type Resource struct {
	Path    string  `json:"path"`
	Content *string `json:"content,omitempty"`
	URL     *string `json:"url,omitempty"`
}

// OutputFile 是 CLSI 响应中的输出文件元数据。
type OutputFile struct {
	Path  string `json:"path"`
	URL   string `json:"url"`
	Type  string `json:"type"`
	Build string `json:"build"`
	Size  int    `json:"size"`
}

// CompileResult 是解析后的 CLSI 编译结果。
type CompileResult struct {
	Status          string       `json:"status"`
	BuildID         *string      `json:"buildId"`
	OutputFiles     []OutputFile `json:"outputFiles"`
	Error           *string      `json:"error"`
	OutputURLPrefix string       `json:"-"`
	Logs            *string      `json:"-"`
}

// SyncPosition 是 SyncTeX 定位。
type SyncPosition struct {
	File   string   `json:"file"`
	Line   *int     `json:"line"`
	Column *int     `json:"column"`
	Page   *int     `json:"page"`
	H      *float64 `json:"h"`
	V      *float64 `json:"v"`
	Width  *float64 `json:"width"`
	Height *float64 `json:"height"`
}

// ClsiClient 抽象 CLSI HTTP 服务（注入 fake 便于单元测试）。
type ClsiClient interface {
	Compile(ctx context.Context, projectID string, resources []Resource, rootResourcePath, compiler, syncType string, syncState *string) (*CompileResult, error)
	StopCompile(ctx context.Context, projectID string) bool
	ClearCache(ctx context.Context, projectID string) bool
	Wordcount(ctx context.Context, projectID, file string, image *string) (map[string]any, error)
	SyncFromCode(ctx context.Context, projectID, file string, line, column int) ([]SyncPosition, error)
	SyncFromPDF(ctx context.Context, projectID string, page int, h, v float64) ([]SyncPosition, error)
	GetOutputZip(ctx context.Context, projectID, buildID string) (int, []byte, error)
	GetOutputFile(ctx context.Context, projectID, buildID, filePath string) (int, []byte, string, error)
}

// HTTPClsiClient 是真实实现。
type HTTPClsiClient struct {
	BaseURL string
	Client  *http.Client
}

// NewClsiClient 构建客户端（timeout 秒）。
func NewClsiClient(baseURL string, timeoutSeconds int) *HTTPClsiClient {
	return &HTTPClsiClient{
		BaseURL: baseURL,
		Client:  &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

// Compile 提交编译请求。
func (c *HTTPClsiClient) Compile(ctx context.Context, projectID string, resources []Resource, rootResourcePath, compiler, syncType string, syncState *string) (*CompileResult, error) {
	options := map[string]any{"compiler": compiler, "syncType": syncType}
	if syncState != nil {
		options["syncState"] = *syncState
	}
	body := map[string]any{
		"compile": map[string]any{
			"options":          options,
			"rootResourcePath": rootResourcePath,
			"resources":        resources,
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/project/%s/compile", projectID), b, "application/json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Compile struct {
			Status      string       `json:"status"`
			BuildID     *string      `json:"buildId"`
			OutputFiles []OutputFile `json:"outputFiles"`
			Error       *string      `json:"error"`
		} `json:"compile"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	status := parsed.Compile.Status
	if status == "" {
		status = StatusError
	}
	return &CompileResult{
		Status: status, BuildID: parsed.Compile.BuildID,
		OutputFiles: parsed.Compile.OutputFiles, Error: parsed.Compile.Error,
	}, nil
}

// StopCompile 停止项目编译（204 视为成功）。
func (c *HTTPClsiClient) StopCompile(ctx context.Context, projectID string) bool {
	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/project/%s/compile/stop", projectID), nil, "")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusNoContent
}

// ClearCache 清理编译缓存。
func (c *HTTPClsiClient) ClearCache(ctx context.Context, projectID string) bool {
	resp, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/project/%s", projectID), nil, "")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusNoContent
}

// Wordcount 查询字数统计。
func (c *HTTPClsiClient) Wordcount(ctx context.Context, projectID, file string, image *string) (map[string]any, error) {
	q := url.Values{"file": {file}}
	if image != nil {
		q.Set("image", *image)
	}
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/project/%s/wordcount?%s", projectID, q.Encode()), nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("clsi wordcount: status %d", resp.StatusCode)
	}
	var data struct {
		Texcount map[string]any `json:"texcount"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Texcount, nil
}

// SyncFromCode SyncTeX 源码→PDF。
func (c *HTTPClsiClient) SyncFromCode(ctx context.Context, projectID, file string, line, column int) ([]SyncPosition, error) {
	q := url.Values{"file": {file}, "line": {strconv.Itoa(line)}, "column": {strconv.Itoa(column)}}
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/project/%s/sync/code?%s", projectID, q.Encode()), nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("clsi sync/code: status %d", resp.StatusCode)
	}
	var data struct {
		PDF []SyncPosition `json:"pdf"`
	}
	return data.PDF, json.NewDecoder(resp.Body).Decode(&data)
}

// SyncFromPDF SyncTeX PDF→源码。
func (c *HTTPClsiClient) SyncFromPDF(ctx context.Context, projectID string, page int, h, v float64) ([]SyncPosition, error) {
	q := url.Values{"page": {strconv.Itoa(page)}, "h": {formatFloat(h)}, "v": {formatFloat(v)}}
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/project/%s/sync/pdf?%s", projectID, q.Encode()), nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("clsi sync/pdf: status %d", resp.StatusCode)
	}
	var data struct {
		Code []SyncPosition `json:"code"`
	}
	return data.Code, json.NewDecoder(resp.Body).Decode(&data)
}

// GetOutputZip 拉取构建输出 zip。
func (c *HTTPClsiClient) GetOutputZip(ctx context.Context, projectID, buildID string) (int, []byte, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/project/%s/build/%s/output/output.zip", projectID, buildID), nil, "")
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

// GetOutputFile 拉取单个输出文件（状态码、字节、content-type）。
func (c *HTTPClsiClient) GetOutputFile(ctx context.Context, projectID, buildID, filePath string) (int, []byte, string, error) {
	resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/project/%s/build/%s/output/%s", projectID, buildID, filePath), nil, "")
	if err != nil {
		return 0, nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header.Get("Content-Type"), nil
}

func (c *HTTPClsiClient) do(ctx context.Context, method, path string, body []byte, contentType string) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.Client.Do(req)
}

// formatFloat 保持与 Python str(float) 语义近似（无多余尾零）。
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
