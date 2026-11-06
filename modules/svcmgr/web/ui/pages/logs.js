import { html, useState } from '@lsongdev/panel/html.js';
import { Page, Check } from '@lsongdev/panel/ui.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi, Journal, Unsupported, unitLabel } from '../services.js';

const PRIORITIES = [['', 'All priorities'], ['3', 'Errors'], ['4', 'Warnings and above'], ['5', 'Notices and above'], ['6', 'Info and above']];
const RANGES = [['', 'Newest entries'], ['15m', 'Last 15 minutes'], ['1h', 'Last hour'], ['6h', 'Last 6 hours'], ['24h', 'Last 24 hours'], ['168h', 'Last 7 days']];

export default function Logs() {
  const { query, setQuery } = usePanel();
  const system = useApi('/system');
  const services = useApi(system.data?.systemd ? '/services' : null);
  const [text, setText] = useState(query.q || '');
  const live = query.live !== '0';

  if (system.data && !system.data.systemd) return html`<${Page} title="Logs"><${Unsupported} /><//>`;
  const journal = { unit: query.unit || '', priority: query.priority || '', since: query.since || '', q: query.q || '', lines: 500 };
  const select = (key, options) => html`<select class="input" value=${query[key] || ''} aria-label=${key}
    onChange=${e => setQuery({ [key]: e.currentTarget.value })}>
    ${options.map(([value, label]) => html`<option key=${value} value=${value}>${label}</option>`)}
  </select>`;
  return html`<${Page} title="Logs" subtitle="The system journal: every service and the kernel.">
    <form class="filters" onSubmit=${e => { e.preventDefault(); setQuery({ q: text.trim() }); }}>
      ${select('unit', [['', 'All services'], ...(services.data || []).filter(u => u.load === 'loaded').map(u => [u.name, unitLabel(u.name)])])}
      ${select('priority', PRIORITIES)}
      ${select('since', RANGES)}
      <input class="input grow" type="search" placeholder="Search messages" aria-label="Search"
        value=${text} onInput=${e => setText(e.currentTarget.value)} onBlur=${() => setQuery({ q: text.trim() })} />
      <${Check} label="Live" checked=${live} onChange=${e => setQuery({ live: e.currentTarget.checked ? '' : '0' })} />
    </form>
    <${Journal} query=${journal} live=${live} />
  <//>`;
}
