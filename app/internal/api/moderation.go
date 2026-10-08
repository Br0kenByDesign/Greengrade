package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/Br0kenByDesign/Greengrade/internal/mail"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

var reportReasons = map[string]string{
	"sale":     "Verkaufs- oder Tauschangebot",
	"personal": "Persönliche Daten zu sehen",
	"abuse":    "Beleidigend oder hetzerisch",
	"spam":     "Spam oder Werbung",
	"other":    "Anderer Grund",
}

// ---------- fingerprints ----------

// normalizeComment ignores case, spaces and punctuation, so trivial variations match.
func normalizeComment(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Server) commentFP(text string) []byte {
	m := hmac.New(sha256.New, security.Derive(s.cfg.AppSecret, "comment-fp"))
	m.Write([]byte(normalizeComment(text)))
	return m.Sum(nil)
}

// commentScope: long comments are blocked for everyone, short generic ones ("sehr lecker")
// only for their author, so other people are not affected.
func commentScope(text string, authorID string) string {
	if len([]rune(normalizeComment(text))) >= 20 || authorID == "" {
		return ""
	}
	return authorID
}

// photoHash returns the stored content hash, computing it for photos uploaded before 1.2.0.
func (s *Server) photoHash(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconnCommandTag, error)
}, photoID string) ([]byte, error) {
	var h []byte
	if err := q.QueryRow(ctx, `SELECT content_hash FROM photos WHERE id=$1`, photoID).Scan(&h); err != nil {
		return nil, err
	}
	if len(h) > 0 {
		return h, nil
	}
	f, err := s.photos.Open(photoID, false)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return nil, err
	}
	h = sum.Sum(nil)
	_, err = q.Exec(ctx, `UPDATE photos SET content_hash=$2 WHERE id=$1`, photoID, h)
	return h, err
}

// shareAllowed checks a public comment and photo against bans and earlier moderation decisions.
func (s *Server) shareAllowed(ctx context.Context, tx pgx.Tx, userID string, comment *string, photoID *string) error {
	var banned bool
	if err := tx.QueryRow(ctx, `SELECT share_banned_at IS NOT NULL FROM users WHERE id=$1`, userID).Scan(&banned); err != nil {
		return err
	}
	if banned {
		return errBad("Dein Konto kann derzeit nichts öffentlich teilen. Die Begründung findest du unter Konto bei den Hinweisen.")
	}
	if comment != nil {
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM moderation_blocks WHERE kind='comment' AND fingerprint=$1 AND scope IN ('', $2))`,
			s.commentFP(*comment), userID).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return errBad("Dieser Kommentar wurde von der Moderation ausgeblendet und kann nicht erneut geteilt werden.")
		}
	}
	if photoID != nil {
		h, err := s.photoHash(ctx, txAdapter{tx}, *photoID)
		if err != nil {
			return err
		}
		var blocked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM moderation_blocks WHERE kind='photo' AND fingerprint=$1)`, h).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return errBad("Dieses Foto wurde von der Moderation ausgeblendet und kann nicht erneut geteilt werden.")
		}
	}
	return nil
}

// ---------- log ----------

