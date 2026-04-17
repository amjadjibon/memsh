package git

import (
	"net/url"
	"strings"

	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// envLookup abstracts shell environment access for auth resolution.
type envLookup func(string) string

func firstNonEmpty(get envLookup, keys ...string) string {
	if get == nil {
		return ""
	}
	for _, k := range keys {
		if v := strings.TrimSpace(get(k)); v != "" {
			return v
		}
	}
	return ""
}

// authFromEnv returns HTTP basic auth for git remotes when credentials are
// provided via environment variables; otherwise it returns nil.
//
// Supported variables:
//   - username/password: GIT_HTTP_USERNAME + GIT_HTTP_PASSWORD
//   - fallback username/password: GIT_USERNAME + GIT_PASSWORD
//   - token: GIT_HTTP_TOKEN, GIT_TOKEN, GITHUB_TOKEN, GITLAB_TOKEN, BITBUCKET_TOKEN
//   - token username override: GIT_HTTP_TOKEN_USERNAME, GIT_TOKEN_USERNAME
func authFromEnv(get envLookup, rawURL string) *githttp.BasicAuth {
	if get == nil {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil
	}

	user := firstNonEmpty(get, "GIT_HTTP_USERNAME", "GIT_USERNAME")
	pass := firstNonEmpty(get, "GIT_HTTP_PASSWORD", "GIT_PASSWORD")
	token := firstNonEmpty(get, "GIT_HTTP_TOKEN", "GIT_TOKEN", "GITHUB_TOKEN", "GITLAB_TOKEN", "BITBUCKET_TOKEN")
	if pass == "" && token != "" {
		pass = token
	}
	if pass == "" {
		return nil
	}

	if user == "" {
		user = firstNonEmpty(get, "GIT_HTTP_TOKEN_USERNAME", "GIT_TOKEN_USERNAME")
		if user == "" {
			host := strings.ToLower(u.Hostname())
			switch {
			case strings.Contains(host, "github.com"):
				user = "x-access-token"
			case strings.Contains(host, "gitlab"):
				user = "oauth2"
			default:
				user = "git"
			}
		}
	}
	return &githttp.BasicAuth{Username: user, Password: pass}
}
