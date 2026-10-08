package api

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handlePublicList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	norm := NormalizeName(q.Get("q"))
	order := map[string]string{
		// Bayesian average: few ratings are pulled towards the overall mean, so 1x 10/10 does not win.
		"best":  `(agg.a*agg.c + g.m*5)/(agg.c+5) DESC, agg.c DESC`,
		"count": `agg.c DESC, agg.a DESC`,
		"new":   `agg.first_at DESC`,
		"mine":  `agg.last_at DESC`,
	}[q.Get("sort")]
	if order == "" {
		order = `(agg.a*agg.c + g.m*5)/(agg.c+5) DESC, agg.c DESC`
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	offset = max(0, min(offset, 100000))
	rows, err := s.db.Query(r.Context(), `
		WITH agg AS (SELECT strain_id, count(*) c, avg(overall) a, min(created_at) first_at, max(updated_at) last_at
		             FROM public_ratings GROUP BY strain_id),
		     g AS (SELECT COALESCE(avg(overall), 7) m FROM public_ratings)
		SELECT s.id, s.name, agg.c, round(agg.a,1)::float8, cover.photo_id,
		       EXISTS (SELECT 1 FROM public_ratings m WHERE m.strain_id=s.id AND m.user_id=$2)
		FROM agg JOIN strains s ON s.id=agg.strain_id CROSS JOIN g
		LEFT JOIN LATERAL (SELECT pr.photo_id FROM public_ratings pr
			WHERE pr.strain_id=s.id AND pr.photo_id IS NOT NULL AND NOT pr.photo_hidden
			ORDER BY (pr.photo_id = s.pinned_photo_id) DESC NULLS LAST, pr.updated_at DESC LIMIT 1) cover ON true
		WHERE ($1 = '' OR s.name_norm LIKE '%' || $1 || '%')
		  AND (NOT $3 OR EXISTS (SELECT 1 FROM public_ratings m WHERE m.strain_id=s.id AND m.user_id=$2))
		ORDER BY `+order+` LIMIT 60 OFFSET $4`, norm, current(r).ID, q.Get("sort") == "mine", offset)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name string
		var c int
		var a float64
		var cover *string
		var mine bool
		if err := rows.Scan(&id, &name, &c, &a, &cover, &mine); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "count": c, "avg": a, "coverPhotoId": cover, "mine": mine})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePublicStrain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ctx := current(r), r.Context()
	var name string
	var c int
	var a, sm, ta, lo, ef, qu *float64
	var since *time.Time
	err := s.db.QueryRow(ctx, `SELECT s.name, count(pr.id), round(avg(pr.overall),1)::float8,
		round(avg(pr.smell),1)::float8, round(avg(pr.taste),1)::float8, round(avg(pr.look),1)::float8,
		round(avg(pr.effect),1)::float8, round(avg(pr.quality),1)::float8, min(pr.created_at)
		FROM strains s LEFT JOIN public_ratings pr ON pr.strain_id=s.id WHERE s.id=$1 GROUP BY s.id`, id).
		Scan(&name, &c, &a, &sm, &ta, &lo, &ef, &qu, &since)
	if err != nil {
		writeErr(w, err)
		return
	}
	hist := make([]int, 10)
	rows, err := s.db.Query(ctx, `SELECT LEAST(10, GREATEST(1, round(overall)))::int b, count(*) FROM public_ratings WHERE strain_id=$1 GROUP BY b`, id)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var b, n int
		rows.Scan(&b, &n)
		hist[b-1] = n
	}
	rows.Close()
	comments := []map[string]any{}
	rows, err = s.db.Query(ctx, `SELECT id, overall::float8, comment, to_char(updated_at,'YYYY-MM'), user_id=$2
		FROM public_ratings WHERE strain_id=$1 AND comment IS NOT NULL AND comment <> '' AND NOT comment_hidden
		ORDER BY updated_at DESC LIMIT 200`, id, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var rid, text, month string
		var ov float64
		var mine bool
		rows.Scan(&rid, &ov, &text, &month, &mine)
		comments = append(comments, map[string]any{"ratingId": rid, "overall": ov, "comment": text, "month": month, "mine": mine})
	}
	rows.Close()
	photos := []map[string]any{}
	rows, err = s.db.Query(ctx, `SELECT pr.id, pr.photo_id, pr.user_id=$2 FROM public_ratings pr JOIN strains s ON s.id=pr.strain_id
		WHERE pr.strain_id=$1 AND pr.photo_id IS NOT NULL AND NOT pr.photo_hidden
		ORDER BY (pr.photo_id = s.pinned_photo_id) DESC NULLS LAST, pr.updated_at DESC LIMIT 24`, id, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var rid, pid string
		var mine bool
		rows.Scan(&rid, &pid, &mine)
		photos = append(photos, map[string]any{"ratingId": rid, "photoId": pid, "mine": mine})
	}
	rows.Close()
	var mine map[string]any
	var mo float64
	var me string
	if err := s.db.QueryRow(ctx, `SELECT overall::float8, entry_id FROM public_ratings WHERE strain_id=$1 AND user_id=$2`, id, u.ID).Scan(&mo, &me); err == nil {
		mine = map[string]any{"overall": mo, "entryId": me}
	}
	var sinceStr *string
	if since != nil {
		v := since.Format("2006-01")
		sinceStr = &v
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "count": c, "avg": a, "since": sinceStr,
		"categories": map[string]*float64{"smell": sm, "taste": ta, "look": lo, "effect": ef, "quality": qu},
		"histogram":  hist, "comments": comments, "photos": photos, "mine": mine,
	})
}
