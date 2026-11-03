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
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	pdflib "github.com/ledongthuc/pdf"
)

// ErrLimitExceeded indicates that extraction exceeded an explicit or safety byte limit.
var ErrLimitExceeded = errors.New("extraction byte limit exceeded")

// Text 按类型抽取正文。maxBytes 为输出字节上限，0 表示无调用方上限。
func Text(ctx context.Context, data []byte, filename, contentType string, maxBytes int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if maxBytes < 0 {
		return "", fmt.Errorf("maxBytes must be non-negative")
	}
	lower := strings.ToLower(filename)

	var (
		text string
		err  error
	)
	switch {
	case strings.Contains(contentType, "pdf") || strings.HasSuffix(lower, ".pdf"):
		text, err = pdf(ctx, data)
	case strings.Contains(contentType, "wordprocessingml") || strings.HasSuffix(lower, ".docx"):
		text, err = docx(ctx, data)
	default:
		// text/*、txt/tex/bib/md 等直接按文本
		text = string(data)
	}
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if maxBytes > 0 && len(text) > maxBytes {
		return "", fmt.Errorf("text exceeds %d bytes: %w", maxBytes, ErrLimitExceeded)
	}
	return text, nil
}

// TextReader holds immutable, UTF-8-validated text for repeated segment reads.
type TextReader struct {
	text string
}

// NewTextReader validates UTF-8 once before any segment reads.
func NewTextReader(text string) (*TextReader, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("text is not valid UTF-8")
	}
	return &TextReader{text: text}, nil
}

// ReadSegment returns a UTF-8 segment and the next byte offset; maxBytes is a byte budget.
// It rejects non-boundary offsets and budgets too small for the next rune.
func (r *TextReader) ReadSegment(offset, maxBytes int) (segment string, next int, done bool, err error) {
	text := r.text
	if offset < 0 || offset > len(text) || (offset < len(text) && !utf8.RuneStart(text[offset])) {
		return "", 0, false, errors.New("invalid UTF-8 byte offset")
	}
	if maxBytes <= 0 {
		return "", 0, false, errors.New("maxBytes must be positive")
	}
	end := len(text)
	if maxBytes < end-offset {
		end = offset + maxBytes
		for end > offset && !utf8.RuneStart(text[end]) {
			end--
		}
		if end == offset {
			return "", 0, false, errors.New("maxBytes too small for next UTF-8 character")
		}
	}
	return text[offset:end], end, end == len(text), nil
}

// PDF：ledongthuc/pdf（纯 Go）抽取全文。
func PDF(data []byte) (string, error) {
	return pdf(context.Background(), data)
}

func pdf(ctx context.Context, data []byte) (string, error) {
	return pdfWithLimit(ctx, data, 10*1024*1024)
}

// The limit bounds output, not PDF decoder memory; cancellation is cooperative between pages.
func pdfWithLimit(ctx context.Context, data []byte, limit int) (result string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ""
			err = fmt.Errorf("pdf parse panic: %v", recovered)
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r, err := pdflib.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf open: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var out strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, err := r.Page(i).GetPlainText(nil)
		if cancelErr := ctx.Err(); cancelErr != nil {
			return "", cancelErr
		}
		if err != nil {
			return "", fmt.Errorf("pdf extract page %d: %w", i, err)
		}
		separator := ""
		if i > 1 {
			separator = "\n"
		}
		if len(separator) > limit-out.Len() || len(text) > limit-out.Len()-len(separator) {
			return "", fmt.Errorf("pdf output exceeds %d bytes: %w", limit, ErrLimitExceeded)
		}
		out.WriteString(separator)
		out.WriteString(text)
	}
	return out.String(), nil
}

// DOCX extracts derived text, not the original document bytes.
func DOCX(data []byte) (string, error) {
	return docx(context.Background(), data)
}

func docx(ctx context.Context, data []byte) (string, error) {
	// Uploads allow 50 MiB; permit twice that for XML markup while bounding extracted text.
	return docxWithLimits(ctx, data, 100*1024*1024, 50*1024*1024)
}

func docxWithLimits(ctx context.Context, data []byte, xmlLimit, outputLimit int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx open: %w", err)
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("docx open document: %w", err)
		}
		defer func() { _ = rc.Close() }()
		limited := &io.LimitedReader{R: &contextReader{ctx: ctx, reader: rc}, N: int64(xmlLimit) + 1}
		dec := xml.NewDecoder(limited)
		var out strings.Builder
		var spaceStack []bool
		paragraphs, runs, runProperties := 0, 0, 0
		isWord := func(name xml.Name) bool {
			return name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" || name.Space == "http://purl.oclc.org/ooxml/wordprocessingml/main"
		}
		appendText := func(s string) error {
			if len(s) > outputLimit-out.Len() {
				return fmt.Errorf("docx output exceeds %d bytes: %w", outputLimit, ErrLimitExceeded)
			}
			out.WriteString(s)
			return nil
		}
		for {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			tok, err := dec.Token()
			if limited.N == 0 {
				return "", fmt.Errorf("docx XML exceeds %d bytes: %w", xmlLimit, ErrLimitExceeded)
			}
			if err == io.EOF {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				return out.String(), nil
			}
			if err != nil {
				return "", fmt.Errorf("docx parse: %w", err)
			}
			if end, ok := tok.(xml.EndElement); ok {
				spaceStack = spaceStack[:len(spaceStack)-1]
				if isWord(end.Name) {
					switch end.Name.Local {
					case "r":
						runs--
					case "rPr":
						if runProperties > 0 {
							runProperties--
						}
					}
				}
				continue
			}
			start, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			preserve := len(spaceStack) > 0 && spaceStack[len(spaceStack)-1]
			for _, attr := range start.Attr {
				if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && attr.Name.Local == "space" {
					preserve = attr.Value == "preserve"
				}
			}
			spaceStack = append(spaceStack, preserve)
			if !isWord(start.Name) {
				continue
			}
			var text string
			switch start.Name.Local {
			case "r":
				runs++
			case "rPr":
				if runs > 0 {
					runProperties++
				}
			case "p":
				if paragraphs > 0 {
					text = "\n"
				}
				paragraphs++
			case "tab":
				if runs > 0 && runProperties == 0 {
					text = "\t"
				}
			case "br", "cr":
				if runs > 0 && runProperties == 0 {
					text = "\n"
				}
			case "t":
				err := dec.DecodeElement(&text, &start)
				spaceStack = spaceStack[:len(spaceStack)-1]
				if limited.N == 0 {
					return "", fmt.Errorf("docx XML exceeds %d bytes: %w", xmlLimit, ErrLimitExceeded)
				}
				if err != nil {
					return "", fmt.Errorf("docx text: %w", err)
				}
				if !preserve {
					text = strings.Trim(text, " \t\r\n")
				}
			}
			if err := appendText(text); err != nil {
				return "", err
			}
		}
	}
	return "", fmt.Errorf("docx: word/document.xml not found")
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
