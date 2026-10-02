// Package api implements the HTTP API and serves the embedded web app.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/config"
	"github.com/Br0kenByDesign/Greengrade/internal/mail"
	"github.com/Br0kenByDesign/Greengrade/internal/media"
	"github.com/Br0kenByDesign/Greengrade/internal/oauth"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg      *config.Config
	db       *pgxpool.Pool
	tokens   *security.Tokens
	emails   *security.EmailHasher
	pow      *security.PoW
	ips      *security.IPResolver
	mail     mail.Sender
	photos   *media.Store
	oauth    *oauth.Registry
	wa       *webauthn.WebAuthn
	web      fs.FS
	admins   map[string]bool // hex(email hmac)
	authLim  *security.Limiter
	writeLim *security.Limiter
	upLim    *security.Limiter
}

func New(cfg *config.Config, db *pgxpool.Pool, sender mail.Sender, photos *media.Store, web fs.FS) (*Server, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: "greengrade",
		RPOrigins:     []string{cfg.AppOrigin},
	})
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg: cfg, db: db, mail: sender, photos: photos, web: web, wa: wa,
		tokens:   security.NewTokens(cfg.AppSecret, cfg.AppOrigin),
		emails:   security.NewEmailHasher(cfg.EmailPepper),
		pow:      security.NewPoW(cfg.AppSecret),
		ips:      security.NewIPResolver(cfg.TrustedProxies),
		oauth:    oauth.NewRegistry(cfg),
		admins:   map[string]bool{},
		authLim:  security.NewLimiter(40, time.Minute),
		writeLim: security.NewLimiter(240, time.Minute),
		upLim:    security.NewLimiter(60, time.Hour),
	}
	var hashes [][]byte
	for _, e := range cfg.AdminEmails {
		h := s.emails.Hash(e)
		s.admins[string(h)] = true
		hashes = append(hashes, h)
	}
	// Keep admin flags in sync with ADMIN_EMAILS on every start.
	if _, err := db.Exec(context.Background(),
		`UPDATE users SET is_admin = (email_hmac IS NOT NULL AND email_hmac = ANY($1::bytea[]))`, hashes); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	// auth (public)
	m.HandleFunc("GET /api/auth/config", s.handleAuthConfig)
	m.HandleFunc("GET /api/auth/challenge", s.limit(s.handleChallenge))
	m.HandleFunc("POST /api/auth/magic", s.limit(s.handleMagicRequest))
	m.HandleFunc("POST /api/auth/magic/verify", s.limit(s.handleMagicVerify))
	m.HandleFunc("POST /api/auth/signup", s.limit(s.handleSignup))
	m.HandleFunc("POST /api/auth/refresh", s.limit(s.handleRefresh))
	m.HandleFunc("POST /api/auth/logout", s.handleLogout)
	m.HandleFunc("POST /api/auth/passkey/login/begin", s.limit(s.handlePasskeyLoginBegin))
	m.HandleFunc("POST /api/auth/passkey/login/finish", s.limit(s.handlePasskeyLoginFinish))
	m.HandleFunc("GET /api/auth/oauth/{provider}/start", s.limit(s.handleOAuthStart))
	m.HandleFunc("GET /api/auth/oauth/{provider}/callback", s.limit(s.handleOAuthCallback))

	// account
	m.HandleFunc("GET /api/me", s.auth(s.handleMe))
	m.HandleFunc("PATCH /api/me", s.auth(s.handleMeUpdate))
	m.HandleFunc("DELETE /api/me", s.auth(s.handleDeleteAccount))
	m.HandleFunc("POST /api/me/logout-all", s.auth(s.handleLogoutAll))
	m.HandleFunc("GET /api/me/export", s.auth(s.handleExport))
	m.HandleFunc("POST /api/me/passkeys/begin", s.auth(s.handlePasskeyRegisterBegin))
	m.HandleFunc("POST /api/me/passkeys/finish", s.auth(s.handlePasskeyRegisterFinish))
	m.HandleFunc("DELETE /api/me/passkeys/{id}", s.auth(s.handlePasskeyDelete))
	m.HandleFunc("DELETE /api/me/identities/{provider}", s.auth(s.handleIdentityDelete))
	m.HandleFunc("POST /api/me/notices/read", s.auth(s.handleNoticesRead))

	// private log
	m.HandleFunc("GET /api/strains/suggest", s.auth(s.handleSuggest))
	m.HandleFunc("GET /api/entries", s.auth(s.handleEntries))
	m.HandleFunc("POST /api/entries", s.auth(s.handleEntryCreate))
	m.HandleFunc("GET /api/entries/{id}", s.auth(s.handleEntryGet))
	m.HandleFunc("PUT /api/entries/{id}", s.auth(s.handleEntryUpdate))
	m.HandleFunc("DELETE /api/entries/{id}", s.auth(s.handleEntryDelete))
	m.HandleFunc("POST /api/entries/{id}/tastings", s.auth(s.handleTastingCreate))
	m.HandleFunc("PUT /api/tastings/{id}", s.auth(s.handleTastingUpdate))
	m.HandleFunc("DELETE /api/tastings/{id}", s.auth(s.handleTastingDelete))
	m.HandleFunc("PUT /api/entries/{id}/share", s.auth(s.handleShare))
	m.HandleFunc("POST /api/entries/{id}/photos", s.auth(s.handlePhotoUpload))
	m.HandleFunc("DELETE /api/photos/{id}", s.auth(s.handlePhotoDelete))
	m.HandleFunc("GET /api/photos/{id}", s.auth(s.handlePhotoGet))
	m.HandleFunc("GET /api/stats", s.auth(s.handleStats))

	m.HandleFunc("GET /api/grows", s.auth(s.handleGrows))
	m.HandleFunc("POST /api/grows", s.auth(s.handleGrowSave))
	m.HandleFunc("GET /api/grows/{id}", s.auth(s.handleGrowGet))
	m.HandleFunc("PUT /api/grows/{id}", s.auth(s.handleGrowSave))
	m.HandleFunc("DELETE /api/grows/{id}", s.auth(s.handleGrowDelete))
	m.HandleFunc("POST /api/grows/{id}/logs", s.auth(s.handleGrowLogCreate))
	m.HandleFunc("DELETE /api/grow-logs/{id}", s.auth(s.handleGrowLogDelete))

	// public ratings
	m.HandleFunc("GET /api/public/strains", s.auth(s.handlePublicList))
	m.HandleFunc("GET /api/public/strains/{id}", s.auth(s.handlePublicStrain))
	m.HandleFunc("POST /api/public/reports", s.auth(s.handleReport))

	// moderation
	m.HandleFunc("GET /api/admin/reports", s.admin(s.handleAdminReports))
	m.HandleFunc("POST /api/admin/reports/{id}", s.admin(s.handleAdminResolve))
	m.HandleFunc("POST /api/admin/ratings/{id}/hide", s.admin(s.handleAdminHide))
	m.HandleFunc("GET /api/admin/strains", s.admin(s.handleAdminStrains))
	m.HandleFunc("PATCH /api/admin/strains/{id}", s.admin(s.handleAdminStrainRename))
	m.HandleFunc("POST /api/admin/strains/merge", s.admin(s.handleAdminMerge))
	m.HandleFunc("GET /api/admin/strains/{id}/photos", s.admin(s.handleAdminStrainPhotos))
	m.HandleFunc("POST /api/admin/strains/{id}/pin", s.admin(s.handleAdminPin))

	m.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, http.StatusNotFound, "Nicht gefunden.") })
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	m.Handle("/", s.spa())

	return s.logging(s.headers(s.sameOrigin(m)))
}

