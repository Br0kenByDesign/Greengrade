<script>
  import { api } from '../lib/api.js';
  import { route } from '../lib/router.svelte.js';
  import { score, SOURCE, FORM, CATS, genetics } from '../lib/format.js';
  import Radar from '../components/Radar.svelte';
  import StrainImage from '../components/StrainImage.svelte';
  let entries = $state(null), a = $state(route.query.get('a') || ''), b = $state(route.query.get('b') || ''), err = $state('');
  api('/api/entries').then(e => { entries = e.filter(x => x.latest); if (!a && entries[0]) a = entries[0].id; if (!b && entries[1]) b = entries[1].id; if (b === a && entries[1]) b = entries.find(x => x.id !== a)?.id || ''; }).catch(e => (err = e.message));
  const A = $derived(entries?.find(x => x.id === a)), B = $derived(entries?.find(x => x.id === b));
  const COLORS = ['var(--resin)', 'var(--leaf)'];
  const unit = e => e.form === 'extract' ? 'mg/ml' : '%';
  const rows = $derived(!A || !B ? [] : [
    ['Gesamtnote', score(A.latest.overall), score(B.latest.overall)],
    ...CATS.map(([k, n]) => [n, A.latest[k], B.latest[k]]),
    ['Herkunft', SOURCE[A.source], SOURCE[B.source]], ['Form', FORM[A.form], FORM[B.form]], ['Genetik', genetics(A.genetics), genetics(B.genetics)],
    ['THC', A.details.thc ? `${A.details.thc} ${unit(A)}` : '-', B.details.thc ? `${B.details.thc} ${unit(B)}` : '-'],
    ['Preis', A.details.price ? `${String(A.details.price).replace('.', ',')} €` : '-', B.details.price ? `${String(B.details.price).replace('.', ',')} €` : '-'],
    ['Aromen', A.latest.aromas.join(', ') || '-', B.latest.aromas.join(', ') || '-'],
    ['Wirkung', A.latest.effects.join(', ') || '-', B.latest.effects.join(', ') || '-'],
    ['Konsum', A.latest.method || '-', B.latest.method || '-']
  ]);
</script>

<div class="head"><div><h1>Vergleichen</h1><p class="muted">Zwei Sorten aus deinem Logbuch nebeneinander.</p></div></div>
{#if err}<p class="err">{err}</p>{/if}
{#if !entries}{#if !err}<div class="spinner"></div>{/if}
{:else if entries.length < 2}
  <div class="empty"><h2>Mindestens zwei Sorten nötig</h2><p>Bewerte noch eine Sorte, dann kannst du sie hier vergleichen.</p><a class="btn btn-primary" href="/entries/new">Sorte bewerten</a></div>
{:else}
  <div class="compare">
    {#each [[A, 'a'], [B, 'b']] as [E, key], i}
      <div class="col">
        <div class="field"><label for="c{key}">Sorte {i + 1}</label>
          {#if key === 'a'}<select id="ca" bind:value={a}>{#each entries as e}<option value={e.id}>{e.strainName}</option>{/each}</select>
          {:else}<select id="cb" bind:value={b}>{#each entries as e}<option value={e.id}>{e.strainName}</option>{/each}</select>{/if}
        </div>
        {#if E}<div class="ph"><StrainImage name={E.strainName} photoId={E.coverPhotoId} /></div>{/if}
      </div>
    {/each}
  </div>
  {#if A && B}
    <div class="compare-chart">
      <Radar series={[{ values: A.latest, color: COLORS[0] }, { values: B.latest, color: COLORS[1] }]} />
      <div>
        <div class="legend"><span><i style="background:{COLORS[0]}"></i>{A.strainName}</span><span><i style="background:{COLORS[1]}"></i>{B.strainName}</span></div>
        <div style="overflow-x:auto"><table class="ctable"><thead><tr><th></th><th>{A.strainName}</th><th>{B.strainName}</th></tr></thead>
          <tbody>{#each rows as [k, x, y]}<tr><th>{k}</th><td>{x}</td><td>{y}</td></tr>{/each}</tbody></table></div>
      </div>
    </div>
  {/if}
{/if}
