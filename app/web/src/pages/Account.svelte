<script>
  import { api, loadMe, ensureFresh } from '../lib/api.js';
  import { session } from '../lib/session.svelte.js';
  import { route, go } from '../lib/router.svelte.js';
  import { date } from '../lib/format.js';
  import { notify } from '../lib/toast.svelte.js';
  import { passkeyRegister, passkeysSupported } from '../lib/webauthn.js';
  import Icon from '../components/Icon.svelte';
  import InstallHint from '../components/InstallHint.svelte';
  import { isStandalone } from '../lib/install.svelte.js';
  const standalone = isStandalone();

  let err = $state(''), name = $state(session.me.displayName), confirmText = $state(''), showDelete = $state(false);
  let theme = $state(localStorage.getItem('gg-theme') || 'system');
  const me = $derived(session.me);
  const msg = {
    vergeben: 'Dieses Konto ist schon mit einem anderen greengrade-Konto verknüpft.',
    vorhanden: 'Du hast bei diesem Anbieter schon ein anderes Konto verknüpft.',
    bestaetigen: 'Bitte bestätige zuerst kurz, dass du es bist, und versuche es dann noch einmal.',
    bestaetigung: 'Die Bestätigung hat nicht geklappt. Nutze eine Anmeldung, die mit diesem Konto verknüpft ist.'
  }[route.query.get('fehler')];
  if (route.query.get('verknuepft')) notify('Anmeldung verknüpft.');
  if (route.query.get('bestaetigt')) notify('Bestätigt. Du kannst jetzt fortfahren.');
  if (me.notices.some(n => !n.read)) api('/api/me/notices/read', { method: 'POST' }).then(loadMe).catch(() => {});

  async function run(fn, ok) { err = ''; try { await fn(); if (ok) notify(ok); await loadMe(); } catch (e) { if (e?.name !== 'NotAllowedError') err = e.message; } }
  const saveName = () => run(() => api('/api/me', { method: 'PATCH', body: { displayName: name } }), 'Name gespeichert.');
  const addPasskey = () => run(passkeyRegister, 'Passkey angelegt.');
  const delPasskey = id => confirm('Passkey entfernen?') && run(() => api('/api/me/passkeys/' + id, { method: 'DELETE' }), 'Passkey entfernt.');
  const unlink = p => confirm('Verknüpfung trennen?') && run(() => api('/api/me/identities/' + p, { method: 'DELETE' }), 'Verknüpfung getrennt.');
  async function logout(all) {
    try { await api(all ? '/api/me/logout-all' : '/api/auth/logout', { method: 'POST' }); } catch {}
    session.me = null; go('/login', { replace: true });
  }
  // these leave the app, so the confirmation happens before
  async function exportData() { try { await ensureFresh(); location.href = '/api/me/export'; } catch (e) { err = e.message; } }
  async function link(id) { try { await ensureFresh(); location.href = `/api/auth/oauth/${id}/start?mode=link`; } catch (e) { err = e.message; } }
  async function deleteAccount() {
    try { await api('/api/me', { method: 'DELETE', body: { confirm: confirmText } }); session.me = null; localStorage.clear(); go('/login', { replace: true }); }
    catch (e) { err = e.message; }
  }
  function setTheme(t) {
    theme = t;
    if (t === 'system') { localStorage.removeItem('gg-theme'); delete document.documentElement.dataset.theme; }
    else { localStorage.setItem('gg-theme', t); document.documentElement.dataset.theme = t; }
  }
</script>