// ---------- middleware ----------

func (s *Server) headers(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'", "script-src 'self'", "style-src 'self' 'unsafe-inline'",
		"img-src 'self' blob: data:", "font-src 'self'", "connect-src 'self'", "worker-src 'self'",
		"manifest-src 'self'", "object-src 'none'", "base-uri 'none'", "frame-ancestors 'none'",
		"form-action 'self'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
		if strings.HasPrefix(s.cfg.AppOrigin, "https://") {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin blocks cross-site state-changing requests (CSRF), in addition to SameSite cookies.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			origin := r.Header.Get("Origin")
			if origin != s.cfg.AppOrigin && !(origin == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin") {
				fail(w, http.StatusForbidden, "Anfrage von fremder Herkunft abgelehnt.")
				return
			}
			if !s.writeLim.Allow(s.ips.ClientIP(r)) {
				fail(w, http.StatusTooManyRequests, "Zu viele Anfragen. Bitte warte kurz.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }

// logging writes method, path and status only - no IPs, no query strings, no bodies.
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			slog.Info("request", "method", r.Method, "path", redactPath(r.URL.Path), "status", sw.code, "ms", time.Since(start).Milliseconds())
		}
	})
}

var reUUIDPart = regexp.MustCompile(`[0-9a-fA-F-]{36}`)

func redactPath(p string) string { return reUUIDPart.ReplaceAllString(p, ":id") }

func (s *Server) limit(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authLim.Allow(s.ips.ClientIP(r)) {
			fail(w, http.StatusTooManyRequests, "Zu viele Versuche. Bitte warte eine Minute.")
			return
		}
		h(w, r)
	}
}

// ---------- current user ----------

type user struct {
	ID      string
	Name    string
	Admin   bool
	HasMail bool
}

type ctxKey struct{}

func current(r *http.Request) *user { u, _ := r.Context().Value(ctxKey{}).(*user); return u }

