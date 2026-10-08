package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/mail"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

func subject(id string) jwt.RegisteredClaims { return jwt.RegisteredClaims{Subject: id} }

// markUsed records a one-time token. Returns false if it was used before.
func (s *Server) markUsed(ctx context.Context, id string, exp time.Time) (bool, error) {
	tag, err := s.db.Exec(ctx, `INSERT INTO used_tokens(id, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, exp)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// rateOK counts events per key in Postgres, so limits survive restarts.
func (s *Server) rateOK(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM rate_events WHERE key=$1 AND at > now() - make_interval(secs => $2)`,
		key, window.Seconds()).Scan(&n); err != nil {
		return false, err
	}
	if n >= limit {
		return false, nil
	}
	_, err := s.db.Exec(ctx, `INSERT INTO rate_events(key) VALUES ($1)`, key)
	return err == nil, err
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"providers": s.oauth.Enabled(), "privacyUrl": s.cfg.PrivacyURL, "imprintUrl": s.cfg.ImprintURL, "termsUrl": s.cfg.TermsURL})
}

func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.pow.New())
}

// ---------- magic link ----------

func (s *Server) handleMagicRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string `json:"email"`
		PoW    string `json:"pow"`
		Reauth bool   `json:"reauth"` // confirm a signed-in account (code is only sent if the address matches)
	}
	if !readJSON(w, r, &in) {
		return
	}
	email := security.NormalizeEmail(in.Email)
	addr, err := netmail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		fail(w, http.StatusBadRequest, "Bitte gib eine gültige E-Mail-Adresse ein.")
		return
	}
	ctx := r.Context()
	ch, err := s.pow.Verify(in.PoW)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if ok, err := s.markUsed(ctx, "pow:"+ch, time.Now().Add(15*time.Minute)); err != nil {
		internal(w, err)
		return
	} else if !ok {
		fail(w, http.StatusBadRequest, "Sicherheitsprüfung schon verwendet. Bitte versuche es erneut.")
		return
	}
	eh := s.emails.Hash(email)
	ehHex := hex.EncodeToString(eh)
	reauthMismatch := false
	if in.Reauth {
		ck, err := r.Cookie(cookieAccess)
		if err != nil {
			fail(w, http.StatusUnauthorized, "Bitte melde dich an.")
			return
		}
		u, err := s.loadUser(ctx, ck.Value, security.TypAccess)
		if err != nil {
			fail(w, http.StatusUnauthorized, "Bitte melde dich an.")
			return
		}
		var own []byte
		s.db.QueryRow(ctx, `SELECT email_hmac FROM users WHERE id=$1`, u.ID).Scan(&own)
		// no hint whether the address matches: same answer, mail only for the right one
		reauthMismatch = !hmac.Equal(own, eh)
	}
	ip := s.ips.LimitKey(r)
	for _, l := range []struct {
		key    string
		limit  int
		window time.Duration
	}{
		{"magic-ip:" + ip, 5, time.Hour},
		{"magic-mail:" + ehHex, 3, 15 * time.Minute},
		{"magic-mail-day:" + ehHex, 10, 24 * time.Hour},
	} {
		ok, err := s.rateOK(ctx, l.key, l.limit, l.window)
		if err != nil {
			internal(w, err)
			return
		}
		if !ok {
			fail(w, http.StatusTooManyRequests, "Es wurden schon mehrere Links angefordert. Bitte warte etwas und schau in dein Postfach.")
			return
		}
	}
	if reauthMismatch {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	id := security.RandomID(16)
	exp := time.Now().Add(10 * time.Minute)
	tok, err := s.tokens.Sign(security.Claims{Typ: security.TypMagic, EmailHash: ehHex, RegisteredClaims: jwt.RegisteredClaims{ID: id}}, 10*time.Minute)
	if err != nil {
		internal(w, err)
		return
	}
	code := security.NewLoginCode()
	if _, err := s.db.Exec(ctx, `INSERT INTO login_codes(id, email_hmac, code_hash, expires_at) VALUES ($1,$2,$3,$4)`,
		id, eh, s.codeHash(id, code), exp); err != nil {
		internal(w, err)
		return
	}
	shown := security.FormatLoginCode(code)
	link := s.cfg.AppOrigin + "/auth/verify#" + tok
	msg := mail.Message{
		To:      email,
		Subject: "Dein Anmeldelink für greengrade",
		Text: "Hallo,\n\nmit diesem Link meldest du dich bei greengrade an. Er ist 10 Minuten gültig und funktioniert nur einmal:\n\n" +
			link + "\n\nOder gib diesen Code auf dem Gerät ein, auf dem du die Anmeldung gestartet hast: " + shown +
			"\n\nFalls du keinen Link angefordert hast, kannst du diese Mail einfach ignorieren.\n\ngreengrade",
		HTML: `<div style="font-family:system-ui,sans-serif;font-size:15px;line-height:1.5;color:#18211B;max-width:520px">` +
			`<p>Hallo,</p><p>mit diesem Link meldest du dich bei greengrade an. Er ist 10 Minuten gültig und funktioniert nur einmal.</p>` +
			`<p style="margin:28px 0"><a href="` + html.EscapeString(link) + `" style="background:#2E5B3B;color:#fff;padding:12px 20px;border-radius:10px;text-decoration:none;font-weight:600">Bei greengrade anmelden</a></p>` +
			`<p>Oder gib diesen Code auf dem Gerät ein, auf dem du die Anmeldung gestartet hast - zum Beispiel in der App auf deinem Homescreen:</p>` +
			`<p style="font-family:ui-monospace,Menlo,Consolas,monospace;font-size:26px;font-weight:700;letter-spacing:4px;margin:8px 0 24px">` + shown + `</p>` +
			`<p style="color:#626C64;font-size:13px">Falls du keinen Link angefordert hast, kannst du diese Mail einfach ignorieren.</p></div>`,
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	if err := s.mail.Send(sendCtx, msg); err != nil {
		slog.Error("mailversand fehlgeschlagen", "err", err)
		fail(w, http.StatusBadGateway, "Die Mail konnte gerade nicht verschickt werden. Bitte versuche es später erneut.")
		return
	}
	// The code only works together with this cookie, i.e. on the device that requested it.
	s.setCookie(w, cookieLoginCode, id, 10*time.Minute)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) codeHash(id, code string) []byte {
	m := hmac.New(sha256.New, security.Derive(s.cfg.AppSecret, "login-code"))
	m.Write([]byte(id + ":" + code))
	return m.Sum(nil)
}

func (s *Server) handleCodeVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ck, err := r.Cookie(cookieLoginCode)
	if err != nil {
		fail(w, http.StatusBadRequest, "Fordere den Code bitte auf diesem Gerät an.")
		return
	}
	ctx := r.Context()
	var eh, want []byte
	var attempts int
	err = s.db.QueryRow(ctx, `UPDATE login_codes SET attempts = attempts + 1
		WHERE id=$1 AND expires_at > now() AND attempts < 5 RETURNING email_hmac, code_hash, attempts`, ck.Value).Scan(&eh, &want, &attempts)
	if err != nil {
		s.clearCookie(w, cookieLoginCode)
		fail(w, http.StatusBadRequest, "Der Code ist abgelaufen, wurde schon verwendet oder zu oft falsch eingegeben. Fordere einfach einen neuen an.")
		return
	}
	if !hmac.Equal(s.codeHash(ck.Value, security.NormalizeLoginCode(in.Code)), want) {
		left := 5 - attempts
		if left <= 0 {
			s.db.Exec(ctx, `DELETE FROM login_codes WHERE id=$1`, ck.Value)
			s.clearCookie(w, cookieLoginCode)
			fail(w, http.StatusBadRequest, "Der Code war zu oft falsch. Fordere einfach einen neuen an.")
			return
		}
		fail(w, http.StatusBadRequest, fmt.Sprintf("Der Code stimmt nicht. Noch %d Versuch%s.", left, map[bool]string{true: "", false: "e"}[left == 1]))
		return
	}
	s.db.Exec(ctx, `DELETE FROM login_codes WHERE id=$1`, ck.Value)
	s.clearCookie(w, cookieLoginCode)
	// link and code belong together: whichever is used first invalidates the other
	if ok, err := s.markUsed(ctx, "magic:"+ck.Value, time.Now().Add(15*time.Minute)); err != nil {
		internal(w, err)
		return
	} else if !ok {
		fail(w, http.StatusBadRequest, "Diese Anmeldung wurde schon über den Link abgeschlossen.")
		return
	}
	s.finishEmailLogin(w, ctx, eh)
}

