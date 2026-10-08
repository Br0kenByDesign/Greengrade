// Package security contains token handling, hashing, proof-of-work and rate limiting.
package security

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token types. Every token carries exactly one, and verification always checks it,
// so a magic link token can never be used as an access token and vice versa.
const (
	TypAccess   = "access"
	TypRefresh  = "refresh"
	TypMagic    = "magic"
	TypSignup   = "signup"
	TypWebAuthn = "webauthn"
	TypOAuth    = "oauth"
)

type Claims struct {
	jwt.RegisteredClaims
	Typ string `json:"typ"`

	AuthAt int64 `json:"aat,omitempty"` // access, refresh: time of the last real sign-in (not refresh)

	EmailHash string          `json:"eh,omitempty"`  // magic, signup
	Provider  string          `json:"prv,omitempty"` // signup, oauth
	ProvSub   string          `json:"psb,omitempty"` // signup
	Mode      string          `json:"mod,omitempty"` // oauth, webauthn
	State     string          `json:"st,omitempty"`  // oauth
	Verifier  string          `json:"pkce,omitempty"`
	Data      json.RawMessage `json:"dat,omitempty"` // webauthn session
}

type Tokens struct {
	priv   ed25519.PrivateKey
	pub    ed25519.PublicKey
	kid    string
	issuer string
}

// Derive returns a purpose specific sub key of the app secret.
func Derive(secret []byte, purpose string) []byte {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("greengrade/" + purpose))
	return m.Sum(nil)
}

func NewTokens(appSecret []byte, issuer string) *Tokens {
	priv := ed25519.NewKeyFromSeed(Derive(appSecret, "jwt-ed25519"))
	pub := priv.Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	return &Tokens{priv: priv, pub: pub, kid: hex.EncodeToString(sum[:8]), issuer: issuer}
}

func RandomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (t *Tokens) Sign(c Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	c.Issuer = t.issuer
	c.IssuedAt = jwt.NewNumericDate(now)
	c.NotBefore = jwt.NewNumericDate(now.Add(-5 * time.Second))
	c.ExpiresAt = jwt.NewNumericDate(now.Add(ttl))
	if c.ID == "" {
		c.ID = RandomID(16)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, c)
	tok.Header["kid"] = t.kid
	return tok.SignedString(t.priv)
}

var ErrInvalidToken = errors.New("ungültiges oder abgelaufenes Token")

func (t *Tokens) Verify(raw, typ string) (*Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(raw, &c, func(tok *jwt.Token) (any, error) {
		return t.pub, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(t.issuer),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(5*time.Second))
	if err != nil || c.Typ != typ {
		return nil, ErrInvalidToken
	}
	return &c, nil
}

// Login codes: 6 characters without easily confused ones (0/O, 1/I/L), shown as "K7Q-4MX".
const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func NewLoginCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, 6)
	for i, v := range b {
		// 256 % 31 introduces a tiny bias; irrelevant with 5 attempts per code
		out[i] = codeAlphabet[int(v)%len(codeAlphabet)]
	}
	return string(out)
}

func FormatLoginCode(c string) string { return c[:3] + "-" + c[3:] }

func NormalizeLoginCode(c string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(c) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// EmailHasher turns an address into a stable, non-reversible identifier.
type EmailHasher struct{ key []byte }

func NewEmailHasher(pepper []byte) *EmailHasher { return &EmailHasher{key: pepper} }

func NormalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func (h *EmailHasher) Hash(email string) []byte {
	m := hmac.New(sha256.New, h.key)
	m.Write([]byte(NormalizeEmail(email)))
	return m.Sum(nil)
}
