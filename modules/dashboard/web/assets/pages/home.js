import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Dashboard({ api, hrefTo }) {
  const [overview, setOverview] = useState(null);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  useEffect(() => { const controller = new AbortController(); api.overview({ signal: controller.signal }).then(setOverview).catch(err => { if (err.name !== 'AbortError') setError(err.message || '概览数据读取失败'); }); return () => controller.abort(); }, [api, refresh]);
  const system = overview?.system;
  const formatBytes = value => { let size = Number(value || 0), unit = 'B'; for (const next of ['KiB','MiB','GiB','TiB']) { if (size < 1024) break; size /= 1024; unit = next; } return `${size.toFixed(size < 10 && unit !== 'B' ? 1 : 0)} ${unit}`; };
  const formatUptime = value => { const seconds = Number(value || 0); const days = Math.floor(seconds / 86400), hours = Math.floor(seconds % 86400 / 3600); return days ? `${days} 天 ${hours} 小时` : `${hours} 小时 ${Math.floor(seconds % 3600 / 60)} 分钟`; };
  const widgetsHref = hrefTo?.("widgets", "/");
  return html`<div><div class="page-toolbar"><h2>系统概览</h2>${widgetsHref ? html`<a class="button" href=${widgetsHref}>管理首页组件</a>` : null}</div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    ${system ? html`<section class="dashboard-system" aria-label="系统状态"><div class="page-toolbar"><h3>${system.hostname || '本机'} · 系统状态</h3><button class="button" onClick=${()=>setRefresh(value=>value+1)}>刷新</button></div><div class="dashboard-stat-grid">
      <article class="card"><h4>处理器</h4><strong>${Number(system.cpu_usage||0).toFixed(1)}%</strong><span>${system.cpu_cores||0} 核心 · 负载 ${Number(system.load_1||0).toFixed(2)}</span></article>
      <article class="card"><h4>内存</h4><strong>${Number(system.memory_usage||0).toFixed(1)}%</strong><span>${formatBytes(system.memory_used)} / ${formatBytes(system.memory_total)}</span></article>
      <article class="card"><h4>磁盘</h4><strong>${Number(system.disk_usage||0).toFixed(1)}%</strong><span>${formatBytes(system.disk_used)} / ${formatBytes(system.disk_total)}</span></article>
      <article class="card"><h4>运行时间</h4><strong>${formatUptime(system.uptime_seconds)}</strong><span>采集于 ${system.timestamp ? new Date(system.timestamp).toLocaleTimeString() : '—'}</span></article>
    </div></section>` : null}
    ${overview?.widgets?.length ? html`<section class="dashboard-widgets"><h3>首页组件</h3><div class="dashboard-widget-grid">${overview.widgets.map(item => html`<article class="card" key=${item.id}><h4>${item.name}</h4><span class="status-pill">${item.type}</span>${item.config ? html`<pre class="operation-log">${item.config}</pre>` : null}</article>`)}</div></section>` : null}
    ${overview && !overview.widgets?.length ? html`<div class="empty-state">还没有启用的首页组件。${widgetsHref ? html`<a href=${widgetsHref}>去配置组件</a>` : null}</div>` : null}
    ${!overview && !error ? html`<div class="loading-state" role="status">正在读取系统状态…</div>` : null}
  </div>`;
}
