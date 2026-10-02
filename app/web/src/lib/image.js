// Downscales and re-encodes a photo in the browser before upload. The server re-encodes again,
// so metadata never reaches the server in the first place and is guaranteed to be gone afterwards.
export async function preparePhoto(file, maxEdge = 2048) {
  let bmp;
  try { bmp = await createImageBitmap(file, { imageOrientation: 'from-image' }); }
  catch { throw new Error('Dieses Bildformat kann dein Browser nicht lesen. Bitte nutze JPEG oder PNG.'); }
  const scale = Math.min(1, maxEdge / Math.max(bmp.width, bmp.height));
  const w = Math.round(bmp.width * scale), h = Math.round(bmp.height * scale);
  const canvas = document.createElement('canvas');
  canvas.width = w; canvas.height = h;
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, w, h);
  ctx.drawImage(bmp, 0, 0, w, h);
  bmp.close?.();
  const blob = await new Promise(res => canvas.toBlob(res, 'image/jpeg', 0.88));
  if (!blob) throw new Error('Das Foto konnte nicht verarbeitet werden.');
  return blob;
}
