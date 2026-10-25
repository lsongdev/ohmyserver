import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, Badge } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, HostStatus, Meter, Chart, KINDS, bytes, rate, pct, fixed, uptime } from '../monitor.js';

const RANGES = [['1h', '1 小时'], ['6h', '6 小时'], ['24h', '24 小时'], ['7d', '7 天'], ['30d', '30 天'], ['90d', '90 天']];
const SPANS = { '1h': 3600e3, '6h': 6 * 3600e3, '24h': 86400e3, '7d': 7 * 86400e3, '30d': 30 * 86400e3, '90d': 90 * 86400e3 };

const percent = v => `${fixed(v)}%`;

export default function Host({ params, query, setQuery }) {
  const { api, href } = usePanel();
  const id = encodeURIComponent(params.id);
  const range = SPANS[query.range] ? query.range : '1h';
  const host = useApi(`/hosts/${id}`, 15000);
  const series = useApi(`/hosts/${id}/series?range=${range}`, range === '1h' ? 15000 : 300000);
  const action = useAction();

  if (!host.data) return html`<${Page} title="主机">${host.error ? html`<${Notice} error=${host.error} />` : html`<${Loading} />`}<//>`;
  const h = host.data;
  const s = h.latest;
  const points = series.data?.points || [];
  const step = series.data?.step || 15;
  const to = Date.now();
  const from = to - SPANS[range];
  const toggle = () => action.run(async () => { await api.post(`/hosts/${id}/${h.enabled ? 'disable' : 'enable'}`); host.reload(); });
  const chart = props => html`<${Chart} points=${points} step=${step} from=${from} to=${to} ...${props} />`;

  return html`<${Page} title=${h.name}
    subtitle=${html`<a href=${href('/')}>← 主机</a> · <${HostStatus} host=${h} />${h.error ? html` <span class="muted">${h.error}</span>` : null}`}
    actions=${h.status === 'gone' ? null : html`<button class="btn" disabled=${action.busy} onClick=${toggle}>${h.enabled ? '停止监控' : '开始监控'}</button>`}>
    <${Notice} error=${action.error || series.error} />
    <div class="grid-2">
      <div class="card">
        <h2>主机信息</h2>
        <${KV} items=${[
          ['类型', html`<${Badge}>${KINDS[h.kind] || h.kind}<//>`],
          ['主机名', h.info.hostname],
          ['系统', h.info.os],
          ['内核', h.info.kernel],
          ['架构', h.info.arch],
          ['CPU', [h.info.cpu_model, h.info.cpu_cores ? `${h.info.cpu_cores} 核` : null].filter(Boolean).join(' · ')],
          ['运行时间', h.uptime ? uptime(h.uptime) : null],
          ['地址', h.info.addresses?.length ? html`<span class="mono">${h.info.addresses.join('  ')}</span>` : null],
        ]} />
      </div>
      <div class="card">
        <h2>当前</h2>
        ${!s ? html`<p class="muted">${h.status === 'disabled' ? '未监控' : '等待采集…'}</p>` : html`<${KV} items=${[
          ['CPU', `${s.cpu.toFixed(1)}%`],
          ['负载', `${s.load1.toFixed(2)} · ${s.load5.toFixed(2)} · ${s.load15.toFixed(2)}`],
          ['内存', `${bytes(s.mem_used)} / ${bytes(s.mem_total)}（${fixed(pct(s.mem_used, s.mem_total))}%）`],
          ['Swap', s.swap_total ? `${bytes(s.swap_used)} / ${bytes(s.swap_total)}` : '无'],
          ['网络', `↓ ${rate(s.net_recv)}  ↑ ${rate(s.net_sent)}`],
          ['磁盘读写', `读 ${rate(s.disk_read)}  写 ${rate(s.disk_write)}`],
        ]} />`}
      </div>
    </div>
    ${h.disks?.length ? html`<section class="section">
      <h2>文件系统</h2>
      <div class="table-wrap"><table class="table">
        <thead><tr><th>挂载点</th><th>设备</th><th>类型</th><th style="width:30%">用量</th><th class="num">已用 / 总量</th></tr></thead>
        <tbody>${h.disks.map(d => html`<tr key=${d.mount}>
          <td class="mono">${d.mount}</td><td class="mono cell-sub">${d.device}</td><td>${d.fstype || '—'}</td>
          <td><${Meter} value=${pct(d.used, d.total)} /></td>
          <td class="num nowrap">${bytes(d.used)} / ${bytes(d.total)}</td>
        </tr>`)}</tbody>
      </table></div>
    </section>` : null}
    <section class="section">
      <div style="display:flex;align-items:center;gap:12px;flex-wrap:wrap;margin-bottom:10px">
        <h2 style="margin:0;font-size:15px">历史</h2>
        <span class="ranges">${RANGES.map(([k, label]) => html`<button key=${k} class=${`btn btn-sm ${k === range ? 'btn-primary' : ''}`}
          onClick=${() => setQuery({ range: k === '1h' ? '' : k })}>${label}</button>`)}</span>
      </div>
      ${series.data === undefined ? html`<${Loading} />` : html`<div class="charts">
        ${chart({ title: 'CPU', format: percent, max: 100, series: [{ label: '使用率', value: p => p.cpu, color: 'var(--series-1)' }] })}
        ${chart({ title: '负载', format: v => (v ?? 0).toFixed(2), series: [
          { label: '1 分钟', value: p => p.load1, color: 'var(--series-1)' },
          { label: '5 分钟', value: p => p.load5, color: 'var(--series-2)' },
          { label: '15 分钟', value: p => p.load15, color: 'var(--series-3)' }] })}
        ${chart({ title: '内存', format: percent, max: 100, series: [
          { label: '内存', value: p => pct(p.mem_used, p.mem_total), color: 'var(--series-1)' },
          { label: 'Swap', value: p => pct(p.swap_used, p.swap_total), color: 'var(--series-3)' }] })}
        ${chart({ title: '网络', format: rate, series: [
          { label: '接收', value: p => p.net_recv, color: 'var(--series-1)' },
          { label: '发送', value: p => p.net_sent, color: 'var(--series-2)' }] })}
        ${chart({ title: '磁盘读写', format: rate, series: [
          { label: '读', value: p => p.disk_read, color: 'var(--series-1)' },
          { label: '写', value: p => p.disk_write, color: 'var(--series-3)' }] })}
        ${chart({ title: '磁盘用量', format: percent, max: 100, series: [{ label: '已用', value: p => pct(p.disk_used, p.disk_total), color: 'var(--series-2)' }] })}
      </div>`}
    </section>
  <//>`;
}
