import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Empty, Badge, Table } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi, HostStatus, Meter, Sparkline, KINDS, bytes, rate, pct, uptime } from '../monitor.js';

export default function Hosts() {
  const { api } = usePanel();
  const hosts = useApi('/hosts?spark=1', 15000);
  const action = useAction();
  const list = hosts.data || [];
  const active = list.filter(h => h.status !== 'gone');
  const gone = list.filter(h => h.status === 'gone');
  const remove = h => {
    if (!window.confirm(`Delete all monitoring data of ${h.name}?`)) return;
    action.run(async () => { await api.del(`/hosts/${encodeURIComponent(h.id)}`); hosts.reload(); });
  };
  return html`<${PageHeader} title="Hosts" subtitle="This machine and the saved SSH hosts, sampled every 15 seconds." />
    <${Alert} error=${hosts.error || action.error} />
    ${hosts.data === undefined ? (hosts.error ? null : html`<${Loading} />`) : !active.length ? html`<${Empty}>No hosts to monitor.<//>` : html`
      <div class="host-grid">${active.map(h => html`<${HostCard} key=${h.id} host=${h} />`)}</div>`}
    ${gone.length ? html`<section class="section">
      <h2>Removed hosts</h2>
      <p class="muted" style="margin-top:0">These hosts are no longer saved as SSH hosts; only their history is left.</p>
      <${Table}>
        <thead><tr><th>Host</th><th>Last sample</th><th class="actions"></th></tr></thead>
        <tbody>${gone.map(h => html`<tr key=${h.id}>
          <td class="cell-main">${h.name} <span class="muted mono">${h.id}</span></td>
          <td class="muted">${h.last_seen ? new Date(h.last_seen).toLocaleString('zh-CN', { hour12: false }) : '—'}</td>
          <td class="actions"><button class="button button-small button-danger" disabled=${action.busy} onClick=${() => remove(h)}>Delete data</button></td>
        </tr>`)}</tbody>
      <//>
    </section>` : null}`;
}

function HostCard({ host: h }) {
  const { href } = usePanel();
  const s = h.latest;
  const disk = s ? pct(s.disk_used, s.disk_total) : 0;
  const mem = s ? pct(s.mem_used, s.mem_total) : 0;
  return html`<a class="card host-card" href=${href(`/hosts/${encodeURIComponent(h.id)}`)}>
    <div class="host-head">
      <div>
        <div class="cell-main">${h.name}</div>
        <div class="cell-sub">${[h.info.os, h.info.cpu_cores ? `${h.info.cpu_cores} cores` : null].filter(Boolean).join(' · ') || h.id}</div>
      </div>
      <${HostStatus} host=${h} />
    </div>
    ${h.status === 'disabled' ? html`<div class="muted">Not monitored</div>` : !s ? html`<div class="muted">${h.error || 'Waiting for a second sample…'}</div>` : html`
      <div class="gauges">
        <div class="gauge"><span>CPU</span><${Meter} value=${s.cpu} /><span class="value">${s.cpu.toFixed(1)}%</span></div>
        <div class="gauge"><span>Memory</span><${Meter} value=${mem} /><span class="value">${bytes(s.mem_used)} / ${bytes(s.mem_total)}</span></div>
        <div class="gauge"><span>Disk</span><${Meter} value=${disk} /><span class="value">${bytes(s.disk_used)} / ${bytes(s.disk_total)}</span></div>
      </div>
      <div class="sparks">
        <div class="spark-box">CPU · 1 hour<${Sparkline} values=${h.spark?.cpu} color="var(--series-1)" /></div>
        <div class="spark-box">Memory · 1 hour<${Sparkline} values=${h.spark?.mem} color="var(--series-2)" /></div>
      </div>
      <div class="host-foot">
        <span>↓ ${rate(s.net_recv)}</span><span>↑ ${rate(s.net_sent)}</span>
        <span>Load ${s.load1.toFixed(2)}</span><span>Up ${uptime(h.uptime)}</span>
        <span style="margin-left:auto"><${Badge}>${KINDS[h.kind] || h.kind}<//></span>
      </div>`}
  </a>`;
}
