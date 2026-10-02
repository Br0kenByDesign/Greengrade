<script>
  let { options = [], selected = $bindable([]), label = '' } = $props();
  let custom = $state('');
  const all = $derived([...new Set([...options, ...selected])]);
  function toggle(o) { selected = selected.includes(o) ? selected.filter(x => x !== o) : [...selected, o]; }
  function add(e) { e.preventDefault(); const v = custom.trim(); if (v && !selected.includes(v)) selected = [...selected, v]; custom = ''; }
</script>
<div class="chips" role="group" aria-label={label}>
  {#each all as o}<button type="button" class="chip" class:on={selected.includes(o)} aria-pressed={selected.includes(o)} onclick={() => toggle(o)}>{o}</button>{/each}
  <input class="chip" style="width:110px;cursor:text" placeholder="+ eigenes" bind:value={custom} onkeydown={e => e.key === 'Enter' && add(e)} onblur={e => custom && add(e)} aria-label="Eigenen Begriff hinzufügen" maxlength="40">
</div>
