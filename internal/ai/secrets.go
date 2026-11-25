package ai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNoServerKey means provider keys cannot be stored: ADRIENT_SECRETS_KEY is unset.
var ErrNoServerKey = errors.New("server secret key is not configured (set ADRIENT_SECRETS_KEY), or use key_env")

func aead(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, ErrNoServerKey
	}
	k := sha256.Sum256([]byte("adrient-ai-provider-v1|" + secret))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func keyAAD(orgID, providerID string) []byte {
	return []byte("ai-provider|" + orgID + "|" + providerID)
}

// sealKey encrypts an API key bound to its org and provider row.
func sealKey(secret, orgID, providerID, key string) ([]byte, error) {
	g, err := aead(secret)
	if err != nil {
		return nil, err
	}
	n := make([]byte, g.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return g.Seal(n, n, []byte(key), keyAAD(orgID, providerID)), nil
}

// openKey decrypts a key sealed by sealKey.
func openKey(secret, orgID, providerID string, blob []byte) (string, error) {
	g, err := aead(secret)
	if err != nil {
		return "", err
	}
	if len(blob) < g.NonceSize() {
		return "", errors.New("stored key is corrupt")
	}
	out, err := g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], keyAAD(orgID, providerID))
	if err != nil {
		return "", errors.New("cannot decrypt stored key (server secret key changed?)")
	}
	return string(out), nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// scrub removes every occurrence of the given secrets from s (for errors and logs).
func scrub(s string, secrets ...string) string {
	for _, k := range secrets {
		if len(k) >= 4 {
			s = strings.ReplaceAll(s, k, "[key]")
		}
	}
	return s
}

// Redaction of secrets found in prompts, document text and tool results.
var redactRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "[redacted private key]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[redacted AWS key]"},
	{regexp.MustCompile(`\bsk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}`), "[redacted API key]"},
	{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`), "[redacted token]"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), "[redacted token]"},
	{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), "[redacted API key]"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), "[redacted token]"},
	{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/-]{16,}=*`), "$1 [redacted]"},
	{regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|api[_-]?key|access[_-]?token|client[_-]?secret)(\s*[:=]\s*)("[^"]*"|'[^']*'|\S+)`), "$1$2[redacted]"},
}

// Redact replaces secrets in s and reports how many were replaced.
func Redact(s string) (string, int) {
	n := 0
	for _, r := range redactRules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			n++
			return r.re.ReplaceAllString(m, r.repl)
		})
	}
	return s, n
}
