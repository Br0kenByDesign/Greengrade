<script>
  import { api } from '../lib/api.js';
  import { notify } from '../lib/toast.svelte.js';
  import { REPORT_REASONS } from '../lib/options.js';
  // target: { ratingId, target: 'comment'|'photo' } or null
  let { target = $bindable(null) } = $props();
  let reason = $state('sale'), details = $state(''), busy = $state(false), err = $state('');
  async function send() {
    busy = true; err = '';
    try {
      await api('/api/public/reports', { method: 'POST', body: { ratingId: target.ratingId, target: target.target, reason, details } });
      target = null; details = ''; notify('Meldung gesendet. Wir prüfen den Inhalt.');
    } catch (e) { err = e.message; } finally { busy = false; }
  }
</script>
{#if target}
  <div class="scrim" role="presentation" onclick={e => e.target === e.currentTarget && (target = null)}>
    <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="rep-h">
      <h2 id="rep-h">{target.target === 'photo' ? 'Foto melden' : 'Kommentar melden'}</h2>
      <p class="muted" style="font-size:14px">Was stimmt damit nicht? Wir prüfen jede Meldung. Wer gemeldet hat, erfährt niemand.</p>
      <div>{#each REPORT_REASONS as [v, l]}<label class="radio"><input type="radio" bind:group={reason} value={v}>{l}</label>{/each}</div>
      <div class="field"><label for="rep-t">Details <span class="muted" style="font-weight:400">(optional)</span></label><textarea id="rep-t" style="min-height:80px" maxlength="500" bind:value={details}></textarea></div>
      {#if err}<p class="err">{err}</p>{/if}
      <div class="actions"><button class="btn btn-ghost" onclick={() => (target = null)}>Abbrechen</button><button class="btn btn-primary" disabled={busy} onclick={send}>Meldung senden</button></div>
    </div>
  </div>
{/if}
