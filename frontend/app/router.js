export function readLocation(hash = location.hash) {
  const source = hash.startsWith('#') ? hash.slice(1) : hash;
  const url = new URL(source || '/', location.origin);
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
  const normalized = path.startsWith('/') ? path : `/${path}`;
  return `#${normalized}`;
}

export function navigate(path, { replace = false } = {}) {
  const target = href(path);
  if (replace) {
    history.replaceState(null, '', target);
    window.dispatchEvent(new HashChangeEvent('hashchange'));
  } else {
    location.hash = target.slice(1);
  }
}

export function scopeModulePath(pageBase, path) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) {
    throw new TypeError('module navigation path must start with a single /');
  }
  const base = pageBase === '/' ? '' : pageBase.replace(/\/+$/, '');
  if (base && (path === base || path.startsWith(`${base}/`))) {
    throw new TypeError('module navigation path must be relative to the module root');
  }
  return `${base}${path}` || '/';
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
