import { discoverModule, loadStyles, validateMount } from '#moduleweb/definition.js';

const hostConfig = globalThis.OhMyServerConfig || {};
const apiRoot = (hostConfig.apiBase || '/v1').replace(/\/$/, '');

export async function discoverModules(fetchImpl = fetch) {
  const response = await fetchImpl(`${apiRoot}/modules`, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`模块列表加载失败 (${response.status})`);
  const payload = await response.json();
  if (!Array.isArray(payload.modules)) throw new Error('模块列表格式无效');
  const mounts = payload.modules.map(validateMount);
  for (const key of ['id', 'apiBase', 'pageBase']) {
    if (new Set(mounts.map(mount => mount[key])).size !== mounts.length) throw new Error(`duplicate module ${key}`);
  }
  return Promise.all(mounts.map(async mount => {
    try {
      const definition = await discoverModule({ ...mount, fetchImpl });
      loadStyles(definition);
      return definition;
    } catch (error) {
      return { mount, manifest: { id: mount.id, title: mount.id, icon: '◇', group: '其他', home: '/', navigation: [] }, manifestError: error.message || '模块清单无效' };
    }
  }));
}
