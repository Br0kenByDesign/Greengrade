// Values on this device that belong to the signed-in person. They are removed when the
// sign-in ends, so nothing personal stays behind on shared devices.
// Kept on purpose: gg-theme and gg-install (look and install hint, nothing personal).
export const DRAFT_KEY = 'gg-draft';
const PERSONAL = [DRAFT_KEY, 'gg-skip-passkey'];

export function hasDraft() {
  try { return !!localStorage.getItem(DRAFT_KEY); } catch { return false; }
}

// pending: also forget a sign-in in progress (only on an explicit sign-out).
export function clearPersonal({ pending = false } = {}) {
  try {
    for (const k of PERSONAL) localStorage.removeItem(k);
    if (pending) localStorage.removeItem('gg-pending-login');
  } catch {}
}
