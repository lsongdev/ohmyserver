// OhMyServer overview module. As part of the shell it imports the shell's
// Preact and components (served at /_shell/) rather than bundling them.
import { html, render } from '@lsongdev/panel/html.js';
import Overview from './pages/overview.js';

export function mount(el, ctx) {
  const draw = c => render(html`<${Overview} ctx=${c} />`, el);
  draw(ctx);
  return { update: draw, unmount() { render(null, el); } };
}
