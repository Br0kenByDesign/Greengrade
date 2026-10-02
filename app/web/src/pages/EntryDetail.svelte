<script>
  import { api, photoURL } from '../lib/api.js';
  import { go } from '../lib/router.svelte.js';
  import { score, date, SOURCE, FORM, CATS, genetics, today, parseNum } from '../lib/format.js';
  import { notify } from '../lib/toast.svelte.js';
  import * as O from '../lib/options.js';
  import Icon from '../components/Icon.svelte';
  import Ring from '../components/Ring.svelte';
  import Radar from '../components/Radar.svelte';
  import Bars from '../components/Bars.svelte';
  import Rate from '../components/Rate.svelte';
  import Chips from '../components/Chips.svelte';
  import StrainImage from '../components/StrainImage.svelte';

  let { params } = $props();
  let data = $state(null), err = $state(''), sel = $state(0), adding = $state(false), busy = $state(false);
  let nt = $state(null);
  const load = () => api('/api/entries/' + params.id).then(d => (data = d)).catch(e => (err = e.message));
  load();

  const e = $derived(data?.entry);
  const l = $derived(e?.latest);
  const d = $derived(e?.details || {});
  const unit = $derived(e?.form === 'extract' ? 'mg/ml' : '%');
  const facts = $derived(!e ? [] : (e.source === 'pharmacy' ? [
    ['Hersteller', d.manufacturer], ['Produkt', d.product], ['THC', d.thc && `${d.thc} ${unit}`], ['CBD', d.cbd && `${d.cbd} ${unit}`],
    ['Charge', d.batch], ['Bestrahlt', d.irradiated], ['Preis', d.price && `${String(d.price).replace('.', ',')} € / ${e.form === 'extract' ? 'ml' : 'g'}`],
    ['Abgabe', d.purchasedOn && date(d.purchasedOn)], ['Apotheke', d.pharmacy]
  ] : [
    ['Saatgut', d.seedType], ['Breeder', d.breeder], ['Umgebung', d.environment], ['Medium', d.medium],
    ['Keimung', d.germinatedOn && date(d.germinatedOn)], ['Blütezeit', d.flowerDays && `${d.flowerDays} Tage`], ['Ernte', d.harvestedOn && date(d.harvestedOn)],
    ['Trocknung', d.dryDays && `${d.dryDays} Tage`], ['Curing', d.cureWeeks && `${d.cureWeeks} Wochen`], ['Licht', d.lightWatts && `${d.lightWatts} W`], ['Ertrag', d.yieldGrams && `${d.yieldGrams} g`]
  ]).concat([
    ['Konsum', l?.method && (l.method + (l.temperature ? `, ${l.temperature} °C` : ''))], ['Wirkungseintritt', l?.onsetMin != null && `${l.onsetMin} Min.`],
    ['Wirkdauer', l?.durationH != null && `ca. ${String(l.durationH).replace('.', ',')} Std.`],
    ['Trichome', d.trichomes], ['Dichte', d.density], ['Trim', d.trim], ['Feuchtigkeit', d.moisture]
  ]).filter(([, v]) => v));

  function startTasting() {
    nt = { tastedOn: today(), method: l?.method || 'Vaporizer', temperature: l?.temperature ?? '', onsetMin: '', durationH: '', smell: 0, taste: 0, look: 0, effect: 0, quality: 0,
      aromas: [...(l?.aromas || [])], flavors: [...(l?.flavors || [])], effects: [], sideEffects: [], daytime: [...(l?.daytime || [])], note: '' };
    adding = true;
  }
  async function saveTasting() {
    if (![nt.smell, nt.taste, nt.look, nt.effect, nt.quality].every(Boolean)) { err = 'Bitte vergib alle fünf Noten.'; return; }
    busy = true; err = '';
    try {
      data = await api(`/api/entries/${params.id}/tastings`, { method: 'POST', body: { ...nt, temperature: parseNum(nt.temperature) && Math.round(parseNum(nt.temperature)), onsetMin: parseNum(nt.onsetMin) && Math.round(parseNum(nt.onsetMin)), durationH: parseNum(nt.durationH) } });
      adding = false; notify('Verkostung gespeichert.');
    } catch (x) { err = x.message; } finally { busy = false; }
  }
  async function delTasting(id) {
    if (!confirm('Diese Verkostung löschen?')) return;
    try { await api('/api/tastings/' + id, { method: 'DELETE' }); await load(); } catch (x) { err = x.message; }
  }
  async function del() {
    if (!confirm(`„${e.strainName}“ mit allen Verkostungen und Fotos endgültig löschen?`)) return;
    try { await api('/api/entries/' + params.id, { method: 'DELETE' }); notify('Eintrag gelöscht.'); go('/', { replace: true }); } catch (x) { err = x.message; }
  }
  async function unshare() {
    try { data = await api(`/api/entries/${params.id}/share`, { method: 'PUT', body: { shared: false } }); notify('Nicht mehr öffentlich.'); } catch (x) { err = x.message; }
  }
</script>

