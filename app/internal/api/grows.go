package api

import (
	"net/http"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

type growOut struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	StrainID           *string    `json:"strainId"`
	StrainName         *string    `json:"strainName"`
	Location           string     `json:"location"`
	SeedType           string     `json:"seedType"`
	Environment        string     `json:"environment"`
	Medium             string     `json:"medium"`
	LightWatts         *int       `json:"lightWatts"`
	GerminatedOn       *string    `json:"germinatedOn"`
	FloweringOn        *string    `json:"floweringOn"`
	ExpectedFlowerDays *int       `json:"expectedFlowerDays"`
	HarvestedOn        *string    `json:"harvestedOn"`
	DryDays            *int       `json:"dryDays"`
	CureWeeks          *int       `json:"cureWeeks"`
	YieldGrams         *int       `json:"yieldGrams"`
	Notes              string     `json:"notes"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	EntryCount         int        `json:"entryCount"`
	LastLogAt          *time.Time `json:"lastLogAt"`
}

const growCols = `g.id, g.name, g.strain_id, s.name, g.location, g.seed_type, g.environment, g.medium, g.light_watts,
	to_char(g.germinated_on,'YYYY-MM-DD'), to_char(g.flowering_on,'YYYY-MM-DD'), g.expected_flower_days,
	to_char(g.harvested_on,'YYYY-MM-DD'), g.dry_days, g.cure_weeks, g.yield_grams, g.notes, g.created_at, g.updated_at,
	(SELECT count(*) FROM entries e WHERE e.grow_id=g.id), (SELECT max(created_at) FROM grow_logs l WHERE l.grow_id=g.id)`

func scanGrow(row interface{ Scan(...any) error }, g *growOut) error {
	return row.Scan(&g.ID, &g.Name, &g.StrainID, &g.StrainName, &g.Location, &g.SeedType, &g.Environment, &g.Medium, &g.LightWatts,
		&g.GerminatedOn, &g.FloweringOn, &g.ExpectedFlowerDays, &g.HarvestedOn, &g.DryDays, &g.CureWeeks, &g.YieldGrams,
		&g.Notes, &g.CreatedAt, &g.UpdatedAt, &g.EntryCount, &g.LastLogAt)
}

func (s *Server) handleGrows(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT `+growCols+` FROM grows g LEFT JOIN strains s ON s.id=g.strain_id
		WHERE g.user_id=$1 ORDER BY (g.harvested_on IS NULL) DESC, COALESCE(g.flowering_on, g.germinated_on, g.created_at::date) DESC`, current(r).ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	out := []growOut{}
	for rows.Next() {
		var g growOut
		if err := scanGrow(rows, &g); err != nil {
			internal(w, err)
			return
		}
		out = append(out, g)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGrowGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ctx := current(r), r.Context()
	var g growOut
	if err := scanGrow(s.db.QueryRow(ctx, `SELECT `+growCols+` FROM grows g LEFT JOIN strains s ON s.id=g.strain_id WHERE g.id=$1 AND g.user_id=$2`, id, u.ID), &g); err != nil {
		writeErr(w, err)
		return
	}
	logs := []map[string]any{}
	rows, err := s.db.Query(ctx, `SELECT id, to_char(logged_on,'YYYY-MM-DD'), text FROM grow_logs WHERE grow_id=$1 ORDER BY logged_on DESC, created_at DESC`, id)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var lid, d, t string
		rows.Scan(&lid, &d, &t)
		logs = append(logs, map[string]any{"id": lid, "loggedOn": d, "text": t})
	}
	rows.Close()
	all, err := s.listEntries(ctx, u.ID, "")
	if err != nil {
		internal(w, err)
		return
	}
	entries := []entrySummary{}
	for _, e := range all {
		if e.GrowID != nil && *e.GrowID == id {
			entries = append(entries, e)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"grow": g, "logs": logs, "entries": entries})
}

