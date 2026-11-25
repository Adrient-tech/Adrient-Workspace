package main

import (
	"os"
	"testing"
	"time"
)

func TestLoadAuthConfigFromEnv_HappyPath(t *testing.T) {
	t.Setenv("ADRIENT_OIDC_ISSUER", "http://localhost:8081")
	t.Setenv("ADRIENT_OIDC_CLIENT_ID", "adrient-dev-client")
	t.Setenv("ADRIENT_OIDC_CLIENT_SECRET", "secret")
	t.Setenv("ADRIENT_OIDC_REDIRECT_URL", "http://localhost:8080/cb")
	t.Setenv("ADRIENT_SESSION_COOKIE_NAME", "adrient_session")
	t.Setenv("ADRIENT_SESSION_COOKIE_SECURE", "false")
	t.Setenv("ADRIENT_SESSION_LIFETIME", "1h")
	t.Setenv("ADRIENT_DEFAULT_ORG_SLUG", "default")

	cfg, err := loadAuthConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IssuerURL != "http://localhost:8081" {
		t.Errorf("IssuerURL: got %q", cfg.IssuerURL)
	}
	if cfg.SessionLifetime != time.Hour {
		t.Errorf("SessionLifetime: got %v, want 1h", cfg.SessionLifetime)
	}
	if cfg.CookieSecure {
		t.Errorf("CookieSecure: got true, want false")
	}
}

func TestLoadAuthConfigFromEnv_Defaults(t *testing.T) {
	// Required vars set; defaultable vars unset.
	t.Setenv("ADRIENT_OIDC_ISSUER", "http://localhost:8081")
	t.Setenv("ADRIENT_OIDC_CLIENT_ID", "adrient-dev-client")
	t.Setenv("ADRIENT_OIDC_CLIENT_SECRET", "secret")
	t.Setenv("ADRIENT_OIDC_REDIRECT_URL", "http://localhost:8080/cb")
	os.Unsetenv("ADRIENT_SESSION_COOKIE_NAME")
	os.Unsetenv("ADRIENT_SESSION_LIFETIME")
	os.Unsetenv("ADRIENT_DEFAULT_ORG_SLUG")

	cfg, err := loadAuthConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CookieName != "adrient_session" {
		t.Errorf("CookieName default: got %q", cfg.CookieName)
	}
	if cfg.SessionLifetime != 168*time.Hour {
		t.Errorf("SessionLifetime default: got %v, want 168h", cfg.SessionLifetime)
	}
	if cfg.DefaultOrgSlug != "default" {
		t.Errorf("DefaultOrgSlug default: got %q", cfg.DefaultOrgSlug)
	}
}

func TestLoadAuthConfigFromEnv_MissingRequired(t *testing.T) {
	// All required vars unset → Validate fails.
	os.Unsetenv("ADRIENT_OIDC_ISSUER")
	os.Unsetenv("ADRIENT_OIDC_CLIENT_ID")
	os.Unsetenv("ADRIENT_OIDC_CLIENT_SECRET")
	os.Unsetenv("ADRIENT_OIDC_REDIRECT_URL")
	t.Setenv("ADRIENT_SESSION_LIFETIME", "1h")

	_, err := loadAuthConfigFromEnv()
	if err == nil {
		t.Fatal("expected error from Validate, got nil")
	}
}

func TestLoadAuthConfigFromEnv_BadLifetime(t *testing.T) {
	t.Setenv("ADRIENT_OIDC_ISSUER", "http://localhost:8081")
	t.Setenv("ADRIENT_OIDC_CLIENT_ID", "x")
	t.Setenv("ADRIENT_OIDC_CLIENT_SECRET", "y")
	t.Setenv("ADRIENT_OIDC_REDIRECT_URL", "http://localhost:8080/cb")
	t.Setenv("ADRIENT_SESSION_LIFETIME", "not-a-duration")

	_, err := loadAuthConfigFromEnv()
	if err == nil {
		t.Fatal("expected duration parse error, got nil")
	}
}

func TestDefaultEnv(t *testing.T) {
	t.Setenv("ADRIENT_TEST_KEY", "")
	if got := defaultEnv("ADRIENT_TEST_KEY", "fallback"); got != "fallback" {
		t.Errorf("empty env: got %q, want fallback", got)
	}
	t.Setenv("ADRIENT_TEST_KEY", "actual")
	if got := defaultEnv("ADRIENT_TEST_KEY", "fallback"); got != "actual" {
		t.Errorf("set env: got %q, want actual", got)
	}
}
