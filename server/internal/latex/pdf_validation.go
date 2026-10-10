package latex

import (
	"bytes"

	pdflib "github.com/ledongthuc/pdf"
)

// maxBrowserPDFPages bounds page-tree traversal for an uploaded browser result.
// The byte limit is enforced by the upload handler before this parser is called.
const maxBrowserPDFPages = 100_000

// isStructurallyValidBrowserPDF uses the PDF parser already used by the server
// rather than trusting a client supplied MIME type or a %PDF- prefix. It checks
// the cross-reference/trailer, catalog, and first page without extracting page
// content streams, which keeps validation work proportional to PDF structure.
// The parser may panic on malformed object graphs, so treat that as rejection.
func isStructurallyValidBrowserPDF(data []byte) (valid bool) {
	defer func() {
		if recover() != nil {
			valid = false
		}
	}()

	if len(data) < 100 || len(data) > maxBrowserPDFBytes {
		return false
	}

	reader, err := pdflib.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}

	root := reader.Trailer().Key("Root")
	if root.Kind() != pdflib.Dict || root.Key("Type").Name() != "Catalog" {
		return false
	}
	pages := root.Key("Pages")
	if pages.Kind() != pdflib.Dict || pages.Key("Type").Name() != "Pages" {
		return false
	}
	pageCount := reader.NumPage()
	if pageCount < 1 || pageCount > maxBrowserPDFPages {
		return false
	}
	firstPage := reader.Page(1).V
	return firstPage.Kind() == pdflib.Dict && firstPage.Key("Type").Name() == "Page"
}
