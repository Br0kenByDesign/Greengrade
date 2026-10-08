const MONTHS = ['Jan.', 'Feb.', 'März', 'Apr.', 'Mai', 'Juni', 'Juli', 'Aug.', 'Sep.', 'Okt.', 'Nov.', 'Dez.'];
export const num = (v, d = 1) => v == null || isNaN(v) ? '-' : Number(v).toLocaleString('de-DE', { minimumFractionDigits: d, maximumFractionDigits: d });
export const score = v => v == null ? '-' : Number.isInteger(+v) ? String(v) : num(v, 1);
export function date(s) {
  if (!s) return '';
  const d = new Date(s.length === 10 ? s + 'T12:00:00' : s);
  return `${d.getDate()}. ${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
}
export function month(s) { if (!s) return ''; const [y, m] = s.split('-'); return `${MONTHS[+m - 1]} ${y}`; }
export const today = () => new Date().toISOString().slice(0, 10);
export function daysSince(s) { if (!s) return null; return Math.floor((Date.now() - new Date(s + 'T00:00:00')) / 86400000); }
export function addDays(s, n) { const d = new Date(s + 'T12:00:00'); d.setDate(d.getDate() + n); return d.toISOString().slice(0, 10); }
export const parseNum = v => { if (v == null || v === '') return null; const n = parseFloat(String(v).replace(',', '.')); return isNaN(n) ? null : n; };
export function greeting() { const h = new Date().getHours(); return h < 11 ? 'Guten Morgen' : h < 18 ? 'Hallo' : 'Guten Abend'; }
// n with the right noun: plural(1, 'Sorte', 'Sorten') -> '1 Sorte'
export const plural = (n, one, many) => `${num(n, 0)} ${n === 1 ? one : many}`;
export const MONTHS_SHORT = MONTHS;
export const SOURCE = { grow: 'Eigener Grow', pharmacy: 'Apotheke' };
export const FORM = { flower: 'Blüten', extract: 'Extrakt' };
export const CATS = [['smell', 'Geruch'], ['taste', 'Geschmack'], ['look', 'Aussehen'], ['effect', 'Wirkung'], ['quality', 'Qualität']];
export function genetics(v) { return v < 30 ? 'Indica-dominant' : v > 70 ? 'Sativa-dominant' : 'Hybrid'; }
