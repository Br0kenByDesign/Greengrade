<script>
  import { route, go, clearHash } from '../lib/router.svelte.js';
  import { api, loadMe } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { solve } from '../lib/pow.js';
  import { passkeyLogin, passkeyRegister, passkeysSupported } from '../lib/webauthn.js';
  import Logo from '../components/Logo.svelte';
  import Icon from '../components/Icon.svelte';

  const PENDING = 'gg-pending-login';
  let step = $state('start'), email = $state(''), emailHint = $state(''), busy = $state(false), err = $state(''), info = $state('');
  let token = $state(''), name = $state(''), age = $state(false), consent = $state(false), countdown = $state(0);
  let code = $state('');
  let providers = $state([]), privacyUrl = $state(''), imprintUrl = $state(''), termsUrl = $state('');

  // capture one-time tokens from the URL fragment and remove them from the address bar immediately
  if (route.path === '/auth/verify' && route.hash) { token = route.hash; step = 'verify'; clearHash(); }
  else if (route.path === '/auth/setup' && route.hash) { token = route.hash; step = 'setup'; clearHash(); }
  else if (route.path === '/auth/passkey') step = 'passkey';
  else {
    // came back to the app after reading the mail (e.g. installed app on iOS): show the code field again
    try {
      // only a masked hint is stored (m•••@posteo.de), never the address itself
      const p = JSON.parse(localStorage.getItem(PENDING));
      if (p && Date.now() - p.at < 10 * 60000) { emailHint = p.hint || ''; step = 'sent'; }
      else if (p) localStorage.removeItem(PENDING);
    } catch {}
  }
  if (route.query.get('fehler')) err = 'Die Anmeldung beim Anbieter hat nicht geklappt. Bitte versuche es erneut.';

  api('/api/auth/config').then(c => { providers = c.providers; privacyUrl = c.privacyUrl; imprintUrl = c.imprintUrl; termsUrl = c.termsUrl || ''; }).catch(() => {});

  function maskEmail(e) {
    const [local, domain] = e.trim().toLowerCase().split('@');
    return local && domain ? `${local[0]}•••@${domain}` : '';
  }
  function clearPending() { try { localStorage.removeItem(PENDING); } catch {} }
  async function afterLogin() {
    clearPending();
    const me = await loadMe();
    if (me && passkeysSupported() && me.passkeys.length === 0 && !localStorage.getItem('gg-skip-passkey')) { step = 'passkey'; go('/auth/passkey', { replace: true }); }
    else go('/', { replace: true });
  }
  async function run(fn) { busy = true; err = ''; try { await fn(); } catch (e) { if (e?.name !== 'NotAllowedError') err = e.message || 'Das hat nicht geklappt.'; } finally { busy = false; } }

  const loginPasskey = () => run(async () => { await passkeyLogin(); await afterLogin(); });
  const sendLink = e => { e.preventDefault(); run(async () => {
    info = 'Sicherheitsprüfung läuft...';
    const ch = await api('/api/auth/challenge');
    const pow = await solve(ch);
    info = '';
    await api('/api/auth/magic', { method: 'POST', body: { email, pow } });
    emailHint = maskEmail(email);
    try { localStorage.setItem(PENDING, JSON.stringify({ hint: emailHint, at: Date.now() })); } catch {}
    code = ''; step = 'sent'; startCountdown();
  }).finally(() => (info = '')); };
  function startCountdown() { countdown = 60; const t = setInterval(() => { if (--countdown <= 0) clearInterval(t); }, 1000); }
  const verify = () => run(async () => {
    const r = await api('/api/auth/magic/verify', { method: 'POST', body: { token } });
    if (r.status === 'signup') { token = r.signupToken; step = 'setup'; } else await afterLogin();
  });
  const verifyCode = e => { e.preventDefault(); run(async () => {
    const r = await api('/api/auth/code', { method: 'POST', body: { code } });
    clearPending();
    if (r.status === 'signup') { token = r.signupToken; step = 'setup'; } else await afterLogin();
  }); };
  function otherAddress() { clearPending(); step = 'start'; }
  const signup = e => { e.preventDefault(); run(async () => {
    await api('/api/auth/signup', { method: 'POST', body: { token, displayName: name, ageConfirmed: age, consent } });
    await afterLogin();
  }); };
  const addPasskey = () => run(async () => { await passkeyRegister(); await loadMe(); go('/', { replace: true }); });
  function later() { localStorage.setItem('gg-skip-passkey', '1'); go('/', { replace: true }); }
</script>