func (s *Server) handleGrowSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name               string  `json:"name"`
		StrainName         string  `json:"strainName"`
		Location           string  `json:"location"`
		SeedType           string  `json:"seedType"`
		Environment        string  `json:"environment"`
		Medium             string  `json:"medium"`
		LightWatts         *int    `json:"lightWatts"`
		GerminatedOn       *string `json:"germinatedOn"`
		FloweringOn        *string `json:"floweringOn"`
		ExpectedFlowerDays *int    `json:"expectedFlowerDays"`
		HarvestedOn        *string `json:"harvestedOn"`
		DryDays            *int    `json:"dryDays"`
		CureWeeks          *int    `json:"cureWeeks"`
		YieldGrams         *int    `json:"yieldGrams"`
		Notes              string  `json:"notes"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var dates [3]*time.Time
	for i, d := range []*string{in.GerminatedOn, in.FloweringOn, in.HarvestedOn} {
		t, err := parseDate(d)
		if err != nil {
			writeErr(w, err)
			return
		}
		dates[i] = t
	}
	name := displayName(in.Name)
	if name == "" {
		name = displayName(in.StrainName)
	}
	if name == "" {
		writeErr(w, errBad("Bitte gib dem Grow einen Namen oder wähle die Sorte."))
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var sid *string
	if NormalizeName(in.StrainName) != "" {
		id, err := ensureStrain(ctx, tx, in.StrainName)
		if err != nil {
			writeErr(w, err)
			return
		}
		sid = &id
	}
	args := []any{u.ID, name, sid, security.CleanText(in.Location, 60), security.CleanText(in.SeedType, 40),
		security.CleanText(in.Environment, 40), security.CleanText(in.Medium, 40), clampInt(in.LightWatts, 0, 100000),
		dates[0], dates[1], clampInt(in.ExpectedFlowerDays, 0, 365), dates[2], clampInt(in.DryDays, 0, 365),
		clampInt(in.CureWeeks, 0, 520), clampInt(in.YieldGrams, 0, 1000000), security.CleanText(in.Notes, 5000)}
	var id string
	if r.Method == http.MethodPost {
		var n int
		tx.QueryRow(ctx, `SELECT count(*) FROM grows WHERE user_id=$1`, u.ID).Scan(&n)
		if n >= 500 {
			writeErr(w, errBad("Du hast die maximale Anzahl an Grows erreicht."))
			return
		}
		err = tx.QueryRow(ctx, `INSERT INTO grows(user_id, name, strain_id, location, seed_type, environment, medium, light_watts,
			germinated_on, flowering_on, expected_flower_days, harvested_on, dry_days, cure_weeks, yield_grams, notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, args...).Scan(&id)
	} else {
		var ok bool
		if id, ok = pathID(w, r); !ok {
			return
		}
		err = tx.QueryRow(ctx, `UPDATE grows SET name=$2, strain_id=$3, location=$4, seed_type=$5, environment=$6, medium=$7, light_watts=$8,
			germinated_on=$9, flowering_on=$10, expected_flower_days=$11, harvested_on=$12, dry_days=$13, cure_weeks=$14, yield_grams=$15,
			notes=$16, updated_at=now() WHERE user_id=$1 AND id=$17 RETURNING id`, append(args, id)...).Scan(&id)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	var g growOut
	if err := scanGrow(s.db.QueryRow(ctx, `SELECT `+growCols+` FROM grows g LEFT JOIN strains s ON s.id=g.strain_id WHERE g.id=$1`, id), &g); err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleGrowDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM grows WHERE id=$1 AND user_id=$2`, id, current(r).ID)
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

func (s *Server) handleGrowLogCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		LoggedOn string `json:"loggedOn"`
		Text     string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	text := security.CleanText(in.Text, 2000)
	if text == "" {
		writeErr(w, errBad("Bitte schreib etwas."))
		return
	}
	if in.LoggedOn == "" {
		in.LoggedOn = time.Now().Format("2006-01-02")
	}
	if _, err := parseDate(&in.LoggedOn); err != nil {
		writeErr(w, err)
		return
	}
	u, ctx := current(r), r.Context()
	var lid string
	err := s.db.QueryRow(ctx, `INSERT INTO grow_logs(grow_id, user_id, logged_on, text)
		SELECT id, user_id, $3, $4 FROM grows WHERE id=$1 AND user_id=$2 AND (SELECT count(*) FROM grow_logs WHERE grow_id=$1) < 1000
		RETURNING id`, id, u.ID, in.LoggedOn, text).Scan(&lid)
	if err == pgx.ErrNoRows {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	} else if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": lid, "loggedOn": in.LoggedOn, "text": text})
}

func (s *Server) handleGrowLogDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM grow_logs WHERE id=$1 AND user_id=$2`, id, current(r).ID); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
