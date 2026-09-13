// Package rag — 资料库检索增强（Hybrid RAG + LLM Wiki）。
//
// 三路数据：
//   - 原件（documents）：入库时抽取全文 → 分块 → 嵌入 → 索引
//   - wiki 页（agent_wiki_pages）：子 agent 检索过程的沉淀，也被索引
//
// 检索为 hybrid：向量（chromem-go 余弦）+ BM25（纯 Go 内存倒排，中文
// bigram/英文分词）双路召回，RRF 融合。chunk 真相源落 rag_chunks 表，
// 向量库持久于 data/rag/（chromem gob），BM25 启动后按项目惰性重建。
package rag

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Embedder 文本 → 向量。实现可注入（HTTP / 测试替身）。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// HTTPEmbedder：OpenAI 兼容 POST {base}/embeddings。
type HTTPEmbedder struct {
	BaseURL string
	APIKey  string
	Model   string
	Client  *http.Client
}

func (e *HTTPEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{"model": e.Model, "input": texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(e.BaseURL, "/")+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	cl := e.Client
	if cl == nil {
		cl = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed: status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	vecs := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index >= 0 && d.Index < len(vecs) {
			vecs[d.Index] = d.Embedding
		}
	}
	for i, v := range vecs {
		if len(v) == 0 {
			return nil, fmt.Errorf("embed: missing vector %d", i)
		}
	}
	return vecs, nil
}

// HashEmbedder：确定性测试替身（sha1 → 64 维），语义无关但稳定，
// 用于验证索引/检索管线而非语义质量。
type HashEmbedder struct{ Dim int }

func (h HashEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	dim := h.Dim
	if dim == 0 {
		dim = 64
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v := make([]float32, dim)
		// 词袋哈希：同词命中同维度（让"查询与块共享词"产生相似性）
		for _, tok := range tokenize(t) {
			sum := sha1.Sum([]byte(tok))
			idx := binary.BigEndian.Uint32(sum[:4]) % uint32(dim)
			v[idx] += 1
		}
		out[i] = v
	}
	return out, nil
}
