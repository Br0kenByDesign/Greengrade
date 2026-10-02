package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ---------- Proof of work (ALTCHA-compatible protocol, no third party) ----------

type PoW struct{ key []byte }

func NewPoW(appSecret []byte) *PoW { return &PoW{key: Derive(appSecret, "pow")} }

type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	MaxNumber int64  `json:"maxnumber"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
}

const powMax = 150000

func (p *PoW) New() Challenge {
	salt := RandomID(12) + "?expires=" + strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	n, _ := rand.Int(rand.Reader, big.NewInt(powMax))
	sum := sha256.Sum256([]byte(salt + n.String()))
	ch := hex.EncodeToString(sum[:])
	return Challenge{Algorithm: "SHA-256", Challenge: ch, MaxNumber: powMax, Salt: salt, Signature: p.sign(ch)}
}

func (p *PoW) sign(ch string) string {
	m := hmac.New(sha256.New, p.key)
	m.Write([]byte(ch))
	return hex.EncodeToString(m.Sum(nil))
}

// Verify checks a base64 payload and returns the challenge (for single-use bookkeeping).
func (p *PoW) Verify(payload string) (string, error) {
	bad := errors.New("Sicherheitsprüfung fehlgeschlagen. Lade die Seite neu und versuche es noch einmal.")
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", bad
	}
	var s struct {
		Algorithm string `json:"algorithm"`
		Challenge string `json:"challenge"`
		Number    int64  `json:"number"`
		Salt      string `json:"salt"`
		Signature string `json:"signature"`
	}
	if json.Unmarshal(raw, &s) != nil || s.Algorithm != "SHA-256" {
		return "", bad
	}
	if !hmac.Equal([]byte(p.sign(s.Challenge)), []byte(s.Signature)) {
		return "", bad
	}
	q := s.Salt[strings.Index(s.Salt, "?")+1:]
	vals, _ := url.ParseQuery(q)
	exp, err := strconv.ParseInt(vals.Get("expires"), 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", bad
	}
	sum := sha256.Sum256([]byte(s.Salt + strconv.FormatInt(s.Number, 10)))
	if hex.EncodeToString(sum[:]) != s.Challenge {
		return "", bad
	}
	return s.Challenge, nil
}

// ---------- In-memory rate limiter (per process; fine for a single app container) ----------

type Limiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func NewLimiter(limit int, window time.Duration) *Limiter {
	l := &Limiter{hits: map[string][]time.Time{}, limit: limit, window: window}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			cut := time.Now().Add(-l.window)
			for k, v := range l.hits {
				if len(v) == 0 || v[len(v)-1].Before(cut) {
					delete(l.hits, k)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.window)
	v := l.hits[key]
	i := 0
	for i < len(v) && v[i].Before(cut) {
		i++
	}
	v = v[i:]
	if len(v) >= l.limit {
		l.hits[key] = v
		return false
	}
	l.hits[key] = append(v, now)
	return true
}

// ---------- Client IP behind a reverse proxy ----------

type IPResolver struct{ trusted []*net.IPNet }

func NewIPResolver(trusted []*net.IPNet) *IPResolver { return &IPResolver{trusted: trusted} }

func (r *IPResolver) isTrusted(ip net.IP) bool {
	for _, n := range r.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIP trusts X-Forwarded-For only when the direct peer is a trusted proxy and
// walks the list from the right, returning the first untrusted address.
func (r *IPResolver) ClientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil || !r.isTrusted(peer) {
		return host
	}
	parts := strings.Split(req.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if !r.isTrusted(ip) || i == 0 {
			return ip.String()
		}
	}
	return host
}

// ---------- Public comment filter ----------

var (
	reURL     = regexp.MustCompile(`(?i)(https?://|www\.|\b[a-z0-9-]{2,}\.(com|de|net|org|io|me|app|shop|store|to|ly|gg|cc|xyz|ru|onion|link|info|biz|eu|at|ch|nl)\b)`)
	rePhone   = regexp.MustCompile(`\+?\d[\d\s\-/().]{6,}\d`)
	reHandle  = regexp.MustCompile(`(^|\s)@[\p{L}0-9_.]{3,}`)
	reContact = regexp.MustCompile(`(?i)\b(telegram|whats\s?app|threema|wickr|snapchat|signal\s?app|t\.me|discord\.gg|instagram|insta|tiktok|schreib\s+mir|meld\s?dich|melde\s+dich|dm\s+me|pm\s+me|privatnachricht|verkaufe|zu\s+verkaufen|liefere|lieferung)\b`)
)

var ErrComment = errors.New("Öffentliche Kommentare dürfen keine Links, Telefonnummern, Nutzernamen oder Kontakt- und Verkaufsangebote enthalten.")

// CleanText removes control characters and trims.
func CleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u200b' || r == '\u202e' || r == '\u202d' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

func CheckPublicComment(s string) error {
	if reURL.MatchString(s) || rePhone.MatchString(s) || reHandle.MatchString(s) || reContact.MatchString(s) {
		return ErrComment
	}
	return nil
}
