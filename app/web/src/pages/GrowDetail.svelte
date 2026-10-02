<script>
  import { api } from '../lib/api.js';
  import { go } from '../lib/router.svelte.js';
  import { date, daysSince, addDays, today, score, parseNum } from '../lib/format.js';
  import { notify } from '../lib/toast.svelte.js';
  import * as O from '../lib/options.js';
  import Icon from '../components/Icon.svelte';
  import StrainInput from '../components/StrainInput.svelte';

  let { params } = $props();
  const isNew = !params.id;
  let g = $state({ name: '', strainName: '', location: '', seedType: '', environment: 'Indoor', medium: '', lightWatts: '', germinatedOn: today(), floweringOn: '', expectedFlowerDays: 63, harvestedOn: '', dryDays: '', cureWeeks: '', yieldGrams: '', notes: '' });
  let logs = $state([]), entries = $state([]), editing = $state(isNew), err = $state(''), busy = $state(false), loaded = $state(isNew);
  let log = $state({ loggedOn: today(), text: '' });

  function fill(x) { g = Object.fromEntries(Object.entries({ ...x, strainName: x.strainName || '' }).map(([k, v]) => [k, v ?? ''])); }
  if (!isNew) api('/api/grows/' + params.id).then(r => { fill(r.grow); logs = r.logs; entries = r.entries; loaded = true; }).catch(e => (err = e.message));

  const prog = $derived.by(() => {
    if (g.harvestedOn) return { phase: 'Geerntet', text: `am ${date(g.harvestedOn)}`, pct: 100 };
    if (g.floweringOn) { const d = daysSince(g.floweringOn), t = parseNum(g.expectedFlowerDays) || 63; return { phase: 'Blüte', text: `Tag ${d} von ca. ${t}`, pct: Math.min(100, d / t * 100), until: addDays(g.floweringOn, t) }; }
    if (g.germinatedOn) return { phase: 'Wachstum', text: `Tag ${daysSince(g.germinatedOn)} seit Keimung`, pct: 12 };
    return { phase: 'Geplant', text: '', pct: 0 };
  });

  async function save(e) {
    e.preventDefault(); busy = true; err = '';
    const ints = ['lightWatts', 'expectedFlowerDays', 'dryDays', 'cureWeeks', 'yieldGrams'];
    const body = { ...g };
    for (const k of ints) body[k] = parseNum(body[k]) == null ? null : Math.round(parseNum(body[k]));
    for (const k of ['germinatedOn', 'floweringOn', 'harvestedOn']) body[k] = body[k] || null;
    for (const k of ['id', 'strainId', 'createdAt', 'updatedAt', 'entryCount', 'lastLogAt']) delete body[k];
    try {
      const r = await api(isNew ? '/api/grows' : '/api/grows/' + params.id, { method: isNew ? 'POST' : 'PUT', body });
      notify('Grow gespeichert.');
      if (isNew) go('/grows/' + r.id, { replace: true }); else { fill(r); editing = false; }
    } catch (x) { err = x.message; } finally { busy = false; }
  }
  async function addLog(e) {
    e.preventDefault();
    try { const r = await api(`/api/grows/${params.id}/logs`, { method: 'POST', body: log }); logs = [r, ...logs].sort((a, b) => b.loggedOn.localeCompare(a.loggedOn)); log.text = ''; }
    catch (x) { err = x.message; }
  }
  async function delLog(id) { try { await api('/api/grow-logs/' + id, { method: 'DELETE' }); logs = logs.filter(l => l.id !== id); } catch (x) { err = x.message; } }
  async function del() {
    if (!confirm('Grow mit Logbuch löschen? Bewertungen bleiben erhalten.')) return;
    try { await api('/api/grows/' + params.id, { method: 'DELETE' }); go('/grows', { replace: true }); } catch (x) { err = x.message; }
  }
  const flipToday = () => { g.floweringOn = today(); };
  const harvestToday = () => { g.harvestedOn = today(); };
</script>

