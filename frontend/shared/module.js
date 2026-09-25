export function defineModule(config) {
  for (const key of ['title', 'icon', 'group', 'entry', 'home', 'navigation', 'styles']) {
    if (Object.hasOwn(config, key)) throw new TypeError(`module metadata ${key} belongs in manifest.json`);
  }
  return {
    id: config.id,
    routes: [...(config.routes || [])],
    createAPI: config.createAPI || (({ request }) => ({ request })),
  };
}
