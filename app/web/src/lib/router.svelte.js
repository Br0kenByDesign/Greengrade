// Minimal history router.
export const route = $state({ path: location.pathname, query: new URLSearchParams(location.search), hash: location.hash.slice(1) });

function sync() {
  route.path = location.pathname;
  route.query = new URLSearchParams(location.search);
  route.hash = location.hash.slice(1);
}

export function go(href, { replace = false } = {}) {
  history[replace ? 'replaceState' : 'pushState']({}, '', href);
  sync();
  window.scrollTo(0, 0);
}

// Removes secrets (e.g. one-time tokens in the fragment) from the address bar.
export function clearHash() {
  history.replaceState({}, '', location.pathname + location.search);
  route.hash = '';
}

addEventListener('popstate', sync);

export function match(pattern, path) {
  const p = pattern.split('/'), s = path.replace(/\/$/, '').split('/');
  if (pattern === '/' ) return path === '/' ? {} : null;
  if (p.length !== s.length) return null;
  const params = {};
  for (let i = 0; i < p.length; i++) {
    if (p[i].startsWith(':')) params[p[i].slice(1)] = decodeURIComponent(s[i]);
    else if (p[i] !== s[i]) return null;
  }
  return params;
}
