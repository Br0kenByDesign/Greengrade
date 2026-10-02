<script>
  import { score } from '../lib/format.js';
  let { value, label = 'von 10', size = 104 } = $props();
</script>
<div class="ring" style="--v:{(value || 0) * 10};width:{size}px;height:{size}px">
  <div style="width:{size * .8}px;height:{size * .8}px"><div><b style="font-size:{size * .29}px">{score(value)}</b><span>{label}</span></div></div>
</div>
