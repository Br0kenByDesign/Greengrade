import { session } from './session.svelte.js';
import { go } from './router.svelte.js';
import { requestReauth } from './reauth.svelte.js';
import { clearPersonal } from './local.js';

export class ApiError extends Error {
  constructor(status, message) { super(message); this.status = status; }
}

let refreshing = null;
async function refresh() {
  refreshing ??= fetch('/api/auth/refresh', { method: 'POST', credentials: 'same-origin' }).then(r => r.ok).finally(() => { refreshing = null; });
  return refreshing;
}

export async function api(path, { method = 'GET', body, form } = {}) {
  const opts = { method, credentials: 'same-origin', headers: {} };
  if (form) opts.body = form;
  else if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
  let res = await fetch(path, opts);
  if (res.status === 401 && !path.startsWith('/api/auth/')) {
    if (await refresh()) res = await fetch(path, opts);
    if (res.status === 401) {
      session.me = null;
      clearPersonal(); // the sign-in has ended (expired or signed out elsewhere)
      // pages that work without an account stay where they are
      if (!/^\/(login|auth\/|kontakt)/.test(location.pathname)) go('/login');
      throw new ApiError(401, 'Bitte melde dich an.');
    }
  }
  if (res.status === 403) {
    let data = {};
    try { data = await res.clone().json(); } catch {}
    if (data.reauth) {
      try { await requestReauth(); } catch { throw new ApiError(403, 'Bestätigung abgebrochen.'); }
      res = await fetch(path, opts);
    }
  }
  if (!res.ok) {
    let msg = 'Da ist etwas schiefgelaufen. Bitte versuche es erneut.';
    try { msg = (await res.json()).error || msg; } catch {}
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return null;
  return res.json();
}

export async function loadMe() {
  try { session.me = await api('/api/me'); } catch (e) {
    session.me = null;
    if (e?.status === 401) clearPersonal(); // not signed in any more: drop leftovers on this device
  }
  session.loaded = true;
  return session.me;
}

// Makes sure a recent sign-in exists before leaving the app (download, social login).
export const ensureFresh = () => api('/api/me/fresh');

export const photoURL = (id, thumb = true) => `/api/photos/${id}${thumb ? '?size=thumb' : ''}`;
