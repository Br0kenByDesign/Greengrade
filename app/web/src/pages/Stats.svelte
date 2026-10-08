<script>
  import { api } from '../lib/api.js';
  import { score, num, plural, MONTHS_SHORT, CATS } from '../lib/format.js';
  import Bars from '../components/Bars.svelte';
  let s = $state(null), err = $state('');
  api('/api/stats').then(x => (s = x)).catch(e => (err = e.message));
  const tagBars = list => list.map(a => ({ label: a.name, value: a.count, max: list[0]?.count || 1, text: a.count }));
</script>

<div class="head"><div><h1>Statistik</h1><p class="muted">Alles berechnet aus deinen neuesten Verkostungen.</p></div></div>
{#if err}<p class="err">{err}</p>{/if}
{#if !s}{#if !err}<div class="spinner"></div>{/if}
{:else if !s.rated}
  <div class="empty"><h2>Noch keine Daten</h2><p>Sobald du ein paar Sorten bewertet hast, siehst du hier deine Vorlieben.</p><a class="btn btn-primary" href="/entries/new">Sorte bewerten</a></div>
{:else}
  <div class="stats">
    <div class="stat"><b>{s.rated}</b><span>Bewertete Sorten</span></div>
    <div class="stat"><b>{score(s.avg)}</b><span>Durchschnittsnote</span></div>
    <div class="stat"><b>{s.avgThc != null ? num(s.avgThc) + ' %' : '-'}</b><span>Ø THC (Apothekenblüten)</span></div>
    <div class="stat"><b>{s.avgPrice != null ? num(s.avgPrice, 2) + ' €' : '-'}</b><span>Ø Apothekenpreis</span></div>
  </div>
  <div class="sgrid">
    {#if s.monthly.length}
      {@const mx = Math.max(1, ...s.monthly.map(x => x.count))}
      <div class="panel" style="grid-column:1/-1">
        <h2>Verkostungen pro Monat</h2>
        <div class="monthbars" role="img" aria-label="Verkostungen pro Monat, letzte 12 Monate">
          {#each s.monthly as m, i}
            {@const [y, mo] = m.month.split('-')}
            {@const yearMark = i === 0 || mo === '01'}
            <div class:year={yearMark && i > 0} title="{MONTHS_SHORT[+mo - 1]} {y}: {plural(m.count, 'Verkostung', 'Verkostungen')}{m.avg != null ? `, Ø ${score(m.avg)}` : ''}">
              <b style="color:var(--ink);font-weight:500">{m.count || ''}</b>
              <span class:zero={!m.count} style="height:{m.count / mx * 100}%"></span>
              <small>{MONTHS_SHORT[+mo - 1].replace('.', '')}{#if yearMark}<em>{y}</em>{/if}</small>
            </div>
          {/each}
        </div>
      </div>
    {/if}
    <div class="panel"><h2>Deine Noten nach Kategorie</h2><Bars items={CATS.map(([k, n]) => ({ label: n, value: s.categories[k] }))} /></div>
    <div class="panel"><h2>Bestenliste</h2>
      <div class="timeline">{#each s.top as t, i}<a class="tl" style="grid-template-columns:28px 1fr auto;text-decoration:none;color:inherit" href="/entries/{t.entryId}"><time>{i + 1}.</time><span>{t.name}</span><span class="score">{score(t.overall)}</span></a>{/each}</div>
    </div>
    <div class="panel"><h2>Grow oder Apotheke?</h2>
      <div class="versus">
        <div><b>{score(s.bySource.grow.avg)}</b><span>Ø eigene Grows ({s.bySource.grow.count})</span></div>
        <div><b>{score(s.bySource.pharmacy.avg)}</b><span>Ø Apotheke ({s.bySource.pharmacy.count})</span></div>
        <div><b>{score(s.byForm.flower.avg)}</b><span>Ø Blüten ({s.byForm.flower.count})</span></div>
        <div><b>{score(s.byForm.extract.avg)}</b><span>Ø Extrakte ({s.byForm.extract.count})</span></div>
      </div>
    </div>
    {#if s.community.strains}<div class="panel"><h2>Du und die Community</h2>
      <div class="versus"><div><b>{score(s.community.mine)}</b><span>Deine Ø-Note</span></div><div><b>{score(s.community.theirs)}</b><span>Ø der anderen</span></div></div>
      <p class="muted" style="font-size:13px;margin-top:10px">Über {plural(s.community.strains, 'gemeinsam bewertete Sorte', 'gemeinsam bewertete Sorten')}.</p></div>{/if}
    {#each [['Aromen', s.aromas], ['Geschmack', s.flavors], ['Wirkungen', s.effects], ['Terpene', s.terpenes], ['Nebenwirkungen', s.sideEffects], ['Konsum', s.methods]] as [title, list]}
      {#if list.length}<div class="panel"><h2>{title}</h2><Bars items={tagBars(list)} /></div>{/if}
    {/each}
  </div>
{/if}
