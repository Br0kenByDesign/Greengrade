<script>
  import { api } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { reauth, finishReauth } from '../lib/reauth.svelte.js';
  import { solve } from '../lib/pow.js';
  import { passkeyReauth, passkeysSupported } from '../lib/webauthn.js';
  import Icon from './Icon.svelte';

  let step = $state('choose'), email = $state(''), code = $state(''), busy = $state(false), err = $state(''), info = $state('');
  const me = $derived(session.me);
  const canPasskey = $derived(!!me?.passkeys?.length && passkeysSupported());
  const linked = $derived((me?.providers || []).filter(p => me?.identities?.includes(p.id)));

  $effect(() => { if (reauth.open) { step = 'choose'; email = ''; code = ''; err = ''; info = ''; } });

  async function run(fn) { busy = true; err = ''; try { await fn(); } catch (e) { if (e?.name !== 'NotAllowedError') err = e.message || 'Das hat nicht geklappt.'; } finally { busy = false; } }
  const withPasskey = () => run(async () => { await passkeyReauth(); finishReauth(true); });
  const sendCode = e => { e.preventDefault(); run(async () => {
    info = 'Sicherheitsprüfung läuft...';
    const pow = await solve(await api('/api/auth/challenge'));
    info = '';
    await api('/api/auth/magic', { method: 'POST', body: { email, pow, reauth: true } });
    step = 'code';
  }).finally(() => (info = '')); };
  const checkCode = e => { e.preventDefault(); run(async () => {
    await api('/api/auth/code', { method: 'POST', body: { code } });
    finishReauth(true);
  }); };
  function withProvider(id) { location.href = `/api/auth/oauth/${id}/start?mode=reauth`; }
</script>

{#if reauth.open && me}
  <div class="scrim" role="presentation" onclick={e => e.target === e.currentTarget && finishReauth(false)}>
    <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="ra-h">
      <h2 id="ra-h">Kurz bestätigen</h2>
      <p class="muted" style="font-size:14px">Für diese Aktion musst du dich neu bestätigen. So kann niemand, der kurz an dein entsperrtes Gerät kommt, dein Konto übernehmen.</p>

      {#if step === 'choose'}
        <div style="display:grid;gap:10px">
          {#if canPasskey}<button class="btn btn-primary" disabled={busy} onclick={withPasskey}><Icon name="key" />Mit Passkey bestätigen</button>{/if}
          {#if me.hasEmail}<button class="btn btn-line" disabled={busy} onclick={() => (step = 'mail')}><Icon name="mail" />Code per Mail</button>{/if}
          {#each linked as p}<button class="btn btn-line" disabled={busy} onclick={() => withProvider(p.id)}>Mit {p.label} bestätigen</button>{/each}
        </div>
      {:else if step === 'mail'}
        <form style="display:grid;gap:10px" onsubmit={sendCode}>
          <div class="field"><label for="ra-mail">E-Mail-Adresse dieses Kontos</label><input id="ra-mail" type="email" bind:value={email} autocomplete="email" required maxlength="254"></div>
          <button class="btn btn-primary" disabled={busy}>{info || 'Code senden'}</button>
        </form>
      {:else}
        <p style="font-size:14px">Falls das die Adresse dieses Kontos ist, haben wir dir einen Code geschickt.</p>
        <form class="codebox" style="margin:0;padding:0;border:0" onsubmit={checkCode}>
          <div class="field"><input bind:value={code} inputmode="text" autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" maxlength="7" placeholder="ABC-123" aria-label="Code aus der Mail" required></div>
          <button class="btn btn-primary" disabled={busy || code.replace(/[^a-z0-9]/gi, '').length !== 6}>Bestätigen</button>
        </form>
      {/if}

      {#if err}<p class="err">{err}</p>{/if}
      <div class="actions"><button class="btn btn-ghost" onclick={() => finishReauth(false)}>Abbrechen</button></div>
    </div>
  </div>
{/if}
