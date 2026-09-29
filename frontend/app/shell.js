import { html, Component, useEffect, useMemo, useState } from '../shared/ui.js';
import { discoverModules } from './modules.js';
import { createModuleView } from '#moduleweb/view.js';
import { loadRuntime } from '#moduleweb/definition.js';
import { href, resolveLocation, scopeModulePath } from '#moduleweb/router.js';

const groupOrder = ['概览', '容器与部署', '网络服务', '基础设施', '存储与系统', '设备与 IoT'];
const ModuleView = createModuleView({ html, Component, useEffect, useMemo, useState });

export function Shell() {
  const [modules, setModules] = useState([]);
  const [discoveryError, setDiscoveryError] = useState('');
  const [locationKey, setLocationKey] = useState(location.hash);
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const current = useMemo(() => resolveLocation(locationKey, modules), [locationKey, modules]);
  const hrefTo = useMemo(() => (moduleID, path) => {
    const target = modules.find(item => item.mount.id === moduleID && !item.manifestError);
    return target ? href(scopeModulePath(target.mount.pageBase, path)) : null;
  }, [modules]);
  useEffect(() => {
    let active = true;
    discoverModules().then(items => { if (active) setModules(items); }).catch(error => { if (active) setDiscoveryError(error.message); });
    return () => { active = false; };
  }, []);
  useEffect(() => {
    const update = () => { setLocationKey(location.hash); setMobileOpen(false); };
    addEventListener('hashchange', update);
    return () => removeEventListener('hashchange', update);
  }, []);
  const moduleGroups = [...new Set(modules.map(item => item.manifest.group))];
  const groups = [...groupOrder, ...moduleGroups.filter(title => !groupOrder.includes(title))]
    .map(title => ({ title, items: modules.filter(item => item.manifest.group === title) })).filter(group => group.items.length);
  const currentPath = current.location.pathname;
  const localLinks = current.module?.manifest.navigation || [];
  const title = current.module?.manifest.title || (modules.length ? '页面不存在' : 'OhMyServer');
  const warmModule = definition => { if (!definition.manifestError) loadRuntime(definition).catch(() => {}); };
  const toggleSidebar = () => {
    if (matchMedia('(max-width: 800px)').matches) setMobileOpen(value => !value);
    else setCollapsed(value => !value);
  };

  return html`<div class=${`app-shell ${collapsed ? 'is-collapsed' : ''} ${mobileOpen ? 'mobile-open' : ''}`}>
    <aside class="app-sidebar">
      <header class="app-brand"><a href="#/" aria-label="OhMyServer 首页"><span class="brand-mark">O</span><span class="brand-name">OhMyServer</span></a>
        <button class="button icon-button sidebar-collapse" aria-label=${mobileOpen || !collapsed ? '切换导航' : '展开侧栏'} aria-expanded=${mobileOpen ? 'true' : 'false'} onclick=${toggleSidebar}>☰</button>
      </header>
      <nav class="module-groups" aria-label="功能模块">
        ${groups.map(group => html`<section class="module-group" key=${group.title}>
          <h2>${group.title}</h2>
          ${group.items.map(item => html`<a class=${`module-link ${item.mount.id === current.module?.mount.id ? 'active' : ''}`} href=${href(scopeModulePath(item.mount.pageBase, item.manifest.home || '/'))} title=${item.manifest.title} key=${item.mount.id} onMouseEnter=${() => warmModule(item)} onFocus=${() => warmModule(item)} onPointerDown=${() => warmModule(item)}>
            <span class="module-icon">${item.manifest.icon}</span><span class="module-label">${item.manifest.title}</span>
          </a>`)}
        </section>`)}
      </nav>
      <footer class="sidebar-footer">模块化管理面板</footer>
    </aside>
    <main class="app-main" id="main-content" tabindex="-1">
      <header class="content-heading"><div><div class="eyebrow">${current.module?.manifest.group || 'OHMYSERVER'}</div><h1>${title}</h1></div>
        ${current.module && current.module.mount.id !== 'dashboard' ? html`<span class="module-chip"><span>${current.module.manifest.icon}</span>${current.module.mount.id}</span>` : null}
      </header>
      ${current.module && localLinks.length ? html`<nav class="module-nav" aria-label=${`${title} 页面`}>
        ${localLinks.map(link => { const target = scopeModulePath(current.module.mount.pageBase, link.path); return html`<a class=${currentPath === target ? 'active' : ''} href=${href(target)} key=${link.path}>${link.title || link.label}</a>`; })}
      </nav>` : null}
      <section class="page-content">
        ${discoveryError ? html`<div class="notice-card error"><h2>模块发现失败</h2><p>${discoveryError}</p></div>` : null}
        ${!modules.length && !discoveryError ? html`<div class="loading-state" role="status">正在发现模块…</div>` : null}
        ${modules.length > 0 && !current.module ? html`<div class="notice-card"><h2>找不到这个页面</h2><p>请从左侧功能分组中选择一个模块。</p><a href="#/">返回概览</a></div>` : null}
        ${current.module?.manifestError ? html`<div class="notice-card error"><h2>模块暂不可用</h2><p>${current.module.manifestError}</p></div>` : null}
        ${current.module && !current.module.manifestError ? html`<${ModuleView} key=${current.module.mount.id} definition=${current.module} hash=${locationKey} hrefTo=${hrefTo} />` : null}
      </section>
    </main>
  </div>`;
}
