import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, Badge, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, HostStatus, Meter, Chart, KINDS, bytes, rate, pct, fixed, uptime } from '../monitor.js';

const RANGES = [['1h', '1 hour'], ['6h', '6 hours'], ['24h', '24 hours'], ['7d', '7 days'], ['30d', '30 days'], ['90d', '90 days']];
const SPANS = { '1h': 3600e3, '6h': 6 * 3600e3, '24h': 86400e3, '7d': 7 * 86400e3, '30d': 30 * 86400e3, '90d': 90 * 86400e3 };

const percent = v => `${fixed(v)}%`;

export default function Host({ params, query, setQuery }) {
  const { api, href } = usePanel();
  const id = encodeURIComponent(params.id);
  const range = SPANS[query.range] ? query.range : '1h';
  const host = useApi(`/hosts/${id}`, 15000);
  const series = useApi(`/hosts/${id}/series?range=${range}`, range === '1h' ? 15000 : 300000);
  const action = useAction();

  if (!host.data) return html`<${Page} title="Host" back=${{ href: href('/'), label: 'Hosts' }}>${host.error ? html`<${Notice} error=${host.error} />` : html`<${Loading} />`}<//>`;
  const h = host.data;
  const s = h.latest;
  const points = series.data?.points || [];
  const step = series.data?.step || 15;
  const to = Date.now();
  const from = to - SPANS[range];
  const toggle = () => action.run(async () => { await api.post(`/hosts/${id}/${h.enabled ? 'disable' : 'enable'}`); host.reload(); });
  const chart = props => html`<${Chart} points=${points} step=${step} from=${from} to=${to} ...${props} />`;

  return html`<${Page} title=${h.name}
    back=${{ href: href('/'), label: 'Hosts' }}
    subtitle=${html`<${HostStatus} host=${h} />${h.error ? html` <span class="muted">${h.error}</span>` : null}`}
    actions=${h.status === 'gone' ? null : html`<button class="button" disabled=${action.busy} onClick=${toggle}>${h.enabled ? 'Stop monitoring' : 'Start monitoring'}</button>`}>
    <${Notice} error=${action.error || series.error} />
    <div class="grid-2">
      <div class="card">
        <h2>System</h2>
        <${KV} items=${[
          ['Kind', html`<${Badge}>${KINDS[h.kind] || h.kind}<//>`],
          ['Hostname', h.info.hostname],
          ['OS', h.info.os],
          ['Kernel', h.info.kernel],
          ['Architecture', h.info.arch],
          ['CPU', [h.info.cpu_model, h.info.cpu_cores ? `${h.info.cpu_cores} cores` : null].filter(Boolean).join(' · ')],
          ['Uptime', h.uptime ? uptime(h.uptime) : null],
          ['Addresses', h.info.addresses?.length ? html`<span class="mono">${h.info.addresses.join('  ')}</span>` : null],
        ]} />
      </div>
      <div class="card">
        <h2>Now</h2>
        ${!s ? html`<p class="muted">${h.status === 'disabled' ? 'Not monitored' : 'Waiting for data…'}</p>` : html`<${KV} items=${[
          ['CPU', `${s.cpu.toFixed(1)}%`],
          ['Load', `${s.load1.toFixed(2)} · ${s.load5.toFixed(2)} · ${s.load15.toFixed(2)}`],
          ['Memory', `${bytes(s.mem_used)} / ${bytes(s.mem_total)} (${fixed(pct(s.mem_used, s.mem_total))}%)`],
          ['Swap', s.swap_total ? `${bytes(s.swap_used)} / ${bytes(s.swap_total)}` : 'None'],
          ['Network', `↓ ${rate(s.net_recv)}  ↑ ${rate(s.net_sent)}`],
          ['Disk I/O', `read ${rate(s.disk_read)}  write ${rate(s.disk_write)}`],
        ]} />`}
      </div>
    </div>
    ${h.disks?.length ? html`<section class="section">
      <h2>Filesystems</h2>
      <${Table}>
        <thead><tr><th>Mount</th><th>Device</th><th>Type</th><th style="width:30%">Usage</th><th class="num">Used / Total</th></tr></thead>
        <tbody>${h.disks.map(d => html`<tr key=${d.mount}>
          <td class="mono">${d.mount}</td><td class="mono cell-sub">${d.device}</td><td>${d.fstype || '—'}</td>
          <td><${Meter} value=${pct(d.used, d.total)} /></td>
          <td class="num nowrap">${bytes(d.used)} / ${bytes(d.total)}</td>
        </tr>`)}</tbody>
      <//>
    </section>` : null}
    <section class="section">
      <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:10px">
        <h2 style="margin:0;font-size:15px">History</h2>
        <span class="ranges">${RANGES.map(([k, label]) => html`<button key=${k} class=${`button button-small ${k === range ? 'button-primary' : ''}`}
          onClick=${() => setQuery({ range: k === '1h' ? '' : k })}>${label}</button>`)}</span>
      </div>
      ${series.data === undefined ? html`<${Loading} />` : html`<div class="charts">
        ${chart({ title: 'CPU', format: percent, max: 100, series: [{ label: 'Usage', value: p => p.cpu, color: 'var(--series-1)' }] })}
        ${chart({ title: 'Load', format: v => (v ?? 0).toFixed(2), series: [
          { label: '1 min', value: p => p.load1, color: 'var(--series-1)' },
          { label: '5 min', value: p => p.load5, color: 'var(--series-2)' },
          { label: '15 min', value: p => p.load15, color: 'var(--series-3)' }] })}
        ${chart({ title: 'Memory', format: percent, max: 100, series: [
          { label: 'Memory', value: p => pct(p.mem_used, p.mem_total), color: 'var(--series-1)' },
          { label: 'Swap', value: p => pct(p.swap_used, p.swap_total), color: 'var(--series-3)' }] })}
        ${chart({ title: 'Network', format: rate, series: [
          { label: 'Received', value: p => p.net_recv, color: 'var(--series-1)' },
          { label: 'Sent', value: p => p.net_sent, color: 'var(--series-2)' }] })}
        ${chart({ title: 'Disk I/O', format: rate, series: [
          { label: 'Read', value: p => p.disk_read, color: 'var(--series-1)' },
          { label: 'Write', value: p => p.disk_write, color: 'var(--series-3)' }] })}
        ${chart({ title: 'Disk usage', format: percent, max: 100, series: [{ label: 'Used', value: p => pct(p.disk_used, p.disk_total), color: 'var(--series-2)' }] })}
      </div>`}
    </section>
  <//>`;
}
