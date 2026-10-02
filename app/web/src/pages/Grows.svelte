<script>
  import { api } from '../lib/api.js';
  import { date, daysSince, addDays } from '../lib/format.js';
  import Icon from '../components/Icon.svelte';
  let grows = $state(null), err = $state('');
  api('/api/grows').then(g => (grows = g)).catch(e => (err = e.message));
  const active = $derived((grows || []).filter(g => !g.harvestedOn));
  const done = $derived((grows || []).filter(g => g.harvestedOn));
  function progress(g) {
    if (g.floweringOn) { const d = daysSince(g.floweringOn), t = g.expectedFlowerDays || 63; return { phase: 'Blüte', text: `Tag ${d} von ca. ${t}`, pct: Math.min(100, d / t * 100), until: addDays(g.floweringOn, t) }; }
    if (g.germinatedOn) return { phase: 'Wachstum', text: `Tag ${daysSince(g.germinatedOn)}`, pct: 12 };
    return { phase: 'Geplant', text: 'Noch kein Startdatum', pct: 0 };
  }
</script>

<div class="head">
  <div><h1>Laufende Grows</h1><p class="muted">Behalte Keimung, Blüte und Ernte im Blick. Nach der Ernte bewertest du direkt aus dem Grow heraus.</p></div>
  <a class="btn btn-primary" href="/grows/new"><Icon name="plus" />Neuer Grow</a>
</div>
{#if err}<p class="err">{err}</p>{/if}
{#if !grows}<div class="spinner"></div>
{:else if !grows.length}
  <div class="empty"><h2>Noch keine Grows</h2><p>Lege deinen ersten Grow an. Du siehst dann jederzeit, an welchem Blütetag du bist und wann die Ernte ansteht.</p><a class="btn btn-primary" href="/grows/new">Grow anlegen</a></div>
{:else}
  {#if active.length}
    <div class="sgrid">
      {#each active as g (g.id)}
        {@const p = progress(g)}
        <a class="growrun" href="/grows/{g.id}" style="text-decoration:none;color:inherit;display:block">
          <div class="row"><b>{g.name}</b><span class="muted" style="font-size:13px">{p.phase}{g.location ? `, ${g.location}` : ''}</span></div>
          <div class="progress" style="--w:{p.pct}%"><span></span></div>
          <div class="row" style="margin:0;font-size:13px"><span>{p.text}</span>{#if p.until}<span class="muted">Ernte um den {date(p.until)}</span>{/if}</div>
        </a>
      {/each}
    </div>
  {:else}<p class="muted">Gerade läuft kein Grow.</p>{/if}
  {#if done.length}
    <div class="dsec">
      <h2>Abgeschlossen</h2>
      <div class="timeline">
        {#each done as g (g.id)}
          <a class="tl" href="/grows/{g.id}" style="text-decoration:none;color:inherit"><time>{date(g.harvestedOn)}</time><span>{g.name}{g.yieldGrams ? `, ${g.yieldGrams} g` : ''}</span><span class="muted" style="font-size:13px">{g.entryCount ? `${g.entryCount} Bewertung${g.entryCount === 1 ? '' : 'en'}` : 'Noch nicht bewertet'}</span></a>
        {/each}
      </div>
    </div>
  {/if}
{/if}
