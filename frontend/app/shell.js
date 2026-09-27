import { html } from '../shared/ui.js';
import { useEffect, useMemo, useState } from 'preact/hooks';
import { discoverModules, loadModuleRuntime } from './modules.js';
import { pageForRoute, pageKey } from './page-state.js';
import { createModuleTransport, createRequest } from '../shared/http.js';
import { href, matchModuleRoute, navigate, readLocation, scopeModulePath } from './router.js';

const groupOrder = ['概览', '容器与部署', '网络服务', '基础设施', '存储与系统', '设备与 IoT'];
const runtimeLoads = new Map();
const pageLoads = new Map();
const pageComponents = new Map();

function getModuleRuntime(module) {
  if (!runtimeLoads.has(module.id)) {
    const pending = loadModuleRuntime(module).catch(error => {
      runtimeLoads.delete(module.id);
      throw error;
    });
    runtimeLoads.set(module.id, pending);
  }
  return runtimeLoads.get(module.id);
}

function getPageComponent(module, page) {
  const key = pageKey(module, page);
  if (!pageLoads.has(key)) {
    const pending = page.load().then(loadedPage => {
      pageComponents.set(key, loadedPage.default);
      return loadedPage.default;
    }).catch(error => {
      pageLoads.delete(key);
      throw error;
    });
    pageLoads.set(key, pending);
  }
  return pageLoads.get(key);
}

function resolve(hash, modules) {
  const location = readLocation(hash);
  const module = modules.find(item => location.segments[0] === item.id)
    || (location.segments.length === 0 ? modules.find(item => item.id === 'dashboard') : null);
  if (!module) return { module: null, page: null, params: {}, location };
  const requested = `/${location.rawSegments.slice(1).join('/')}`;
  const relative = requested === '/' ? (module.home || '/') : requested;
  const matched = matchModuleRoute(module.routes || [], relative);
  return { module, page: matched?.route || null, params: matched?.params || {}, location };
}