func (s *Server) loadUser(ctx context.Context, raw string, typ string) (*user, error) {
	c, err := s.tokens.Verify(raw, typ)
	if err != nil {
		return nil, err
	}
	u := &user{}
	var tva time.Time
	err = s.db.QueryRow(ctx, `SELECT id, display_name, is_admin, email_hmac IS NOT NULL, tokens_valid_after FROM users WHERE id=$1`, c.Subject).
		Scan(&u.ID, &u.Name, &u.Admin, &u.HasMail, &tva)
	if err != nil {
		return nil, security.ErrInvalidToken
	}
	// "Überall abmelden" and account deletion take effect immediately.
	if c.IssuedAt == nil || c.IssuedAt.Time.Before(tva) {
		return nil, security.ErrInvalidToken
	}
	return u, nil
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(cookieAccess)
		if err != nil {
			fail(w, http.StatusUnauthorized, "Bitte melde dich an.")
			return
		}
		u, err := s.loadUser(r.Context(), ck.Value, security.TypAccess)
		if err != nil {
			fail(w, http.StatusUnauthorized, "Bitte melde dich an.")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	}
}

func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if !current(r).Admin {
			fail(w, http.StatusForbidden, "Kein Zugriff.")
			return
		}
		h(w, r)
	})
}

// ---------- cookies ----------

const (
	cookieAccess   = "__Host-gg_at"
	cookieRefresh  = "__Host-gg_rt"
	cookieWebAuthn = "__Host-gg_wa"
	cookieOAuth    = "__Host-gg_oa"
	accessTTL      = 15 * time.Minute
	refreshTTL     = 30 * 24 * time.Hour
)

func (s *Server) setCookie(w http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteLaxMode})
}

func (s *Server) startSession(w http.ResponseWriter, userID string) error {
	at, err := s.tokens.Sign(security.Claims{Typ: security.TypAccess, RegisteredClaims: subject(userID)}, accessTTL)
	if err != nil {
		return err
	}
	rt, err := s.tokens.Sign(security.Claims{Typ: security.TypRefresh, RegisteredClaims: subject(userID)}, refreshTTL)
	if err != nil {
		return err
	}
	s.setCookie(w, cookieAccess, at, accessTTL)
	s.setCookie(w, cookieRefresh, rt, refreshTTL)
	return nil
}

// ---------- JSON helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func internal(w http.ResponseWriter, err error) {
	slog.Error("interner Fehler", "err", err)
	fail(w, http.StatusInternalServerError, "Da ist etwas schiefgelaufen. Bitte versuche es erneut.")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "Ungültige Anfrage.")
		return false
	}
	return true
}

var reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// pathID returns a validated lower-case UUID from the path, or writes 404.
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("id"))
	if !reUUID.MatchString(id) {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return "", false
	}
	return id, true
}

func validUUID(s string) bool { return reUUID.MatchString(s) }

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ---------- web app ----------

func (s *Server) spa() http.Handler {
	files := http.FileServerFS(s.web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := fs.Stat(s.web, p); err != nil || f.IsDir() {
			// client-side route: serve the app shell
			w.Header().Set("Cache-Control", "no-cache")
			b, err := fs.ReadFile(s.web, "index.html")
			if err != nil {
				http.Error(w, "web app not built", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(b)
			return
		}
		switch {
		case strings.HasPrefix(p, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// ---------- housekeeping ----------

func (s *Server) Housekeeping(ctx context.Context) {
	run := func() {
		db := s.db
		db.Exec(ctx, `DELETE FROM used_tokens WHERE expires_at < now()`)
		db.Exec(ctx, `DELETE FROM rate_events WHERE at < now() - interval '2 days'`)
		db.Exec(ctx, `DELETE FROM strains s WHERE NOT EXISTS (SELECT 1 FROM entries e WHERE e.strain_id=s.id)
			AND NOT EXISTS (SELECT 1 FROM public_ratings p WHERE p.strain_id=s.id)
			AND NOT EXISTS (SELECT 1 FROM grows g WHERE g.strain_id=s.id)
			AND s.created_at < now() - interval '1 day'`)
		// remove photo files whose database row is gone
		var ids []string
		s.photos.Walk(func(id string) { ids = append(ids, id) })
		for len(ids) > 0 {
			n := min(500, len(ids))
			batch := ids[:n]
			ids = ids[n:]
			valid := batch[:0:0]
			for _, id := range batch {
				if validUUID(id) {
					valid = append(valid, id)
				}
			}
			rows, err := db.Query(ctx, `SELECT id::text FROM photos WHERE id = ANY($1::uuid[])`, valid)
			if err != nil {
				continue
			}
			known := map[string]bool{}
			for rows.Next() {
				var id string
				rows.Scan(&id)
				known[id] = true
			}
			rows.Close()
			for _, id := range valid {
				if !known[id] {
					s.photos.Delete(id)
				}
			}
		}
	}
	run()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
