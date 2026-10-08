package api

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

type count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type group struct {
	Count int      `json:"count"`
	Avg   *float64 `json:"avg"`
	sum   float64
	n     int
}

func (g *group) add(v *float64) {
	g.Count++
	if v != nil {
		g.sum += *v
		g.n++
		a := math.Round(g.sum/float64(g.n)*10) / 10
		g.Avg = &a
	}
}

func topCounts(m map[string]int, n int) []count {
	out := make([]count, 0, len(m))
	for k, v := range m {
		out = append(out, count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		x = strings.ReplaceAll(strings.TrimSpace(x), ",", ".")
		var f float64
		var frac, div float64 = 0, 1
		seen, dot := false, false
		for _, r := range x {
			switch {
			case r >= '0' && r <= '9':
				seen = true
				if dot {
					div *= 10
					frac = frac*10 + float64(r-'0')
				} else {
					f = f*10 + float64(r-'0')
				}
			case r == '.' && !dot:
				dot = true
			default:
				return 0, false
			}
		}
		return f + frac/div, seen
	}
	return 0, false
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	u, ctx := current(r), r.Context()
	entries, err := s.listEntries(ctx, u.ID, "")
	if err != nil {
		internal(w, err)
		return
	}
	bySource := map[string]*group{"grow": {}, "pharmacy": {}}
	byForm := map[string]*group{"flower": {}, "extract": {}}
	monthly := map[string]*group{}
	aromas, flavors, effects, sides, terps, methods := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	var cats [5]float64
	var rated, shared int
	var priceSum, thcSum float64
	var priceN, thcN int
	type top struct {
		EntryID string  `json:"entryId"`
		Name    string  `json:"name"`
		Overall float64 `json:"overall"`
		Source  string  `json:"source"`
	}
	tops := []top{}
	all := &group{}
	for _, e := range entries {
		var ov *float64
		if e.Shared {
			shared++
		}
		if t := e.Latest; t != nil {
			o := t.Overall
			ov = &o
			rated++
			for i, v := range []int{t.Smell, t.Taste, t.Look, t.Effect, t.Quality} {
				cats[i] += float64(v)
			}
			for _, x := range t.Aromas {
				aromas[x]++
			}
			for _, x := range t.Flavors {
				flavors[x]++
			}
			for _, x := range t.Effects {
				effects[x]++
			}
			for _, x := range t.SideEffects {
				sides[x]++
			}
			if t.Method != "" {
				methods[t.Method]++
			}
			m := t.TastedOn[:7]
			if monthly[m] == nil {
				monthly[m] = &group{}
			}
			monthly[m].add(ov)
			tops = append(tops, top{e.ID, e.StrainName, o, e.Source})
		}
		all.add(ov)
		if g := bySource[e.Source]; g != nil {
			g.add(ov)
		}
		if g := byForm[e.Form]; g != nil {
			g.add(ov)
		}
		if ts, ok := e.Details["terpenes"].([]any); ok {
			for _, t := range ts {
				if s, ok := t.(string); ok {
					terps[s]++
				}
			}
		}
		if e.Source == "pharmacy" {
			if p, ok := num(e.Details["price"]); ok && p > 0 && p < 1000 {
				priceSum += p
				priceN++
			}
			if e.Form == "flower" {
				if p, ok := num(e.Details["thc"]); ok && p > 0 && p <= 100 {
					thcSum += p
					thcN++
				}
			}
		}
	}
	sort.Slice(tops, func(i, j int) bool { return tops[i].Overall > tops[j].Overall })
	if len(tops) > 5 {
		tops = tops[:5]
	}
	months := []map[string]any{}
	// continuous timeline of 12 months, empty months included. It ends with the current
	// month, or with the latest tasting if there was none in the last year.
	end := time.Now()
	latest := ""
	for k := range monthly {
		if k > latest {
			latest = k
		}
	}
	if latest != "" {
		if t, err := time.Parse("2006-01", latest); err == nil && t.Before(end.AddDate(-1, 0, 0)) {
			end = t
		}
	}
	end = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	keys := make([]string, 0, 12)
	for i := 11; i >= 0; i-- {
		k := end.AddDate(0, -i, 0).Format("2006-01")
		if monthly[k] == nil {
			monthly[k] = &group{}
		}
		keys = append(keys, k)
	}
	if latest == "" {
		keys = nil
	}
	for _, k := range keys {
		months = append(months, map[string]any{"month": k, "count": monthly[k].Count, "avg": monthly[k].Avg})
	}
	catAvg := map[string]*float64{}
	for i, k := range []string{"smell", "taste", "look", "effect", "quality"} {
		if rated > 0 {
			v := math.Round(cats[i]/float64(rated)*10) / 10
			catAvg[k] = &v
		} else {
			catAvg[k] = nil
		}
	}
	avgOf := func(sum float64, n int) *float64 {
		if n == 0 {
			return nil
		}
		v := math.Round(sum/float64(n)*100) / 100
		return &v
	}
	var harvests int
	s.db.QueryRow(ctx, `SELECT count(*) FROM grows WHERE user_id=$1 AND harvested_on IS NOT NULL`, u.ID).Scan(&harvests)
	var activeGrows int
	s.db.QueryRow(ctx, `SELECT count(*) FROM grows WHERE user_id=$1 AND harvested_on IS NULL`, u.ID).Scan(&activeGrows)

	// You vs. community on the same strains (others' public ratings only)
	var mine, theirs *float64
	var commonN int
	s.db.QueryRow(ctx, `WITH my AS (
			SELECT DISTINCT ON (e.strain_id) e.strain_id,
				round((t.smell+t.taste+t.look+t.effect+t.quality)/5.0, 1) AS o
			FROM entries e JOIN tastings t ON t.entry_id=e.id
			WHERE e.user_id=$1 ORDER BY e.strain_id, t.tasted_on DESC, t.created_at DESC),
		other AS (SELECT strain_id, avg(overall) a FROM public_ratings WHERE user_id<>$1 GROUP BY strain_id)
		SELECT round(avg(my.o),1)::float8, round(avg(other.a),1)::float8, count(*) FROM my JOIN other USING (strain_id)`, u.ID).Scan(&mine, &theirs, &commonN)

	writeJSON(w, http.StatusOK, map[string]any{
		"entries": len(entries), "rated": rated, "shared": shared, "avg": all.Avg,
		"harvests": harvests, "activeGrows": activeGrows,
		"bySource": bySource, "byForm": byForm, "monthly": months, "categories": catAvg,
		"aromas": topCounts(aromas, 8), "flavors": topCounts(flavors, 8), "effects": topCounts(effects, 8),
		"sideEffects": topCounts(sides, 6), "terpenes": topCounts(terps, 8), "methods": topCounts(methods, 6),
		"top": tops, "avgPrice": avgOf(priceSum, priceN), "avgThc": avgOf(thcSum, thcN),
		"community": map[string]any{"mine": mine, "theirs": theirs, "strains": commonN},
	})
}
