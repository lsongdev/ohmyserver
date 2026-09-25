const hostConfig = globalThis.OhMyServerConfig || {};
const apiRoot = (hostConfig.apiBase || '/v1').replace(/\/$/, '');
const origin = globalThis.location?.origin || 'http://localhost';

export async function discoverModules(fetchImpl = fetch) {
  const response = await fetchImpl(`${apiRoot}/modules`, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`模块列表加载失败 (${response.status})`);
  const payload = await response.json();
  if (!Array.isArray(payload.modules)) throw new Error('模块列表格式无效');
  if (payload.modules.some(item => !/^[a-z][a-z0-9-]*$/.test(item.id || '') || typeof item.apiBase !== 'string' || typeof item.manifestURL !== 'string')) {
    throw new Error('模块索引包含无效条目');
  }
  return Promise.all(payload.modules.map(async descriptor => {
    try {
      const moduleBase = new URL(`${descriptor.apiBase.replace(/\/$/, '')}/`, origin);
      const manifestURL = new URL(descriptor.manifestURL, origin);
      if (manifestURL.origin !== origin || manifestURL.href !== new URL('manifest.json', moduleBase).href) throw new Error('模块清单地址无效');
      const manifestResponse = await fetchImpl(manifestURL.href, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
      if (!manifestResponse.ok) throw new Error(`模块清单加载失败 (${manifestResponse.status})`);
      const manifest = await manifestResponse.json();
      if (manifest.contractVersion !== 1 || manifest.id !== descriptor.id || typeof manifest.entry !== 'string') throw new Error('模块清单身份或版本无效');
      const entryURL = resolveAsset(manifest.entry, moduleBase);
      if (!Array.isArray(manifest.styles || []) || !(manifest.styles || []).every(path => typeof path === 'string')) throw new Error('模块样式清单格式无效');
      const styleURLs = (manifest.styles || []).map(path => resolveAsset(path, moduleBase));
      if (!Array.isArray(manifest.navigation || []) || !(manifest.navigation || []).every(item => typeof item.title === 'string' && typeof item.path === 'string' && item.path.startsWith('/'))) throw new Error('模块导航清单格式无效');
      for (const absolute of styleURLs) {
        if (![...document.querySelectorAll('link[data-module-style]')].some(link => link.href === absolute.href)) {
          const link = document.createElement('link'); link.rel = 'stylesheet'; link.href = absolute.href; link.dataset.moduleStyle = descriptor.id; document.head.append(link);
        }
      }
      return { ...descriptor, ...manifest, navigation: (manifest.navigation || []).map(item => ({ ...item, label: item.title })), manifest, moduleURL: entryURL.href };
    } catch (error) {
      return { ...descriptor, title: descriptor.id, icon: '◇', group: '其他', home: '/', navigation: [], manifestError: error.message || '模块清单无效' };
    }
  }));
}

function resolveAsset(path, moduleBase) {
  if (typeof path !== 'string' || path.startsWith('/') || /(^|\/)\.\.?($|\/)|[?#\\]/.test(path) || /^[a-z][a-z0-9+.-]*:/i.test(path)) throw new Error('模块资源路径无效');
  const url = new URL(path, moduleBase);
  if (url.origin !== origin || !url.pathname.startsWith(`${moduleBase.pathname}assets/`)) throw new Error('模块资源必须位于自身 assets 目录');
  return url;
}

export async function loadModuleRuntime(module) {
  const runtime = (await import(module.moduleURL)).default;
  if (!runtime || runtime.id !== module.id || !Array.isArray(runtime.routes) || typeof runtime.createAPI !== 'function') throw new Error(`模块 ${module.id} JS 启动契约无效`);
  for (const key of ['title', 'icon', 'group', 'entry', 'home', 'navigation', 'styles']) {
    if (Object.hasOwn(runtime, key)) throw new Error(`模块 ${module.id} 的 ${key} 元数据只能声明在 manifest.json`);
  }
  return {
    ...module,
    routes: [...runtime.routes],
    createAPI: runtime.createAPI,
  };
}
