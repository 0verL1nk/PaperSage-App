package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/0verL1nk/GraduateProject/server/internal/extract"
	"github.com/0verL1nk/GraduateProject/server/internal/storage"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

type documentReadInput struct {
	Name           string `json:"name,omitempty" jsonschema:"description=文献文件名；存在同名文献时使用document_id"`
	DocumentID     string `json:"document_id,omitempty" jsonschema:"description=list_documents返回的文献ID，用于区分同名资料"`
	Offset         int    `json:"offset,omitempty" jsonschema:"description=UTF-8字节偏移，首次从0开始，续读使用next_offset"`
	MaxBytes       int    `json:"max_bytes,omitempty" jsonschema:"description=本次正文的字节预算；省略时只返回读取信息，不注入全文"`
	ExpectedDigest string `json:"expected_digest,omitempty" jsonschema:"description=续读必须传回content_digest，防止混读已变化的内容"`
}

type documentReadResult struct {
	Name            string `json:"name"`
	ContentDigest   string `json:"content_digest"`
	VersionKind     string `json:"version_kind"`
	Unit            string `json:"unit"`
	TotalBytes      int    `json:"total_bytes"`
	MaxSegmentBytes int    `json:"max_segment_bytes"`
	Offset          int    `json:"offset"`
	NextOffset      int    `json:"next_offset"`
	Done            bool   `json:"done"`
	Text            string `json:"text,omitempty"`
	ReadHint        string `json:"read_hint,omitempty"`
}

type documentTextReader struct {
	mu              sync.Mutex
	id              store.UUID
	key             string
	updatedAt       time.Time
	reader          *extract.TextReader
	digest          string
	size            int
	maxSegmentBytes int
}

func (r *documentTextReader) read(ctx context.Context, stg storage.Storage, doc store.Document, in documentReadInput) (*documentReadResult, error) {
	if in.Offset < 0 || in.MaxBytes < 0 {
		return nil, fmt.Errorf("offset and max_bytes must be non-negative")
	}
	if r.maxSegmentBytes > 0 && in.MaxBytes > r.maxSegmentBytes {
		return nil, fmt.Errorf("max_bytes exceeds configured per-read limit of %d bytes; continue with multiple reads", r.maxSegmentBytes)
	}
	if in.Offset > 0 && (in.MaxBytes == 0 || in.ExpectedDigest == "") {
		return nil, fmt.Errorf("continuation requires max_bytes and expected_digest from the previous read")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// shortcut: retain one document per Run; use revision-keyed caching when source versions are integrated.
	if r.reader == nil || r.id != doc.ID || r.key != doc.S3Key || !r.updatedAt.Equal(doc.UpdatedAt) {
		content, err := stg.Get(ctx, doc.S3Key)
		if err != nil {
			return nil, err
		}
		text, err := extract.Text(ctx, content, doc.Name, doc.Type, 0)
		if err != nil {
			return nil, err
		}
		reader, err := extract.NewTextReader(text)
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256([]byte(text))
		r.reader, r.digest, r.size = reader, hex.EncodeToString(digest[:]), len(text)
		r.id, r.key, r.updatedAt = doc.ID, doc.S3Key, doc.UpdatedAt
	}
	if in.ExpectedDigest != "" && in.ExpectedDigest != r.digest {
		return nil, fmt.Errorf("document changed; read metadata again before continuing")
	}
	result := &documentReadResult{
		Name: doc.Name, ContentDigest: r.digest, VersionKind: "extracted-text-sha256",
		Unit: "utf8-byte", TotalBytes: r.size, MaxSegmentBytes: r.maxSegmentBytes, Offset: in.Offset,
		NextOffset: in.Offset, Done: r.size == 0,
	}
	if in.MaxBytes == 0 {
		result.ReadHint = "指定正数max_bytes按任务需要读取正文；max_segment_bytes为0表示没有配置单次上限。续读传next_offset和content_digest。该摘要标识抽取文本快照，不是文档历史修订。"
		return result, nil
	}
	text, next, done, err := r.reader.ReadSegment(in.Offset, in.MaxBytes)
	if err != nil {
		return nil, err
	}
	result.Text, result.NextOffset, result.Done = text, next, done
	return result, nil
}
