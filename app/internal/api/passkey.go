package api

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type waUser struct {
	id    []byte
	name  string
	creds []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { return u.id }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// The WebAuthn user handle is the raw 16 bytes of the user's UUID.
func uuidBytes(id string) []byte {
	b, _ := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return b
}

func uuidString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (s *Server) loadWAUser(ctx context.Context, uid string) (*waUser, error) {
	u := &waUser{id: uuidBytes(uid)}
	if err := s.db.QueryRow(ctx, `SELECT display_name FROM users WHERE id=$1`, uid).Scan(&u.name); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT credential FROM passkeys WHERE user_id=$1`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var c webauthn.Credential
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &c) == nil {
			u.creds = append(u.creds, c)
		}
	}
	return u, rows.Err()
}

func (s *Server) saveCeremony(w http.ResponseWriter, mode, uid string, sess *webauthn.SessionData) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	c := security.Claims{Typ: security.TypWebAuthn, Mode: mode, Data: data}
	c.Subject = uid
	tok, err := s.tokens.Sign(c, 5*time.Minute)
	if err != nil {
		return err
	}
	s.setCookie(w, cookieWebAuthn, tok, 5*time.Minute)
	return nil
}

func (s *Server) loadCeremony(w http.ResponseWriter, r *http.Request, mode string) (*security.Claims, *webauthn.SessionData, error) {
	ck, err := r.Cookie(cookieWebAuthn)
	s.clearCookie(w, cookieWebAuthn)
	if err != nil {
		return nil, nil, errors.New("no ceremony")
	}
	c, err := s.tokens.Verify(ck.Value, security.TypWebAuthn)
	if err != nil || c.Mode != mode {
		return nil, nil, errors.New("bad ceremony")
	}
	var sess webauthn.SessionData
	if err := json.Unmarshal(c.Data, &sess); err != nil {
		return nil, nil, err
	}
	return c, &sess, nil
}

func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	opts, sess, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationPreferred))
	if err != nil {
		internal(w, err)
		return
	}
	if err := s.saveCeremony(w, "login", "", sess); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	_, sess, err := s.loadCeremony(w, r, "login")
	if err != nil {
		fail(w, http.StatusBadRequest, "Die Anmeldung ist abgelaufen. Bitte versuche es erneut.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	ctx := r.Context()
	var uid string
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		uid = uuidString(userHandle)
		var owner string
		if err := s.db.QueryRow(ctx, `SELECT user_id FROM passkeys WHERE id=$1`, rawID).Scan(&owner); err != nil || owner != uid {
			return nil, errors.New("unbekannter Passkey")
		}
		return s.loadWAUser(ctx, uid)
	}
	_, cred, err := s.wa.FinishPasskeyLogin(handler, *sess, r)
	if err != nil {
		slog.Info("passkey-login abgelehnt", "err", err)
		fail(w, http.StatusUnauthorized, "Dieser Passkey ist uns nicht bekannt. Melde dich per Link an und lege einen neuen an.")
		return
	}
	if cred.Authenticator.CloneWarning {
		slog.Warn("passkey: möglicher Klon erkannt (Signaturzähler)")
	}
	raw, _ := json.Marshal(cred)
	if _, err := s.db.Exec(ctx, `UPDATE passkeys SET credential=$2, last_used_at=now() WHERE id=$1`, cred.ID, raw); err != nil {
		internal(w, err)
		return
	}
	if err := s.startSession(w, uid); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	wu, err := s.loadWAUser(r.Context(), u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if len(wu.creds) >= 20 {
		fail(w, http.StatusConflict, "Du hast bereits 20 Passkeys. Entferne zuerst einen alten.")
		return
	}
	excl := make([]protocol.CredentialDescriptor, 0, len(wu.creds))
	for i := range wu.creds {
		excl = append(excl, wu.creds[i].Descriptor())
	}
	opts, sess, err := s.wa.BeginRegistration(wu,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(excl))
	if err != nil {
		internal(w, err)
		return
	}
	if err := s.saveCeremony(w, "register", u.ID, sess); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	c, sess, err := s.loadCeremony(w, r, "register")
	if err != nil || c.Subject != u.ID {
		fail(w, http.StatusBadRequest, "Die Einrichtung ist abgelaufen. Bitte versuche es erneut.")
		return
	}
	ctx := r.Context()
	wu, err := s.loadWAUser(ctx, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	cred, err := s.wa.FinishRegistration(wu, *sess, r)
	if err != nil {
		slog.Info("passkey-registrierung abgelehnt", "err", err)
		fail(w, http.StatusBadRequest, "Der Passkey konnte nicht angelegt werden.")
		return
	}
	name := security.CleanText(r.URL.Query().Get("name"), 60)
	if name == "" {
		name = "Passkey vom " + time.Now().Format("02.01.2006")
	}
	raw, _ := json.Marshal(cred)
	if _, err := s.db.Exec(ctx, `INSERT INTO passkeys(id, user_id, name, credential) VALUES ($1,$2,$3,$4)`, cred.ID, u.ID, name, raw); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": base64.RawURLEncoding.EncodeToString(cred.ID), "name": name})
}

// loginMethods counts how a user can still sign in (passkeys, social logins, e-mail).
func (s *Server) loginMethods(ctx context.Context, u *user) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM passkeys WHERE user_id=$1) + (SELECT count(*) FROM oauth_identities WHERE user_id=$1)`, u.ID).Scan(&n)
	if u.HasMail {
		n++
	}
	return n, err
}

func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	id, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	if n, err := s.loginMethods(r.Context(), u); err != nil {
		internal(w, err)
		return
	} else if n <= 1 {
		fail(w, http.StatusConflict, "Das ist deine letzte Anmeldemethode. Lege zuerst eine andere an.")
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM passkeys WHERE id=$1 AND user_id=$2`, id, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleIdentityDelete(w http.ResponseWriter, r *http.Request) {
	u := current(r)
	if n, err := s.loginMethods(r.Context(), u); err != nil {
		internal(w, err)
		return
	} else if n <= 1 {
		fail(w, http.StatusConflict, "Das ist deine letzte Anmeldemethode. Lege zuerst einen Passkey an.")
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM oauth_identities WHERE user_id=$1 AND provider=$2`, u.ID, r.PathValue("provider")); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
