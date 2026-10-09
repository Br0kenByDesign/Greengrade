<script>
  import { api } from '../lib/api.js';
  import { solve } from '../lib/pow.js';
  import { session } from '../lib/session.svelte.js';
  import Logo from '../components/Logo.svelte';
  import Icon from '../components/Icon.svelte';

  let name = $state(''), email = $state(''), message = $state('');
  let busy = $state(false), err = $state(''), sent = $state(false);
  let enabled = $state(null), privacyUrl = $state(''), imprintUrl = $state('');
  api('/api/auth/config').then(c => { enabled = !!c.contact; privacyUrl = c.privacyUrl; imprintUrl = c.imprintUrl; }).catch(() => (enabled = false));

  async function send(e) {
    e.preventDefault(); err = ''; busy = true;
    try {
      const ch = await api('/api/auth/challenge');
      const pow = await solve(ch);
      await api('/api/contact', { method: 'POST', body: { name, email, message, pow } });
      sent = true;
    } catch (e) { err = e.message; }
    busy = false;
  }
</script>

<div class="solo">
  <a class="brand" href={session.me ? '/' : '/login'}><Logo />greengrade</a>
  {#if enabled === null}
    <div class="spinner" role="status" aria-label="Lädt"></div>
  {:else if !enabled}
    <h1>Kontakt</h1>
    <p class="muted">Das Kontaktformular ist auf dieser Instanz nicht eingerichtet.{#if imprintUrl}{' '}Die Kontaktdaten findest du im <a href={imprintUrl} target="_blank" rel="noopener">Impressum</a>.{/if}</p>
  {:else if sent}
    <div class="mailicon"><Icon name="mail" size={26} /></div>
    <h1>Danke für deine Nachricht</h1>
    <p class="muted">Sie ist bei uns angekommen. Wir antworten so schnell wie möglich an <b style="color:var(--ink)">{email}</b>.</p>
  {:else}
    <h1>Kontakt</h1>
    <p class="muted">Fragen, Hinweise oder Widerspruch gegen eine Moderationsentscheidung? Schreib uns. Rechtswidrige Inhalte meldest du am schnellsten direkt in der App über "Melden".</p>
    <form class="stack" onsubmit={send}>
      <div class="field"><label for="c-name">Name <span class="muted">(optional)</span></label><input id="c-name" bind:value={name} maxlength="80" autocomplete="name"></div>
      <div class="field"><label for="c-mail">E-Mail-Adresse für die Antwort</label><input id="c-mail" type="email" bind:value={email} maxlength="254" autocomplete="email" required></div>
      <div class="field"><label for="c-msg">Nachricht</label><textarea id="c-msg" bind:value={message} maxlength="4000" rows="7" required></textarea><div class="hint">{message.length} / 4000</div></div>
      <button class="btn btn-primary" disabled={busy || message.trim().length < 10 || !email}>{busy ? 'Wird gesendet …' : 'Nachricht senden'}</button>
    </form>
    {#if err}<p class="err" style="margin-top:16px">{err}</p>{/if}
    <small>Wir speichern deine Nachricht nicht in der App. Sie wird nur als E-Mail an uns weitergeleitet, damit wir dir antworten können. Bitte schick uns hier keine Gesundheitsdaten.{#if privacyUrl}{' '}Mehr in der <a href={privacyUrl} target="_blank" rel="noopener">Datenschutzerklärung</a>.{/if}</small>
  {/if}
</div>
