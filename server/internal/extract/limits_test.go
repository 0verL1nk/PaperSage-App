package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// makePDF builds a valid, parseable PDF whose page streams are supplied by the test.
func makePDF(streams ...string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var kids strings.Builder
	for i, stream := range streams {
		page := 4 + 2*i
		fmt.Fprintf(&kids, "%d 0 R ", page)
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", page+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [ %s] /Count %d >>", kids.String(), len(streams))
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return buf.Bytes()
}

type cancelAtCheck struct {
	context.Context
	checks int
	at     int
}

func (c *cancelAtCheck) Err() error {
	c.checks++
	if c.checks >= c.at {
		return context.Canceled
	}
	return nil
}

func makeDOCX(t *testing.T, body string) ([]byte, string) {
	t.Helper()
	doc := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(doc)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), doc
}

func TestDOCXDecompressionLimit(t *testing.T) {
	data, doc := makeDOCX(t, `<w:p><w:r><w:t>`+strings.Repeat("a", 4000)+`</w:t></w:r></w:p>`)
	if len(data) >= len(doc) {
		t.Fatal("fixture must actually compress")
	}
	got, err := docxWithLimits(context.Background(), data, len(doc)-1, len(doc))
	if !errors.Is(err, ErrLimitExceeded) || got != "" {
		t.Fatalf("overflow = %q, %v", got, err)
	}
	got, err = docxWithLimits(context.Background(), data, len(doc), len(doc))
	if err != nil || got != strings.Repeat("a", 4000) {
		t.Fatalf("exact limit = %q, %v", got, err)
	}
}

func TestDOCXOutputLimitAcrossParagraphs(t *testing.T) {
	data, doc := makeDOCX(t, `<w:p><w:r><w:t>abc</w:t></w:r></w:p><w:p><w:r><w:t>def</w:t></w:r></w:p>`)
	got, err := docxWithLimits(context.Background(), data, len(doc), 6)
	if !errors.Is(err, ErrLimitExceeded) || got != "" {
		t.Fatalf("overflow = %q, %v", got, err)
	}
	got, err = docxWithLimits(context.Background(), data, len(doc), 7)
	if err != nil || got != "abc\ndef" {
		t.Fatalf("exact limit = %q, %v", got, err)
	}
}

func TestDOCXRunsWhitespaceAndBoundaries(t *testing.T) {
	data, _ := makeDOCX(t, `<w:p><w:r><w:t>anti</w:t></w:r><w:r><w:t>body</w:t></w:r><w:r><w:t xml:space="preserve"> binds </w:t><w:tab/><w:t>cell</w:t><w:br/><w:t>line</w:t></w:r></w:p><w:p><w:r><w:t xml:space="preserve"> next </w:t></w:r></w:p>`)
	got, err := Text(context.Background(), data, "paper.docx", "", 0)
	if err != nil || got != "antibody binds \tcell\nline\n next " {
		t.Fatalf("text = %q, %v", got, err)
	}
}

func TestDOCXIgnoresTabDefinitionsButKeepsRunTabs(t *testing.T) {
	data, _ := makeDOCX(t, `<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="720"/></w:tabs></w:pPr><w:r><w:t>hello</w:t><w:tab/><w:t>world</w:t><w:br/><w:t xml:space="preserve"> next </w:t></w:r></w:p>`)
	got, err := Text(context.Background(), data, "paper.docx", "", 0)
	if err != nil || got != "hello\tworld\n next " {
		t.Fatalf("text = %q, %v", got, err)
	}
}

func TestDOCXInheritedXMLSpace(t *testing.T) {
	data, _ := makeDOCX(t, `<w:p xml:space="preserve"><w:hyperlink><w:r><w:t> before </w:t><w:t xml:space="default"> after </w:t></w:r></w:hyperlink></w:p>`)
	got, err := Text(context.Background(), data, "paper.docx", "", 0)
	if err != nil || got != " before after" {
		t.Fatalf("text = %q, %v", got, err)
	}
}