type modAction struct {
	Action, Reason, Note, StrainName string
	Content                          *string
	TargetUserID, ActorID            string
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func logAction(ctx context.Context, tx pgx.Tx, a modAction) error {
	_, err := tx.Exec(ctx, `INSERT INTO moderation_actions(action, reason, note, strain_name, content, target_user_id, actor_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, a.Action, a.Reason, a.Note, a.StrainName, a.Content, nullable(a.TargetUserID), nullable(a.ActorID))
	return err
}

func notify(ctx context.Context, tx pgx.Tx, userID, title, body string) error {
	if userID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO notices(user_id, title, body) VALUES ($1,$2,$3)`, userID, title, body)
	return err
}

// redress is appended to every decision that affects an author (Art. 17 DSA).
const redress = " Hältst du die Entscheidung für falsch, kannst du widersprechen. Wie das geht, steht in den Nutzungsbedingungen."

// closeReports resolves matching open reports and tells each reporter the outcome (Art. 16 Abs. 5 DSA).
// The reporter learns nothing about the author.
func closeReports(ctx context.Context, tx pgx.Tx, status, strain, outcome, where string, args ...any) error {
	rows, err := tx.Query(ctx, `UPDATE reports SET status='`+status+`', resolved_at=now() WHERE status='open' AND (`+where+`) RETURNING reporter_id::text`, args...)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var rid *string
		if err := rows.Scan(&rid); err != nil {
			rows.Close()
			return err
		}
		if rid != nil {
			seen[*rid] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for rid := range seen {
		if err := notify(ctx, tx, rid, "Deine Meldung zu "+strain+" wurde geprüft", outcome+" Danke, dass du hilfst, greengrade sauber zu halten."); err != nil {
			return err
		}
	}
	return nil
}

// ---------- hiding and bans ----------

type modItem struct {
	ReportID, RatingID string
	Target             string // comment | photo
	StrainName         string
	AuthorID           string
	Content            *string
	PhotoID            *string
}

func (s *Server) moderate(ctx context.Context, actorID string, it modItem, reasonKey, note string, ban bool) error {
	reason := reportReasons[reasonKey]
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var what string
		switch it.Target {
		case "comment":
			what = "Dein Kommentar"
			if it.Content != nil && *it.Content != "" {
				fp := s.commentFP(*it.Content)
				if _, err := tx.Exec(ctx, `INSERT INTO moderation_blocks(kind, fingerprint, scope) VALUES ('comment',$1,$2) ON CONFLICT DO NOTHING`,
					fp, commentScope(*it.Content, it.AuthorID)); err != nil {
					return err
				}
				// hide every copy of the same text (all users for long texts, otherwise only the author's)
				if _, err := tx.Exec(ctx, `UPDATE public_ratings SET comment_hidden=true
					WHERE comment_fp=$1 AND ($2='' OR user_id::text=$2)`, fp, commentScope(*it.Content, it.AuthorID)); err != nil {
					return err
				}
			}
			if it.RatingID != "" {
				if _, err := tx.Exec(ctx, `UPDATE public_ratings SET comment_hidden=true WHERE id=$1`, it.RatingID); err != nil {
					return err
				}
			}
		case "photo":
			what = "Dein Foto"
			if it.PhotoID != nil {
				h, err := s.photoHash(ctx, txAdapter{tx}, *it.PhotoID)
				if err != nil && !notFound(err) {
					return err
				}
				if len(h) > 0 {
					if _, err := tx.Exec(ctx, `INSERT INTO moderation_blocks(kind, fingerprint) VALUES ('photo',$1) ON CONFLICT DO NOTHING`, h); err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `UPDATE public_ratings SET photo_hidden=true
						WHERE photo_id IN (SELECT id FROM photos WHERE content_hash=$1)`, h); err != nil {
						return err
					}
				}
			}
			if it.RatingID != "" {
				if _, err := tx.Exec(ctx, `UPDATE public_ratings SET photo_hidden=true WHERE id=$1`, it.RatingID); err != nil {
					return err
				}
			}
		default:
			return errBad("Ungültige Angaben.")
		}
		if err := closeReports(ctx, tx, "hidden", it.StrainName, "Der gemeldete Inhalt wurde ausgeblendet.",
			`target=$1 AND (id::text=$2 OR ($3<>'' AND public_rating_id::text=$3))`,
			it.Target, it.ReportID, it.RatingID); err != nil {
			return err
		}
		action := "hide_" + it.Target
		var content *string
		if it.Target == "comment" {
			content = it.Content
		}
		if err := logAction(ctx, tx, modAction{Action: action, Reason: reason, Note: note, StrainName: it.StrainName,
			Content: content, TargetUserID: it.AuthorID, ActorID: actorID}); err != nil {
			return err
		}
		body := "Grund: " + reason + "."
		if note != "" {
			body += " " + note
		}
		body += " Deine Note zählt weiterhin. Derselbe Inhalt kann nicht erneut geteilt werden." + redress
		if err := notify(ctx, tx, it.AuthorID, what+" zu "+it.StrainName+" wurde ausgeblendet", body); err != nil {
			return err
		}
		if ban && it.AuthorID != "" {
			return banShare(ctx, tx, it.AuthorID, reason, note, actorID)
		}
		return nil
	})
}

