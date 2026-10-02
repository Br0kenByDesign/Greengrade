<script>
  import { CATS } from '../lib/format.js';
  // series: [{ values: {smell, taste, ...}, color }]
  let { series = [] } = $props();
  const c = 120, R = 88, n = CATS.length;
  const pt = (i, v) => { const a = -Math.PI / 2 + i * 2 * Math.PI / n; return [c + Math.cos(a) * R * v / 10, c + Math.sin(a) * R * v / 10]; };
  const poly = vals => CATS.map(([k], i) => pt(i, vals[k] || 0).join(',')).join(' ');
</script>
<svg viewBox="-34 -6 308 252" role="img" aria-label="Bewertung nach Kategorie">
  {#each [2.5, 5, 7.5, 10] as l}<polygon points={CATS.map((_, i) => pt(i, l).join(',')).join(' ')} fill="none" stroke="var(--line)"/>{/each}
  {#each CATS as [k, label], i}
    {@const e = pt(i, 10)}{@const t = pt(i, 12.4)}
    <line x1={c} y1={c} x2={e[0]} y2={e[1]} stroke="var(--line)"/>
    <text x={t[0]} y={t[1]} font-size="11" fill="var(--muted)" text-anchor="middle" dominant-baseline="middle">{label}</text>
  {/each}
  {#each series as s}
    <polygon points={poly(s.values)} fill="color-mix(in srgb, {s.color} 20%, transparent)" stroke={s.color} stroke-width="2" stroke-linejoin="round"/>
  {/each}
</svg>