func TestDOCXCancelDuringExtraction(t *testing.T) {
	data, doc := makeDOCX(t, `<w:p><w:r><w:t>`+strings.Repeat("a", 4000)+`</w:t></w:r></w:p>`)
	ctx := &cancelAtCheck{Context: context.Background(), at: 6}
	got, err := docxWithLimits(ctx, data, len(doc), len(doc))
	if !errors.Is(err, context.Canceled) || got != "" {
		t.Fatalf("canceled = %q, %v", got, err)
	}
}

func TestPDFCorruptPageOffsetReturnsError(t *testing.T) {
	data := makePDF("BT /F1 12 Tf (first page) Tj ET", "BT /F1 12 Tf (second page) Tj ET")
	xref := bytes.Index(data, []byte("xref\n"))
	if xref < 0 {
		t.Fatal("missing xref")
	}
	entries := xref + len("xref\n0 8\n")
	copy(data[entries+4*20:entries+4*20+10], "0000000001")
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Errorf("malformed PDF panicked: %v", recovered)
		}
	}()
	text, err := PDF(data)
	if err == nil || text != "" {
		t.Fatalf("expected error and empty text, got %q, %v", text, err)
	}
}

func TestPDFCancelBetweenPages(t *testing.T) {
	data := makePDF("BT /F1 12 Tf (first page) Tj ET", "BT /F1 12 Tf Tj ET")
	ctx := &cancelAtCheck{Context: context.Background(), at: 5}
	text, err := Text(ctx, data, "test.pdf", "application/pdf", 0)
	if !errors.Is(err, context.Canceled) || text != "" {
		t.Fatalf("expected cancellation before malformed second page, got %q, %v", text, err)
	}
}

func TestPDFPagesHaveSeparator(t *testing.T) {
	data := makePDF("BT /F1 12 Tf (first page) Tj ET", "BT /F1 12 Tf (second page) Tj ET")
	text, err := Text(context.Background(), data, "test.pdf", "application/pdf", 0)
	if err != nil || !strings.Contains(text, "first page\n\nsecond page") {
		t.Fatalf("page boundary missing: %q, %v", text, err)
	}
}

func TestPDFPageOutputLimit(t *testing.T) {
	data := makePDF("BT /F1 12 Tf (first page) Tj ET", "BT /F1 12 Tf (second page) Tj ET")
	full, err := pdfWithLimit(context.Background(), data, 1024)
	if err != nil || !strings.Contains(full, "second page") {
		t.Fatalf("full extraction = %q, %v", full, err)
	}
	exact, err := pdfWithLimit(context.Background(), data, len(full))
	if err != nil || exact != full {
		t.Fatalf("exact limit = %q, %v", exact, err)
	}
	got, err := pdfWithLimit(context.Background(), data, len(full)-1)
	if !errors.Is(err, ErrLimitExceeded) || got != "" {
		t.Fatalf("overflow = %q, %v", got, err)
	}
	badLaterPage := makePDF("BT /F1 12 Tf (first page) Tj ET", "BT /F1 12 Tf Tj ET")
	got, err = pdfWithLimit(context.Background(), badLaterPage, 2)
	if !errors.Is(err, ErrLimitExceeded) || got != "" {
		t.Fatalf("should stop before second page = %q, %v", got, err)
	}
}

type cancelReader struct {
	cancel context.CancelFunc
}

func (r cancelReader) Read(p []byte) (int, error) {
	r.cancel()
	p[0] = 'x'
	return 1, nil
}

func TestContextReaderStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &contextReader{ctx: ctx, reader: cancelReader{cancel: cancel}}
	buf := make([]byte, 1)
	if n, err := r.Read(buf); n != 1 || err != nil {
		t.Fatalf("first read = %d, %v", n, err)
	}
	if n, err := r.Read(buf); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %d, %v", n, err)
	}
}
