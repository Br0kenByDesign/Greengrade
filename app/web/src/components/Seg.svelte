<script>
  // options: [[value, label, sub?]]
  let { options = [], value = $bindable(), label = '' } = $props();
</script>
<div class="seg" role="radiogroup" aria-label={label}>
  {#each options as [v, l, sub]}
    <button type="button" role="radio" aria-checked={value === v} class:on={value === v} onclick={() => (value = v)}>{l}{#if sub}<small>{sub}</small>{/if}</button>
  {/each}
</div>