<section class="auth">
  <div class="art">
    <span class="brand" style="color:inherit"><Logo light />greengrade</span>
    <div>
      <h1>Jede Sorte, ehrlich bewertet.</h1>
      <p>Dein Logbuch für Grows und Apothekensorten. Privat - oder mit der Community geteilt.</p>
    </div>
    <div class="bigmark" style="position:absolute;right:-110px;bottom:-110px;opacity:.13"><Logo size={520} light /></div>
  </div>
  <div class="pane">
    {#if step === 'start'}
      <h2>Anmelden oder registrieren</h2>
      <p class="muted">Ohne Passwort. Neu hier? Über den Link per Mail legst du automatisch ein Konto an.</p>
      <div class="stack">
        {#if passkeysSupported()}
          <button class="btn btn-primary" disabled={busy} onclick={loginPasskey}><Icon name="key" />Mit Passkey anmelden</button>
          <div class="divider">oder</div>
        {/if}
        <form onsubmit={sendLink} style="display:grid;gap:12px">
          <div class="field"><label for="a-mail">E-Mail-Adresse</label><input id="a-mail" type="email" bind:value={email} autocomplete="email" required maxlength="254"></div>
          <button class="btn btn-line" disabled={busy}><Icon name="mail" />{info || 'Anmeldelink senden'}</button>
        </form>
        {#if providers.length}
          <div class="divider">oder weiter mit</div>
          <div class="socials" style="grid-template-columns:repeat({providers.length},1fr)">
            {#each providers as p}<a class="btn btn-line" href="/api/auth/oauth/{p.id}/start" onclick={e => { e.preventDefault(); location.href = `/api/auth/oauth/${p.id}/start`; }}>{p.label}</a>{/each}
          </div>
        {/if}
      </div>
      {#if err}<p class="err" style="margin-top:16px">{err}</p>{/if}
      {#if privacyUrl || imprintUrl || termsUrl}<small class="row-actions" style="display:flex;gap:6px 16px;flex-wrap:wrap">{#if imprintUrl}<a href={imprintUrl} target="_blank" rel="noopener">Impressum</a>{/if}{#if privacyUrl}<a href={privacyUrl} target="_blank" rel="noopener">Datenschutz</a>{/if}{#if termsUrl}<a href={termsUrl} target="_blank" rel="noopener">Nutzungsbedingungen</a>{/if}</small>{/if}
      <small>Deine E-Mail-Adresse speichern wir nicht, nur einen nicht umkehrbaren Fingerabdruck davon. {#if providers.length}Bei Social Login erfährt der Anbieter nur, dass du dich bei greengrade anmeldest - nicht, was du einträgst.{/if}</small>
    {:else if step === 'sent'}
      <div class="mailicon"><Icon name="mail" size={26} /></div>
      <h2>Schau in dein Postfach</h2>
      <p class="muted">Wir haben einen Anmeldelink an <b style="color:var(--ink)">{email || emailHint || 'deine Adresse'}</b> geschickt. Er ist 10 Minuten gültig und funktioniert einmal.</p>
      <form class="codebox" onsubmit={verifyCode}>
        <label class="flabel" for="a-code" style="margin:0">Oder gib den Code aus der Mail hier ein</label>
        <div class="field"><input id="a-code" bind:value={code} inputmode="text" autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" maxlength="7" placeholder="ABC-123" required></div>
        <button class="btn btn-primary" disabled={busy || code.replace(/[^a-z0-9]/gi, '').length !== 6}>Mit Code anmelden</button>
      </form>
      {#if err}<p class="err" style="margin-top:16px">{err}</p>{/if}
      <div class="stack" style="margin-top:14px">
        <button class="btn btn-ghost" disabled={countdown > 0 || busy} onclick={e => (email ? sendLink(e) : otherAddress())}>{countdown > 0 ? `Erneut senden in 0:${String(countdown).padStart(2, '0')}` : 'Erneut senden'}</button>
      </div>
      <small>Der Code funktioniert nur auf diesem Gerät. <button class="linkbtn" onclick={otherAddress}>Andere Adresse verwenden</button></small>
    {:else if step === 'verify'}
      <h2>Anmeldung bestätigen</h2>
      <p class="muted">Tippe auf den Button, um dich auf diesem Gerät anzumelden.</p>
      <div class="stack"><button class="btn btn-primary" disabled={busy} onclick={verify}>Jetzt anmelden</button></div>
      {#if err}<p class="err" style="margin-top:16px">{err}</p><small><button class="linkbtn" onclick={() => go('/login')}>Neuen Link anfordern</button></small>{/if}
    {:else if step === 'setup'}
      <h2>Willkommen bei greengrade</h2>
      <p class="muted">Noch zwei Dinge, dann geht's los.</p>
      <form class="stack" onsubmit={signup}>
        <div class="field"><label for="a-name">Wie sollen wir dich nennen?</label><input id="a-name" bind:value={name} maxlength="40" required><div class="hint">Nur für dich sichtbar. Öffentliche Bewertungen sind immer anonym.</div></div>
        <div>
          <label class="check"><input type="checkbox" bind:checked={age}><span>Ich bin mindestens 18 Jahre alt{#if termsUrl} und akzeptiere die <a href={termsUrl} target="_blank" rel="noopener">Nutzungsbedingungen</a>{/if}.</span></label>
          <label class="check"><input type="checkbox" bind:checked={consent}><span>Ich willige ein, dass greengrade meine Einträge speichert - auch gesundheitsbezogene Angaben wie Apothekensorten, Wirkungen und Nebenwirkungen. Ich kann die Einwilligung jederzeit widerrufen, indem ich mein Konto lösche.{#if privacyUrl} <a href={privacyUrl} target="_blank" rel="noopener">Datenschutzerklärung</a>{/if}</span></label>
        </div>
        <button class="btn btn-primary" disabled={!age || !consent || !name.trim() || busy}>Konto anlegen</button>
      </form>
      {#if err}<p class="err" style="margin-top:16px">{err}</p>{/if}
    {:else if step === 'passkey'}
      <div class="mailicon"><Icon name="key" size={26} /></div>
      <h2>Beim nächsten Mal schneller</h2>
      <p class="muted">Lege einen Passkey an. Dann meldest du dich mit Face ID, Fingerabdruck oder der Geräte-PIN an - ganz ohne Mail.</p>
      <div class="stack">
        <button class="btn btn-primary" disabled={busy || !session.me} onclick={addPasskey}>Passkey anlegen</button>
        <button class="btn btn-ghost" onclick={later}>Später</button>
      </div>
      {#if err}<p class="err" style="margin-top:16px">{err}</p>{/if}
      <small>Du kannst Passkeys jederzeit unter Konto hinzufügen oder entfernen.</small>
    {/if}
  </div>
</section>
