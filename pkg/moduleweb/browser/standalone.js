import { discoverModule, loadStyles } from './definition.js';
import { navigationFor } from './router.js';
import { createModuleView } from './view.js';

export function mountStandalone(ui, { root = document.getElementById('app'), apiBase = document.querySelector('meta[name="api-base"]')?.content } = {}) {
  const { html, render, useEffect, useState } = ui;
  if (!root || !apiBase) throw new TypeError('standalone root and api-base are required');
  const base = new URL(apiBase, document.baseURI);
  if (base.origin !== location.origin || base.search || base.hash) throw new TypeError('invalid standalone api-base');
  const ModuleView = createModuleView(ui);
  const navigation = navigationFor('/');
  const style = document.createElement('link');
  style.rel = 'stylesheet'; style.href = new URL('standalone.css', import.meta.url).href;
  document.head.append(style);

  function App() {
    const [definition, setDefinition] = useState(null);
    const [error, setError] = useState('');
    const [retry, setRetry] = useState(0);
    const [hash, setHash] = useState(location.hash);
    useEffect(() => {
      let active = true;
      setError('');
      discoverModule({ apiBase: base.pathname }).then(value => {
        if (!active) return;
        loadStyles(value); document.title = value.manifest.title; setDefinition(value);
      }).catch(error => { if (active) setError(error.message); });
      return () => { active = false; };
    }, [retry]);
    useEffect(() => {
      const update = () => setHash(location.hash);
      addEventListener('hashchange', update);
      return () => removeEventListener('hashchange', update);
    }, []);
    return html`<main class="module-standalone">
      <header class="page-toolbar"><h1>${definition?.manifest.title || '加载中…'}</h1></header>
      ${definition?.manifest.navigation.length ? html`<nav class="button-group">${definition.manifest.navigation.map(item => html`<a class="button" href=${navigation.href(item.path)}>${item.title}</a>`)}</nav>` : null}
      ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p><button class="button" onclick=${() => setRetry(value => value + 1)}>重试</button></div>` : null}
      ${definition ? html`<${ModuleView} definition=${definition} hash=${hash} />` : null}
    </main>`;
  }
  render(html`<${App} />`, root);
  return () => { render(null, root); style.remove(); };
}