<a class="back" href="/"><Icon name="back" size={16} />Übersicht</a>
{#if err}<p class="err" style="margin-bottom:16px">{err}</p>{/if}
{#if !data}{#if !err}<div class="spinner"></div>{/if}{:else}
  <div class="dhero">
    <div class="gallery">
      <div class="big"><StrainImage name={e.strainName} photoId={data.photos[sel]?.id} thumb={false} /></div>
      {#if data.photos.length > 1}
        <div class="row">{#each data.photos as p, i}<button class:on={i === sel} onclick={() => (sel = i)} aria-label="Foto {i + 1}"><img src={photoURL(p.id)} alt=""></button>{/each}</div>
      {/if}
    </div>
    <div>
      <div class="dtitle">
        <div>
          <h1>{e.strainName}</h1>
          <div class="pills"><span class="pill leaf">{SOURCE[e.source]}</span><span class="pill">{FORM[e.form]}</span><span class="pill">{genetics(e.genetics)}</span>{#if e.rebuy}<span class="pill">Merkliste</span>{/if}{#each l?.daytime || [] as dt}<span class="pill">{dt}</span>{/each}</div>
        </div>
        {#if l}<Ring value={l.overall} />{/if}
      </div>
      {#if facts.length}<div class="facts">{#each facts as [k, v]}<div><span>{k}</span><b>{v}</b></div>{/each}</div>{/if}
      {#if data.share}
        <div class="sharedbox">
          <div><Icon name="globe" size={20} /><span><b style="font-weight:500">Öffentlich geteilt</b><br><span class="muted" style="font-size:13px">Noten{data.share.comment ? ', Kommentar' : ''}{data.share.photoId ? ' und ein Foto' : ''}{data.share.commentHidden || data.share.photoHidden ? ' - teilweise ausgeblendet' : ''}</span></span></div>
          <span class="row-actions"><a class="btn btn-ghost" href="/public/{e.strainId}">Sortenseite ansehen</a><button class="btn btn-line" onclick={unshare}>Nicht mehr teilen</button></span>
        </div>
      {/if}
      {#if l}
        <div class="radarwrap" style="margin-top:22px">
          <Radar series={[{ values: l, color: 'var(--resin)' }]} />
          <Bars items={CATS.map(([k, n]) => ({ label: n, value: l[k] }))} />
        </div>
      {/if}
    </div>
  </div>

  {#if l && (l.aromas.length || l.flavors.length || l.effects.length || d.terpenes?.length)}
    <div class="dsec">
      <h2>Aromen, Wirkung und Terpene</h2>
      <div class="chips">
        {#each [...new Set([...l.aromas, ...l.flavors])] as a}<span class="pill leaf">{a}</span>{/each}
        {#each l.effects as a}<span class="pill">{a}</span>{/each}
        {#each d.terpenes || [] as a}<span class="pill">{a}</span>{/each}
      </div>
      {#if l.sideEffects.length}<p class="muted" style="margin-top:10px;font-size:14px">Nebenwirkungen: {l.sideEffects.join(', ')}</p>{/if}
    </div>
  {/if}
  {#if data.notes || d.nextTime}
    <div class="dsec">
      <h2>Private Notizen</h2>
      {#if data.notes}<p class="notes" style="white-space:pre-line">{data.notes}</p>{/if}
      {#if d.nextTime}<p class="notes" style="margin-top:10px"><b style="font-weight:500">Nächstes Mal:</b> {d.nextTime}</p>{/if}
    </div>
  {/if}

  <div class="dsec">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:14px;gap:12px;flex-wrap:wrap">
      <h2 style="margin:0">Verkostungen</h2>
      {#if !adding}<button class="btn btn-ghost" onclick={startTasting}>Erneut verkosten</button>{/if}
    </div>
    {#if adding}
      <div class="inline-form">
        <div class="fgrid">
          <div class="field"><label for="t0">Datum</label><input id="t0" type="date" bind:value={nt.tastedOn}></div>
          <div class="field"><label for="t1">Konsum</label><select id="t1" bind:value={nt.method}>{#each O.METHODS as o}<option>{o}</option>{/each}</select></div>
        </div>
        {#each CATS as [k, n]}<div><span class="flabel">{n}</span><Rate label={n} bind:value={nt[k]} /></div>{/each}
        <Chips label="Aromen" options={O.AROMAS} bind:selected={nt.aromas} />
        <Chips label="Wirkungen" options={O.EFFECTS} bind:selected={nt.effects} />
        <div class="field"><label for="t2">Notiz</label><textarea id="t2" style="min-height:70px" maxlength="2000" bind:value={nt.note}></textarea></div>
        <div class="row-actions"><button class="btn btn-primary" disabled={busy} onclick={saveTasting}>Verkostung speichern</button><button class="btn btn-ghost" onclick={() => (adding = false)}>Abbrechen</button></div>
      </div>
    {/if}
    <div class="timeline" style="margin-top:14px">
      {#each data.tastings as tt}
        <div class="tl"><time>{date(tt.tastedOn)}</time><span>{[tt.method + (tt.temperature ? `, ${tt.temperature} °C` : ''), tt.effects.join(', '), tt.note].filter(Boolean).join('. ')}{#if data.tastings.length > 1} <button class="linkbtn" onclick={() => delTasting(tt.id)}>Löschen</button>{/if}</span><span class="score">{score(tt.overall)}</span></div>
      {/each}
    </div>
    {#if data.share}<p class="muted" style="font-size:13px;margin-top:10px">Öffentlich zählt immer deine neueste Verkostung.</p>{/if}
  </div>

  <div class="dsec row-actions">
    <a class="btn btn-line" href="/entries/{e.id}/edit"><Icon name="edit" />Bearbeiten</a>
    <a class="btn btn-line" href="/compare?a={e.id}"><Icon name="compare" />Mit anderer Sorte vergleichen</a>
    {#if e.growId}<a class="btn btn-line" href="/grows/{e.growId}"><Icon name="grow" />Zum Grow</a>{/if}
    <button class="btn btn-danger" onclick={del}><Icon name="trash" />Löschen</button>
  </div>
{/if}
