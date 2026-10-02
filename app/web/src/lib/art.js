// Generated strain illustration: deterministic from the name, so every strain always gets the same picture.
function hash(str) { let h = 2166136261; for (const c of str) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; }
export function strainArt(name = '') {
  let s = hash(name.toLowerCase()) || 1;
  const r = () => { s = Math.imul(s ^ (s >>> 15), 2246822507) >>> 0; s = Math.imul(s ^ (s >>> 13), 3266489909) >>> 0; return (s % 10000) / 10000; };
  const hue = 70 + Math.floor(r() * 90);
  const bg = `hsl(${hue} 32% 90%)`, a = `hsl(${hue} 34% 34%)`, b = `hsl(${hue} 30% 52%)`, c = `hsl(${hue} 40% 74%)`;
  let g = `<rect width="200" height="250" fill="${bg}"/>`;
  for (let i = 0; i < 5; i++) g += `<circle cx="${(100 + (r() - .5) * 30).toFixed(1)}" cy="${(150 + (r() - .5) * 30).toFixed(1)}" r="${150 - i * 26}" fill="none" stroke="${c}" stroke-width="${(1 + r() * 2).toFixed(1)}" opacity=".7"/>`;
  const n = 3 + Math.floor(r() * 3) * 2, cx = 100, cy = 196;
  for (let i = 0; i < n; i++) {
    const mid = (n - 1) / 2, deg = -70 + 140 * (i / (n - 1));
    const len = (i === mid ? 120 : 70 + 40 * (1 - Math.abs(i - mid) / mid)) * (.85 + r() * .2), w = 10 + len * .12;
    g += `<path d="M${cx} ${cy}C${cx - w} ${cy - len * .45} ${cx - w * .6} ${cy - len * .8} ${cx} ${cy - len}C${cx + w * .6} ${cy - len * .8} ${cx + w} ${cy - len * .45} ${cx} ${cy}Z" fill="${i % 2 ? b : a}" transform="rotate(${deg.toFixed(1)} ${cx} ${cy})"/>`;
  }
  g += `<path d="M100 196V250" stroke="${a}" stroke-width="5"/>`;
  return `<svg viewBox="0 0 200 250" preserveAspectRatio="xMidYMid slice" aria-hidden="true">${g}</svg>`;
}
