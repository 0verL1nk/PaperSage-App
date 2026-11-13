package foldersync

import "testing"

func TestSafeRelPath(t *testing.T) {
	valid := map[string]string{
		"main.tex":            "main.tex",
		"sections/intro.tex":  "sections/intro.tex",
		`sections\intro.tex`:  "sections/intro.tex",
		"images/./figure.png": "images/figure.png",
		"a/b/../c.tex":        "a/c.tex",
	}
	for in, want := range valid {
		got, err := SafeRelPath(in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}

	invalid := []string{"", "..", "../x.tex", "a/../../x.tex", "/etc/passwd", `C:\x.tex`, "a\x00b"}
	for _, in := range invalid {
		if _, err := SafeRelPath(in); err == nil {
			t.Errorf("%q: expected rejection", in)
		}
	}
}

func TestIsSupported(t *testing.T) {
	for _, name := range []string{"main.tex", "refs.bib", "class.cls", "pkg.sty", "style.bst", "fig.PNG", "a.jpg", "b.jpeg", "c.svg", "d.eps"} {
		if !IsSupported(name) {
			t.Errorf("%q should be supported", name)
		}
	}
	for _, name := range []string{"paper.pdf", "main.aux", "image.webp", "noext", "script.sh"} {
		if IsSupported(name) {
			t.Errorf("%q should be unsupported", name)
		}
	}
}

func TestIsIgnored(t *testing.T) {
	for _, name := range []string{"main.aux", "main.log", "main.pdf", "x.synctex.gz", "main.fdb_latexmk", "run.run.xml"} {
		if !IsIgnored(name) {
			t.Errorf("%q should be ignored", name)
		}
	}
	for _, name := range []string{"main.tex", "refs.bib", "figure.png"} {
		if IsIgnored(name) {
			t.Errorf("%q should not be ignored", name)
		}
	}
}
