<script>
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { greeting, score, num, SOURCE, FORM, daysSince, addDays, date } from '../lib/format.js';
  import Icon from '../components/Icon.svelte';
  import StrainImage from '../components/StrainImage.svelte';
  import Bars from '../components/Bars.svelte';

  let entries = $state(null), stats = $state(null), grows = $state([]), err = $state('');
  let filter = $state('all'), q = $state('');
  Promise.all([api('/api/entries'), api('/api/stats'), api('/api/grows')])
    .then(([e, s, g]) => { entries = e; stats = s; grows = g.filter(x => !x.harvestedOn); })
    .catch(e => (err = e.message));

  const shown = $derived((entries || []).filter(e =>
    (filter === 'all' || e.source === filter || e.form === filter || (filter === 'shared' && e.shared) || (filter === 'rebuy' && e.rebuy)) &&
    (!q || [e.strainName, ...(e.latest?.aromas || []), ...(e.details?.terpenes || [])].join(' ').toLowerCase().includes(q.toLowerCase()))));
  const filters = [['all', 'Alle'], ['grow', 'Eigener Grow'], ['pharmacy', 'Apotheke'], ['flower', 'Blüten'], ['extract', 'Extrakt'], ['shared', 'Geteilt'], ['rebuy', 'Merkliste']];
  function sub(e) {
    if (e.source === 'pharmacy') {
      const u = e.form === 'extract' ? 'mg/ml' : '%';
      return [e.details?.thc && `${e.details.thc} ${u} THC`, e.details?.price && `${String(e.details.price).replace('.', ',')} €/${e.form === 'extract' ? 'ml' : 'g'}`].filter(Boolean).join(', ') || 'Apotheke';
    }
    return e.details?.harvestedOn ? `Ernte ${date(e.details.harvestedOn)}` : 'Eigener Grow';
  }
  function growProgress(g) {
    if (g.floweringOn) { const d = daysSince(g.floweringOn), t = g.expectedFlowerDays || 63; return { text: `Blüte Tag ${d} von ca. ${t}`, pct: Math.min(100, d / t * 100), until: addDays(g.floweringOn, t) }; }
    if (g.germinatedOn) return { text: `Wachstum Tag ${daysSince(g.germinatedOn)}`, pct: 15 };
    return { text: 'Noch nicht gestartet', pct: 0 };
  }
</script>

<div class="head">
  <div>
    <h1>{greeting()}, {session.me.displayName}</h1>
    {#if stats?.top?.[0]}<p class="muted">{stats.entries} {stats.entries === 1 ? 'Sorte' : 'Sorten'} in deinem Logbuch. Vorne liegt {stats.top[0].name} mit {score(stats.top[0].overall)}.</p>{/if}
  </div>
  <a class="btn btn-primary" href="/entries/new"><Icon name="plus" />Sorte bewerten</a>
</div>

{#if err}<p class="err">{err}</p>{/if}
{#if !entries}
  <div class="spinner"></div>
{:else if entries.length === 0}
  <div class="empty">
    <h2>Dein Logbuch ist noch leer</h2>
    <p>Bewerte deine erste Sorte - aus der Apotheke oder aus deinem eigenen Grow. Alles bleibt privat, bis du etwas teilst.</p>
    <a class="btn btn-primary" href="/entries/new"><Icon name="plus" />Erste Sorte bewerten</a>
  </div>
{:else}
  <div class="stats">
    <div class="stat"><b>{stats.entries}</b><span>Bewertete Sorten</span></div>
    <div class="stat"><b>{score(stats.avg)}</b><span>Deine Durchschnittsnote</span></div>
    <div class="stat"><b>{stats.harvests}</b><span>Eigene Ernten</span></div>
    <div class="stat"><b>{stats.shared}</b><span>Öffentlich geteilt</span></div>
  </div>
  <div class="grid2">
    <div>
      <div class="toolbar">
        <label class="search"><Icon name="search" size={16} /><input placeholder="Sorte, Aroma oder Terpen suchen" aria-label="Suchen" bind:value={q}></label>
      </div>
      <div class="chips" style="margin-bottom:20px">
        {#each filters as [v, l]}<button class="chip" class:on={filter === v} onclick={() => (filter = v)}>{l}</button>{/each}
      </div>
      <div class="cards">
        {#each shown as e (e.id)}
          <a class="card" href="/entries/{e.id}" style="text-decoration:none;color:inherit">
            <div class="ph"><StrainImage name={e.strainName} photoId={e.coverPhotoId} /><span class="tag">{SOURCE[e.source]}{e.form === 'extract' ? ', Extrakt' : ''}</span>{#if e.shared}<span class="shared" title="Öffentlich geteilt"><Icon name="globe" size={15} /></span>{/if}</div>
            <div class="meta"><div><h3>{e.strainName}</h3><small>{sub(e)}</small></div>{#if e.latest}<span class="score">{score(e.latest.overall)}</span>{/if}</div>
          </a>
        {:else}<p class="muted">Keine Einträge in dieser Auswahl.</p>{/each}
      </div>
    </div>
    <aside>
      {#if grows.length}
        <div class="panel">
          <h2>Laufende Grows</h2>
          <div style="display:grid;gap:10px">
            {#each grows.slice(0, 3) as g}
              {@const p = growProgress(g)}
              <a class="growrun" href="/grows/{g.id}" style="text-decoration:none;color:inherit;display:block">
                <div class="row"><b>{g.name}</b>{#if g.location}<span class="muted" style="font-size:13px">{g.location}</span>{/if}</div>
                <div class="progress" style="--w:{p.pct}%"><span></span></div>
                <div class="row" style="margin:0;font-size:13px"><span>{p.text}</span>{#if p.until}<span class="muted">Ernte um den {date(p.until)}</span>{/if}</div>
              </a>
            {/each}
          </div>
        </div>
      {/if}
      {#if stats.aromas.length}
        <div class="panel">
          <h2>Dein Aromaprofil</h2>
          <Bars items={stats.aromas.map(a => ({ label: a.name, value: a.count, max: stats.aromas[0].count, text: a.count }))} />
        </div>
      {/if}
      {#if stats.community.strains > 0}
        <div class="panel">
          <h2>Du und die Community</h2>
          <div class="versus">
            <div><b>{score(stats.community.mine)}</b><span>Deine Ø-Note</span></div>
            <div><b>{score(stats.community.theirs)}</b><span>Community-Ø derselben Sorten</span></div>
          </div>
          <p class="muted" style="font-size:13px;margin-top:10px">Verglichen über {stats.community.strains} Sorte{stats.community.strains === 1 ? '' : 'n'}, die auch andere bewertet haben.</p>
        </div>
      {/if}
    </aside>
  </div>
{/if}
