<script>
  import { strainArt } from '../lib/art.js';
  import { photoURL } from '../lib/api.js';
  let { name = '', photoId = null, thumb = true } = $props();
</script>
{#if photoId}<img src={photoURL(photoId, thumb)} alt="Foto von {name}" loading="lazy" style="width:100%;height:100%;object-fit:cover;display:block">{:else}{@html strainArt(name)}{/if}
