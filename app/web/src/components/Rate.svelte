<script>
  let { value = $bindable(0), label = '' } = $props();
</script>
<div class="rate" role="group" aria-label={label}>
  {#each Array(10) as _, i}
    <button type="button" class:on={i < value} aria-label="{i + 1} von 10" aria-pressed={i + 1 === value} onclick={() => (value = i + 1)}></button>
  {/each}
  <output>{value || '-'}</output>
</div>
