// All public module paths are relative to the module root, written with one /.
export function localPath(path, { query = true } = {}) {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//') || /[\\#\s]/.test(path)) {
    throw new TypeError('module path must start with a single / and stay inside its mount');
  }
  const pathname = path.split('?')[0];
  if (!query && pathname !== path) throw new TypeError('query is not allowed in this path');
  for (const segment of pathname.split('/')) {
    let decoded;
    try { decoded = decodeURIComponent(segment); } catch { throw new TypeError('invalid path encoding'); }
    if (decoded === '.' || decoded === '..' || decoded.includes('\\')) throw new TypeError('module path escaped its mount');
  }
  return path;
}

export function mountPath(path) {
  localPath(path, { query: false });
  if (/[%:]|\/\//.test(path)) throw new TypeError('invalid module mount');
  return path === '/' ? '/' : path.replace(/\/$/, '');
}

export function moduleRoot(apiBase, origin = globalThis.location?.origin || 'http://localhost') {
  const base = mountPath(apiBase);
  return new URL(base === '/' ? '/' : `${base}/`, origin);
}

export function assetURL(path, root) {
  if (typeof path !== 'string' || !path.startsWith('assets/') || /[?#\\%\s]/.test(path) || /(^|\/)\.\.?($|\/)/.test(path)) {
    throw new TypeError('invalid module asset path');
  }
  const url = new URL(path, root);
  if (url.origin !== root.origin || !url.pathname.startsWith(`${root.pathname}assets/`)) throw new TypeError('module asset escaped its mount');
  return url;
}
