package deeplink

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalid = errors.New("deeplink: invalid link")
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	stages     = map[string]bool{"prepare": true, "writing": true, "review": true}
)

type Target struct {
	ProjectID string
	Stage     string
}

// Parse accepts papersage://project/<id>[/<stage>] and rejects everything else.
func Parse(raw string) (Target, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "papersage" || u.Host != "project" {
		return Target{}, ErrInvalid
	}
	trimmed := strings.Trim(u.Path, "/")
	if trimmed == "" {
		return Target{}, ErrInvalid
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 || !idPattern.MatchString(parts[0]) {
		return Target{}, ErrInvalid
	}
	target := Target{ProjectID: parts[0]}
	if len(parts) == 2 {
		stage := strings.ToLower(parts[1])
		if !stages[stage] {
			return Target{}, ErrInvalid
		}
		target.Stage = stage
	}
	return target, nil
}
