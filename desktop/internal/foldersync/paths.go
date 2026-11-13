package foldersync

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrUnsafePath  = errors.New("foldersync: unsafe path")
	ErrUnsupported = errors.New("foldersync: unsupported file")
)

const MaxFileSize = 5 << 20

var supportedExt = map[string]bool{
	".tex": true, ".bib": true, ".cls": true, ".sty": true, ".bst": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".eps": true,
}

var ignoredExt = map[string]bool{
	".aux": true, ".log": true, ".out": true, ".toc": true, ".bbl": true, ".blg": true,
	".fls": true, ".dvi": true, ".ps": true, ".pdf": true, ".nav": true, ".snm": true,
	".vrb": true, ".idx": true, ".ilg": true, ".ind": true, ".lof": true, ".lot": true,
	".fdb_latexmk": true, ".run.xml": true,
}

// SafeRelPath normalises a project-relative path and rejects anything that can
// escape the bound folder.
func SafeRelPath(p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) {
		return "", ErrUnsafePath
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "/") {
		return "", ErrUnsafePath
	}
	if len(p) >= 2 && p[1] == ':' {
		return "", ErrUnsafePath
	}
	cleaned := path.Clean(p)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrUnsafePath
	}
	return cleaned, nil
}

func IsSupported(name string) bool {
	return supportedExt[strings.ToLower(filepath.Ext(name))]
}

func IsIgnored(name string) bool {
	lower := strings.ToLower(name)
	if ignoredExt[filepath.Ext(lower)] {
		return true
	}
	if strings.HasSuffix(lower, ".run.xml") {
		return true
	}
	return strings.Contains(lower, ".synctex.")
}
