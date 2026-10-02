// Copies the shared brand assets (../../design) into the app.
import { cpSync, mkdirSync } from 'node:fs';
const d = new URL('../../../design/', import.meta.url);
mkdirSync(new URL('../public/', import.meta.url), { recursive: true });
mkdirSync(new URL('../src/design/', import.meta.url), { recursive: true });
cpSync(new URL('icons/', d), new URL('../public/icons/', import.meta.url), { recursive: true });
cpSync(new URL('favicon.svg', d), new URL('../public/favicon.svg', import.meta.url));
cpSync(new URL('fonts/', d), new URL('../src/design/fonts/', import.meta.url), { recursive: true });
cpSync(new URL('fonts.css', d), new URL('../src/design/fonts.css', import.meta.url));
cpSync(new URL('tokens.css', d), new URL('../src/design/tokens.css', import.meta.url));
