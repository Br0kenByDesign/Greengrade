<script>
  import { api, photoURL } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { score, month, CATS } from '../lib/format.js';
  import { notify } from '../lib/toast.svelte.js';
  import Icon from '../components/Icon.svelte';
  import Ring from '../components/Ring.svelte';
  import Bars from '../components/Bars.svelte';
  import StrainImage from '../components/StrainImage.svelte';
  import ReportDialog from '../components/ReportDialog.svelte';
  let { params } = $props();
  let s = $state(null), err = $state(''), sel = $state(0), report = $state(null);
  const load = () => api('/api/public/strains/' + params.id).then(x => (s = x)).catch(e => (err = e.message));
  load();
  const photo = $derived(s?.photos[sel]);
  const mx = $derived(Math.max(1, ...(s?.histogram || [])));
  async function adminHide(ratingId, target) {
    const note = prompt('Begründung für die Person (optional):', '');
    if (note === null) return;
    const ban = confirm('Zusätzlich das öffentliche Teilen für dieses Konto sperren?\n\nOK = ausblenden und sperren, Abbrechen = nur ausblenden');
    try { await api(`/api/admin/ratings/${ratingId}/hide`, { method: 'POST', body: { target, reason: 'other', note, ban } }); notify('Ausgeblendet.'); sel = 0; load(); } catch (e) { err = e.message; }
  }
</script>

<a class="back" href="/public"><Icon name="back" size={16} />Öffentliche Bewertungen</a>
{#if err}<p class="err">{err}</p>{/if}
{#if s}
  <div class="dhero">
    <div class="gallery">
      <div class="big">
        <StrainImage name={s.name} photoId={photo?.photoId} thumb={false} />
        <span class="tag">{photo ? 'Community-Foto' : 'Illustration'}</span>
        {#if photo && !photo.mine}<button class="report" onclick={() => (report = { ratingId: photo.ratingId, target: 'photo' })}>Foto melden</button>{/if}
      </div>
      {#if s.photos.length > 1}<div class="row">{#each s.photos as p, i}<button class:on={i === sel} onclick={() => (sel = i)} aria-label="Foto {i + 1}"><img src={photoURL(p.photoId)} alt=""></button>{/each}</div>{/if}
      {#if session.me.isAdmin && photo}<button class="linkbtn" style="margin-top:8px" onclick={() => adminHide(photo.ratingId, 'photo')}>Foto ausblenden (Moderation)</button>{/if}
    </div>
    <div>
      <div class="dtitle">
        <div><h1>{s.name}</h1><p class="muted" style="margin-top:6px">{s.count} öffentliche Bewertung{s.count === 1 ? '' : 'en'}{s.since ? ` seit ${month(s.since)}` : ''} <button class="linkbtn" onclick={() => (report = { strainId: s.id, target: 'name' })}>Name melden</button></p></div>
        <Ring value={s.avg} label="Ø von 10" />
      </div>
      {#if s.mine}<div class="mine"><span>Deine Bewertung: <b>{score(s.mine.overall)}</b></span><a class="btn btn-ghost" style="background:var(--bg)" href="/entries/{s.mine.entryId}">Zu deinem Eintrag</a></div>{/if}
      {#if s.count}
        <div class="dsec" style="margin-top:28px"><h2>Teilnoten</h2><Bars items={CATS.map(([k, n]) => ({ label: n, value: s.categories[k] }))} /></div>
        <div class="dsec" style="margin-top:28px"><h2>Verteilung</h2>
          <div class="histo">{#each s.histogram as n, i}<div class:hi={n === mx && n > 0} style="--h:{n / mx * 100}%" title="{n} Bewertungen mit {i + 1}"></div>{/each}</div>
          <div class="histo-x">{#each s.histogram as _, i}<span>{i + 1}</span>{/each}</div>
        </div>
      {/if}
    </div>
  </div>
  <div class="dsec">
    <h2>Kommentare</h2>
    {#if !s.comments.length}<p class="muted">Noch keine Kommentare.</p>{/if}
    <div class="comments">
      {#each s.comments as c (c.ratingId)}
        <article class="cmt">
          <header><div><span class="score">{score(c.overall)}</span><time>{month(c.month)}</time>{#if c.mine}<span class="pill">Von dir</span>{/if}</div>
            <span class="row-actions">{#if !c.mine}<button class="linkbtn" onclick={() => (report = { ratingId: c.ratingId, target: 'comment' })}>Melden</button>{/if}{#if session.me.isAdmin}<button class="linkbtn" onclick={() => adminHide(c.ratingId, 'comment')}>Ausblenden</button>{/if}</span></header>
          <p style="white-space:pre-line">{c.comment}</p>
        </article>
      {/each}
    </div>
  </div>
{:else if !err}<div class="spinner"></div>{/if}
<ReportDialog bind:target={report} />
