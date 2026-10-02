package api

import (
	"context"
	"html"
	"net/http"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

func htmlEscape(s string) string { return html.EscapeString(s) }

func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "hidden" && status != "dismissed" {
		status = "open"
	}
	rows, err := s.db.Query(r.Context(), `SELECT rp.id, rp.target, rp.reason, rp.details, rp.created_at,
		pr.id, s.id, s.name, pr.comment, pr.comment_hidden, pr.photo_id, pr.photo_hidden,
		(SELECT count(*) FROM reports x WHERE x.public_rating_id=pr.id AND x.target=rp.target AND x.status='open')
		FROM reports rp JOIN public_ratings pr ON pr.id=rp.public_rating_id JOIN strains s ON s.id=pr.strain_id
		WHERE rp.status=$1 ORDER BY rp.created_at DESC LIMIT 200`, status)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, target, reason, details, rid, sid, sname string
		var created time.Time
		var comment, photo *string
		var ch, ph bool
		var n int
		if err := rows.Scan(&id, &target, &reason, &details, &created, &rid, &sid, &sname, &comment, &ch, &photo, &ph, &n); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "target": target, "reason": reason, "reasonLabel": reportReasons[reason],
			"details": details, "createdAt": created, "ratingId": rid, "strainId": sid, "strainName": sname,
			"comment": comment, "commentHidden": ch, "photoId": photo, "photoHidden": ph, "openReports": n})
	}
	writeJSON(w, http.StatusOK, out)
}