func (s *Server) handleMagicVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c, err := s.tokens.Verify(in.Token, security.TypMagic)
	if err != nil {
		fail(w, http.StatusBadRequest, "Der Link ist ungültig oder abgelaufen. Fordere einfach einen neuen an.")
		return
	}
	ctx := r.Context()
	if ok, err := s.markUsed(ctx, "magic:"+c.ID, c.ExpiresAt.Time); err != nil {
		internal(w, err)
		return
	} else if !ok {
		fail(w, http.StatusBadRequest, "Dieser Link wurde schon verwendet. Fordere einfach einen neuen an.")
		return
	}
	s.db.Exec(ctx, `DELETE FROM login_codes WHERE id=$1`, c.ID)
	eh, err := hex.DecodeString(c.EmailHash)
	if err != nil {
		fail(w, http.StatusBadRequest, "Der Link ist ungültig.")
		return
	}
	s.finishEmailLogin(w, ctx, eh)
}

// finishEmailLogin signs in an existing account or hands out a signup token.
func (s *Server) finishEmailLogin(w http.ResponseWriter, ctx context.Context, eh []byte) {
	var uid string
	err := s.db.QueryRow(ctx, `UPDATE users SET is_admin=$2 WHERE email_hmac=$1 RETURNING id`, eh, s.admins[string(eh)]).Scan(&uid)
	if err == nil {
		if err := s.startSession(w, uid, time.Now()); err != nil {
			internal(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if !notFound(err) {
		internal(w, err)
		return
	}
	st, err := s.tokens.Sign(security.Claims{Typ: security.TypSignup, EmailHash: hex.EncodeToString(eh)}, 30*time.Minute)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "signup", "signupToken": st})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token        string `json:"token"`
		DisplayName  string `json:"displayName"`
		AgeConfirmed bool   `json:"ageConfirmed"`
		Consent      bool   `json:"consent"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	c, err := s.tokens.Verify(in.Token, security.TypSignup)
	if err != nil {
		fail(w, http.StatusBadRequest, "Die Registrierung ist abgelaufen. Bitte starte noch einmal.")
		return
	}
	if !in.AgeConfirmed || !in.Consent {
		fail(w, http.StatusBadRequest, "Bitte bestätige beide Punkte, um ein Konto anzulegen.")
		return
	}
	name := security.CleanText(in.DisplayName, 40)
	if name == "" {
		fail(w, http.StatusBadRequest, "Bitte gib einen Namen ein.")
		return
	}
	ctx := r.Context()
	if ok, err := s.markUsed(ctx, "signup:"+c.ID, c.ExpiresAt.Time); err != nil {
		internal(w, err)
		return
	} else if !ok {
		fail(w, http.StatusBadRequest, "Diese Registrierung wurde schon abgeschlossen.")
		return
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var uid string
	if c.EmailHash != "" {
		eh, err := hex.DecodeString(c.EmailHash)
		if err != nil {
			fail(w, http.StatusBadRequest, "Ungültige Registrierung.")
			return
		}
		err = tx.QueryRow(ctx, `INSERT INTO users(email_hmac, display_name, is_admin, age_confirmed_at, consent_at)
			VALUES ($1,$2,$3,now(),now()) ON CONFLICT (email_hmac) DO UPDATE SET email_hmac=EXCLUDED.email_hmac RETURNING id`,
			eh, name, s.admins[string(eh)]).Scan(&uid)
		if err != nil {
			internal(w, err)
			return
		}
	} else if c.Provider != "" && c.ProvSub != "" {
		err := tx.QueryRow(ctx, `SELECT user_id FROM oauth_identities WHERE provider=$1 AND subject=$2`, c.Provider, c.ProvSub).Scan(&uid)
		if notFound(err) {
			if err := tx.QueryRow(ctx, `INSERT INTO users(display_name, age_confirmed_at, consent_at) VALUES ($1, now(), now()) RETURNING id`, name).Scan(&uid); err != nil {
				internal(w, err)
				return
			}
			if _, err := tx.Exec(ctx, `INSERT INTO oauth_identities(provider, subject, user_id) VALUES ($1,$2,$3)`, c.Provider, c.ProvSub, uid); err != nil {
				internal(w, err)
				return
			}
		} else if err != nil {
			internal(w, err)
			return
		}
	} else {
		fail(w, http.StatusBadRequest, "Ungültige Registrierung.")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	if err := s.startSession(w, uid, time.Now()); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	ck, err := r.Cookie(cookieRefresh)
	if err == nil {
		var u *user
		if u, err = s.loadUser(r.Context(), ck.Value, security.TypRefresh); err == nil {
			if err = s.startSession(w, u.ID, u.AuthAt); err == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
	}
	s.clearCookie(w, cookieAccess)
	s.clearCookie(w, cookieRefresh)
	fail(w, http.StatusUnauthorized, "Bitte melde dich an.")
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearCookie(w, cookieAccess)
	s.clearCookie(w, cookieRefresh)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogoutAll(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET tokens_valid_after = date_trunc('second', now()) + interval '1 second' WHERE id=$1`, current(r).ID); err != nil {
		internal(w, err)
		return
	}
	s.handleLogout(w, r)
}

