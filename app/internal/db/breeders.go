package db

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/Br0kenByDesign/Greengrade/internal/names"
	"github.com/Br0kenByDesign/Greengrade/internal/security"
	"github.com/jackc/pgx/v5"
)

var goSteps = map[int]func(context.Context, pgx.Tx) error{
	4: breedersFromDetails,
}

// breedersFromDetails moves the free-text "Hersteller" (pharmacy) and "Breeder" (grow) of every entry
// into the shared breeder list and splits strains that were rated with different breeders.
func breedersFromDetails(ctx context.Context, tx pgx.Tx) error {
	type entry struct {
		id, strain, user string
		grow             *string
		breeder          string // normalised, "" = none
		shared           int
	}
	rows, err := tx.Query(ctx, `SELECT e.id, e.strain_id, e.user_id, e.grow_id, e.source,
		COALESCE(e.details->>'manufacturer',''), COALESCE(e.details->>'breeder',''),
		(SELECT count(*) FROM public_ratings pr WHERE pr.entry_id=e.id)
		FROM entries e ORDER BY e.created_at`)
	if err != nil {
		return err
	}
	var entries []*entry
	spellings := map[string]map[string]int{} // norm -> display spelling -> count
	cleaned := map[string]bool{}             // entries whose free text became a breeder
	for rows.Next() {
		var e entry
		var source, manu, breed string
		if err := rows.Scan(&e.id, &e.strain, &e.user, &e.grow, &source, &manu, &breed, &e.shared); err != nil {
			rows.Close()
			return err
		}
		text := breed
		if source == "pharmacy" || text == "" {
			if manu != "" {
				text = manu
			}
		}
		text = strings.Join(strings.Fields(security.CleanText(text, 80)), " ")
		if norm := names.Breeder(text); norm != "" && security.CheckBreederName(text) == nil {
			e.breeder = norm
			if spellings[norm] == nil {
				spellings[norm] = map[string]int{}
			}
			spellings[norm][text]++
			cleaned[e.id] = true
		}
		entries = append(entries, &e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	// one breeder per normalised name, shown in its most frequent spelling
	ids := map[string]string{}
	for norm, sp := range spellings {
		best, n := "", -1
		for text, c := range sp {
			if c > n || (c == n && text < best) {
				best, n = text, c
			}
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO breeders(name, name_norm) VALUES ($1,$2) RETURNING id`, best, norm).Scan(&id); err != nil {
			return err
		}
		ids[norm] = id
	}

	// grows get the breeder of their linked entries
	growBreeder := map[string]map[string]int{}
	byStrain := map[string]map[string][]*entry{} // strain -> breeder -> entries
	for _, e := range entries {
		if byStrain[e.strain] == nil {
			byStrain[e.strain] = map[string][]*entry{}
		}
		byStrain[e.strain][e.breeder] = append(byStrain[e.strain][e.breeder], e)
		if e.grow != nil && e.breeder != "" {
			if growBreeder[*e.grow] == nil {
				growBreeder[*e.grow] = map[string]int{}
			}
			growBreeder[*e.grow][e.breeder]++
		}
	}
	type grow struct{ id, strain, breeder string }
	var grows []grow
	rows, err = tx.Query(ctx, `SELECT id, strain_id FROM grows WHERE strain_id IS NOT NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g grow
		if err := rows.Scan(&g.id, &g.strain); err != nil {
			rows.Close()
			return err
		}
		n := 0
		for b, c := range growBreeder[g.id] {
			if c > n || (c == n && b < g.breeder) {
				g.breeder, n = b, c
			}
		}
		grows = append(grows, g)
	}
	rows.Close()
	growsByStrain := map[string]map[string][]string{}
	for _, g := range grows {
		if g.breeder == "" {
			continue // stays with the strain row that keeps the most ratings
		}
		if growsByStrain[g.strain] == nil {
			growsByStrain[g.strain] = map[string][]string{}
		}
		growsByStrain[g.strain][g.breeder] = append(growsByStrain[g.strain][g.breeder], g.id)
		if byStrain[g.strain] == nil {
			byStrain[g.strain] = map[string][]*entry{}
		}
		if _, ok := byStrain[g.strain][g.breeder]; !ok {
			byStrain[g.strain][g.breeder] = nil
		}
	}

	split := 0
	for strain, groups := range byStrain {
		type group struct {
			breeder         string
			shared, entries int
		}
		var gs []group
		for b, es := range groups {
			g := group{breeder: b, entries: len(es)}
			for _, e := range es {
				g.shared += e.shared
			}
			gs = append(gs, g)
		}
		// the existing strain keeps the group with the most public ratings, so links and covers stay
		sort.Slice(gs, func(i, j int) bool {
			a, b := gs[i], gs[j]
			if a.shared != b.shared {
				return a.shared > b.shared
			}
			if a.entries != b.entries {
				return a.entries > b.entries
			}
			if (a.breeder == "") != (b.breeder == "") {
				return a.breeder != ""
			}
			return a.breeder < b.breeder
		})
		for i, g := range gs {
			bid := nullable(ids[g.breeder])
			target := strain
			if i == 0 {
				if _, err := tx.Exec(ctx, `UPDATE strains SET breeder_id=$2 WHERE id=$1`, strain, bid); err != nil {
					return err
				}
			} else {
				if err := tx.QueryRow(ctx, `INSERT INTO strains(name, name_norm, breeder_id) SELECT name, name_norm, $2 FROM strains WHERE id=$1 RETURNING id`,
					strain, bid).Scan(&target); err != nil {
					return err
				}
				split++
				var eids []string
				for _, e := range groups[g.breeder] {
					eids = append(eids, e.id)
				}
				for _, q := range []string{
					`UPDATE entries SET strain_id=$1 WHERE id = ANY($2)`,
					`UPDATE public_ratings SET strain_id=$1 WHERE entry_id = ANY($2)`,
				} {
					if _, err := tx.Exec(ctx, q, target, eids); err != nil {
						return err
					}
				}
				if _, err := tx.Exec(ctx, `UPDATE grows SET strain_id=$1 WHERE id = ANY($2)`, target, growsByStrain[strain][g.breeder]); err != nil {
					return err
				}
				// a cover photo belongs to the strain whose rating shows it
				if _, err := tx.Exec(ctx, `UPDATE strains s SET pinned_photo_id=o.pinned_photo_id FROM strains o
					WHERE s.id=$1 AND o.id=$2 AND o.pinned_photo_id IN (SELECT photo_id FROM public_ratings WHERE strain_id=$1)`, target, strain); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE strains SET pinned_photo_id=NULL WHERE id=$2
					AND pinned_photo_id IN (SELECT photo_id FROM public_ratings WHERE strain_id=$1)`, target, strain); err != nil {
					return err
				}
			}
		}
	}
	// reports on ratings follow their rating
	if _, err := tx.Exec(ctx, `UPDATE reports r SET strain_id=pr.strain_id FROM public_ratings pr WHERE pr.id=r.public_rating_id AND r.strain_id IS DISTINCT FROM pr.strain_id`); err != nil {
		return err
	}
	// the free text now lives in the breeder list
	var done []string
	for id := range cleaned {
		done = append(done, id)
	}
	if _, err := tx.Exec(ctx, `UPDATE entries SET details = details - 'manufacturer' - 'breeder' WHERE id = ANY($1)`, done); err != nil {
		return err
	}
	slog.Info("hersteller und breeder übernommen", "breeder", len(ids), "einträge", len(done), "aufgeteilte_sorten", split)
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
