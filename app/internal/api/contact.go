package api

import (
	"context"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/mail"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
)

// validEmail returns the normalized address if it is a single plain address.
func validEmail(in string) (string, bool) {
	email := security.NormalizeEmail(in)
	addr, err := netmail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") ||
		!strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "", false
	}
	return email, true
}

// handleContact forwards a message to CONTACT_EMAIL. Nothing is stored. Mail only ever goes to
// the operator, never to the address entered, so the form cannot be used to send mail to others.
func (s *Server) handleContact(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ContactMail == "" {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Message string `json:"message"`
		PoW     string `json:"pow"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	email, ok := validEmail(in.Email)
	if !ok {
		fail(w, http.StatusBadRequest, "Bitte gib eine gültige E-Mail-Adresse ein, damit wir antworten können.")
		return
	}
	name := strings.ReplaceAll(security.CleanText(in.Name, 80), "\n", " ")
	msg := security.CleanText(in.Message, 4000)
	if len([]rune(msg)) < 10 {
		fail(w, http.StatusBadRequest, "Bitte schreib uns etwas mehr.")
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
	ip := s.ips.LimitKey(r)
	for _, l := range []struct {
		key    string
		limit  int
		window time.Duration
	}{
		{"contact-ip:" + ip, 3, time.Hour},
		{"contact-ip-day:" + ip, 10, 24 * time.Hour},
		{"contact-all", 50, 24 * time.Hour},
	} {
		ok, err := s.rateOK(ctx, l.key, l.limit, l.window)
		if err != nil {
			internal(w, err)
			return
		}
		if !ok {
			fail(w, http.StatusTooManyRequests, "Gerade sind schon viele Nachrichten eingegangen. Bitte versuche es später erneut oder schreib uns per E-Mail.")
			return
		}
	}
	from := email
	if name != "" {
		from = name + " <" + email + ">"
	}
	m := mail.Message{
		To:      s.cfg.ContactMail,
		ReplyTo: email,
		Subject: "greengrade: Nachricht über das Kontaktformular",
		Text:    "Von: " + from + "\n\n" + msg + "\n\n-- \nAntworten geht direkt an die angegebene Adresse.",
		HTML: `<div style="font-family:system-ui,sans-serif;font-size:15px;line-height:1.5;color:#18211B;max-width:620px">` +
			`<p style="color:#626C64">Von: ` + htmlEscape(from) + `</p>` +
			`<p style="white-space:pre-wrap">` + htmlEscape(msg) + `</p>` +
			`<p style="color:#626C64;font-size:13px">Antworten geht direkt an die angegebene Adresse.</p></div>`,
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 25*time.Second)
	defer cancel()
	if err := s.mail.Send(sendCtx, m); err != nil {
		slog.Error("kontakt-mail fehlgeschlagen", "err", err)
		fail(w, http.StatusBadGateway, "Die Nachricht konnte gerade nicht verschickt werden. Bitte versuche es später erneut.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
