// OhMyServer overview module. As part of the shell it imports the shell's
// Preact and components (served at /_shell/) rather than bundling them.
import { html, render } from '../../../_shell/html.js';
import Overview from './pages/overview.js';

export function mount(el, ctx) {
  // The module lives in a shadow root: bring in the shell's stylesheet.
  const sheet = Object.assign(document.createElement('link'), { rel: 'stylesheet', href: new URL('../../../_shell/style.css', import.meta.url).href });
  el.before(sheet);
  const draw = c => render(html`<${Overview} ctx=${c} />`, el);
  draw(ctx);
  return { update: draw, unmount() { render(null, el); sheet.remove(); } };
}
