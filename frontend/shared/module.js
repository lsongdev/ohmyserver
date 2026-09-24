import { requestPlaceholder } from './placeholder-page.js';

export function defineModule(config) {
  const entry = config.entry || '/';
  const navigation = config.navigation || [];
  const routes = [...(config.routes || [])];
  for (const item of navigation) {
    if (!routes.some(route => route.path === item.path)) {
      routes.push({ path: item.path, load: () => Promise.resolve({ default: requestPlaceholder }) });
    }
  }
  if (!routes.length) routes.push({ path: entry, load: () => Promise.resolve({ default: requestPlaceholder }) });
  return {
    ...config,
    entry,
    navigation,
    routes,
    createAPI: config.createAPI || (({ request }) => ({ request })),
  };
}
