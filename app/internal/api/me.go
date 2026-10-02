package api

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/security"
)

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ctx := current(r), r.Context()
	passkeys := []map[string]any{}
	rows, err := s.db.Query(ctx, `SELECT id, name, created_at, last_used_at FROM passkeys WHERE user_id=$1 ORDER BY created_at`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var id []byte
		var name string
		var created time.Time
		var used *time.Time
		rows.Scan(&id, &name, &created, &used)
		passkeys = append(passkeys, map[string]any{"id": base64.RawURLEncoding.EncodeToString(id), "name": name, "createdAt": created, "lastUsedAt": used})
	}
	rows.Close()
	identities := []string{}
	rows, err = s.db.Query(ctx, `SELECT provider FROM oauth_identities WHERE user_id=$1 ORDER BY provider`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var p string
		rows.Scan(&p)
		identities = append(identities, p)
	}
	rows.Close()
	notices := []map[string]any{}
	rows, err = s.db.Query(ctx, `SELECT id, title, body, created_at, read_at IS NOT NULL FROM notices WHERE user_id=$1 ORDER BY created_at DESC LIMIT 20`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var id, title, body string
		var created time.Time
		var read bool
		rows.Scan(&id, &title, &body, &created, &read)
		notices = append(notices, map[string]any{"id": id, "title": title, "body": body, "createdAt": created, "read": read})
	}
	rows.Close()
	writeJSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "displayName": u.Name, "isAdmin": u.Admin, "hasEmail": u.HasMail,
		"passkeys": passkeys, "identities": identities, "providers": s.oauth.Enabled(), "notices": notices,
	})
}

func (s *Server) handleMeUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DisplayName string `json:"displayName"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	name := security.CleanText(in.DisplayName, 40)
	if name == "" {
		writeErr(w, errBad("Bitte gib einen Namen ein."))
		return
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET display_name=$2 WHERE id=$1`, current(r).ID, name); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNoticesRead(w http.ResponseWriter, r *http.Request) {
	s.db.Exec(r.Context(), `UPDATE notices SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, current(r).ID)
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteAccount removes everything immediately: rows cascade, files are deleted afterwards.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Confirm != "LÖSCHEN" {
		writeErr(w, errBad("Bitte tippe LÖSCHEN zur Bestätigung."))
		return
	}
	u, ctx := current(r), r.Context()
	ids, err := s.entryPhotos(ctx, s.db, `SELECT id FROM photos WHERE user_id=$1`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		internal(w, err)
		return
	}
	for _, id := range ids {
		s.photos.Delete(id)
	}
	s.clearCookie(w, cookieAccess)
	s.clearCookie(w, cookieRefresh)
	w.WriteHeader(http.StatusNoContent)
}

// handleExport streams a ZIP with all personal data and original-size photos.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	u, ctx := current(r), r.Context()
	entries, err := s.listEntries(ctx, u.ID, "")
	if err != nil {
		internal(w, err)
		return
	}
	full := []map[string]any{}
	for _, e := range entries {
		x, err := s.loadEntry(ctx, u.ID, e.ID)
		if err != nil {
			internal(w, err)
			return
		}
		full = append(full, x)
	}
	grows := []growOut{}
	rows, err := s.db.Query(ctx, `SELECT `+growCols+` FROM grows g LEFT JOIN strains s ON s.id=g.strain_id WHERE g.user_id=$1`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var g growOut
		if scanGrow(rows, &g) == nil {
			grows = append(grows, g)
		}
	}
	rows.Close()
	logs := []map[string]any{}
	rows, err = s.db.Query(ctx, `SELECT grow_id, to_char(logged_on,'YYYY-MM-DD'), text FROM grow_logs WHERE user_id=$1 ORDER BY logged_on`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var g, d, t string
		rows.Scan(&g, &d, &t)
		logs = append(logs, map[string]any{"growId": g, "loggedOn": d, "text": t})
	}
	rows.Close()
	var created time.Time
	s.db.QueryRow(ctx, `SELECT created_at FROM users WHERE id=$1`, u.ID).Scan(&created)
	photoIDs, err := s.entryPhotos(ctx, s.db, `SELECT id FROM photos WHERE user_id=$1`, u.ID)
	if err != nil {
		internal(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="greengrade-export-`+time.Now().Format("2006-01-02")+`.zip"`)
	zw := zip.NewWriter(w)
	f, _ := zw.Create("greengrade.json")
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"exportedAt": time.Now(), "account": map[string]any{"displayName": u.Name, "createdAt": created},
		"entries": full, "grows": grows, "growLogs": logs,
	})
	for _, id := range photoIDs {
		src, err := s.photos.Open(id, false)
		if err != nil {
			continue
		}
		dst, err := zw.Create("fotos/" + id + ".jpg")
		if err == nil {
			io.Copy(dst, src)
		}
		src.Close()
	}
	zw.Close()
}