<div class="settings">
  <h1>Konto</h1>
  <p class="muted" style="margin-top:6px">Angemeldet als {me.displayName}. {me.hasEmail ? 'Deine E-Mail-Adresse kennen wir nur als Fingerabdruck.' : ''}</p>
  {#if err || msg}<p class="err" style="margin-top:16px">{err || msg}</p>{/if}

  {#if me.shareBanned}
    <div class="notice" style="margin-top:20px;border-color:var(--danger)"><b style="font-weight:500">Öffentliches Teilen ist für dein Konto gesperrt</b><span class="muted">{me.shareBanReason ? `Grund: ${me.shareBanReason}. ` : ''}Dein privates Logbuch kannst du weiter wie gewohnt nutzen.</span></div>
  {/if}

  {#if me.notices.length}
    <div class="sgroup">
      <h2>Hinweise</h2>
      <p class="lead">Nachrichten zu deinen öffentlichen Inhalten. Wir können dir keine Mails schicken, deshalb landen sie hier.</p>
      <div style="display:grid;gap:10px">{#each me.notices as n}<div class="notice"><b style="font-weight:500">{n.title}</b><span class="muted">{n.body} {date(n.createdAt)}</span></div>{/each}</div>
    </div>
  {/if}

  <div class="sgroup">
    <h2>Name</h2>
    <div class="srow" style="border-bottom:1px solid var(--line)">
      <div class="field" style="flex:1"><label for="nm" class="muted">Nur für dich sichtbar</label><input id="nm" bind:value={name} maxlength="40"></div>
      <button class="btn btn-line" disabled={!name.trim() || name === me.displayName} onclick={saveName}>Speichern</button>
    </div>
  </div>

  <div class="sgroup">
    <h2>Passkeys</h2>
    {#each me.passkeys as p}
      <div class="srow"><div class="l"><span class="ico"><Icon name="key" /></span><div>{p.name}<small>Angelegt am {date(p.createdAt)}{p.lastUsedAt ? `, zuletzt ${date(p.lastUsedAt)}` : ''}</small></div></div><button class="linkbtn" onclick={() => delPasskey(p.id)}>Entfernen</button></div>
    {:else}<p class="muted">Noch kein Passkey angelegt.</p>{/each}
    {#if passkeysSupported()}<button class="btn btn-ghost" style="margin-top:14px" onclick={addPasskey}><Icon name="plus" />Passkey hinzufügen</button>{/if}
  </div>

  {#if me.providers.length}
    <div class="sgroup">
      <h2>Verknüpfte Anmeldungen</h2>
      <p class="lead">Verknüpfen geht nur, während du angemeldet bist - so kann niemand dein Konto über eine fremde Anmeldung übernehmen.</p>
      {#each me.providers as p}
        {@const linked = me.identities.includes(p.id)}
        <div class="srow"><div class="l"><span class="ico">{p.label[0]}</span><div>{p.label}<small>{linked ? 'Verknüpft' : 'Nicht verknüpft'}</small></div></div>
          {#if linked}<button class="linkbtn" onclick={() => unlink(p.id)}>Trennen</button>{:else}<button class="btn btn-line" onclick={() => link(p.id)}>Verknüpfen</button>{/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if !standalone}
    <div class="sgroup">
      <h2>App installieren</h2>
      <InstallHint variant="inline" />
    </div>
  {/if}

  <div class="sgroup">
    <h2>Darstellung</h2>
    <div class="seg" style="max-width:420px">{#each [['system', 'System'], ['light', 'Hell'], ['dark', 'Dunkel']] as [v, l]}<button class:on={theme === v} onclick={() => setTheme(v)}>{l}</button>{/each}</div>
  </div>

  <div class="sgroup">
    <h2>Sicherheit und Daten</h2>
    <div class="srow"><div class="l"><div>Abmelden<small>Nur auf diesem Gerät.</small></div></div><button class="btn btn-line" onclick={() => logout(false)}>Abmelden</button></div>
    <div class="srow"><div class="l"><div>Überall abmelden<small>Beendet die Anmeldung auf allen Geräten, auch diesem.</small></div></div><button class="btn btn-line" onclick={() => logout(true)}>Überall abmelden</button></div>
    <div class="srow"><div class="l"><div>Daten exportieren<small>Alle Einträge, Grows und Fotos als ZIP mit JSON-Datei.</small></div></div><button class="btn btn-line" onclick={exportData}>Export laden</button></div>
  </div>

  <div class="sgroup">
    <h2>Konto löschen</h2>
    <p class="lead">Löscht sofort und endgültig alle Einträge, Fotos, Grows, öffentlichen Bewertungen, Kommentare und Passkeys. Das lässt sich nicht rückgängig machen.</p>
    {#if !showDelete}<button class="btn btn-danger" onclick={() => (showDelete = true)}>Konto endgültig löschen</button>
    {:else}
      <div class="inline-form">
        <div class="field"><label for="del">Tippe LÖSCHEN zur Bestätigung</label><input id="del" bind:value={confirmText} autocomplete="off"></div>
        <div class="row-actions"><button class="btn btn-danger" disabled={confirmText !== 'LÖSCHEN'} onclick={deleteAccount}>Jetzt alles löschen</button><button class="btn btn-ghost" onclick={() => (showDelete = false)}>Abbrechen</button></div>
      </div>
    {/if}
  </div>
</div>
