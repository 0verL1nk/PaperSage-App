// Package extract — 文档正文抽取（PDF / DOCX / TXT）。
// 供 agent 的 read_document 工具使用；预览域等后续可复用。
// 策略模式：按 content-type/扩展名分发到具体抽取器，新增格式只加
// 一个 case，不改调用方。
package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	pdflib "github.com/ledongthuc/pdf"
)

// Text 按类型抽取正文。maxBytes 截断（模型上下文保护，0 = 默认 48KB）。
func Text(_ context.Context, data []byte, filename, contentType string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 48 * 1024
	}
	lower := strings.ToLower(filename)

	var (
		text string
		err  error
	)
	switch {
	case strings.Contains(contentType, "pdf") || strings.HasSuffix(lower, ".pdf"):
		text, err = PDF(data)
	case strings.Contains(contentType, "wordprocessingml") || strings.HasSuffix(lower, ".docx"):
		text, err = DOCX(data)
	default:
		// text/*、txt/tex/bib/md 等直接按文本
		text = string(data)
	}
	if err != nil {
		return "", err
	}
	if len(text) > maxBytes {
		text = text[:maxBytes] + "\n…（内容过长已截断）"
	}
	return text, nil
}

// PDF：ledongthuc/pdf（纯 Go）抽取全文。
func PDF(data []byte) (string, error) {
	r, err := pdflib.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf open: %w", err)
	}
	pr, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf extract: %w", err)
	}
	out, err := io.ReadAll(io.LimitReader(pr, 10*1024*1024))
	if err != nil {
		return "", fmt.Errorf("pdf read: %w", err)
	}
	return string(out), nil
}

// DOCX：word/document.xml 的 <w:t> 文本拼接（stdlib，无三方依赖）。
func DOCX(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx open: %w", err)
	}
	var docXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			docXML, _ = io.ReadAll(rc)
			_ = rc.Close()
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("docx: word/document.xml not found")
	}
	dec := xml.NewDecoder(bytes.NewReader(docXML))
	var b strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
				b.WriteByte(' ')
			}
			if t.Name.Local == "p" {
				b.WriteByte('\n')
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}
