<script>
  import { api, photoURL } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { date } from '../lib/format.js';
  import { notify } from '../lib/toast.svelte.js';
  let tab = $state('reports'), status = $state('open'), reports = $state(null), strains = $state([]), q = $state(''), err = $state('');
  let notes = $state({}), mergeFrom = $state(null), pinFor = $state(null), pinPhotos = $state([]);
  const loadReports = () => api('/api/admin/reports?status=' + status).then(r => (reports = r)).catch(e => (err = e.message));
  const loadStrains = () => api('/api/admin/strains?q=' + encodeURIComponent(q)).then(r => (strains = r)).catch(e => (err = e.message));
  loadReports(); loadStrains();
  async function resolve(r, action) {
    try { await api('/api/admin/reports/' + r.id, { method: 'POST', body: { action, note: notes[r.id] || '' } }); notify(action === 'hide' ? 'Ausgeblendet, Person wurde informiert.' : 'Meldung verworfen.'); loadReports(); }
    catch (e) { err = e.message; }
  }
  async function rename(s) {
    const n = prompt('Neuer Name:', s.name); if (!n) return;
    try { await api('/api/admin/strains/' + s.id, { method: 'PATCH', body: { name: n } }); loadStrains(); } catch (e) { err = e.message; }
  }
  async function merge(into) {
    if (!confirm(`„${mergeFrom.name}“ in „${into.name}“ zusammenführen? Alle Einträge und Bewertungen wandern mit.`)) return;
    try { await api('/api/admin/strains/merge', { method: 'POST', body: { fromId: mergeFrom.id, intoId: into.id } }); mergeFrom = null; notify('Zusammengeführt.'); loadStrains(); } catch (e) { err = e.message; }
  }
  async function openPin(s) { pinFor = s; pinPhotos = await api(`/api/admin/strains/${s.id}/photos`).catch(() => []); }
  async function pin(photoId) {
    try { await api(`/api/admin/strains/${pinFor.id}/pin`, { method: 'POST', body: { photoId } }); notify('Titelbild gesetzt.'); pinFor = null; loadStrains(); } catch (e) { err = e.message; }
  }
</script>

{#if !session.me.isAdmin}<p class="err">Kein Zugriff.</p>{:else}
<div class="head"><div><h1>Moderation</h1><p class="muted">Meldungen prüfen, Sorten pflegen, Titelbilder festlegen.</p></div></div>
<div class="tabs">{#each [['reports', 'Meldungen'], ['strains', 'Sorten']] as [v, l]}<button class="chip" class:on={tab === v} onclick={() => (tab = v)}>{l}</button>{/each}</div>
{#if err}<p class="err" style="margin-bottom:14px">{err}</p>{/if}

{#if tab === 'reports'}
  <div class="chips" style="margin-bottom:16px">{#each [['open', 'Offen'], ['hidden', 'Ausgeblendet'], ['dismissed', 'Verworfen']] as [v, l]}<button class="chip" class:on={status === v} onclick={() => { status = v; loadReports(); }}>{l}</button>{/each}</div>
  {#if !reports}<div class="spinner"></div>{:else if !reports.length}<p class="muted">Keine Meldungen.</p>{/if}
  <div style="display:grid;gap:12px;max-width:760px">
    {#each reports || [] as r (r.id)}
      <div class="admin-item">
        <div style="display:flex;justify-content:space-between;gap:10px;flex-wrap:wrap"><b>{r.target === 'photo' ? 'Foto' : 'Kommentar'} zu <a href="/public/{r.strainId}">{r.strainName}</a></b><span class="muted" style="font-size:13px">{date(r.createdAt)}{r.openReports > 1 ? `, ${r.openReports} offene Meldungen` : ''}</span></div>
        <div><span class="pill">{r.reasonLabel}</span>{#if r.details}<span class="muted" style="font-size:14px"> {r.details}</span>{/if}</div>
        {#if r.target === 'comment'}<p class="notes" style="background:var(--surface);border-radius:10px;padding:10px 12px;white-space:pre-line">{r.comment || '(Kommentar inzwischen entfernt)'}</p>
        {:else if r.photoId}<div class="mini-img"><img src={photoURL(r.photoId)} alt="Gemeldetes Foto"></div>{/if}
        {#if status === 'open'}
          <div class="field"><label for="n{r.id}">Begründung für die Person (optional)</label><input id="n{r.id}" bind:value={notes[r.id]} maxlength="300"></div>
          <div class="row-actions"><button class="btn btn-danger" onclick={() => resolve(r, 'hide')}>Ausblenden</button><button class="btn btn-ghost" onclick={() => resolve(r, 'dismiss')}>Verwerfen</button></div>
        {/if}
      </div>
    {/each}
  </div>
{:else}
  <div class="toolbar"><label class="search"><input placeholder="Sorte suchen" aria-label="Sorte suchen" bind:value={q} oninput={loadStrains}></label></div>
  {#if mergeFrom}<div class="note warn" style="margin-bottom:14px"><span>Wähle die Ziel-Sorte, in die <b>{mergeFrom.name}</b> überführt werden soll. <button class="linkbtn" onclick={() => (mergeFrom = null)}>Abbrechen</button></span></div>{/if}
  {#if pinFor}
    <div class="inline-form" style="margin-bottom:16px">
      <b>Titelbild für {pinFor.name}</b>
      {#if !pinPhotos.length}<p class="muted">Für diese Sorte gibt es keine geteilten Fotos.</p>{/if}
      <div class="pickphotos">
        <button class="none" class:on={!pinFor.pinnedPhotoId} onclick={() => pin(null)}>Automatisch</button>
        {#each pinPhotos as p}<button class:on={pinFor.pinnedPhotoId === p} onclick={() => pin(p)} aria-label="Foto wählen"><img src={photoURL(p)} alt=""></button>{/each}
      </div>
      <div><button class="btn btn-ghost" onclick={() => (pinFor = null)}>Schließen</button></div>
    </div>
  {/if}
  <div class="timeline" style="max-width:860px">
    {#each strains as s (s.id)}
      <div class="tl" style="grid-template-columns:1fr auto auto"><span><b style="font-weight:500">{s.name}</b><br><span class="muted" style="font-size:13px">{s.entries} Einträge, {s.ratings} öffentlich</span></span>
        <span class="muted" style="font-size:13px">{s.pinnedPhotoId ? 'Titelbild gesetzt' : ''}</span>
        <span class="row-actions">
          {#if mergeFrom && mergeFrom.id !== s.id}<button class="btn btn-primary" onclick={() => merge(s)}>Hierhin</button>
          {:else if !mergeFrom}<button class="linkbtn" onclick={() => rename(s)}>Umbenennen</button><button class="linkbtn" onclick={() => (mergeFrom = s)}>Zusammenführen</button>{#if s.ratings}<button class="linkbtn" onclick={() => openPin(s)}>Titelbild</button>{/if}{/if}
        </span>
      </div>
    {/each}
  </div>
{/if}
{/if}