// hideContent hides a comment or photo, closes open reports on it and informs the author in the app.
func (s *Server) hideContent(ctx context.Context, ratingID, target, reason, note string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	col, what := "comment_hidden", "Dein Kommentar"
	if target == "photo" {
		col, what = "photo_hidden", "Dein Foto"
	}
	var uid, strain string
	if err := tx.QueryRow(ctx, `UPDATE public_ratings pr SET `+col+`=true FROM strains s
		WHERE pr.id=$1 AND s.id=pr.strain_id RETURNING pr.user_id, s.name`, ratingID).Scan(&uid, &strain); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE reports SET status='hidden', resolved_at=now() WHERE public_rating_id=$1 AND target=$2 AND status='open'`, ratingID, target); err != nil {
		return err
	}
	body := "Grund: " + reason + "."
	if note != "" {
		body += " " + note
	}
	body += " Deine Note zählt weiterhin. Wenn du den Inhalt in deinem Eintrag änderst, wird er wieder angezeigt und kann erneut geprüft werden."
	if _, err := tx.Exec(ctx, `INSERT INTO notices(user_id, title, body) VALUES ($1,$2,$3)`, uid, what+" zu "+strain+" wurde ausgeblendet", body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) handleAdminResolve(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	var rid, target, reason string
	if err := s.db.QueryRow(ctx, `SELECT public_rating_id, target, reason FROM reports WHERE id=$1`, id).Scan(&rid, &target, &reason); err != nil {
		writeErr(w, err)
		return
	}
	switch in.Action {
	case "hide":
		if err := s.hideContent(ctx, rid, target, reportReasons[reason], security.CleanText(in.Note, 300)); err != nil {
			writeErr(w, err)
			return
		}
	case "dismiss":
		if _, err := s.db.Exec(ctx, `UPDATE reports SET status='dismissed', resolved_at=now() WHERE public_rating_id=$1 AND target=$2 AND status='open'`, rid, target); err != nil {
			internal(w, err)
			return
		}
	default:
		writeErr(w, errBad("Unbekannte Aktion."))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminHide(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Target string `json:"target"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if (in.Target != "comment" && in.Target != "photo") || reportReasons[in.Reason] == "" {
		writeErr(w, errBad("Ungültige Angaben."))
		return
	}
	if err := s.hideContent(r.Context(), id, in.Target, reportReasons[in.Reason], security.CleanText(in.Note, 300)); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminStrains(w http.ResponseWriter, r *http.Request) {
	norm := NormalizeName(r.URL.Query().Get("q"))
	rows, err := s.db.Query(r.Context(), `SELECT s.id, s.name, s.pinned_photo_id,
		(SELECT count(*) FROM entries e WHERE e.strain_id=s.id), (SELECT count(*) FROM public_ratings p WHERE p.strain_id=s.id)
		FROM strains s WHERE ($1='' OR s.name_norm LIKE '%'||$1||'%' OR similarity(s.name_norm,$1) > 0.3)
		ORDER BY (SELECT count(*) FROM public_ratings p WHERE p.strain_id=s.id) DESC, s.name LIMIT 100`, norm)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var pin *string
		var e, p int
		rows.Scan(&id, &name, &pin, &e, &p)
		out = append(out, map[string]any{"id": id, "name": name, "pinnedPhotoId": pin, "entries": e, "ratings": p})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminStrainRename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name := displayName(in.Name)
	norm := NormalizeName(name)
	if norm == "" {
		writeErr(w, errBad("Bitte gib einen Namen ein."))
		return
	}
	var other string
	err := s.db.QueryRow(r.Context(), `SELECT id FROM strains WHERE name_norm=$1 AND id<>$2`, norm, id).Scan(&other)
	if err == nil {
		fail(w, http.StatusConflict, "Diese Sorte gibt es schon. Führe die beiden zusammen.")
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE strains SET name=$2, name_norm=$3 WHERE id=$1`, id, name, norm); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminMerge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FromID string `json:"fromId"`
		IntoID string `json:"intoId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validUUID(in.FromID) || !validUUID(in.IntoID) || in.FromID == in.IntoID {
		writeErr(w, errBad("Bitte wähle zwei verschiedene Sorten."))
		return
	}
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM strains WHERE id IN ($1,$2)`, in.FromID, in.IntoID).Scan(&n); err != nil {
			return err
		}
		if n != 2 {
			return pgx.ErrNoRows
		}
		// a user with ratings on both keeps the newer one
		if _, err := tx.Exec(ctx, `DELETE FROM public_ratings a USING public_ratings b
			WHERE a.user_id=b.user_id AND a.strain_id IN ($1,$2) AND b.strain_id IN ($1,$2) AND a.id<>b.id
			AND (a.updated_at, a.id) < (b.updated_at, b.id)`, in.FromID, in.IntoID); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE public_ratings SET strain_id=$2 WHERE strain_id=$1`,
			`UPDATE entries SET strain_id=$2 WHERE strain_id=$1`,
			`UPDATE grows SET strain_id=$2 WHERE strain_id=$1`,
			`UPDATE strains SET pinned_photo_id = COALESCE(pinned_photo_id, (SELECT pinned_photo_id FROM strains WHERE id=$1)) WHERE id=$2`,
			`DELETE FROM strains WHERE id=$1`,
		} {
			if _, err := tx.Exec(ctx, q, in.FromID, in.IntoID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminStrainPhotos(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.Query(r.Context(), `SELECT pr.photo_id FROM public_ratings pr WHERE pr.strain_id=$1 AND pr.photo_id IS NOT NULL AND NOT pr.photo_hidden ORDER BY pr.updated_at DESC`, id)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		out = append(out, p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminPin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		PhotoID *string `json:"photoId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if in.PhotoID != nil {
		var ok bool
		if !validUUID(*in.PhotoID) {
			writeErr(w, errBad("Unbekanntes Foto."))
			return
		}
		s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public_ratings WHERE strain_id=$1 AND photo_id=$2 AND NOT photo_hidden)`, id, *in.PhotoID).Scan(&ok)
		if !ok {
			writeErr(w, errBad("Nur öffentlich geteilte Fotos dieser Sorte können festgelegt werden."))
			return
		}
	}
	if _, err := s.db.Exec(ctx, `UPDATE strains SET pinned_photo_id=$2 WHERE id=$1`, id, in.PhotoID); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
