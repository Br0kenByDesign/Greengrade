<script>
  import { api, photoURL } from '../lib/api.js';
  import { score } from '../lib/format.js';
  import { strainArt } from '../lib/art.js';
  // Strain + breeder are one unit: picking a suggestion fills both.
  let { value = $bindable(''), breeder = $bindable(''), id = 'strain', onpick = null } = $props();
  let list = $state([]), open = $state(false), timer;
  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (value.trim().length < 2) { list = []; return; }
      try { list = await api('/api/strains/suggest?q=' + encodeURIComponent(value)); open = true; } catch { list = []; }
    }, 180);
  }
  const exact = $derived(list.some(s => s.name.toLowerCase() === value.trim().toLowerCase()));
  function pick(s) { value = s.name; breeder = s.breeder || ''; open = false; onpick?.(s); }
  function meta(s) {
    return s.count ? `Ø ${score(s.avg)} · ${s.count} Bewertung${s.count === 1 ? '' : 'en'}` : s.mine ? 'In deinem Logbuch' : '';
  }
</script>
<div class="ac">
  <input {id} bind:value oninput={onInput} onfocus={() => (open = list.length > 0)} onblur={() => setTimeout(() => (open = false), 150)}
    autocomplete="off" role="combobox" aria-expanded={open} aria-controls="{id}-list" maxlength="80" placeholder="z. B. Ghost Train Haze" required>
  {#if open && value.trim().length >= 2 && (list.length || !exact)}
    <div class="ac-list" id="{id}-list" role="listbox">
      {#each list as s (s.id)}
        <button type="button" class="sug" role="option" aria-selected="false" onmousedown={e => e.preventDefault()} onclick={() => pick(s)}>
          <span class="sug-img">{#if s.photoId}<img src={photoURL(s.photoId, true)} alt="" loading="lazy">{:else}{@html strainArt(s.name)}{/if}</span>
          <span class="sug-txt"><b>{s.name}</b><small>{s.breeder || 'ohne Angabe'}</small></span>
          <span class="sug-meta">{meta(s)}</span>
        </button>
      {/each}
      {#if !exact}<button type="button" class="newone" onmousedown={e => e.preventDefault()} onclick={() => (open = false)}>„{value.trim()}“ als neue Sorte anlegen</button>{/if}
    </div>
  {/if}
</div>
