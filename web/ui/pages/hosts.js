import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Badge, confirmDanger } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, HostStatus, Meter, Sparkline, KINDS, bytes, rate, pct, uptime } from '../monitor.js';

export default function Hosts() {
  const { api } = usePanel();
  const hosts = useApi('/hosts?spark=1', 15000);
  const action = useAction();
  const list = hosts.data || [];
  const active = list.filter(h => h.status !== 'gone');
  const gone = list.filter(h => h.status === 'gone');
  const remove = h => {
    if (!confirmDanger(`删除 ${h.name} 的全部监控记录？`)) return;
    action.run(async () => { await api.del(`/hosts/${encodeURIComponent(h.id)}`); hosts.reload(); });
  };
  return html`<${Page} title="主机监控" subtitle="本机和已保存的 SSH 主机，每 15 秒采集一次。">
    <${Notice} error=${hosts.error || action.error} />
    ${hosts.data === undefined ? (hosts.error ? null : html`<${Loading} />`) : !active.length ? html`<${Empty}>没有可监控的主机。<//>` : html`
      <div class="host-grid">${active.map(h => html`<${HostCard} key=${h.id} host=${h} />`)}</div>`}
    ${gone.length ? html`<section class="section">
      <h2>已移除的主机</h2>
      <p class="muted" style="margin-top:0">这些主机已经不在 SSH 模块里了，只剩历史记录。</p>
      <div class="table-wrap"><table class="table">
        <thead><tr><th>主机</th><th>最后采集</th><th class="actions"></th></tr></thead>
        <tbody>${gone.map(h => html`<tr key=${h.id}>
          <td class="cell-main">${h.name} <span class="muted mono">${h.id}</span></td>
          <td class="muted">${h.last_seen ? new Date(h.last_seen).toLocaleString('zh-CN', { hour12: false }) : '—'}</td>
          <td class="actions"><button class="btn btn-sm btn-danger" disabled=${action.busy} onClick=${() => remove(h)}>删除记录</button></td>
        </tr>`)}</tbody>
      </table></div>
    </section>` : null}
  <//>`;
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
        <div class="cell-sub">${[h.info.os, h.info.cpu_cores ? `${h.info.cpu_cores} 核` : null].filter(Boolean).join(' · ') || h.id}</div>
      </div>
      <${HostStatus} host=${h} />
    </div>
    ${h.status === 'disabled' ? html`<div class="muted">未监控</div>` : !s ? html`<div class="muted">${h.error || '等待第二次采集…'}</div>` : html`
      <div class="gauges">
        <div class="gauge"><span>CPU</span><${Meter} value=${s.cpu} /><span class="value">${s.cpu.toFixed(1)}%</span></div>
        <div class="gauge"><span>内存</span><${Meter} value=${mem} /><span class="value">${bytes(s.mem_used)} / ${bytes(s.mem_total)}</span></div>
        <div class="gauge"><span>磁盘</span><${Meter} value=${disk} /><span class="value">${bytes(s.disk_used)} / ${bytes(s.disk_total)}</span></div>
      </div>
      <div class="sparks">
        <div class="spark-box">CPU · 1 小时<${Sparkline} values=${h.spark?.cpu} color="var(--series-1)" /></div>
        <div class="spark-box">内存 · 1 小时<${Sparkline} values=${h.spark?.mem} color="var(--series-2)" /></div>
      </div>
      <div class="host-foot">
        <span>↓ ${rate(s.net_recv)}</span><span>↑ ${rate(s.net_sent)}</span>
        <span>负载 ${s.load1.toFixed(2)}</span><span>运行 ${uptime(h.uptime)}</span>
        <span style="margin-left:auto"><${Badge}>${KINDS[h.kind] || h.kind}<//></span>
      </div>`}
  </a>`;
}
