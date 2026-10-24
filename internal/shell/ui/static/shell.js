// The panel shell: sign-in, navigation, and hosting of contract modules
// (docs/module-contract.md). Each module renders into its own shadow root,
// receives its route through mount/update, and changes it through
// ctx.navigate. The shell owns the address bar: "#/<module id><module path>".
import { html, render, useState } from './html.js';
import { t } from './i18n.js';

const url = path => new URL(path, document.baseURI);

async function getJSON(path) {
  const response = await fetch(url(path), { headers: { Accept: 'application/json' } });
  return { status: response.status, body: await response.json().catch(() => null) };
}

// ---- Location ---------------------------------------------------------------

function parseHash() {
  const raw = location.hash.replace(/^#/, '') || '/';
  const [path, search = ''] = raw.split('?');
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(search)) };
}

const searchString = query => {
  const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
  return s ? '?' + s : '';
};

// full joins a module id and a module path ("/" is the module root).
const full = (id, path) => '/' + id + (path === '/' ? '' : path);

function go(to, { replace = false } = {}) {
  if (replace) history.replaceState(null, '', '#' + to);
  else history.pushState(null, '', '#' + to);
  dispatchEvent(new HashChangeEvent('hashchange'));
}

// split finds the module a shell path belongs to and its module path.
function split(panel, path) {
  const [, id = '', ...rest] = path.split('/');
  const module = panel.modules.find(m => m.id === id);
  return module ? { module, path: '/' + rest.join('/') } : null;
}

// ---- Module host ------------------------------------------------------------

// ModuleHost keeps at most one module mounted, in a shadow root of its own.
class ModuleHost {
  constructor(panel, element) {
    this.panel = panel;
    this.element = element;
    this.current = null; // { id, instance }
    this.entries = new Map(); // id → imported entry module
  }

  context(module, path, query) {
    return {
      id: module.id,
      apiBase: url(module.api).href,
      assetsBase: url(module.ui).href,
      path, query,
      navigate: (to, opts) => go(full(module.id, to), opts),
      href: (to, q) => '#' + full(module.id, to) + searchString(q),
      link: (id, to, q) => (this.panel.modules.some(m => m.id === id) ? '#' + full(id, to) + searchString(q) : null),
      lang: this.panel.lang,
    };
  }

  async show(module, path, query) {
    const ctx = this.context(module, path, query);
    if (this.current?.id === module.id) {
      this.current.instance?.update?.(ctx);
      return;
    }
    this.unmount();
    const token = (this.token = Symbol(module.id));
    const container = document.createElement('div');
    container.className = 'module-host';
    container.dataset.module = module.id;
    const root = container.attachShadow({ mode: 'open' });
    // Stylesheets only load once connected to the document, so attach the
    // container hidden and reveal it when they have loaded: the module
    // never renders unstyled.
    container.style.visibility = 'hidden';
    this.element.replaceChildren(container);
    this.current = { id: module.id, instance: null };
    await Promise.all((module.styles || []).map(sheet => new Promise(resolve => {
      const link = Object.assign(document.createElement('link'), { rel: 'stylesheet', href: url(module.ui + sheet).href });
      link.onload = link.onerror = resolve;
      root.append(link);
    })));
    if (this.token !== token) return; // another module was opened meanwhile
    container.style.visibility = '';
    const el = document.createElement('div');
    el.className = 'module-root';
    root.append(el);
    try {
      let entry = this.entries.get(module.id);
      if (!entry) {
        entry = await import(url(module.ui + module.entry).href);
        this.entries.set(module.id, entry);
      }
      if (this.token !== token) return; // another module was opened meanwhile
      this.current.instance = entry.mount(el, ctx);
    } catch (error) {
      if (this.token !== token) return;
      this.element.replaceChildren(Object.assign(document.createElement('div'), { className: 'notice notice-error', textContent: t('moduleFailed', module.title, error.message) }));
    }
  }

  unmount() {
    this.current?.instance?.unmount?.();
    this.current = null;
    this.element.replaceChildren();
  }
}

// ---- Layout -----------------------------------------------------------------