<a class="back" href="/grows"><Icon name="back" size={16} />Grows</a>
{#if err}<p class="err" style="margin-bottom:16px">{err}</p>{/if}
{#if !loaded}<div class="spinner"></div>{:else}
  <div class="head">
    <div><h1>{isNew ? 'Neuer Grow' : g.name}</h1>{#if !isNew}<p class="muted">{prog.phase}{prog.text ? `, ${prog.text}` : ''}</p>{/if}</div>
    {#if !isNew && !editing}
      <span class="row-actions">
        {#if g.harvestedOn}<a class="btn btn-primary" href="/entries/new?grow={params.id}">Ernte bewerten</a>{/if}
        <button class="btn btn-line" onclick={() => (editing = true)}><Icon name="edit" />Bearbeiten</button>
      </span>
    {/if}
  </div>

  {#if !isNew}
    <div class="growrun" style="margin-bottom:28px;max-width:640px">
      <div class="progress" style="--w:{prog.pct}%"><span></span></div>
      <div class="row" style="margin:0;font-size:13px"><span>{prog.phase}</span>{#if prog.until}<span class="muted">Ernte um den {date(prog.until)}</span>{/if}</div>
      {#if !editing && !g.harvestedOn}
        <div class="row-actions" style="margin-top:12px">
          {#if !g.floweringOn}<button class="btn btn-ghost" style="background:var(--bg)" onclick={async () => { flipToday(); await save(new Event('submit')); }}>Heute auf Blüte umgestellt</button>
          {:else}<button class="btn btn-ghost" style="background:var(--bg)" onclick={async () => { harvestToday(); await save(new Event('submit')); }}>Heute geerntet</button>{/if}
        </div>
      {/if}
    </div>
  {/if}

  {#if editing}
    <form class="form-wrap" onsubmit={save}>
      <div class="fgrid">
        <div class="field full"><label for="strain">Sorte</label><StrainInput bind:value={g.strainName} /></div>
        <div class="field"><label for="n">Name <span class="muted" style="font-weight:400">(optional)</span></label><input id="n" bind:value={g.name} maxlength="80" placeholder="z. B. Blue Dream, Zelt 2"></div>
        <div class="field"><label for="loc">Ort</label><input id="loc" bind:value={g.location} maxlength="60" placeholder="z. B. Zelt 2"></div>
        <div class="field"><label for="s">Saatgut</label><select id="s" bind:value={g.seedType}><option value=""></option>{#each O.SEEDS as o}<option>{o}</option>{/each}</select></div>
        <div class="field"><label for="env">Umgebung</label><select id="env" bind:value={g.environment}><option value=""></option>{#each O.ENVIRONMENTS as o}<option>{o}</option>{/each}</select></div>
        <div class="field"><label for="m">Medium</label><select id="m" bind:value={g.medium}><option value=""></option>{#each O.MEDIA as o}<option>{o}</option>{/each}</select></div>
        <div class="field"><label for="lw">Licht</label><div class="unit"><input id="lw" inputmode="numeric" bind:value={g.lightWatts}><span>W</span></div></div>
        <div class="field"><label for="d1">Keimung</label><input id="d1" type="date" bind:value={g.germinatedOn}></div>
        <div class="field"><label for="d2">Umstellung auf Blüte</label><input id="d2" type="date" bind:value={g.floweringOn}></div>
        <div class="field"><label for="d3">Erwartete Blütezeit</label><div class="unit"><input id="d3" inputmode="numeric" bind:value={g.expectedFlowerDays}><span>Tage</span></div></div>
        <div class="field"><label for="d4">Ernte</label><input id="d4" type="date" bind:value={g.harvestedOn}></div>
        <div class="field"><label for="d5">Trocknung</label><div class="unit"><input id="d5" inputmode="numeric" bind:value={g.dryDays}><span>Tage</span></div></div>
        <div class="field"><label for="d6">Curing</label><div class="unit"><input id="d6" inputmode="numeric" bind:value={g.cureWeeks}><span>Wochen</span></div></div>
        <div class="field"><label for="d7">Ertrag (trocken)</label><div class="unit"><input id="d7" inputmode="numeric" bind:value={g.yieldGrams}><span>g</span></div></div>
        <div class="field full"><label for="no">Notizen</label><textarea id="no" maxlength="5000" bind:value={g.notes}></textarea></div>
      </div>
      <div class="formfoot" style="margin-top:22px">
        {#if !isNew}<button type="button" class="btn btn-danger" onclick={del}><Icon name="trash" />Löschen</button>{:else}<span></span>{/if}
        <span class="row-actions">{#if !isNew}<button type="button" class="btn btn-ghost" onclick={() => (editing = false)}>Abbrechen</button>{/if}<button class="btn btn-primary" disabled={busy}>Speichern</button></span>
      </div>
    </form>
  {:else}
    <div class="facts" style="max-width:640px">
      {#each [['Sorte', g.strainName], ['Saatgut', g.seedType], ['Umgebung', g.environment], ['Medium', g.medium], ['Licht', g.lightWatts && `${g.lightWatts} W`], ['Keimung', g.germinatedOn && date(g.germinatedOn)], ['Blüte seit', g.floweringOn && date(g.floweringOn)], ['Ernte', g.harvestedOn && date(g.harvestedOn)], ['Trocknung', g.dryDays && `${g.dryDays} Tage`], ['Curing', g.cureWeeks && `${g.cureWeeks} Wochen`], ['Ertrag', g.yieldGrams && `${g.yieldGrams} g`]].filter(x => x[1]) as [k, v]}<div><span>{k}</span><b>{v}</b></div>{/each}
    </div>
    {#if g.notes}<p class="notes" style="white-space:pre-line">{g.notes}</p>{/if}

    {#if entries.length}
      <div class="dsec"><h2>Bewertungen aus diesem Grow</h2>
        <div class="timeline">{#each entries as e}<a class="tl" href="/entries/{e.id}" style="text-decoration:none;color:inherit"><time>{e.latest ? date(e.latest.tastedOn) : ''}</time><span>{e.strainName}</span>{#if e.latest}<span class="score">{score(e.latest.overall)}</span>{/if}</a>{/each}</div>
      </div>
    {/if}

    <div class="dsec" style="max-width:720px">
      <h2>Logbuch</h2>
      <form class="inline-form" onsubmit={addLog} style="margin:0 0 14px">
        <div class="fgrid">
          <div class="field"><label for="ld">Datum</label><input id="ld" type="date" bind:value={log.loggedOn}></div>
          <div class="field full"><label for="lt">Was ist passiert?</label><textarea id="lt" style="min-height:70px" maxlength="2000" bind:value={log.text} placeholder="z. B. Entlaubt, Dünger gewechselt, erste Trichome milchig"></textarea></div>
        </div>
        <div><button class="btn btn-primary" disabled={!log.text.trim()}>Eintragen</button></div>
      </form>
      <div class="timeline">{#each logs as l (l.id)}<div class="tl"><time>{date(l.loggedOn)}</time><span style="white-space:pre-line">{l.text}</span><button class="linkbtn" onclick={() => delLog(l.id)}>Löschen</button></div>{/each}</div>
    </div>
  {/if}
{/if}