export function Shell() {
  const [modules, setModules] = useState([]);
  const [discoveryError, setDiscoveryError] = useState('');
  const [locationKey, setLocationKey] = useState(location.hash);
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [loaded, setLoaded] = useState(null);
  const [loadError, setLoadError] = useState('');
  const [retryVersion, setRetryVersion] = useState(0);
  const current = useMemo(() => resolve(locationKey, modules), [locationKey, modules]);
  const request = useMemo(() => createRequest(), []);
  const activeModule = modules.find(item => item.id === current.module?.id) || null;
  const currentPageKey = pageKey(activeModule || current.module, current.page);
  const transport = useMemo(() => activeModule
    ? createModuleTransport({ apiBase: activeModule.apiBase, request })
    : null, [activeModule, request]);
  const api = useMemo(() => activeModule?.createAPI && transport
    ? activeModule.createAPI(transport)
    : null, [activeModule, transport]);
  const moduleNavigate = useMemo(() => activeModule
    ? (path, options) => navigate(scopeModulePath(activeModule.pageBase, path), options)
    : null, [activeModule]);
  const moduleHref = useMemo(() => activeModule
    ? path => href(scopeModulePath(activeModule.pageBase, path))
    : null, [activeModule]);
  const hrefTo = useMemo(() => (moduleID, path) => {
    const target = modules.find(item => item.id === moduleID);
    return target ? href(scopeModulePath(target.pageBase, path)) : null;
  }, [modules]);

  useEffect(() => {
    let active = true;
    discoverModules().then(items => { if (active) setModules(items); }).catch(error => { if (active) setDiscoveryError(error.message || '模块发现失败'); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (!current.module || activeModule?.routes) return;
    if (current.module.manifestError) {
      return;
    }
    let active = true;
    getModuleRuntime(current.module).then(runtime => {
      if (active) setModules(items => items.map(item => item.id === runtime.id ? runtime : item));
    }).catch(error => { if (active) setLoadError(error.message || '无法加载模块 JS'); });
    return () => { active = false; };
  }, [current.module?.id, activeModule?.routes, retryVersion]);

  useEffect(() => {
    const update = () => {
      setLocationKey(location.hash);
      setMobileOpen(false);
    };
    addEventListener('hashchange', update);
    return () => removeEventListener('hashchange', update);
  }, []);

  useEffect(() => {
    let active = true;
    setLoaded(null);
    setLoadError('');
    if (!activeModule || !current.page) return () => { active = false; };
    getPageComponent(activeModule, current.page).then(component => {
      if (active) setLoaded({ key: currentPageKey, component });
    }).catch(error => {
      if (active) setLoadError(error.message || '无法加载模块页面');
    });
    return () => { active = false; };
  }, [currentPageKey, retryVersion, activeModule]);

  const moduleGroups = [...new Set(modules.map(item => item.group || '其他'))];
  const groups = [...groupOrder, ...moduleGroups.filter(title => !groupOrder.includes(title))]
    .map(title => ({ title, items: modules.filter(item => (item.group || '其他') === title) }))
    .filter(group => group.items.length);
  const currentPath = current.location.pathname;
  const localLinks = current.module?.navigation || [];
  const title = current.module?.title || (modules.length ? '页面不存在' : 'OhMyServer');
  // Route state changes render before effects run. Never pass the new module
  // API to the previous page component during that intermediate render.
  const Page = pageComponents.get(currentPageKey) || pageForRoute(loaded, currentPageKey);

  const warmModule = module => {
    if (!module || module.routes || module.manifestError) return;
    getModuleRuntime(module).then(runtime => {
      setModules(items => {
        const current = items.find(item => item.id === runtime.id);
        return current?.routes ? items : items.map(item => item.id === runtime.id ? runtime : item);
      });
      const home = matchModuleRoute(runtime.routes, module.home || '/');
      if (home) getPageComponent(runtime, home.route).catch(() => {});
    }).catch(() => {});
  };

  const toggleSidebar = () => {
    if (matchMedia('(max-width: 800px)').matches) setMobileOpen(value => !value);
    else setCollapsed(value => !value);
  };

  return html`<div class=${`app-shell ${collapsed ? 'is-collapsed' : ''} ${mobileOpen ? 'mobile-open' : ''}`}>
    <aside class="app-sidebar">
      <header class="app-brand"><a href="#/dashboard" aria-label="OhMyServer 首页"><span class="brand-mark">O</span><span class="brand-name">OhMyServer</span></a>
        <button class="button icon-button sidebar-collapse" aria-label=${mobileOpen || !collapsed ? '切换导航' : '展开侧栏'} aria-expanded=${mobileOpen ? 'true' : 'false'} onclick=${toggleSidebar}>☰</button>
      </header>
      <nav class="module-groups" aria-label="功能模块">
        ${groups.map(group => html`<section class="module-group" key=${group.title}>
          <h2>${group.title}</h2>
          ${group.items.map(item => html`<a class=${`module-link ${item.id === current.module?.id ? 'active' : ''}`} href=${href(scopeModulePath(item.pageBase, item.home || '/'))} title=${item.title} key=${item.id} onMouseEnter=${() => warmModule(item)} onFocus=${() => warmModule(item)} onPointerDown=${() => warmModule(item)}>
            <span class="module-icon">${item.icon}</span><span class="module-label">${item.title}</span>
          </a>`)}
        </section>`)}
      </nav>
      <footer class="sidebar-footer">模块化管理面板</footer>
    </aside>
    <main class="app-main" id="main-content" tabindex="-1">
      <header class="content-heading"><div><div class="eyebrow">${current.module?.group || 'OHMYSERVER'}</div><h1>${title}</h1></div>
        ${current.module && current.module.id !== 'dashboard' ? html`<span class="module-chip"><span>${current.module.icon}</span>${current.module.id}</span>` : null}
      </header>
      ${current.module && localLinks.length ? html`<nav class="module-nav" aria-label=${`${title} 页面`}>
        ${localLinks.map(link => { const target = scopeModulePath(current.module.pageBase, link.path); return html`<a class=${currentPath === target ? 'active' : ''} href=${href(target)} key=${link.path}>${link.title || link.label}</a>`; })}
      </nav>` : null}
      <section class="page-content">
        ${discoveryError ? html`<div class="notice-card error"><h2>模块发现失败</h2><p>${discoveryError}</p></div>` : null}
        ${!modules.length && !discoveryError ? html`<div class="loading-state" role="status">正在发现模块…</div>` : null}
        ${modules.length > 0 && !current.module ? html`<div class="notice-card"><h2>找不到这个页面</h2><p>请从左侧功能分组中选择一个模块。</p><a href="#/dashboard">返回概览</a></div>` : null}
        ${current.module?.manifestError ? html`<div class="notice-card error"><h2>模块暂不可用</h2><p>${current.module.manifestError}</p></div>` : null}
        ${current.module && !activeModule?.routes && !current.module.manifestError && !loadError ? html`<div class="loading-state" role="status">正在加载${title}模块…</div>` : null}
        ${current.module && activeModule?.routes && !current.page ? html`<div class="notice-card"><h2>模块中没有此页面</h2><p>选择上方的模块页面继续操作。</p></div>` : null}
        ${loadError ? html`<div class="notice-card error"><h2>页面加载失败</h2><p>${loadError}</p><button class="button" onclick=${() => setRetryVersion(value => value + 1)}>重试</button></div>` : null}
        ${current.page && !Page && !loadError ? html`<div class="loading-state" role="status">正在加载${title}…</div>` : null}
        ${Page ? html`<${Page} key=${currentPageKey} api=${api} params=${current.params} search=${current.location.search} navigate=${moduleNavigate} href=${moduleHref} hrefTo=${hrefTo} ...${current.page?.props || {}} />` : null}
      </section>
    </main>
  </div>`;
}
