package dnsmail

import "os"

func mailBackendEnv() string {
	b := os.Getenv("ADRIENT_MAIL_BACKEND")
	if b == "" {
		return "local"
	}
	return b
}

// ServerSecret returns the secret that seals DKIM private keys:
// ADRIENT_SECRETS_KEY, falling back to the OIDC client secret.
func ServerSecret() string {
	if v := os.Getenv("ADRIENT_SECRETS_KEY"); v != "" {
		return v
	}
	return os.Getenv("ADRIENT_OIDC_CLIENT_SECRET")
}
