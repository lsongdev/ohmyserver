export function readLocation(hash = location.hash) {
  const source = hash.startsWith('#') ? hash.slice(1) : hash;
  let url;
  try {
    localPath(source || '/');
    url = new URL(source || '/', location.origin);
  } catch {
    return { pathname: '/__invalid_path__', rawSegments: [], segments: [], search: new URLSearchParams() };
  }
  const rawSegments = url.pathname.split('/').filter(Boolean);
  let segments;
  try {
    segments = rawSegments.map(decodeURIComponent);
  } catch {
    segments = ['__invalid_path__'];
  }
  return { pathname: url.pathname, rawSegments, segments, search: url.searchParams };
}

export function href(path) {
  return `#${localPath(path)}`;
}

export function navigate(path, { replace = false } = {}) {
  const target = href(path);
  if (replace) {
    history.replaceState(null, '', target);
    globalThis.dispatchEvent(new Event('hashchange'));
  } else {
    location.hash = target.slice(1);
  }
}

export function scopeModulePath(pageBase, path) {
  localPath(path);
  const normalized = mountPath(pageBase);
  const base = normalized === '/' ? '' : normalized;
  return `${base}${path}` || '/';
}

export function resolveLocation(hash, modules) {
  const location = readLocation(hash);
  const module = modules.filter(item => {
    const base = item.mount.pageBase;
    return base === '/' || location.pathname === base || location.pathname.startsWith(`${base}/`);
  }).sort((a, b) => b.mount.pageBase.length - a.mount.pageBase.length)[0] || null;
  if (!module) return { module: null, page: null, params: {}, location };
  const base = module.mount.pageBase === '/' ? '' : module.mount.pageBase;
  const requested = location.pathname.slice(base.length) || '/';
  const pathname = requested === '/' ? (module.manifest?.home || '/') : requested;
  const matched = matchModuleRoute(module.runtime?.routes || [], pathname);
  return { module, page: matched?.route || null, params: matched?.params || {}, location };
}

export function navigationFor(pageBase, hrefTo = () => null) {
  return Object.freeze({
    href: path => href(scopeModulePath(pageBase, path)),
    navigate: (path, options) => navigate(scopeModulePath(pageBase, path), options),
    hrefTo,
  });
}

export function matchRoute(route, pathname) {
  const expected = route.split('/').filter(Boolean);
  const actual = pathname.split('/').filter(Boolean);
  if (expected.length !== actual.length) return null;
  const params = {};
  for (let i = 0; i < expected.length; i += 1) {
    let segment;
    try {
      segment = decodeURIComponent(actual[i]);
    } catch {
      return null;
    }
    if (expected[i].startsWith(':')) params[expected[i].slice(1)] = segment;
    else if (expected[i] !== segment) return null;
  }
  return params;
}

export function matchModuleRoute(routes, pathname) {
  let best = null;
  let bestSpecificity = -1;
  for (const route of routes) {
    const params = matchRoute(route.path, pathname);
    if (params === null) continue;
    const specificity = route.path.split('/').filter(Boolean)
      .map(segment => segment.startsWith(':') ? 0 : 1);
    let outranks = best === null;
    if (!outranks) {
      for (let i = 0; i < specificity.length; i += 1) {
        if (specificity[i] !== bestSpecificity[i]) {
          outranks = specificity[i] > bestSpecificity[i];
          break;
        }
      }
    }
    if (outranks) {
      best = { route, params };
      bestSpecificity = specificity;
    }
  }
  return best;
}
import { localPath, mountPath } from './paths.js';
