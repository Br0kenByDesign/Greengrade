<script>
  import { install, isIOS, isMobile, promptInstall, dismissBanner } from '../lib/install.svelte.js';
  // variant: 'banner' (dashboard, dismissible) or 'inline' (account page)
  let { variant = 'banner', onclose = () => {} } = $props();
  const ios = isIOS(), mobile = isMobile();
  function close() { dismissBanner(); onclose(); }
  async function doInstall() { if (await promptInstall()) onclose(); }
</script>

{#snippet shareIcon()}<svg class="share-ico" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-label="Teilen"><path d="M12 3v12M8 7l4-4 4 4"/><path d="M5 11v8a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-8"/></svg>{/snippet}

<div class="install-hint" role={variant === 'banner' ? 'region' : undefined} aria-label="greengrade als App nutzen">
  <span class="ico"><img src="/icons/icon-192.png" alt=""></span>
  <div class="txt">
    <b>greengrade als App nutzen</b>
    {#if install.prompt}
      <p>Starte greengrade direkt vom {mobile ? 'Homescreen' : 'Desktop'}, im eigenen Fenster und ohne Browserleiste.</p>
      <div class="acts"><button class="btn btn-primary" onclick={doInstall}>App installieren</button>{#if variant === 'banner'}<button class="btn btn-ghost" onclick={close}>Nicht jetzt</button>{/if}</div>
    {:else if ios}
      <p>Tippe auf Teilen {@render shareIcon()} und dann auf „Zum Home-Bildschirm“. greengrade startet dann wie eine App, im Vollbild.</p>
      <p>In der App meldest du dich einmal neu an - per Passkey oder mit dem Code aus der Anmeldemail.</p>
    {:else if mobile}
      <p>Öffne das Menü deines Browsers und wähle „Zum Startbildschirm hinzufügen“ oder „App installieren“.</p>
    {:else}
      <p>In Chrome oder Edge über das Installations-Symbol rechts in der Adressleiste, in Safari auf dem Mac über Ablage &gt; „Zum Dock hinzufügen“.</p>
    {/if}
  </div>
  {#if variant === 'banner'}<button class="close" aria-label="Hinweis schließen" onclick={close}>×</button>{/if}
</div>