function Sidebar({ panel, path, onLogout }) {
  const [open, setOpen] = useState(false);
  document.querySelector('.layout')?.classList.toggle('menu-open', open);
  const items = panel.modules.flatMap(m => (m.nav || []).map(item => ({ ...item, module: m, to: full(m.id, item.path), root: item.path === '/' })));
  // The longest matching entry is active; module roots only match exactly.
  const active = items
    .filter(i => path === i.to || (!i.root && path.startsWith(i.to + '/')))
    .sort((a, b) => b.to.length - a.to.length)[0];
  return html`
    <a class="brand" href="#/"><span class="brand-mark">◆</span>${panel.title}</a>
    <button class="btn btn-ghost menu-toggle" aria-label=${t('menu')} onClick=${() => setOpen(o => !o)}>☰</button>
    <nav class="nav" onClick=${() => setOpen(false)}>
      ${panel.modules.map(m => html`<div class="nav-group">
        ${panel.modules.length > 1 ? html`<div class="nav-heading">${m.title}</div>` : null}
        ${items.filter(i => i.module === m).map(i => html`<a class=${`nav-link ${i === active ? 'active' : ''}`} href=${'#' + i.to}>
          <span class="nav-icon">${i.icon || '•'}</span>${i.title}</a>`)}
      </div>`)}
    </nav>
    ${panel.auth ? html`<button class="btn btn-ghost sidebar-logout" onClick=${onLogout}>${t('signOut')}</button>` : null}`;
}

function Login({ title, onLogin }) {
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const submit = async e => {
    e.preventDefault();
    // Read the field on submit: password managers may fill it without
    // firing input events.
    const secret = new FormData(e.currentTarget).get('secret');
    setBusy(true);
    setError(null);
    try {
      const response = await fetch(url('_auth/login'), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ secret }),
      });
      if (!response.ok) throw new Error(response.status === 401 ? t('wrongPassword') : t('signInFailed', response.status));
      onLogin();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  };
  return html`<main class="login">
    <form class="card login-card" onSubmit=${submit}>
      <div class="brand"><span class="brand-mark">◆</span>${title}</div>
      <label class="field"><span class="field-label">${t('password')}</span>
        <input class="input" type="password" name="secret" autocomplete="current-password" autofocus required /></label>
      ${error ? html`<div class="notice notice-error" role="alert">${error.message}</div>` : null}
      <button class="btn btn-primary" disabled=${busy}>${t('signIn')}</button>
    </form>
  </main>`;
}

// ---- Boot -------------------------------------------------------------------

const app = document.getElementById('app');
let stop = () => {};

function showMessage(text) {
  stop();
  render(null, app);
  app.innerHTML = '';
  app.append(Object.assign(document.createElement('div'), { className: 'notice notice-error main', textContent: text }));
}

function showLogin(title) {
  stop();
  app.innerHTML = '';
  render(html`<${Login} title=${title} onLogin=${boot} />`, app);
}

function showPanel(panel) {
  stop();
  render(null, app);
  app.innerHTML = '<div class="layout"><aside class="sidebar"></aside><main class="main"></main></div>';
  const sidebar = app.querySelector('.sidebar');
  const host = new ModuleHost(panel, app.querySelector('.main'));
  const logout = async () => {
    await fetch(url('_auth/logout'), { method: 'POST' }).catch(() => {});
    showLogin(panel.title);
  };
  const update = () => {
    const { path, query } = parseHash();
    const target = split(panel, path);
    if (!target) {
      // "/" and unknown paths open the home module.
      const home = panel.modules.find(m => m.id === panel.home) || panel.modules[0];
      if (path === '/' || path === '') return go(full(home.id, home.home || home.nav?.[0]?.path || '/'), { replace: true });
      host.unmount();
      host.element.append(Object.assign(document.createElement('div'), { className: 'notice notice-error', textContent: t('notFound') }));
    } else {
      host.show(target.module, target.path, query);
    }
    render(html`<${Sidebar} panel=${panel} path=${path} onLogout=${logout} />`, sidebar);
  };
  addEventListener('hashchange', update);
  addEventListener('popstate', update);
  stop = () => {
    removeEventListener('hashchange', update);
    removeEventListener('popstate', update);
    host.unmount();
  };
  update();
}

async function boot() {
  try {
    const session = await getJSON('_auth/session');
    if (session.status === 200 && !session.body.authenticated) return showLogin(document.title);
    const { status, body: panel } = await getJSON('_panel.json');
    if (status === 401) return showLogin(document.title);
    if (status !== 200) throw new Error(t('panelFailed', status));
    document.title = panel.title;
    showPanel(panel);
  } catch (error) {
    showMessage(error.message);
  }
}

// Any module API answering 401 sends the user back to sign-in.
addEventListener('panel:unauthorized', () => boot());
boot();