// ---------- social login ----------

func (s *Server) oauthRedirect(provider string) string {
	return s.cfg.AppOrigin + "/api/auth/oauth/" + provider + "/callback"
}

func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.oauth.Get(r.PathValue("provider"))
	if !ok {
		http.Redirect(w, r, "/login?fehler=anbieter", http.StatusSeeOther)
		return
	}
	c := security.Claims{Typ: security.TypOAuth, Provider: p.Name, State: security.RandomID(24), Verifier: security.RandomID(48), Mode: "login"}
	if mode := r.URL.Query().Get("mode"); mode == "link" || mode == "reauth" {
		ck, err := r.Cookie(cookieAccess)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		u, err := s.loadUser(r.Context(), ck.Value, security.TypAccess)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		// linking another sign-in is sensitive: it needs a recent sign-in
		if mode == "link" && !isFresh(u) {
			http.Redirect(w, r, "/account?fehler=bestaetigen", http.StatusSeeOther)
			return
		}
		c.Mode, c.Subject = mode, u.ID
	}
	tok, err := s.tokens.Sign(c, 10*time.Minute)
	if err != nil {
		internal(w, err)
		return
	}
	s.setCookie(w, cookieOAuth, tok, 10*time.Minute)
	http.Redirect(w, r, p.AuthCodeURL(c.State, c.Verifier, s.oauthRedirect(p.Name)), http.StatusSeeOther)
}

