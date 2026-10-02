package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/Br0kenByDesign/Greengrade/internal/media"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ---------- strain names ----------

// NormalizeName makes "Gorilla Glue #4", "gorilla-glue 4" and "GORILLA GLUE 4" the same strain.
func NormalizeName(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return b.String()
}

func displayName(s string) string {
	return strings.Join(strings.Fields(security.CleanText(s, 80)), " ")
}

func ensureStrain(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	name = displayName(name)
	norm := NormalizeName(name)
	if norm == "" {
		return "", errBad("Bitte gib den Namen der Sorte ein.")
	}
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO strains(name, name_norm) VALUES ($1,$2)
		ON CONFLICT (name_norm) DO UPDATE SET name_norm=EXCLUDED.name_norm RETURNING id`, name, norm).Scan(&id)
	return id, err
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	q := NormalizeName(r.URL.Query().Get("q"))
	out := []map[string]any{}
	if len([]rune(q)) < 2 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	rows, err := s.db.Query(r.Context(), `
		SELECT s.id, s.name, COALESCE(p.c,0), p.a,
		       EXISTS (SELECT 1 FROM entries e WHERE e.strain_id=s.id AND e.user_id=$2) AS mine
		FROM strains s
		LEFT JOIN (SELECT strain_id, count(*) c, round(avg(overall),1)::float8 a FROM public_ratings GROUP BY strain_id) p ON p.strain_id=s.id
		WHERE (p.c > 0 OR EXISTS (SELECT 1 FROM entries e WHERE e.strain_id=s.id AND e.user_id=$2)
		                OR EXISTS (SELECT 1 FROM grows g WHERE g.strain_id=s.id AND g.user_id=$2))
		  AND (s.name_norm LIKE '%' || $1 || '%' OR similarity(s.name_norm, $1) > 0.3)
		ORDER BY (s.name_norm LIKE $1 || '%') DESC, similarity(s.name_norm, $1) DESC, p.c DESC NULLS LAST
		LIMIT 8`, q, current(r).ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var c int
		var a *float64
		var mine bool
		if err := rows.Scan(&id, &name, &c, &a, &mine); err != nil {
			internal(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "count": c, "avg": a, "mine": mine})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---------- validation ----------

type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }
func errBad(m string) error        { return badRequest{m} }

func writeErr(w http.ResponseWriter, err error) {
	var b badRequest
	switch {
	case errors.As(err, &b):
		fail(w, http.StatusBadRequest, b.msg)
	case notFound(err):
		fail(w, http.StatusNotFound, "Nicht gefunden.")
	default:
		internal(w, err)
	}
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = displayName(t)
		if r := []rune(t); len(r) > 40 {
			t = string(r[:40])
		}
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
		if len(out) == 30 {
			break
		}
	}
	return out
}

func parseDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil || t.Year() < 1990 || t.Year() > 2100 {
		return nil, errBad("Ungültiges Datum.")
	}
	return &t, nil
}

func clampInt(p *int, lo, hi int) *int {
	if p == nil {
		return nil
	}
	v := max(lo, min(hi, *p))
	return &v
}

// details holds the source specific fields (pharmacy data, grow data, appearance, terpenes).
// They are free-form but limited in size and to flat values.
func validDetails(d map[string]any) (map[string]any, error) {
	if d == nil {
		return map[string]any{}, nil
	}
	if len(d) > 60 {
		return nil, errBad("Zu viele Angaben.")
	}
	out := map[string]any{}
	for k, v := range d {
		if len(k) > 40 {
			continue
		}
		switch x := v.(type) {
		case string:
			out[k] = security.CleanText(x, 200)
		case float64, bool, nil:
			out[k] = x
		case []any:
			var tags []string
			for _, t := range x {
				if s, ok := t.(string); ok {
					tags = append(tags, s)
				}
			}
			out[k] = cleanTags(tags)
		default:
			return nil, errBad("Ungültige Angaben.")
		}
	}
	if b, _ := json.Marshal(out); len(b) > 16<<10 {
		return nil, errBad("Zu viele Angaben.")
	}
	return out, nil
}

type tastingIn struct {
	TastedOn    string   `json:"tastedOn"`
	Method      string   `json:"method"`
	Temperature *int     `json:"temperature"`
	OnsetMin    *int     `json:"onsetMin"`
	DurationH   *float64 `json:"durationH"`
	Smell       int      `json:"smell"`
	Taste       int      `json:"taste"`
	Look        int      `json:"look"`
	Effect      int      `json:"effect"`
	Quality     int      `json:"quality"`
	Aromas      []string `json:"aromas"`
	Flavors     []string `json:"flavors"`
	Effects     []string `json:"effects"`
	SideEffects []string `json:"sideEffects"`
	Daytime     []string `json:"daytime"`
	Note        string   `json:"note"`
}

func (t *tastingIn) validate() error {
	for _, v := range []int{t.Smell, t.Taste, t.Look, t.Effect, t.Quality} {
		if v < 1 || v > 10 {
			return errBad("Bitte vergib in allen fünf Kategorien eine Note von 1 bis 10.")
		}
	}
	if t.TastedOn == "" {
		t.TastedOn = time.Now().Format("2006-01-02")
	}
	if _, err := parseDate(&t.TastedOn); err != nil {
		return err
	}
	t.Method = security.CleanText(t.Method, 40)
	t.Temperature = clampInt(t.Temperature, 0, 400)
	t.OnsetMin = clampInt(t.OnsetMin, 0, 600)
	if t.DurationH != nil {
		v := math.Max(0, math.Min(48, math.Round(*t.DurationH*10)/10))
		t.DurationH = &v
	}
	t.Aromas, t.Flavors, t.Effects = cleanTags(t.Aromas), cleanTags(t.Flavors), cleanTags(t.Effects)
	t.SideEffects, t.Daytime = cleanTags(t.SideEffects), cleanTags(t.Daytime)
	t.Note = security.CleanText(t.Note, 2000)
	return nil
}

func overall(a, b, c, d, e int) float64 {
	return math.Round(float64(a+b+c+d+e)/5*10) / 10
}

type entryIn struct {
	StrainName string         `json:"strainName"`
	GrowID     *string        `json:"growId"`
	Source     string         `json:"source"`
	Form       string         `json:"form"`
	Genetics   int            `json:"genetics"`
	Details    map[string]any `json:"details"`
	Notes      string         `json:"notes"`
	Rebuy      bool           `json:"rebuy"`
	Tasting    *tastingIn     `json:"tasting"`
}

func (e *entryIn) validate() error {
	if e.Source != "grow" && e.Source != "pharmacy" {
		return errBad("Bitte wähle, ob die Sorte aus der Apotheke oder aus deinem Grow kommt.")
	}
	if e.Form != "flower" && e.Form != "extract" {
		return errBad("Bitte wähle Blüten oder Extrakt.")
	}
	e.Genetics = max(0, min(100, e.Genetics))
	e.Notes = security.CleanText(e.Notes, 5000)
	if e.GrowID != nil && (*e.GrowID == "" || !validUUID(*e.GrowID)) {
		e.GrowID = nil
	}
	d, err := validDetails(e.Details)
	e.Details = d
	return err
}

// ---------- entries ----------

type entrySummary struct {
	ID           string         `json:"id"`
	StrainID     string         `json:"strainId"`
	StrainName   string         `json:"strainName"`
	GrowID       *string        `json:"growId"`
	Source       string         `json:"source"`
	Form         string         `json:"form"`
	Genetics     int            `json:"genetics"`
	Details      map[string]any `json:"details"`
	Rebuy        bool           `json:"rebuy"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	Latest       *tastingOut    `json:"latest"`
	TastingCount int            `json:"tastingCount"`
	CoverPhoto   *string        `json:"coverPhotoId"`
	Shared       bool           `json:"shared"`
}

