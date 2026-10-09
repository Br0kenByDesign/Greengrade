<script>
  import { DRAFT_KEY } from '../lib/local.js';
  import { api, photoURL } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { route, go } from '../lib/router.svelte.js';
  import { today, parseNum, date } from '../lib/format.js';
  import { preparePhoto } from '../lib/image.js';
  import * as O from '../lib/options.js';
  import Icon from '../components/Icon.svelte';
  import Rate from '../components/Rate.svelte';
  import Chips from '../components/Chips.svelte';
  import Seg from '../components/Seg.svelte';
  import Switch from '../components/Switch.svelte';
  import StrainInput from '../components/StrainInput.svelte';

  let { params } = $props();
  const editId = params.id || null;
  const DRAFT = DRAFT_KEY;

  const blankTasting = () => ({ tastedOn: today(), method: 'Vaporizer', temperature: '', onsetMin: '', durationH: '', smell: 0, taste: 0, look: 0, effect: 0, quality: 0, aromas: [], flavors: [], effects: [], sideEffects: [], daytime: [], note: '' });
  let f = $state({ strainName: '', source: 'pharmacy', form: 'flower', genetics: 50, growId: '', notes: '', rebuy: false, details: { terpenes: [] } });
  let t = $state(blankTasting());
  let share = $state({ enabled: false, comment: '', photo: null });
  let existing = $state([]), pending = $state([]), grows = $state([]);
  let loading = $state(!!editId), busy = $state(false), err = $state(''), draft = $state(null), activeStep = $state('s1');

  api('/api/grows').then(g => (grows = g)).catch(() => {});
  if (editId) {
    api('/api/entries/' + editId).then(e => {
      const en = e.entry;
      f = { strainName: en.strainName, source: en.source, form: en.form, genetics: en.genetics, growId: en.growId || '', notes: e.notes, rebuy: en.rebuy, details: { terpenes: [], ...en.details } };
      if (en.latest) t = { ...blankTasting(), ...Object.fromEntries(Object.entries(en.latest).map(([k, v]) => [k, v ?? ''])) };
      existing = e.photos;
      share = { enabled: !!e.share, comment: e.share?.comment || '', photo: e.share?.photoId ? { id: e.share.photoId } : null };
      loading = false;
    }).catch(e => { err = e.message; loading = false; });
  } else {
    try { const d = JSON.parse(localStorage.getItem(DRAFT)); if (d?.f) draft = d; } catch {}
    const gid = route.query.get('grow');
    if (gid) api('/api/grows/' + gid).then(({ grow: g }) => {
      f.source = 'grow'; f.growId = g.id; f.strainName = g.strainName || g.name;
      Object.assign(f.details, { seedType: g.seedType, environment: g.environment, medium: g.medium, germinatedOn: g.germinatedOn || '', harvestedOn: g.harvestedOn || '',
        flowerDays: g.expectedFlowerDays ?? '', dryDays: g.dryDays ?? '', cureWeeks: g.cureWeeks ?? '', lightWatts: g.lightWatts ?? '', yieldGrams: g.yieldGrams ?? '' });
    }).catch(() => {});
  }

  const unit = $derived(f.form === 'extract' ? 'mg/ml' : '%');
  const total = $derived([t.smell, t.taste, t.look, t.effect, t.quality].every(Boolean) ? Math.round((t.smell + t.taste + t.look + t.effect + t.quality) / 5 * 10) / 10 : null);
  const steps = [['s1', 'Grunddaten'], ['s2', 'Herkunft'], ['s3', 'Sensorik'], ['s4', 'Wirkung'], ['s5', 'Teilen']];

  $effect(() => {
    if (loading) return;
    const io = new IntersectionObserver(es => es.forEach(e => e.isIntersecting && (activeStep = e.target.id)), { rootMargin: '-40% 0px -55% 0px' });
    steps.forEach(([id]) => { const el = document.getElementById(id); el && io.observe(el); });
    return () => io.disconnect();
  });

  async function addPhotos(e) {
    err = '';
    for (const file of [...e.target.files].slice(0, 12 - existing.length - pending.length)) {
      try { const blob = await preparePhoto(file); pending = [...pending, { blob, url: URL.createObjectURL(blob) }]; }
      catch (x) { err = x.message; }
    }
    e.target.value = '';
  }
  function removePending(i) {
    URL.revokeObjectURL(pending[i].url);
    if (share.photo?.pending === i) share.photo = null;
    pending = pending.filter((_, j) => j !== i);
  }
  async function removeExisting(id) {
    if (!confirm('Foto endgültig löschen?')) return;
    try { await api('/api/photos/' + id, { method: 'DELETE' }); existing = existing.filter(p => p.id !== id); if (share.photo?.id === id) share.photo = null; }
    catch (x) { err = x.message; }
  }
  function saveDraft() { localStorage.setItem(DRAFT, JSON.stringify({ f, t, share: { ...share, photo: null }, at: new Date().toISOString() })); err = ''; alert('Entwurf auf diesem Gerät gespeichert. Fotos werden im Entwurf nicht gespeichert.'); }
  function restoreDraft() { f = draft.f; t = draft.t; share = draft.share; draft = null; }
  function dropDraft() { localStorage.removeItem(DRAFT); draft = null; }

  function cleanDetails(d) {
    const out = {};
    for (const [k, v] of Object.entries(d)) if (v !== '' && v != null && !(Array.isArray(v) && !v.length)) out[k] = v;
    return out;
  }

  async function submit(e) {
    e.preventDefault();
    err = '';
    if (!f.strainName.trim()) { err = 'Bitte gib den Namen der Sorte ein.'; return; }
    if (!total) { err = 'Bitte vergib in allen fünf Kategorien eine Note (Geruch, Geschmack, Aussehen, Wirkung, Qualität).'; document.getElementById('s3').scrollIntoView({ behavior: 'smooth' }); return; }
    busy = true;
    try {
      const body = { ...f, growId: f.growId || null, details: cleanDetails(f.details),
        tasting: { ...t, temperature: parseNum(t.temperature), onsetMin: parseNum(t.onsetMin), durationH: parseNum(t.durationH) } };
      ['id', 'overall', 'createdAt'].forEach(k => delete body.tasting[k]);
      body.tasting.temperature = body.tasting.temperature === null ? null : Math.round(body.tasting.temperature);
      body.tasting.onsetMin = body.tasting.onsetMin === null ? null : Math.round(body.tasting.onsetMin);
      const res = await api(editId ? '/api/entries/' + editId : '/api/entries', { method: editId ? 'PUT' : 'POST', body });
      const id = res.entry.id;
      const uploaded = [];
      for (const p of pending) {
        const fd = new FormData(); fd.append('file', p.blob, 'foto.jpg');
        uploaded.push((await api(`/api/entries/${id}/photos`, { method: 'POST', form: fd })).id);
      }
      if ((share.enabled || editId) && !session.me?.shareBanned) {
        const photoId = share.photo?.id ?? (share.photo?.pending != null ? uploaded[share.photo.pending] : null);
        await api(`/api/entries/${id}/share`, { method: 'PUT', body: { shared: share.enabled, comment: share.comment, photoId } });
      }
      if (!editId) localStorage.removeItem(DRAFT);
      pending.forEach(p => URL.revokeObjectURL(p.url));
      go('/entries/' + id, { replace: true });
    } catch (x) { err = x.message; busy = false; }
  }
