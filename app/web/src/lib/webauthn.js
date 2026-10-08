import { api } from './api.js';

const dec = s => Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(s.length / 4) * 4, '=')), c => c.charCodeAt(0)).buffer;
const enc = b => btoa(String.fromCharCode(...new Uint8Array(b))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');

export const passkeysSupported = () => !!window.PublicKeyCredential;

export function deviceName() {
  const ua = navigator.userAgent;
  if (/iPhone/.test(ua)) return 'iPhone';
  if (/iPad/.test(ua)) return 'iPad';
  if (/Android/.test(ua)) return 'Android';
  if (/Macintosh/.test(ua)) return 'Mac';
  if (/Windows/.test(ua)) return 'Windows';
  if (/Linux/.test(ua)) return 'Linux';
  return 'Passkey';
}

export async function passkeyLogin() {
  const { publicKey } = await api('/api/auth/passkey/login/begin', { method: 'POST' });
  publicKey.challenge = dec(publicKey.challenge);
  (publicKey.allowCredentials || []).forEach(c => (c.id = dec(c.id)));
  const cred = await navigator.credentials.get({ publicKey });
  const r = cred.response;
  return api('/api/auth/passkey/login/finish', { method: 'POST', body: {
    id: cred.id, rawId: enc(cred.rawId), type: cred.type, authenticatorAttachment: cred.authenticatorAttachment,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: { clientDataJSON: enc(r.clientDataJSON), authenticatorData: enc(r.authenticatorData), signature: enc(r.signature), userHandle: r.userHandle ? enc(r.userHandle) : null }
  } });
}

// Confirms the signed-in account with one of its own passkeys.
export async function passkeyReauth() {
  const { publicKey } = await api('/api/me/reauth/passkey/begin', { method: 'POST' });
  publicKey.challenge = dec(publicKey.challenge);
  (publicKey.allowCredentials || []).forEach(c => (c.id = dec(c.id)));
  const cred = await navigator.credentials.get({ publicKey });
  const r = cred.response;
  return api('/api/me/reauth/passkey/finish', { method: 'POST', body: {
    id: cred.id, rawId: enc(cred.rawId), type: cred.type, authenticatorAttachment: cred.authenticatorAttachment,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: { clientDataJSON: enc(r.clientDataJSON), authenticatorData: enc(r.authenticatorData), signature: enc(r.signature), userHandle: r.userHandle ? enc(r.userHandle) : null }
  } });
}

export async function passkeyRegister() {
  const { publicKey } = await api('/api/me/passkeys/begin', { method: 'POST' });
  publicKey.challenge = dec(publicKey.challenge);
  publicKey.user.id = dec(publicKey.user.id);
  (publicKey.excludeCredentials || []).forEach(c => (c.id = dec(c.id)));
  const cred = await navigator.credentials.create({ publicKey });
  const r = cred.response;
  return api('/api/me/passkeys/finish?name=' + encodeURIComponent(deviceName()), { method: 'POST', body: {
    id: cred.id, rawId: enc(cred.rawId), type: cred.type, authenticatorAttachment: cred.authenticatorAttachment,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: { clientDataJSON: enc(r.clientDataJSON), attestationObject: enc(r.attestationObject), transports: r.getTransports ? r.getTransports() : [] }
  } });
}
