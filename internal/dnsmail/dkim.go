package dnsmail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// Key states.
const (
	StateActive  = "active"
	StateNext    = "next"
	StateRetired = "retired"
)

// Algorithms.
const (
	AlgoRSA     = "rsa-2048"
	AlgoEd25519 = "ed25519"
)

// ErrNoServerKey means no secret is configured to encrypt private keys.
var ErrNoServerKey = errors.New("no server secret configured (set ADRIENT_SECRETS_KEY)")

// Selector returns the selector for a key created at t: adr<YYYYMM>, with a
// letter suffix when that selector exists.
func Selector(t time.Time, taken func(string) bool) string {
	base := "adr" + t.UTC().Format("200601")
	if !taken(base) {
		return base
	}
	for c := 'b'; c <= 'z'; c++ {
		s := base + string(c)
		if !taken(s) {
			return s
		}
	}
	return base + fmt.Sprint(t.UnixNano())
}

// GenerateKey creates a key pair. It returns the base64 public key (as used in
// the DKIM p= tag) and the PKCS#8 PEM private key.
func GenerateKey(algo string) (pub string, privPEM []byte, err error) {
	switch algo {
	case AlgoEd25519:
		pk, sk, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return "", nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(sk)
		if err != nil {
			return "", nil, err
		}
		return base64.StdEncoding.EncodeToString(pk), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
	case AlgoRSA, "":
		sk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", nil, err
		}
		pubDER, err := x509.MarshalPKIXPublicKey(&sk.PublicKey)
		if err != nil {
			return "", nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(sk)
		if err != nil {
			return "", nil, err
		}
		return base64.StdEncoding.EncodeToString(pubDER), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
	}
	return "", nil, fmt.Errorf("unsupported algorithm %q", algo)
}

func gcm(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, ErrNoServerKey
	}
	k := sha256.Sum256([]byte("adrient-dkim-v1|" + secret))
	b, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// Encrypt seals data with AES-256-GCM; the random nonce is prepended. aad binds
// the ciphertext to its row (domain id + selector).
func Encrypt(secret string, data, aad []byte) ([]byte, error) {
	g, err := gcm(secret)
	if err != nil {
		return nil, err
	}
	n := make([]byte, g.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return g.Seal(n, n, data, aad), nil
}

// Decrypt reverses Encrypt.
func Decrypt(secret string, blob, aad []byte) ([]byte, error) {
	g, err := gcm(secret)
	if err != nil {
		return nil, err
	}
	if len(blob) < g.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return g.Open(nil, blob[:g.NonceSize()], blob[g.NonceSize():], aad)
}

// Key is a stored DKIM key row.
type Key struct {
	ID          string     `json:"id"`
	DomainID    string     `json:"domain_id"`
	Selector    string     `json:"selector"`
	Algo        string     `json:"algo"`
	PublicKey   string     `json:"public_key"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at,omitempty"`
	RotatedAt   *time.Time `json:"rotated_at,omitempty"`
	Private     []byte     `json:"-"`
}

// Rotation is the pure state machine over a domain's keys. It never touches
// storage so it can be tested with a fake clock.
type Rotation struct {
	Keys []Key
	Now  time.Time
	// WaitDays is how long a published next key must exist before it may be
	// activated (default 7), and how long a retired key stays published (7).
	WaitDays int
}

func (r Rotation) wait() time.Duration {
	d := r.WaitDays
	if d == 0 {
		d = 7
	}
	return time.Duration(d) * 24 * time.Hour
}

// CanActivate reports whether the next key may become active. publishedInDNS is
// the live check result for that key. The wait period is advisory for the first
// key (nothing signs yet) and enforced for rotations.
func (r Rotation) CanActivate(publishedInDNS bool) (Key, error) {
	var next, active *Key
	for i := range r.Keys {
		switch r.Keys[i].State {
		case StateNext:
			next = &r.Keys[i]
		case StateActive:
			active = &r.Keys[i]
		}
	}
	if next == nil {
		return Key{}, errors.New("no next key to activate; generate one first")
	}
	if !publishedInDNS {
		return Key{}, errors.New("the DKIM record for this key is not visible in DNS yet")
	}
	if active != nil && r.Now.Sub(next.CreatedAt) < r.wait() {
		left := r.wait() - r.Now.Sub(next.CreatedAt)
		return Key{}, fmt.Errorf("rotation waits %d day(s) so receivers cache the new key (%d day(s) left)", r.WaitDays7(), int((left+24*time.Hour-1)/(24*time.Hour)))
	}
	return *next, nil
}

// WaitDays7 returns the configured wait in days.
func (r Rotation) WaitDays7() int { return int(r.wait() / (24 * time.Hour)) }

// Retirable lists retired keys whose DNS record may be removed now.
func (r Rotation) Retirable() []Key {
	var out []Key
	for _, k := range r.Keys {
		if k.State == StateRetired && k.RotatedAt != nil && r.Now.Sub(*k.RotatedAt) >= r.wait() {
			out = append(out, k)
		}
	}
	return out
}
