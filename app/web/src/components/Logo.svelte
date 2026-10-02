<script>let { size = 30, light = false } = $props();</script>
<svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" style="flex:none">
  <circle cx="32" cy="32" r="26" fill="none" stroke={light ? 'rgba(255,255,255,.22)' : 'var(--leaf-soft)'} stroke-width="6"/>
  <circle cx="32" cy="32" r="26" fill="none" stroke={light ? '#E2B05A' : 'var(--resin)'} stroke-width="6" stroke-linecap="round" stroke-dasharray="131 164" transform="rotate(-90 32 32)"/>
  <g fill={light ? '#fff' : 'var(--leaf)'}>
    <path d="M32 46C28.5 38 28.5 26 32 18C35.5 26 35.5 38 32 46Z"/>
    <path d="M32 46C29.5 40.5 29.5 32 32 26C34.5 32 34.5 40.5 32 46Z" transform="rotate(-55 32 46)"/>
    <path d="M32 46C29.5 40.5 29.5 32 32 26C34.5 32 34.5 40.5 32 46Z" transform="rotate(55 32 46)"/>
  </g>
</svg>
