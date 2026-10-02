<script>
  import { api } from '../lib/api.js';
  import { score } from '../lib/format.js';
  let { value = $bindable(''), id = 'strain' } = $props();
  let list = $state([]), open = $state(false), timer;
  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (value.trim().length < 2) { list = []; return; }
      try { list = await api('/api/strains/suggest?q=' + encodeURIComponent(value)); open = true; } catch { list = []; }
    }, 180);
  }
  const exact = $derived(list.some(s => s.name.toLowerCase() === value.trim().toLowerCase()));
  function pick(n) { value = n; open = false; }
</script>
<div class="ac">
  <input {id} bind:value oninput={onInput} onfocus={() => (open = list.length > 0)} onblur={() => setTimeout(() => (open = false), 150)}
    autocomplete="off" role="combobox" aria-expanded={open} aria-controls="{id}-list" maxlength="80" placeholder="z. B. Ghost Train Haze" required>
  {#if open && value.trim().length >= 2 && (list.length || !exact)}
    <div class="ac-list" id="{id}-list" role="listbox">
      {#each list as s}
        <button type="button" role="option" aria-selected="false" onmousedown={e => e.preventDefault()} onclick={() => pick(s.name)}>
          <b>{s.name}</b><span>{s.count ? `${s.count} öffentliche Bewertung${s.count === 1 ? '' : 'en'}, Ø ${score(s.avg)}` : s.mine ? 'Schon in deinem Logbuch' : ''}</span>
        </button>
      {/each}
      {#if !exact}<button type="button" class="newone" onmousedown={e => e.preventDefault()} onclick={() => (open = false)}>„{value.trim()}“ als neue Sorte anlegen</button>{/if}
    </div>
  {/if}
</div>
