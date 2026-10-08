// "Add to home screen" support. Imported early (main.js) so the install event is not missed.
export const install = $state({ prompt: null });

const DAY = 86400000;
const KEY = 'gg-install';
const read = () => { try { return JSON.parse(localStorage.getItem(KEY)) || {}; } catch { return {}; } };
const write = v => { try { localStorage.setItem(KEY, JSON.stringify(v)); } catch {} };

export const isStandalone = () => matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
export const isIOS = () => /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
export const isMobile = () => isIOS() || /Android/i.test(navigator.userAgent) || matchMedia('(pointer: coarse) and (max-width: 900px)').matches;

addEventListener('beforeinstallprompt', e => { e.preventDefault(); install.prompt = e; });
addEventListener('appinstalled', () => { install.prompt = null; write({ ...read(), installed: Date.now() }); });

export async function promptInstall() {
  const p = install.prompt;
  if (!p) return false;
  p.prompt();
  const { outcome } = await p.userChoice;
  install.prompt = null;
  return outcome === 'accepted';
}

// Counts app starts (once per browser session). The hint shows from the second start on.
export function countVisit() {
  try {
    if (sessionStorage.getItem(KEY)) return;
    sessionStorage.setItem(KEY, '1');
  } catch { return; }
  const st = read();
  write({ ...st, visits: (st.visits || 0) + 1 });
}

export function showBanner() {
  if (isStandalone() || !isMobile()) return false;
  const st = read();
  if (st.installed) return false;
  if (st.dismissed && Date.now() - st.dismissed < 60 * DAY) return false;
  return (st.visits || 0) >= 2;
}

export const dismissBanner = () => write({ ...read(), dismissed: Date.now() });
