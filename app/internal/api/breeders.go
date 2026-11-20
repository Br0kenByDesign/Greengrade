package api

import (
	"context"
	"net/http"

	"github.com/Br0kenByDesign/Greengrade/internal/names"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

// Moderation of manufacturer and breeder names. Like strains, only names that are visible to others
// (strains with shared ratings or open reports) are listed.
func (s *Server) handleAdminBreeders(w http.ResponseWriter, r *http.Request) {
	q := names.Breeder(r.URL.Query().Get("q"))
	rows, err := s.db.Query(r.Context(), `SELECT b.id, b.name,
		(SELECT count(*) FROM strains s WHERE s.breeder_id=b.id AND EXISTS (SELECT 1 FROM public_ratings p WHERE p.strain_id=s.id)),
		(SELECT count(*) FROM public_ratings p JOIN strains s ON s.id=p.strain_id WHERE s.breeder_id=b.id)
		FROM breeders b
		WHERE ($1='' OR b.name_norm LIKE '%'||$1||'%' OR similarity(b.name_norm,$1) > 0.3)
		  AND EXISTS (SELECT 1 FROM strains s WHERE s.breeder_id=b.id AND (
		        EXISTS (SELECT 1 FROM public_ratings p WHERE p.strain_id=s.id)
		        OR EXISTS (SELECT 1 FROM reports rp WHERE rp.strain_id=s.id AND rp.status='open')))
		ORDER BY 4 DESC, b.name LIMIT 100`, q)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var strains, ratings int
		rows.Scan(&id, &name, &strains, &ratings)
		out = append(out, map[string]any{"id": id, "name": name, "strains": strains, "ratings": ratings})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminBreederRename(w http.ResponseWriter, r *http.Request) {
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
	if err := s.renameBreeder(r.Context(), id, in.Name, current(r).ID, ""); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminBreederMerge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FromID string `json:"fromId"`
		IntoID string `json:"intoId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !validUUID(in.FromID) || !validUUID(in.IntoID) || in.FromID == in.IntoID {
		writeErr(w, errBad("Bitte wähle zwei verschiedene Hersteller bzw. Breeder."))
		return
	}
	ctx := r.Context()
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var from, into string
		if err := tx.QueryRow(ctx, `SELECT (SELECT name FROM breeders WHERE id=$1), (SELECT name FROM breeders WHERE id=$2)`,
			in.FromID, in.IntoID).Scan(&from, &into); err != nil {
			return pgx.ErrNoRows
		}
		if err := logAction(ctx, tx, modAction{Action: "merge_breeder", StrainName: into, Content: &from, ActorID: current(r).ID}); err != nil {
			return err
		}
		return mergeBreeders(ctx, tx, in.FromID, in.IntoID)
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// renameBreeder changes a manufacturer or breeder name everywhere. If the new name already exists,
// both are merged and the new spelling is kept.
func (s *Server) renameBreeder(ctx context.Context, id, newName, actorID, reason string) error {
	name := displayName(newName)
	norm := names.Breeder(name)
	if norm == "" {
		return errBad("Bitte gib einen Namen ein.")
	}
	if err := security.CheckBreederName(name); err != nil {
		return errBad(err.Error())
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var old string
		if err := tx.QueryRow(ctx, `SELECT name FROM breeders WHERE id=$1`, id).Scan(&old); err != nil {
			return err
		}
		if err := logAction(ctx, tx, modAction{Action: "rename_breeder", Reason: reason, StrainName: name, Content: &old, ActorID: actorID}); err != nil {
			return err
		}
		var other string
		if err := tx.QueryRow(ctx, `SELECT id FROM breeders WHERE name_norm=$1 AND id<>$2`, norm, id).Scan(&other); err == nil {
			if err := mergeBreeders(ctx, tx, id, other); err != nil {
				return err
			}
			id = other
		} else if !notFound(err) {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE breeders SET name=$2, name_norm=$3 WHERE id=$1`, id, name, norm)
		return err
	})
}

// mergeBreeders moves all strains of one breeder to another. "Lemon Sorbet" of both becomes one strain.
func mergeBreeders(ctx context.Context, tx pgx.Tx, from, into string) error {
	rows, err := tx.Query(ctx, `SELECT s.id, t.id FROM strains s LEFT JOIN strains t ON t.name_norm=s.name_norm AND t.breeder_id=$2
		WHERE s.breeder_id=$1`, from, into)
	if err != nil {
		return err
	}
	type pair struct {
		from string
		into *string
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.from, &p.into); err != nil {
			rows.Close()
			return err
		}
		pairs = append(pairs, p)
	}
	rows.Close()
	for _, p := range pairs {
		if p.into != nil {
			if err := mergeStrains(ctx, tx, p.from, *p.into); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `UPDATE strains SET breeder_id=$2 WHERE id=$1`, p.from, into); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM breeders WHERE id=$1`, from)
	return err
}
