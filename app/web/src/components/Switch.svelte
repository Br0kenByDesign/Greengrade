<script>
  let { checked = $bindable(false), label } = $props();
</script>
<button type="button" class="switch" class:on={checked} role="switch" aria-checked={checked} aria-label={label} onclick={() => (checked = !checked)}></button>
