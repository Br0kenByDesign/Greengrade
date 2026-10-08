// Step-up confirmation for sensitive actions. api() calls requestReauth() when the server
// answers 403 { reauth: true }; the dialog in App.svelte resolves it.
export const reauth = $state({ open: false });
let waiters = [];

export function requestReauth() {
  reauth.open = true;
  return new Promise((resolve, reject) => waiters.push({ resolve, reject }));
}

export function finishReauth(ok) {
  reauth.open = false;
  const w = waiters;
  waiters = [];
  for (const x of w) ok ? x.resolve() : x.reject(new Error('cancelled'));
}
