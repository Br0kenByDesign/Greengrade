<script>
  import { api } from '../lib/api.js';
  let { value = $bindable(''), id = 'breeder', placeholder = '', disabled = false } = $props();
  let list = $state([]), open = $state(false), timer;
  function onInput() {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      if (!value.trim()) { list = []; return; }
      try { list = await api('/api/breeders/suggest?q=' + encodeURIComponent(value)); open = true; } catch { list = []; }
    }, 180);
  }
  const exact = $derived(list.some(b => b.name.toLowerCase() === value.trim().toLowerCase()));
</script>
<div class="ac">
  <input {id} bind:value oninput={onInput} onfocus={() => (open = list.length > 0)} onblur={() => setTimeout(() => (open = false), 150)}
    autocomplete="off" role="combobox" aria-expanded={open} aria-controls="{id}-list" maxlength="80" {placeholder} {disabled}>
  {#if open && !disabled && list.length && !exact}
    <div class="ac-list" id="{id}-list" role="listbox">
      {#each list as b (b.name)}
        <button type="button" role="option" aria-selected="false" onmousedown={e => e.preventDefault()} onclick={() => { value = b.name; open = false; }}><b>{b.name}</b></button>
      {/each}
    </div>
  {/if}
</div>
