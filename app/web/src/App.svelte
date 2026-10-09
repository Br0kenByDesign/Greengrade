<script>
  import { route, go, match } from './lib/router.svelte.js';
  import { session } from './lib/session.svelte.js';
  import { loadMe } from './lib/api.js';
  import { toast } from './lib/toast.svelte.js';
  import Icon from './components/Icon.svelte';
  import Logo from './components/Logo.svelte';
  import Auth from './pages/Auth.svelte';
  import Dashboard from './pages/Dashboard.svelte';
  import EntryForm from './pages/EntryForm.svelte';
  import EntryDetail from './pages/EntryDetail.svelte';
  import Grows from './pages/Grows.svelte';
  import GrowDetail from './pages/GrowDetail.svelte';
  import Stats from './pages/Stats.svelte';
  import Compare from './pages/Compare.svelte';
  import Public from './pages/Public.svelte';
  import PublicStrain from './pages/PublicStrain.svelte';
  import Account from './pages/Account.svelte';
  import Admin from './pages/Admin.svelte';
  import NotFound from './pages/NotFound.svelte';
  import ReauthDialog from './components/ReauthDialog.svelte';
  import Contact from './pages/Contact.svelte';

  const routes = [
    ['/', Dashboard], ['/entries/new', EntryForm], ['/entries/:id/edit', EntryForm], ['/entries/:id', EntryDetail],
    ['/grows', Grows], ['/grows/new', GrowDetail], ['/grows/:id', GrowDetail], ['/stats', Stats], ['/compare', Compare],
    ['/public', Public], ['/public/:id', PublicStrain], ['/account', Account], ['/admin', Admin]
  ];
  const isAuth = $derived(route.path === '/login' || route.path.startsWith('/auth/'));
  const isContact = $derived(route.path === '/kontakt'); // reachable without an account
  const current = $derived.by(() => {
    for (const [p, c] of routes) { const params = match(p, route.path); if (params) return { c, params }; }
    return { c: NotFound, params: {} };
  });
  const section = $derived(route.path === '/' || route.path.startsWith('/entries') && route.path !== '/entries/new' ? '/' : '/' + (route.path.split('/')[1] || ''));
  // Mobile: Übersicht, Statistik and Vergleichen share one tab with a sub-navigation.
  const overview = [['/', 'Übersicht'], ['/stats', 'Statistik'], ['/compare', 'Vergleichen']];
  const inOverview = $derived(route.path === '/' || route.path === '/stats' || route.path === '/compare');
  const unread = $derived(session.me?.notices?.filter(n => !n.read).length || 0);

  loadMe().then(me => { if (!me && !isAuth && !isContact) go('/login', { replace: true }); });

  // in-app navigation for plain links
  function onclick(e) {
    const a = e.target.closest('a[href]');
    if (!a || a.target || a.hasAttribute('download') || e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
    const url = new URL(a.href);
    if (url.origin !== location.origin || url.pathname.startsWith('/api/')) return;
    e.preventDefault();
    go(url.pathname + url.search + url.hash);
  }
  const nav = [['/', 'dash', 'Übersicht'], ['/entries/new', 'plus', 'Neuer Eintrag'], ['/grows', 'grow', 'Laufende Grows'], ['/stats', 'stats', 'Statistik'], ['/compare', 'compare', 'Vergleichen']];
  const initials = n => (n || '?').split(/\s+/).map(x => x[0]).join('').slice(0, 2).toUpperCase();
</script>

<svelte:document {onclick} />

{#if isAuth}
  <Auth />
{:else if isContact}
  <Contact />
{:else if !session.loaded}
  <div class="spinner" role="status" aria-label="Lädt"></div>
{:else if session.me}
  <div class="app">
    <aside class="side">
      <a class="brand" href="/"><Logo />greengrade</a>
      <nav class="nav" aria-label="Hauptnavigation">
        {#each nav as [href, icon, label]}<a {href} class:on={section === href || route.path === href}><Icon name={icon} />{label}</a>{/each}
        <hr>
        <a href="/public" class:on={section === '/public'}><Icon name="globe" />Öffentlich</a>
        {#if session.me.isAdmin}<a href="/admin" class:on={section === '/admin'}><Icon name="shield" />Moderation</a>{/if}
      </nav>
      <a class="me" class:on={section === '/account'} href="/account" style="text-decoration:none">
        <span class="avatar">{initials(session.me.displayName)}</span>
        <span><span style="display:block">{session.me.displayName}</span><span class="muted" style="font-size:12px">Konto und Datenschutz</span></span>
        {#if unread}<span class="badge">{unread}</span>{/if}
      </a>
    </aside>
    <main class="main">
      {#if inOverview}<nav class="subnav" aria-label="Übersicht">{#each overview as [href, label]}<a {href} class:on={route.path === href} aria-current={route.path === href ? 'page' : undefined}>{label}</a>{/each}</nav>{/if}
      {#key route.path}<current.c params={current.params} />{/key}
    </main>
    <nav class="tabbar" aria-label="Navigation">
      <a href="/" class:on={section === '/' || inOverview}><Icon name="dash" size={22} />Übersicht</a>
      <a href="/public" class:on={section === '/public'}><Icon name="globe" size={22} />Öffentlich</a>
      <a href="/entries/new" class="plus" aria-label="Neuer Eintrag"><Icon name="plus" size={22} /></a>
      <a href="/grows" class:on={section === '/grows'}><Icon name="grow" size={22} />Grows</a>
      <a href="/account" class:on={section === '/account'}><span class="avatar" style="width:22px;height:22px;font-size:9px">{initials(session.me.displayName)}</span>Konto{#if unread} ({unread}){/if}</a>
    </nav>
  </div>
{/if}

<ReauthDialog />

{#if toast.text}<div class="toast" role="status">{toast.text}</div>{/if}
