package git

import (
	"testing"
)

func TestAuthFromEnv(t *testing.T) {
	t.Run("returns nil for non-http URLs", func(t *testing.T) {
		env := map[string]string{
			"GIT_TOKEN": "secret",
		}
		auth := authFromEnv(func(k string) string { return env[k] }, "git@github.com:org/repo.git")
		if auth != nil {
			t.Fatalf("expected nil auth for ssh URL")
		}
	})

	t.Run("uses explicit username and password", func(t *testing.T) {
		env := map[string]string{
			"GIT_HTTP_USERNAME": "alice",
			"GIT_HTTP_PASSWORD": "pass",
		}
		auth := authFromEnv(func(k string) string { return env[k] }, "https://example.com/repo.git")
		if auth == nil {
			t.Fatalf("expected auth")
		}
		if auth.Username != "alice" || auth.Password != "pass" {
			t.Fatalf("unexpected auth: %#v", auth)
		}
	})

	t.Run("uses token with github default username", func(t *testing.T) {
		env := map[string]string{
			"GITHUB_TOKEN": "ghp_123",
		}
		auth := authFromEnv(func(k string) string { return env[k] }, "https://github.com/org/repo.git")
		if auth == nil {
			t.Fatalf("expected auth")
		}
		if auth.Username != "x-access-token" {
			t.Fatalf("expected x-access-token username, got %q", auth.Username)
		}
		if auth.Password != "ghp_123" {
			t.Fatalf("unexpected password")
		}
	})

	t.Run("supports custom token username override", func(t *testing.T) {
		env := map[string]string{
			"GIT_TOKEN":          "tok",
			"GIT_TOKEN_USERNAME": "bot",
		}
		auth := authFromEnv(func(k string) string { return env[k] }, "https://git.example.com/repo.git")
		if auth == nil {
			t.Fatalf("expected auth")
		}
		if auth.Username != "bot" || auth.Password != "tok" {
			t.Fatalf("unexpected auth: %#v", auth)
		}
	})
}
