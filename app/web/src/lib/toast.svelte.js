export const toast = $state({ text: '' });
let t;
export function notify(text) { toast.text = text; clearTimeout(t); t = setTimeout(() => (toast.text = ''), 2800); }
