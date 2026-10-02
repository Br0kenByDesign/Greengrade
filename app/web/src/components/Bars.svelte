<script>
  import { score } from '../lib/format.js';
  // items: [{ label, value, max?, text? }]
  let { items = [], max = 10 } = $props();
</script>
<div class="bars">
  {#each items as it}
    <div class="bar"><span>{it.label}</span><i style="--w:{Math.min(100, ((it.value || 0) / (it.max ?? max)) * 100)}%"></i><em>{it.text ?? score(it.value)}</em></div>
  {/each}
</div>
