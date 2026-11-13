import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { Alert, Loading } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';

export const sections = {
  inbounds: { title: 'Inbounds', item: 'inbound', description: 'Listeners that accept connections.', example: { tag: 'local-socks', listen: '127.0.0.1', port: 1080, protocol: 'socks', settings: { udp: false } } },
  outbounds: { title: 'Outbounds', item: 'outbound', description: 'The first outbound is the default when no routing rule matches.', example: { tag: 'direct', protocol: 'freedom', settings: {} } },
  rules: { title: 'Rules', item: 'rule', description: 'Rules are checked from top to bottom. The first match wins.', example: { type: 'field', network: 'tcp,udp', outboundTag: 'direct' } },
};
export const sectionFor = key => Object.hasOwn(sections, key) ? sections[key] : null;
export const entries = (config, key) => (key === 'rules' ? config.routing?.rules : config[key]) || [];
export const withEntries = (config, key, values) => key === 'rules'
  ? { ...config, routing: { ...config.routing, rules: values } }
  : { ...config, [key]: values };
export const format = value => JSON.stringify(value, null, 2);

export function WithConfig({ render }) {
  const { api } = usePanel();
  const resource = useResource(signal => api.get('/config', { signal }), []);
  if (resource.error) return html`<${Alert} error=${resource.error} />`;
  if (!resource.data) return html`<${Loading} />`;
  return render(resource.data, resource.reload);
}
