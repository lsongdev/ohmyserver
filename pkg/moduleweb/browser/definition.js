import { assetURL, localPath, moduleRoot, mountPath } from './paths.js';
import { matchModuleRoute } from './router.js';

export const contractVersion = 1;
const identifier = /^[a-z][a-z0-9-]*$/;
const mountKeys = new Set(['id', 'apiBase', 'pageBase']);
const manifestKeys = new Set(['contractVersion', 'id', 'title', 'icon', 'group', 'entry', 'home', 'styles', 'navigation']);
const pageKeys = new Set(['api', 'params', 'search', 'navigate', 'href', 'hrefTo']);

export function validateManifest(value, expectedID) {
  if (!value || value.contractVersion !== contractVersion || !identifier.test(value.id || '') || (expectedID && value.id !== expectedID)) {
    throw new TypeError('invalid module manifest identity or version');
  }
  for (const key of Object.keys(value)) {
    if (!manifestKeys.has(key)) throw new TypeError(`unknown module manifest field: ${key}`);
  }
  for (const key of ['title', 'icon', 'group']) {
    if (value[key] !== undefined && typeof value[key] !== 'string') throw new TypeError(`invalid manifest ${key}`);
  }
  assetURL(value.entry, new URL('http://module.invalid/'));
  localPath(value.home ?? '/', { query: false });
  if (!Array.isArray(value.styles ?? []) || !(value.styles ?? []).every(path => typeof path === 'string')) throw new TypeError('invalid manifest styles');
  for (const path of value.styles ?? []) assetURL(path, new URL('http://module.invalid/'));
  if (!Array.isArray(value.navigation ?? [])) throw new TypeError('invalid manifest navigation');
  const navigation = (value.navigation ?? []).map(item => {
    if (!item || typeof item.title !== 'string' || Object.keys(item).some(key => !['title', 'path'].includes(key))) throw new TypeError('invalid manifest navigation');
    localPath(item.path, { query: false });
    return Object.freeze({ title: item.title, path: item.path });
  });
  return Object.freeze({ ...value, title: value.title || value.id, icon: value.icon || '◇', group: value.group || '其他', home: value.home ?? '/', styles: Object.freeze([...(value.styles ?? [])]), navigation: Object.freeze(navigation) });
}

export function validateRuntime(value, manifest) {
  if (!value || value.id !== manifest.id || typeof value.createAPI !== 'function' || !Array.isArray(value.routes)) throw new TypeError('invalid module runtime');
  if (Object.keys(value).some(key => !['id', 'createAPI', 'routes'].includes(key))) throw new TypeError('module runtime only declares id, createAPI and routes');
  const patterns = new Set();
  const routes = value.routes.map(route => {
    if (!route || typeof route.load !== 'function' || Object.keys(route).some(key => !['path', 'load', 'props'].includes(key))) throw new TypeError('invalid module route');
    localPath(route.path, { query: false });
    if (route.path !== '/' && (route.path.endsWith('/') || route.path.includes('//'))) throw new TypeError('invalid route path');
    const names = new Set();
    const pattern = route.path.split('/').map(segment => {
      if (!segment.startsWith(':')) return segment;
      const name = segment.slice(1);
      if (!/^[a-zA-Z][a-zA-Z0-9_]*$/.test(name) || names.has(name)) throw new TypeError('invalid route parameter');
      names.add(name);
      return ':';
    }).join('/');
    if (patterns.has(pattern)) throw new TypeError('duplicate module route');
    patterns.add(pattern);
    if (route.props != null && (typeof route.props !== 'object' || Array.isArray(route.props) || Object.keys(route.props).some(key => pageKeys.has(key)))) throw new TypeError('route props conflict with host capabilities');
    return Object.freeze({ ...route, props: Object.freeze({ ...route.props }) });
  });
  for (const path of [manifest.home, ...manifest.navigation.map(item => item.path)]) {
    if (!matchModuleRoute(routes, path)) throw new TypeError(`manifest path has no route: ${path}`);
  }
  return Object.freeze({ id: value.id, createAPI: value.createAPI, routes: Object.freeze(routes) });
}

export function validateMount(value) {
  if (!value || !identifier.test(value.id || '')) throw new TypeError('invalid module mount identity');
  for (const key of Object.keys(value)) {
    if (!mountKeys.has(key)) throw new TypeError(`unknown module mount field: ${key}`);
  }
  return Object.freeze({ id: value.id, apiBase: mountPath(value.apiBase), pageBase: mountPath(value.pageBase) });
}

export async function discoverModule({ apiBase, pageBase = '/', id, fetchImpl = fetch, origin = globalThis.location?.origin || 'http://localhost' }) {
  const root = moduleRoot(apiBase, origin);
  const response = await fetchImpl(new URL('manifest.json', root).href, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`module manifest failed (${response.status})`);
  const manifest = validateManifest(await response.json(), id);
  const entryURL = assetURL(manifest.entry, root).href;
  const styleURLs = manifest.styles.map(path => assetURL(path, root).href);
  const mount = validateMount({ id: manifest.id, apiBase, pageBase });
  return Object.freeze({ mount, manifest, entryURL, styleURLs: Object.freeze(styleURLs) });
}

export function loadStyles(definition, document = globalThis.document) {
  for (const url of definition.styleURLs) {
    if ([...document.querySelectorAll('link[data-module-style]')].some(link => link.href === url)) continue;
    const link = document.createElement('link');
    link.rel = 'stylesheet'; link.href = url; link.dataset.moduleStyle = definition.manifest.id;
    document.head.append(link);
  }
}

const runtimeLoads = new WeakMap();
export function loadRuntime(definition) {
  if (!runtimeLoads.has(definition)) {
    const pending = import(definition.entryURL).then(loaded => validateRuntime(loaded.default, definition.manifest)).catch(error => {
      runtimeLoads.delete(definition);
      throw error;
    });
    runtimeLoads.set(definition, pending);
  }
  return runtimeLoads.get(definition);
}