type tastingOut struct {
	ID          string    `json:"id"`
	TastedOn    string    `json:"tastedOn"`
	Method      string    `json:"method"`
	Temperature *int      `json:"temperature"`
	OnsetMin    *int      `json:"onsetMin"`
	DurationH   *float64  `json:"durationH"`
	Smell       int       `json:"smell"`
	Taste       int       `json:"taste"`
	Look        int       `json:"look"`
	Effect      int       `json:"effect"`
	Quality     int       `json:"quality"`
	Overall     float64   `json:"overall"`
	Aromas      []string  `json:"aromas"`
	Flavors     []string  `json:"flavors"`
	Effects     []string  `json:"effects"`
	SideEffects []string  `json:"sideEffects"`
	Daytime     []string  `json:"daytime"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"createdAt"`
}

const tastingCols = `t.id, to_char(t.tasted_on,'YYYY-MM-DD'), t.method, t.temperature, t.onset_min, t.duration_h::float8,
	t.smell, t.taste, t.look, t.effect, t.quality, t.aromas, t.flavors, t.effects, t.side_effects, t.daytime, t.note, t.created_at`

func scanTasting(row interface{ Scan(...any) error }, t *tastingOut) error {
	err := row.Scan(&t.ID, &t.TastedOn, &t.Method, &t.Temperature, &t.OnsetMin, &t.DurationH,
		&t.Smell, &t.Taste, &t.Look, &t.Effect, &t.Quality, &t.Aromas, &t.Flavors, &t.Effects, &t.SideEffects, &t.Daytime, &t.Note, &t.CreatedAt)
	t.Overall = overall(t.Smell, t.Taste, t.Look, t.Effect, t.Quality)
	return err
}