</script>

<div class="form-wrap">
  <a class="back" href={editId ? '/entries/' + editId : '/'}><Icon name="back" size={16} />{editId ? 'Zurück zum Eintrag' : 'Übersicht'}</a>
  <h1>{editId ? 'Eintrag bearbeiten' : 'Neue Sorte bewerten'}</h1>

  {#if draft}
    <div class="note" style="margin-top:18px;justify-content:space-between;align-items:center;flex-wrap:wrap">
      <span>Du hast einen Entwurf vom {date(draft.at)}.</span>
      <span class="row-actions"><button class="btn btn-primary" onclick={restoreDraft}>Wiederherstellen</button><button class="btn btn-ghost" onclick={dropDraft}>Verwerfen</button></span>
    </div>
  {/if}

  {#if loading}<div class="spinner"></div>{:else}
  <nav class="steps" aria-label="Abschnitte">
    {#each steps as [id, l], i}<a href="#{id}" class:on={activeStep === id} class:done={steps.findIndex(s => s[0] === activeStep) > i} onclick={e => { e.preventDefault(); e.stopPropagation(); document.getElementById(id).scrollIntoView({ behavior: 'smooth' }); }}>{i + 1} {l}</a>{/each}
  </nav>

  <form onsubmit={submit} novalidate>
    <fieldset id="s1">
      <legend>Grunddaten</legend>
      <p class="lead">Was hast du vor dir? Fotos kannst du auch später noch ergänzen.</p>
      <div class="fgrid">
        <div class="field full"><label for="strain">Sorte</label><StrainInput bind:value={f.strainName} /></div>
        <div class="full"><span class="flabel">Woher kommt sie?</span><Seg label="Herkunft" bind:value={f.source} options={[['grow', 'Eigener Grow', 'Selbst angebaut'], ['pharmacy', 'Apotheke', 'Rezept']]} /></div>
        <div class="full"><span class="flabel">Form</span><Seg label="Form" bind:value={f.form} options={[['flower', 'Blüten'], ['extract', 'Extrakt']]} /></div>
        <div class="full spectrum"><label class="flabel" for="gen">Genetik</label>
          <input id="gen" type="range" min="0" max="100" step="5" bind:value={f.genetics}>
          <div class="ends"><span>Indica</span><span>Hybrid</span><span>Sativa</span></div>
        </div>
        <div class="full"><span class="flabel">Fotos</span>
          <div class="drop">
            {#each existing as p (p.id)}<div class="thumbwrap"><div class="thumb"><img src={photoURL(p.id)} alt=""></div><button type="button" class="x" aria-label="Foto löschen" onclick={() => removeExisting(p.id)}>×</button></div>{/each}
            {#each pending as p, i (p.url)}<div class="thumbwrap"><div class="thumb"><img src={p.url} alt=""></div><button type="button" class="x" aria-label="Foto entfernen" onclick={() => removePending(i)}>×</button></div>{/each}
            {#if existing.length + pending.length < 12}
              <label class="add" aria-label="Foto hinzufügen"><Icon name="camera" size={22} /><input type="file" accept="image/*" multiple hidden onchange={addPhotos}></label>
            {/if}
            <p>Kamera oder Galerie. Jedes Foto wird neu gespeichert - Standort, Aufnahmezeit und alle anderen Metadaten werden dabei entfernt.</p>
          </div>
        </div>
      </div>
    </fieldset>

    <fieldset id="s2">
      <legend>Herkunft</legend>
      {#if f.source === 'pharmacy'}
        <p class="lead">Die Angaben stehen auf dem Etikett oder im Analysezertifikat. Sie bleiben immer privat.</p>
        <div class="fgrid">
          <div class="field"><label for="d1">Hersteller</label><input id="d1" bind:value={f.details.manufacturer} maxlength="120"></div>
          <div class="field"><label for="d2">Produktname</label><input id="d2" bind:value={f.details.product} maxlength="120"></div>
          <div class="field"><label for="d3">THC</label><div class="unit"><input id="d3" inputmode="decimal" bind:value={f.details.thc}><span>{unit}</span></div></div>
          <div class="field"><label for="d4">CBD</label><div class="unit"><input id="d4" inputmode="decimal" bind:value={f.details.cbd}><span>{unit}</span></div></div>
          <div class="field"><label for="d5">Charge</label><input id="d5" bind:value={f.details.batch} maxlength="60"></div>
          <div class="field"><label for="d6">Bestrahlt</label><select id="d6" bind:value={f.details.irradiated}><option value="">Unbekannt</option><option>Ja</option><option>Nein</option></select></div>
          <div class="field"><label for="d7">Preis</label><div class="unit"><input id="d7" inputmode="decimal" bind:value={f.details.price}><span>€ / {f.form === 'extract' ? 'ml' : 'g'}</span></div></div>
          <div class="field"><label for="d8">Abgabedatum</label><input id="d8" type="date" bind:value={f.details.purchasedOn}></div>
          <div class="field full"><label for="d9">Apotheke</label><input id="d9" bind:value={f.details.pharmacy} maxlength="120"></div>
        </div>
      {:else}
        <p class="lead">Halte fest, was zum Ergebnis geführt hat - damit der nächste Durchgang besser wird.</p>
        <div class="fgrid">
          {#if grows.length}
            <div class="field full"><label for="g0">Aus deinem Grow-Logbuch</label>
              <select id="g0" bind:value={f.growId}><option value="">Kein verknüpfter Grow</option>{#each grows as g}<option value={g.id}>{g.name}</option>{/each}</select>
            </div>
          {/if}
          <div class="field"><label for="g1">Saatgut</label><select id="g1" bind:value={f.details.seedType}><option value=""></option>{#each O.SEEDS as o}<option>{o}</option>{/each}</select></div>
          <div class="field"><label for="g2">Breeder</label><input id="g2" bind:value={f.details.breeder} maxlength="80"></div>
          <div class="field"><label for="g3">Umgebung</label><select id="g3" bind:value={f.details.environment}><option value=""></option>{#each O.ENVIRONMENTS as o}<option>{o}</option>{/each}</select></div>
          <div class="field"><label for="g4">Medium</label><select id="g4" bind:value={f.details.medium}><option value=""></option>{#each O.MEDIA as o}<option>{o}</option>{/each}</select></div>
        </div>
        <div class="sub">
          <h3>Zeiten</h3>
          <p class="lead">Die Blütezeit zählt ab Umstellung auf 12/12.</p>
          <div class="fgrid">
            <div class="field"><label for="g5">Keimung</label><input id="g5" type="date" bind:value={f.details.germinatedOn}></div>
            <div class="field"><label for="g6">Wachstumsphase</label><div class="unit"><input id="g6" inputmode="numeric" bind:value={f.details.vegWeeks}><span>Wochen</span></div></div>
            <div class="field"><label for="g7">Blütezeit</label><div class="unit"><input id="g7" inputmode="numeric" bind:value={f.details.flowerDays}><span>Tage</span></div></div>
            <div class="field"><label for="g8">Ernte</label><input id="g8" type="date" bind:value={f.details.harvestedOn}></div>
            <div class="field"><label for="g9">Trocknung</label><div class="unit"><input id="g9" inputmode="numeric" bind:value={f.details.dryDays}><span>Tage</span></div></div>
            <div class="field"><label for="g10">Curing</label><div class="unit"><input id="g10" inputmode="numeric" bind:value={f.details.cureWeeks}><span>Wochen</span></div></div>
            <div class="field"><label for="g11">Licht</label><div class="unit"><input id="g11" inputmode="numeric" bind:value={f.details.lightWatts}><span>W</span></div></div>
            <div class="field"><label for="g12">Ertrag (trocken)</label><div class="unit"><input id="g12" inputmode="numeric" bind:value={f.details.yieldGrams}><span>g</span></div></div>
            <div class="field full"><label for="g13">Was würdest du beim nächsten Mal anders machen?</label><textarea id="g13" maxlength="200" bind:value={f.details.nextTime} placeholder="z. B. früher entlauben, länger trocknen"></textarea></div>
          </div>
        </div>
      {/if}
      <div class="sub">
        <h3>Terpene</h3>
        <p class="lead">Laut Analyse oder nach deiner Nase.</p>
        <Chips label="Terpene" options={O.TERPENES} bind:selected={f.details.terpenes} />
      </div>
    </fieldset>

    <fieldset id="s3">
      <legend>Sensorik</legend>
      <p class="lead">Tippe auf die Skala von 1 bis 10. Die Aromen helfen später beim Suchen und Vergleichen.</p>
      <div class="sense">
        <div><span class="flabel">Geruch</span><Rate label="Geruch" bind:value={t.smell} /></div>
        <Chips label="Aromen" options={O.AROMAS} bind:selected={t.aromas} />
        <div class="sub" style="margin-top:6px;padding-top:20px"><span class="flabel">Geschmack</span><Rate label="Geschmack" bind:value={t.taste} /></div>
        <Chips label="Geschmack" options={O.FLAVORS} bind:selected={t.flavors} />
        <div class="sub" style="margin-top:6px;padding-top:20px"><span class="flabel">Aussehen</span><Rate label="Aussehen" bind:value={t.look} /></div>
        {#if f.form === 'flower'}
          <div class="fgrid">
            <div class="field"><label for="a1">Trichome</label><select id="a1" bind:value={f.details.trichomes}><option value=""></option>{#each O.TRICHOMES as o}<option>{o}</option>{/each}</select></div>
            <div class="field"><label for="a2">Dichte</label><select id="a2" bind:value={f.details.density}><option value=""></option>{#each O.DENSITY as o}<option>{o}</option>{/each}</select></div>
            <div class="field"><label for="a3">Trim</label><select id="a3" bind:value={f.details.trim}><option value=""></option>{#each O.TRIM as o}<option>{o}</option>{/each}</select></div>
            <div class="field"><label for="a4">Feuchtigkeit</label><select id="a4" bind:value={f.details.moisture}><option value=""></option>{#each O.MOISTURE as o}<option>{o}</option>{/each}</select></div>
          </div>
        {/if}
      </div>
    </fieldset>

    <fieldset id="s4">
      <legend>Wirkung</legend>
      <p class="lead">Wie hast du sie konsumiert und wie hat sie gewirkt?</p>
      <div class="fgrid">
        <div class="field"><label for="w0">Datum der Verkostung</label><input id="w0" type="date" bind:value={t.tastedOn}></div>
        <div class="field"><label for="w1">Konsum</label><select id="w1" bind:value={t.method}>{#each O.METHODS as o}<option>{o}</option>{/each}</select></div>
        {#if t.method === 'Vaporizer' || t.method === 'Inhalator'}<div class="field"><label for="w2">Temperatur</label><div class="unit"><input id="w2" inputmode="numeric" bind:value={t.temperature}><span>°C</span></div></div>{/if}
        <div class="field"><label for="w3">Wirkungseintritt</label><div class="unit"><input id="w3" inputmode="numeric" bind:value={t.onsetMin}><span>Min.</span></div></div>
        <div class="field"><label for="w4">Wirkdauer</label><div class="unit"><input id="w4" inputmode="decimal" bind:value={t.durationH}><span>Std.</span></div></div>
      </div>
      <div class="sub">
        <span class="flabel">Wirkung</span>
        <div style="margin-bottom:14px"><Rate label="Wirkung" bind:value={t.effect} /></div>
        <div style="margin-bottom:14px"><Chips label="Wirkungen" options={O.EFFECTS} bind:selected={t.effects} /></div>
        <span class="flabel">Nebenwirkungen</span>
        <Chips label="Nebenwirkungen" options={O.SIDE_EFFECTS} bind:selected={t.sideEffects} />
      </div>
      <div class="sub"><span class="flabel">Qualität und Verarbeitung</span><Rate label="Qualität" bind:value={t.quality} /></div>
      <div class="sub">
        <div class="field"><label for="n1">Private Notizen</label><textarea id="n1" maxlength="5000" bind:value={f.notes}></textarea></div>
        <div class="toggle"><div><div style="font-weight:500">Wieder holen</div><div class="muted" style="font-size:13px">Erscheint in deiner Merkliste.</div></div><Switch label="Wieder holen" bind:checked={f.rebuy} /></div>
        <div class="toggle" style="flex-wrap:wrap"><div><div style="font-weight:500">Tageszeit</div><div class="muted" style="font-size:13px">Wann passt sie für dich?</div></div><Chips label="Tageszeit" options={O.DAYTIME} bind:selected={t.daytime} /></div>
      </div>
      <div class="verdict">
        <div><div style="font-weight:500">Deine Gesamtnote</div><div class="muted" style="font-size:13px">Durchschnitt aus fünf Kategorien</div></div>
        <b>{total ? String(total).replace('.', ',') : '-'}</b>
      </div>
    </fieldset>

    <fieldset id="s5">
      <legend>Teilen</legend>
      <p class="lead">Deine Bewertung bleibt privat, solange du sie nicht freigibst.</p>
      {#if session.me?.shareBanned}
        <div class="note warn"><Icon name="info" size={20} /><span>Öffentliches Teilen ist für dein Konto gesperrt. Die Begründung findest du unter Konto. Deine Bewertung wird privat gespeichert.</span></div>
      {:else}
      <div class="share">
        <div class="toggle">
          <div><div style="font-weight:500">Bewertung öffentlich teilen</div><div class="muted" style="font-size:13px">Fließt anonym in die öffentliche Sortenseite ein.</div></div>
          <Switch label="Bewertung öffentlich teilen" bind:checked={share.enabled} />
        </div>
        {#if share.enabled}
          <div class="share-body">
            <div class="note"><Icon name="info" size={20} /><span>Öffentlich sichtbar sind nur deine Gesamtnote, die fünf Teilnoten und - wenn du willst - ein Kommentar und ein Foto. Wer du bist, woher die Sorte kommt und wie du sie konsumiert hast, bleibt privat.</span></div>
            <div class="field">
              <label for="pubc">Öffentlicher Kommentar <span class="muted" style="font-weight:400">(optional)</span></label>
              <textarea id="pubc" maxlength="500" bind:value={share.comment}></textarea>
              <div class="hint">{share.comment.length} / 500 Zeichen. Keine Links, Telefonnummern oder Messenger-Namen.</div>
            </div>
            {#if existing.length + pending.length}
              <div>
                <span class="flabel">Foto auf der Sortenseite zeigen <span class="muted" style="font-weight:400">(optional)</span></span>
                <div class="pickphotos">
                  <button type="button" class="none" class:on={!share.photo} onclick={() => (share.photo = null)}>Kein Foto</button>
                  {#each existing as p}<button type="button" class:on={share.photo?.id === p.id} onclick={() => (share.photo = { id: p.id })} aria-label="Foto wählen"><img src={photoURL(p.id)} alt=""></button>{/each}
                  {#each pending as p, i}<button type="button" class:on={share.photo?.pending === i} onclick={() => (share.photo = { pending: i })} aria-label="Foto wählen"><img src={p.url} alt=""></button>{/each}
                </div>
              </div>
              <div class="note warn"><Icon name="info" size={20} /><span>Achte darauf, dass kein Apothekenetikett mit deinem Namen oder deiner Adresse zu sehen ist.</span></div>
            {/if}
          </div>
        {/if}
      </div>
      {/if}
    </fieldset>

    {#if err}<p class="err" style="margin-bottom:16px">{err}</p>{/if}
    <div class="formfoot">
      {#if !editId}<button type="button" class="btn btn-line" onclick={saveDraft}>Als Entwurf speichern</button>{:else}<span></span>{/if}
      <button class="btn btn-primary" disabled={busy}>{busy ? 'Wird gespeichert...' : editId ? 'Änderungen speichern' : 'Bewertung speichern'}</button>
    </div>
  </form>
  {/if}
</div>
