import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

const bytes = value => { const units = ['B', 'KB', 'MB', 'GB', 'TB']; let n = Number(value || 0), i = 0; while (n >= 1024 && i < units.length - 1) { n /= 1024; i += 1; } return `${n.toFixed(i ? 1 : 0)} ${units[i]}`; };
const uptime = value => { const s = Number(value || 0), d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60); return `${d ? `${d}天 ` : ''}${h ? `${h}时 ` : ''}${m}分`; };
function Meter({ label, value, tone }) { const amount = Math.max(0, Math.min(100, Number(value || 0))); return html`<div class="monitor-meter"><span>${label}</span><div class="monitor-track"><i class=${amount > 90 ? 'danger' : tone} style=${{ width: `${amount}%` }} /></div><b>${amount.toFixed(1)}%</b></div>`; }

export default function Hosts({ api, href }) {
  const [hosts, setHosts] = useState([]); const [error, setError] = useState('');
  useEffect(() => {
    let active = true; let controller;
    const refresh = () => { controller?.abort(); controller = new AbortController(); api.hosts({ signal: controller.signal }).then(v => { if (active) { setHosts(v || []); setError(''); } }).catch(e => { if (active && e.name !== 'AbortError') setError(e.message); }); };
    refresh(); const timer = setInterval(refresh, 10000);
    return () => { active = false; clearInterval(timer); controller?.abort(); };
  }, [api]);
  return html`<div class="page-toolbar"><h2>主机状态</h2><span class="muted">每 10 秒刷新</span></div>
    ${error ? html`<div class="notice-card error">${error}</div>` : null}
    ${hosts.length ? html`<div class="monitor-grid">${hosts.map(s => html`<a class="card monitor-card" href=${href(`/hosts/${encodeURIComponent(s.hostname)}`)} key=${s.hostname}>
      <div class="monitor-card-head"><h3>${s.hostname}</h3><time>${new Date(s.timestamp).toLocaleString()}</time></div>
      <${Meter} label="CPU" value=${s.cpu_usage} tone="cpu"/><${Meter} label="内存" value=${s.memory_usage} tone="memory"/>${s.disk_total ? html`<${Meter} label="磁盘" value=${s.disk_usage} tone="disk"/>` : null}
      <div class="monitor-facts"><span>负载 ${Number(s.load_1).toFixed(2)} / ${Number(s.load_5).toFixed(2)} / ${Number(s.load_15).toFixed(2)}</span><span>↑ ${bytes(s.net_bytes_sent)}　↓ ${bytes(s.net_bytes_recv)}</span><span>运行 ${uptime(s.uptime)}</span></div>
    </a>`)}</div>` : html`<div class="empty-state">${error ? '监控数据暂不可用' : '尚无主机监控数据。远程主机可将 Telegraf 数据发送到 /monitor/telegraf。'}</div>`}`;
}
