<script>
  import { api } from '../lib/api.js';
  import { score } from '../lib/format.js';
  import Icon from '../components/Icon.svelte';
  import StrainImage from '../components/StrainImage.svelte';
  let list = $state(null), q = $state(''), sort = $state('best'), err = $state(''), more = $state(false), timer;
  async function load(append = false) {
    try {
      const r = await api(`/api/public/strains?sort=${sort}&q=${encodeURIComponent(q)}&offset=${append ? list.length : 0}`);
      list = append ? [...list, ...r] : r; more = r.length === 60;
    } catch (e) { err = e.message; }
  }
  load();
  const sorts = [['best', 'Am besten bewertet'], ['count', 'Meiste Bewertungen'], ['new', 'Neu dabei'], ['mine', 'Von mir bewertet']];
  function onSearch() { clearTimeout(timer); timer = setTimeout(() => load(), 250); }
</script>

<div class="head"><div><h1>Öffentliche Bewertungen</h1><p class="muted">Erfahrungen der Community, anonym zusammengefasst. Hier stehen nur Noten und Kommentare - keine Namen.</p></div></div>
<div class="toolbar"><label class="search"><Icon name="search" size={16} /><input placeholder="Sorte suchen" aria-label="Sorte suchen" bind:value={q} oninput={onSearch}></label></div>
<div class="chips" style="margin-bottom:20px">{#each sorts as [v, l]}<button class="chip" class:on={sort === v} onclick={() => { sort = v; load(); }}>{l}</button>{/each}</div>
{#if err}<p class="err">{err}</p>{/if}
{#if !list}{#if !err}<div class="spinner"></div>{/if}
{:else if !list.length}
  <div class="empty"><h2>{q ? 'Nichts gefunden' : 'Noch keine öffentlichen Bewertungen'}</h2><p>{q ? 'Zu dieser Suche gibt es noch keine geteilte Bewertung.' : 'Teile eine deiner Bewertungen, dann erscheint sie hier für alle - anonym.'}</p></div>
{:else}
  <div class="cards">
    {#each list as s (s.id)}
      <a class="card" href="/public/{s.id}" style="text-decoration:none;color:inherit">
        <div class="ph"><StrainImage name={s.name} photoId={s.coverPhotoId} /><span class="tag">{s.coverPhotoId ? 'Community-Foto' : 'Illustration'}</span>{#if s.mine}<span class="shared" title="Von dir bewertet"><Icon name="user" size={15} /></span>{/if}</div>
        <div class="meta"><div><h3>{s.name}</h3><small>{s.count} Bewertung{s.count === 1 ? '' : 'en'}</small></div><span class="score">Ø {score(s.avg)}</span></div>
      </a>
    {/each}
  </div>
  {#if more}<div class="loadmore"><button class="btn btn-ghost" onclick={() => load(true)}>Mehr laden</button></div>{/if}
{/if}
