package api

import (
	"context"
	"html"
	"net/http"

	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

func htmlEscape(s string) string { return html.EscapeString(s) }

func (s *Server) handleAdminStrains(w http.ResponseWriter, r *http.Request) {
	norm := NormalizeName(r.URL.Query().Get("q"))
	// Only strains that are visible to others (shared ratings) or reported. Names that exist only
	// in private logs are none of the moderation's business.
	rows, err := s.db.Query(r.Context(), `SELECT s.id, s.name, s.pinned_photo_id, (SELECT count(*) FROM public_ratings p WHERE p.strain_id=s.id)
		FROM strains s WHERE ($1='' OR s.name_norm LIKE '%'||$1||'%' OR similarity(s.name_norm,$1) > 0.3)
		  AND (EXISTS (SELECT 1 FROM public_ratings p WHERE p.strain_id=s.id)
		       OR EXISTS (SELECT 1 FROM reports rp WHERE rp.strain_id=s.id AND rp.status='open'))
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
		var p int
		rows.Scan(&id, &name, &pin, &p)
		out = append(out, map[string]any{"id": id, "name": name, "pinnedPhotoId": pin, "ratings": p})
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
	if err := s.renameStrain(r.Context(), id, in.Name, current(r).ID, ""); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var errStrainExists = errBad("Diese Sorte gibt es schon. Führe die beiden zusammen.")

// renameStrain changes the public name of a strain and logs it.
func (s *Server) renameStrain(ctx context.Context, id, newName, actorID, reason string) error {
	name := displayName(newName)
	norm := NormalizeName(name)
	if norm == "" {
		return errBad("Bitte gib einen Namen ein.")
	}
	if err := security.CheckStrainName(name); err != nil {
		return errBad(err.Error())
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var other string
		if err := tx.QueryRow(ctx, `SELECT id FROM strains WHERE name_norm=$1 AND id<>$2`, norm, id).Scan(&other); err == nil {
			return errStrainExists
		}
		var old string
		if err := tx.QueryRow(ctx, `SELECT name FROM strains WHERE id=$1`, id).Scan(&old); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE strains SET name=$2, name_norm=$3 WHERE id=$1`, id, name, norm); err != nil {
			return err
		}
		return logAction(ctx, tx, modAction{Action: "rename_strain", Reason: reason, StrainName: name, Content: &old, ActorID: actorID})
	})
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
		var from, into string
		tx.QueryRow(ctx, `SELECT (SELECT name FROM strains WHERE id=$1), (SELECT name FROM strains WHERE id=$2)`, in.FromID, in.IntoID).Scan(&from, &into)
		if err := logAction(ctx, tx, modAction{Action: "merge_strain", StrainName: into, Content: &from, ActorID: current(r).ID}); err != nil {
			return err
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