// banShare stops an account from sharing publicly. The private log stays fully usable.
func banShare(ctx context.Context, tx pgx.Tx, userID, reason, note, actorID string) error {
	tag, err := tx.Exec(ctx, `UPDATE users SET share_banned_at=now(), share_ban_reason=$2 WHERE id=$1 AND share_banned_at IS NULL`, userID, reason)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM public_ratings WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if err := logAction(ctx, tx, modAction{Action: "share_ban", Reason: reason, Note: note, TargetUserID: userID, ActorID: actorID}); err != nil {
		return err
	}
	body := "Grund: " + reason + "."
	if note != "" {
		body += " " + note
	}
	body += " Deine bisherigen öffentlichen Bewertungen wurden entfernt. Dein privates Logbuch kannst du wie gewohnt weiter nutzen." + redress
	return notify(ctx, tx, userID, "Dein Konto kann vorerst nichts mehr öffentlich teilen", body)
}

// anonKey shows admins a stable short label instead of names or e-mail addresses.
func anonKey(userID *string) string {
	if userID == nil {
		return ""
	}
	sum := sha256.Sum256([]byte("greengrade-user:" + *userID))
	return strings.ToUpper(hex.EncodeToString(sum[:3]))
}

// ---------- reports ----------

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RatingID string `json:"ratingId"`
		StrainID string `json:"strainId"`
		Target   string `json:"target"`
		Reason   string `json:"reason"`
		Details  string `json:"details"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	valid := reportReasons[in.Reason] != "" &&
		((in.Target == "comment" || in.Target == "photo") && validUUID(in.RatingID) ||
			in.Target == "name" && validUUID(in.StrainID))
	if !valid {
		writeErr(w, errBad("Ungültige Meldung."))
		return
	}
	u, ctx := current(r), r.Context()
	if ok, err := s.rateOK(ctx, "report:"+u.ID, 20, 24*time.Hour); err != nil {
		internal(w, err)
		return
	} else if !ok {
		fail(w, http.StatusTooManyRequests, "Du hast heute schon viele Meldungen geschickt. Danke! Bitte versuche es morgen wieder.")
		return
	}
	details := security.CleanText(in.Details, 500)
	var strain string
	var err error
	if in.Target == "name" {
		err = s.db.QueryRow(ctx, `INSERT INTO reports(target, reason, details, reporter_id, strain_id, strain_name)
			SELECT 'name', $2, $3, $4, st.id, st.name FROM strains st WHERE st.id=$1
			AND EXISTS (SELECT 1 FROM public_ratings p WHERE p.strain_id=st.id)
			AND NOT EXISTS (SELECT 1 FROM reports x WHERE x.strain_id=st.id AND x.target='name' AND x.reporter_id=$4 AND x.status='open')
			RETURNING strain_name`, in.StrainID, in.Reason, details, u.ID).Scan(&strain)
	} else {
		// snapshot: the report keeps what was reported, even if the rating is withdrawn later
		err = s.db.QueryRow(ctx, `INSERT INTO reports(public_rating_id, target, reason, details, reporter_id, strain_id, strain_name, author_id, content, photo_id)
			SELECT pr.id, $2, $3, $4, $5, st.id, st.name, pr.user_id,
			       CASE WHEN $2='comment' THEN pr.comment END, CASE WHEN $2='photo' THEN pr.photo_id END
			FROM public_ratings pr JOIN strains st ON st.id=pr.strain_id WHERE pr.id=$1 AND pr.user_id<>$5
			AND NOT EXISTS (SELECT 1 FROM reports x WHERE x.public_rating_id=pr.id AND x.target=$2 AND x.reporter_id=$5 AND x.status='open')
			RETURNING strain_name`, in.RatingID, in.Target, in.Reason, details, u.ID).Scan(&strain)
	}
	if err != nil && !notFound(err) {
		internal(w, err)
		return
	}
	if err == nil && s.cfg.ModerationMail != "" {
		go func(strain, reason string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			link := s.cfg.AppOrigin + "/admin"
			if err := s.mail.Send(ctx, mail.Message{
				To:      s.cfg.ModerationMail,
				Subject: "greengrade: neue Meldung",
				Text:    "Neue Meldung zu " + strain + " (" + reason + ").\n\nPrüfen: " + link,
				HTML:    "<p>Neue Meldung zu <b>" + htmlEscape(strain) + "</b> (" + htmlEscape(reason) + ").</p><p><a href=\"" + link + "\">Meldungen prüfen</a></p>",
			}); err != nil {
				slog.Error("meldungs-mail fehlgeschlagen", "err", err)
			}
		}(strain, reportReasons[in.Reason])
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "hidden" && status != "dismissed" {
		status = "open"
	}
	rows, err := s.db.Query(r.Context(), `SELECT rp.id, rp.target, rp.reason, rp.details, rp.created_at,
		rp.public_rating_id, rp.strain_id, rp.strain_name, rp.content, rp.photo_id, rp.author_id,
		COALESCE(pr.comment_hidden, false), COALESCE(pr.photo_hidden, false),
		(SELECT count(*) FROM moderation_actions m WHERE m.target_user_id=rp.author_id AND m.action LIKE 'hide_%' AND m.created_at > now() - interval '365 days'),
		COALESCE((SELECT share_banned_at IS NOT NULL FROM users WHERE id=rp.author_id), false),
		(SELECT count(*) FROM reports x WHERE x.status='open' AND x.target=rp.target AND
			((rp.public_rating_id IS NOT NULL AND x.public_rating_id=rp.public_rating_id) OR (rp.target='name' AND x.strain_id=rp.strain_id)))
		FROM reports rp LEFT JOIN public_ratings pr ON pr.id=rp.public_rating_id
		WHERE rp.status=$1 ORDER BY rp.created_at DESC LIMIT 200`, status)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, target, reason, details, sname string
		var created time.Time
		var rid, sid, content, photo, author *string
		var ch, ph, banned bool
		var strikes, open int
		if err := rows.Scan(&id, &target, &reason, &details, &created, &rid, &sid, &sname, &content, &photo, &author,
			&ch, &ph, &strikes, &banned, &open); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "target": target, "reason": reason, "reasonLabel": reportReasons[reason],
			"details": details, "createdAt": created, "ratingId": rid, "strainId": sid, "strainName": sname,
			"content": content, "photoId": photo, "withdrawn": target != "name" && rid == nil,
			"hidden": (target == "comment" && ch) || (target == "photo" && ph),
			"author": anonKey(author), "authorStrikes": strikes, "authorBanned": banned, "openReports": open})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminResolve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"` // hide | hide_ban | dismiss | rename
		Note   string `json:"note"`
		Name   string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, ctx := current(r), r.Context()
	var target, reason, sname string
	var rid, sid, author, content, photo *string
	if err := s.db.QueryRow(ctx, `SELECT target, reason, public_rating_id, strain_id, strain_name, author_id, content, photo_id
		FROM reports WHERE id=$1`, id).Scan(&target, &reason, &rid, &sid, &sname, &author, &content, &photo); err != nil {
		writeErr(w, err)
		return
	}
	note := security.CleanText(in.Note, 300)
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	var err error
	switch {
	case in.Action == "dismiss":
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			if err := closeReports(ctx, tx, "dismissed", sname, "Wir konnten keinen Verstoß gegen die Nutzungsbedingungen oder geltendes Recht feststellen. Der Inhalt bleibt sichtbar.",
				`target=$1 AND (id=$2 OR ($3::uuid IS NOT NULL AND public_rating_id=$3) OR ($1='name' AND strain_id=$4))`, target, id, rid, sid); err != nil {
				return err
			}
			return logAction(ctx, tx, modAction{Action: "dismiss_" + target, Reason: reportReasons[reason], Note: note, StrainName: sname, ActorID: u.ID})
		})
	case target == "name" && in.Action == "rename":
		if sid == nil {
			err = errBad("Diese Sorte gibt es nicht mehr.")
			break
		}
		if err = s.renameStrain(ctx, *sid, in.Name, u.ID, reportReasons[reason]); err == nil {
			err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
				return closeReports(ctx, tx, "hidden", sname, "Der gemeldete Sortenname wurde geändert.", `target='name' AND strain_id=$1`, *sid)
			})
		}
	case target != "name" && (in.Action == "hide" || in.Action == "hide_ban"):
		err = s.moderate(ctx, u.ID, modItem{ReportID: id, RatingID: deref(rid), Target: target, StrainName: sname,
			AuthorID: deref(author), Content: content, PhotoID: photo}, reason, note, in.Action == "hide_ban")
	default:
		err = errBad("Unbekannte Aktion.")
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAdminHide hides content directly from a strain page (without a report).
func (s *Server) handleAdminHide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Target string `json:"target"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
		Ban    bool   `json:"ban"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if (in.Target != "comment" && in.Target != "photo") || reportReasons[in.Reason] == "" {
		writeErr(w, errBad("Ungültige Angaben."))
		return
	}
	ctx := r.Context()
	it := modItem{RatingID: id, Target: in.Target}
	if err := s.db.QueryRow(ctx, `SELECT st.name, pr.user_id, pr.comment, pr.photo_id FROM public_ratings pr JOIN strains st ON st.id=pr.strain_id WHERE pr.id=$1`, id).
		Scan(&it.StrainName, &it.AuthorID, &it.Content, &it.PhotoID); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.moderate(ctx, current(r).ID, it, in.Reason, security.CleanText(in.Note, 300), in.Ban); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminBans(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id::text, share_banned_at, share_ban_reason,
		(SELECT count(*) FROM moderation_actions m WHERE m.target_user_id=u.id AND m.action LIKE 'hide_%')
		FROM users u WHERE share_banned_at IS NOT NULL ORDER BY share_banned_at DESC`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, reason string
		var since time.Time
		var strikes int
		if err := rows.Scan(&id, &since, &reason, &strikes); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "key": anonKey(&id), "since": since, "reason": reason, "strikes": strikes})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminUnban(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET share_banned_at=NULL, share_ban_reason='' WHERE id=$1 AND share_banned_at IS NOT NULL`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		if err := logAction(ctx, tx, modAction{Action: "share_unban", TargetUserID: id, ActorID: current(r).ID}); err != nil {
			return err
		}
		return notify(ctx, tx, id, "Du kannst wieder öffentlich teilen", "Die Sperre für öffentliche Bewertungen wurde aufgehoben.")
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminLog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT action, reason, note, strain_name, content, target_user_id::text, created_at
		FROM moderation_actions ORDER BY created_at DESC LIMIT 300`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var action, reason, note, strain string
		var content, target *string
		var at time.Time
		if err := rows.Scan(&action, &reason, &note, &strain, &content, &target, &at); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"action": action, "reason": reason, "note": note, "strainName": strain,
			"content": content, "account": anonKey(target), "createdAt": at})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------- small helpers ----------

type pgconnCommandTag = interface{ RowsAffected() int64 }

// txAdapter lets photoHash work with both the pool and a transaction.
type txAdapter struct{ tx pgx.Tx }

func (a txAdapter) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return a.tx.QueryRow(ctx, sql, args...)
}
func (a txAdapter) Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error) {
	return a.tx.Exec(ctx, sql, args...)
}