func (s *Server) listEntries(ctx context.Context, uid string, onlyID string) ([]entrySummary, error) {
	q := `SELECT e.id, e.strain_id, st.name, e.grow_id, e.source, e.form, e.genetics, e.details, e.rebuy, e.created_at, e.updated_at,
		(SELECT count(*) FROM tastings x WHERE x.entry_id=e.id),
		(SELECT p.id FROM photos p WHERE p.entry_id=e.id ORDER BY p.created_at LIMIT 1),
		EXISTS (SELECT 1 FROM public_ratings pr WHERE pr.entry_id=e.id),
		lt.id IS NOT NULL, ` + strings.ReplaceAll(tastingCols, "t.", "lt.") + `
		FROM entries e JOIN strains st ON st.id=e.strain_id
		LEFT JOIN LATERAL (SELECT * FROM tastings t WHERE t.entry_id=e.id ORDER BY t.tasted_on DESC, t.created_at DESC LIMIT 1) lt ON true
		WHERE e.user_id=$1 AND ($2 = '' OR e.id::text = $2)
		ORDER BY COALESCE(lt.tasted_on, e.created_at::date) DESC, e.updated_at DESC`
	rows, err := s.db.Query(ctx, q, uid, onlyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entrySummary{}
	for rows.Next() {
		var e entrySummary
		var has bool
		var t tastingOut
		var tid, tdate, tmethod, tnote *string
		var tsm, tta, tlo, tef, tqu *int
		var tcreated *time.Time
		var tarom, tflav, teff, tside, tday []string
		if err := rows.Scan(&e.ID, &e.StrainID, &e.StrainName, &e.GrowID, &e.Source, &e.Form, &e.Genetics, &e.Details, &e.Rebuy,
			&e.CreatedAt, &e.UpdatedAt, &e.TastingCount, &e.CoverPhoto, &e.Shared, &has,
			&tid, &tdate, &tmethod, &t.Temperature, &t.OnsetMin, &t.DurationH, &tsm, &tta, &tlo, &tef, &tqu,
			&tarom, &tflav, &teff, &tside, &tday, &tnote, &tcreated); err != nil {
			return nil, err
		}
		if has {
			t.ID, t.TastedOn, t.Method, t.Note, t.CreatedAt = *tid, *tdate, *tmethod, *tnote, *tcreated
			t.Smell, t.Taste, t.Look, t.Effect, t.Quality = *tsm, *tta, *tlo, *tef, *tqu
			t.Aromas, t.Flavors, t.Effects, t.SideEffects, t.Daytime = tarom, tflav, teff, tside, tday
			t.Overall = overall(t.Smell, t.Taste, t.Look, t.Effect, t.Quality)
			e.Latest = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	list, err := s.listEntries(r.Context(), current(r).ID, "")
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type photoOut struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type shareOut struct {
	RatingID      string  `json:"ratingId"`
	Comment       *string `json:"comment"`
	CommentHidden bool    `json:"commentHidden"`
	PhotoID       *string `json:"photoId"`
	PhotoHidden   bool    `json:"photoHidden"`
}

func (s *Server) loadEntry(ctx context.Context, uid, id string) (map[string]any, error) {
	list, err := s.listEntries(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, pgx.ErrNoRows
	}
	e := list[0]
	var notes string
	if err := s.db.QueryRow(ctx, `SELECT notes FROM entries WHERE id=$1`, id).Scan(&notes); err != nil {
		return nil, err
	}
	tastings := []tastingOut{}
	rows, err := s.db.Query(ctx, `SELECT `+tastingCols+` FROM tastings t WHERE t.entry_id=$1 ORDER BY t.tasted_on DESC, t.created_at DESC`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t tastingOut
		if err := scanTasting(rows, &t); err != nil {
			rows.Close()
			return nil, err
		}
		tastings = append(tastings, t)
	}
	rows.Close()
	photos := []photoOut{}
	rows, err = s.db.Query(ctx, `SELECT id, width, height FROM photos WHERE entry_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p photoOut
		if err := rows.Scan(&p.ID, &p.Width, &p.Height); err != nil {
			rows.Close()
			return nil, err
		}
		photos = append(photos, p)
	}
	rows.Close()
	var share *shareOut
	var sh shareOut
	err = s.db.QueryRow(ctx, `SELECT id, comment, comment_hidden, photo_id, photo_hidden FROM public_ratings WHERE entry_id=$1`, id).
		Scan(&sh.RatingID, &sh.Comment, &sh.CommentHidden, &sh.PhotoID, &sh.PhotoHidden)
	if err == nil {
		share = &sh
	} else if !notFound(err) {
		return nil, err
	}
	return map[string]any{"entry": e, "notes": notes, "tastings": tastings, "photos": photos, "share": share}, nil
}

func (s *Server) handleEntryGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	e, err := s.loadEntry(r.Context(), current(r).ID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) checkGrow(ctx context.Context, tx pgx.Tx, uid string, gid *string) error {
	if gid == nil {
		return nil
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM grows WHERE id=$1 AND user_id=$2)`, *gid, uid).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return errBad("Unbekannter Grow.")
	}
	return nil
}

func insertTasting(ctx context.Context, tx pgx.Tx, uid, entryID string, t *tastingIn) error {
	_, err := tx.Exec(ctx, `INSERT INTO tastings(entry_id, user_id, tasted_on, method, temperature, onset_min, duration_h,
		smell, taste, look, effect, quality, aromas, flavors, effects, side_effects, daytime, note)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		entryID, uid, t.TastedOn, t.Method, t.Temperature, t.OnsetMin, t.DurationH,
		t.Smell, t.Taste, t.Look, t.Effect, t.Quality, t.Aromas, t.Flavors, t.Effects, t.SideEffects, t.Daytime, t.Note)
	return err
}

func (s *Server) handleEntryCreate(w http.ResponseWriter, r *http.Request) {
	var in entryIn
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	if in.Tasting == nil {
		writeErr(w, errBad("Bitte bewerte die Sorte."))
		return
	}
	if err := in.Tasting.validate(); err != nil {
		writeErr(w, err)
		return
	}
	u, ctx := current(r), r.Context()
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM entries WHERE user_id=$1`, u.ID).Scan(&n); err != nil || n >= 5000 {
		writeErr(w, errBad("Du hast die maximale Anzahl an Einträgen erreicht."))
		return
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err := s.checkGrow(ctx, tx, u.ID, in.GrowID); err != nil {
		writeErr(w, err)
		return
	}
	sid, err := ensureStrain(ctx, tx, in.StrainName)
	if err != nil {
		writeErr(w, err)
		return
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO entries(user_id, strain_id, grow_id, source, form, genetics, details, notes, rebuy)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		u.ID, sid, in.GrowID, in.Source, in.Form, in.Genetics, in.Details, in.Notes, in.Rebuy).Scan(&id); err != nil {
		internal(w, err)
		return
	}
	if err := insertTasting(ctx, tx, u.ID, id, in.Tasting); err != nil {
		internal(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	e, err := s.loadEntry(ctx, u.ID, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleEntryUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in entryIn
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if err := s.checkGrow(ctx, tx, u.ID, in.GrowID); err != nil {
		writeErr(w, err)
		return
	}
	sid, err := ensureStrain(ctx, tx, in.StrainName)
	if err != nil {
		writeErr(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE entries SET strain_id=$3, grow_id=$4, source=$5, form=$6, genetics=$7, details=$8, notes=$9, rebuy=$10, updated_at=now()
		WHERE id=$1 AND user_id=$2`, id, u.ID, sid, in.GrowID, in.Source, in.Form, in.Genetics, in.Details, in.Notes, in.Rebuy)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	if in.Tasting != nil {
		if err := in.Tasting.validate(); err != nil {
			writeErr(w, err)
			return
		}
		var tid string
		err := tx.QueryRow(ctx, `SELECT id FROM tastings WHERE entry_id=$1 ORDER BY tasted_on DESC, created_at DESC LIMIT 1`, id).Scan(&tid)
		if err == nil {
			err = updateTasting(ctx, tx, tid, in.Tasting)
		} else if notFound(err) {
			err = insertTasting(ctx, tx, u.ID, id, in.Tasting)
		}
		if err != nil {
			internal(w, err)
			return
		}
	}
	if err := syncPublic(ctx, tx, id); err != nil {
		internal(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	e, err := s.loadEntry(ctx, u.ID, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func updateTasting(ctx context.Context, tx pgx.Tx, tid string, t *tastingIn) error {
	_, err := tx.Exec(ctx, `UPDATE tastings SET tasted_on=$2, method=$3, temperature=$4, onset_min=$5, duration_h=$6,
		smell=$7, taste=$8, look=$9, effect=$10, quality=$11, aromas=$12, flavors=$13, effects=$14, side_effects=$15, daytime=$16, note=$17
		WHERE id=$1`, tid, t.TastedOn, t.Method, t.Temperature, t.OnsetMin, t.DurationH,
		t.Smell, t.Taste, t.Look, t.Effect, t.Quality, t.Aromas, t.Flavors, t.Effects, t.SideEffects, t.Daytime, t.Note)
	return err
}

// syncPublic keeps the public rating of an entry in line with its strain and latest tasting.
func syncPublic(ctx context.Context, tx pgx.Tx, entryID string) error {
	var prID, uid, sid string
	err := tx.QueryRow(ctx, `SELECT pr.id, e.user_id, e.strain_id FROM public_ratings pr JOIN entries e ON e.id=pr.entry_id WHERE pr.entry_id=$1`, entryID).Scan(&prID, &uid, &sid)
	if notFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var sm, ta, lo, ef, qu int
	err = tx.QueryRow(ctx, `SELECT smell, taste, look, effect, quality FROM tastings WHERE entry_id=$1 ORDER BY tasted_on DESC, created_at DESC LIMIT 1`, entryID).
		Scan(&sm, &ta, &lo, &ef, &qu)
	if notFound(err) {
		_, err = tx.Exec(ctx, `DELETE FROM public_ratings WHERE id=$1`, prID)
		return err
	}
	if err != nil {
		return err
	}
	// one public rating per user and strain: a newer shared entry replaces an older one
	if _, err := tx.Exec(ctx, `DELETE FROM public_ratings WHERE user_id=$1 AND strain_id=$2 AND id<>$3`, uid, sid, prID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE public_ratings SET strain_id=$2, overall=$3, smell=$4, taste=$5, look=$6, effect=$7, quality=$8, updated_at=now() WHERE id=$1`,
		prID, sid, overall(sm, ta, lo, ef, qu), sm, ta, lo, ef, qu)
	return err
}

func (s *Server) entryPhotos(ctx context.Context, q queryer, sql string, args ...any) ([]string, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Server) handleEntryDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ctx := current(r), r.Context()
	ids, err := s.entryPhotos(ctx, s.db, `SELECT p.id FROM photos p JOIN entries e ON e.id=p.entry_id WHERE e.id=$1 AND e.user_id=$2`, id, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM entries WHERE id=$1 AND user_id=$2`, id, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	for _, p := range ids {
		s.photos.Delete(p)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- tastings ----------

func (s *Server) handleTastingCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in tastingIn
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var n int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM tastings WHERE entry_id=e.id) FROM entries e WHERE e.id=$1 AND e.user_id=$2`, id, u.ID).Scan(&n); err != nil {
		writeErr(w, err)
		return
	}
	if n >= 200 {
		writeErr(w, errBad("Zu viele Verkostungen für diesen Eintrag."))
		return
	}
	if err := insertTasting(ctx, tx, u.ID, id, &in); err != nil {
		internal(w, err)
		return
	}
	tx.Exec(ctx, `UPDATE entries SET updated_at=now() WHERE id=$1`, id)
	if err := syncPublic(ctx, tx, id); err != nil {
		internal(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	e, err := s.loadEntry(ctx, u.ID, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) tastingEntry(ctx context.Context, tx pgx.Tx, tid, uid string) (string, error) {
	var eid string
	err := tx.QueryRow(ctx, `SELECT entry_id FROM tastings WHERE id=$1 AND user_id=$2`, tid, uid).Scan(&eid)
	return eid, err
}

func (s *Server) handleTastingUpdate(w http.ResponseWriter, r *http.Request) {
	tid, ok := pathID(w, r)
	if !ok {
		return
	}
	var in tastingIn
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, err)
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	eid, err := s.tastingEntry(ctx, tx, tid, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := updateTasting(ctx, tx, tid, &in); err != nil {
		internal(w, err)
		return
	}
	if err := syncPublic(ctx, tx, eid); err != nil {
		internal(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	e, err := s.loadEntry(ctx, u.ID, eid)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleTastingDelete(w http.ResponseWriter, r *http.Request) {
	tid, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	eid, err := s.tastingEntry(ctx, tx, tid, u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var n int
	tx.QueryRow(ctx, `SELECT count(*) FROM tastings WHERE entry_id=$1`, eid).Scan(&n)
	if n <= 1 {
		writeErr(w, errBad("Die letzte Verkostung kann nicht gelöscht werden. Lösche stattdessen den Eintrag."))
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tastings WHERE id=$1`, tid); err != nil {
		internal(w, err)
		return
	}
	if err := syncPublic(ctx, tx, eid); err != nil {
		internal(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- sharing ----------

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Shared  bool    `json:"shared"`
		Comment *string `json:"comment"`
		PhotoID *string `json:"photoId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, ctx := current(r), r.Context()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var sid string
	if err := tx.QueryRow(ctx, `SELECT strain_id FROM entries WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, u.ID).Scan(&sid); err != nil {
		writeErr(w, err)
		return
	}
	if !in.Shared {
		if _, err := tx.Exec(ctx, `DELETE FROM public_ratings WHERE entry_id=$1`, id); err != nil {
			internal(w, err)
			return
		}
	} else {
		var comment *string
		if in.Comment != nil {
			c := security.CleanText(*in.Comment, 500)
			if c != "" {
				if err := security.CheckPublicComment(c); err != nil {
					writeErr(w, errBad(err.Error()))
					return
				}
				comment = &c
			}
		}
		var photo *string
		if in.PhotoID != nil && *in.PhotoID != "" {
			var ok bool
			if !validUUID(*in.PhotoID) {
				writeErr(w, errBad("Unbekanntes Foto."))
				return
			}
			tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM photos WHERE id=$1 AND entry_id=$2)`, *in.PhotoID, id).Scan(&ok)
			if !ok {
				writeErr(w, errBad("Unbekanntes Foto."))
				return
			}
			photo = in.PhotoID
		}
		var sm, ta, lo, ef, qu int
		err := tx.QueryRow(ctx, `SELECT smell, taste, look, effect, quality FROM tastings WHERE entry_id=$1 ORDER BY tasted_on DESC, created_at DESC LIMIT 1`, id).
			Scan(&sm, &ta, &lo, &ef, &qu)
		if err != nil {
			writeErr(w, errBad("Bewerte die Sorte zuerst, bevor du sie teilst."))
			return
		}
		if _, err := tx.Exec(ctx, `DELETE FROM public_ratings WHERE user_id=$1 AND strain_id=$2 AND entry_id<>$3`, u.ID, sid, id); err != nil {
			internal(w, err)
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO public_ratings(strain_id, user_id, entry_id, overall, smell, taste, look, effect, quality, comment, photo_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (entry_id) DO UPDATE SET
				strain_id=EXCLUDED.strain_id, overall=EXCLUDED.overall, smell=EXCLUDED.smell, taste=EXCLUDED.taste, look=EXCLUDED.look,
				effect=EXCLUDED.effect, quality=EXCLUDED.quality,
				comment_hidden = CASE WHEN public_ratings.comment IS DISTINCT FROM EXCLUDED.comment THEN false ELSE public_ratings.comment_hidden END,
				photo_hidden = CASE WHEN public_ratings.photo_id IS DISTINCT FROM EXCLUDED.photo_id THEN false ELSE public_ratings.photo_hidden END,
				comment=EXCLUDED.comment, photo_id=EXCLUDED.photo_id, updated_at=now()`,
			sid, u.ID, id, overall(sm, ta, lo, ef, qu), sm, ta, lo, ef, qu, comment, photo)
		if err != nil {
			internal(w, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	e, err := s.loadEntry(ctx, u.ID, id)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// ---------- photos ----------

func (s *Server) handlePhotoUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, ctx := current(r), r.Context()
	if !s.upLim.Allow(u.ID) {
		fail(w, http.StatusTooManyRequests, "Zu viele Uploads. Bitte warte etwas.")
		return
	}
	var perEntry, perUser int
	if err := s.db.QueryRow(ctx, `SELECT (SELECT count(*) FROM photos WHERE entry_id=e.id), (SELECT count(*) FROM photos WHERE user_id=$2)
		FROM entries e WHERE e.id=$1 AND e.user_id=$2`, id, u.ID).Scan(&perEntry, &perUser); err != nil {
		writeErr(w, err)
		return
	}
	if perEntry >= 12 {
		writeErr(w, errBad("Pro Eintrag sind höchstens 12 Fotos möglich."))
		return
	}
	if perUser >= 2000 {
		writeErr(w, errBad("Du hast die maximale Anzahl an Fotos erreicht."))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, media.MaxUploadBytes+(1<<20))
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, errBad("Kein Foto empfangen oder Datei zu groß (max. 15 MB)."))
		return
	}
	defer f.Close()
	res, err := media.Process(io.Reader(f))
	if err != nil {
		if errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrTooLarge) {
			writeErr(w, errBad(err.Error()))
			return
		}
		internal(w, err)
		return
	}
	var pid string
	if err := s.db.QueryRow(ctx, `INSERT INTO photos(user_id, entry_id, width, height) VALUES ($1,$2,$3,$4) RETURNING id`,
		u.ID, id, res.Width, res.Height).Scan(&pid); err != nil {
		internal(w, err)
		return
	}
	if err := s.photos.Save(pid, res); err != nil {
		s.db.Exec(ctx, `DELETE FROM photos WHERE id=$1`, pid)
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, photoOut{ID: pid, Width: res.Width, Height: res.Height})
}

func (s *Server) handlePhotoDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tag, err := s.db.Exec(r.Context(), `DELETE FROM photos WHERE id=$1 AND user_id=$2`, id, current(r).ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	s.photos.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

// Photos are private unless attached to a visible public rating.
func (s *Server) handlePhotoGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u := current(r)
	var allowed bool
	err := s.db.QueryRow(r.Context(), `SELECT
		EXISTS (SELECT 1 FROM photos WHERE id=$1 AND user_id=$2)
		OR EXISTS (SELECT 1 FROM public_ratings WHERE photo_id=$1 AND (NOT photo_hidden OR $3))`, id, u.ID, u.Admin).Scan(&allowed)
	if err != nil || !allowed {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	f, err := s.photos.Open(id, r.URL.Query().Get("size") == "thumb")
	if err != nil {
		fail(w, http.StatusNotFound, "Nicht gefunden.")
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("Content-Disposition", "inline")
	io.Copy(w, f)
}