func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	back := func(path string) { http.Redirect(w, r, path, http.StatusSeeOther) }
	p, ok := s.oauth.Get(r.PathValue("provider"))
	ck, err := r.Cookie(cookieOAuth)
	s.clearCookie(w, cookieOAuth)
	if !ok || err != nil {
		back("/login?fehler=oauth")
		return
	}
	c, err := s.tokens.Verify(ck.Value, security.TypOAuth)
	q := r.URL.Query()
	if err != nil || c.Provider != p.Name || subtle.ConstantTimeCompare([]byte(c.State), []byte(q.Get("state"))) != 1 || q.Get("code") == "" {
		back("/login?fehler=oauth")
		return
	}
	ctx := r.Context()
	sub, err := s.oauth.Subject(ctx, p, q.Get("code"), c.Verifier, s.oauthRedirect(p.Name))
	if err != nil {
		slog.Warn("oauth fehlgeschlagen", "provider", p.Name, "err", err)
		back("/login?fehler=oauth")
		return
	}
	if c.Mode == "reauth" {
		// confirm the signed-in account: the provider identity must belong to it
		var owner string
		err := s.db.QueryRow(ctx, `SELECT user_id::text FROM oauth_identities WHERE provider=$1 AND subject=$2`, p.Name, sub).Scan(&owner)
		if err != nil || owner != c.Subject {
			back("/account?fehler=bestaetigung")
			return
		}
		if err := s.startSession(w, owner, time.Now()); err != nil {
			internal(w, err)
			return
		}
		back("/account?bestaetigt=1")
		return
	}
	if c.Mode == "link" {
		at, err := r.Cookie(cookieAccess)
		var u *user
		if err == nil {
			u, err = s.loadUser(ctx, at.Value, security.TypAccess)
		}
		if err != nil || u.ID != c.Subject {
			back("/login")
			return
		}
		var owner string
		err = s.db.QueryRow(ctx, `SELECT user_id FROM oauth_identities WHERE provider=$1 AND subject=$2`, p.Name, sub).Scan(&owner)
		switch {
		case err == nil && owner == u.ID:
			back("/account?verknuepft=" + p.Name)
		case err == nil:
			back("/account?fehler=vergeben")
		case notFound(err):
			if _, err := s.db.Exec(ctx, `INSERT INTO oauth_identities(provider, subject, user_id) VALUES ($1,$2,$3)`, p.Name, sub, u.ID); err != nil {
				back("/account?fehler=vorhanden")
				return
			}
			s.db.Exec(ctx, `INSERT INTO notices(user_id, title, body) VALUES ($1,$2,$3)`, u.ID, p.Label+" wurde mit deinem Konto verknüpft",
				"Am "+time.Now().Format("02.01.2006 um 15:04")+" Uhr. Warst du das nicht? Trenne die Verknüpfung unter Konto und melde dich überall ab.")
			back("/account?verknuepft=" + p.Name)
		default:
			internal(w, err)
		}
		return
	}
	var uid string
	err = s.db.QueryRow(ctx, `SELECT user_id FROM oauth_identities WHERE provider=$1 AND subject=$2`, p.Name, sub).Scan(&uid)
	if err == nil {
		if err := s.startSession(w, uid, time.Now()); err != nil {
			internal(w, err)
			return
		}
		back("/")
		return
	}
	if err != pgx.ErrNoRows {
		internal(w, err)
		return
	}
	st, err := s.tokens.Sign(security.Claims{Typ: security.TypSignup, Provider: p.Name, ProvSub: sub}, 30*time.Minute)
	if err != nil {
		internal(w, err)
		return
	}
	back("/auth/setup#" + st)
}
