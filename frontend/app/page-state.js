export function pageKey(module, page) {
  return module && page ? `${module.id}:${page.path}` : '';
}

export function pageForRoute(loaded, key) {
  return loaded?.key === key ? loaded.component : null;
}
