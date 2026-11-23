package profile

import "embed"

// The profile artifacts are produced by .github/workflows/publish-latex-profile.yml
// from one immutable snapshot and verified by `latexprofile verify` before they
// are committed. Add "derived" here if a profile ever ships generated blobs.
//
//go:embed profile.json profile-manifest.json tlpdb.xz lookups.json.gz
var embedded embed.FS

// Embedded returns the profile compiled into the binary: the binary is the
// profile, so a rollback is just redeploying the previous binary.
func Embedded() (*Bundle, error) {
	return LoadFS(embedded)
}
