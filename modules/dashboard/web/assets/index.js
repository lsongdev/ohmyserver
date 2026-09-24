import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/dashboard';
const request = async (path, options = {}) => {
  const response = await fetch(`${base}${path}`, options);
  const result = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(result.error?.message || `HTTP ${response.status}`);
  return result.data;
};
const api = createAPI({ request });
const formatBytes = value => {
  let size = Number(value || 0), unit = 'B';
  for (const next of ['KiB', 'MiB', 'GiB', 'TiB']) { if (size < 1024) break; size /= 1024; unit = next; }
  return `${size.toFixed(size < 10 && unit !== 'B' ? 1 : 0)} ${unit}`;
};
const formatUptime = value => {
  const seconds = Number(value || 0), days = Math.floor(seconds / 86400), hours = Math.floor(seconds % 86400 / 3600);
  return days ? `${days} 天 ${hours} 小时` : `${hours} 小时 ${Math.floor(seconds % 3600 / 60)} 分钟`;
};
function App() {
  const [overview, setOverview] = useState(null), [error, setError] = useState(''), [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.overview({ signal: controller.signal }).then(value => { setOverview(value); setError(''); }).catch(err => { if (err.name !== 'AbortError') setError(err.message || '概览数据读取失败'); });
    return () => controller.abort();
  }, [refresh]);
  const system = overview?.system;
  return html`<section><div class="page-toolbar"><h1>系统概览${system?.hostname ? ` · ${system.hostname}` : ''}</h1><button class="button" onClick=${() => setRefresh(value => value + 1)}>刷新</button></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${system ? html`<div class="dashboard-stat-grid"><article class="card"><h2>处理器</h2><strong>${Number(system.cpu_usage || 0).toFixed(1)}%</strong><span>${system.cpu_cores || 0} 核心 · 负载 ${Number(system.load_1 || 0).toFixed(2)}</span></article>
      <article class="card"><h2>内存</h2><strong>${Number(system.memory_usage || 0).toFixed(1)}%</strong><span>${formatBytes(system.memory_used)} / ${formatBytes(system.memory_total)}</span></article>
      <article class="card"><h2>磁盘</h2><strong>${Number(system.disk_usage || 0).toFixed(1)}%</strong><span>${formatBytes(system.disk_used)} / ${formatBytes(system.disk_total)}</span></article>
      <article class="card"><h2>运行时间</h2><strong>${formatUptime(system.uptime_seconds)}</strong><span>采集于 ${system.timestamp ? new Date(system.timestamp).toLocaleTimeString() : '—'}</span></article></div>` : null}
    ${overview?.widgets?.length ? html`<section><h2>首页组件</h2><div class="dashboard-widget-grid">${overview.widgets.map(item => html`<article class="card" key=${item.id}><h3>${item.name}</h3><span>${item.type}</span>${item.config ? html`<pre>${item.config}</pre>` : null}</article>`)}</div></section>` : null}
    ${overview && !overview.widgets?.length ? html`<p class="empty-state">当前服务没有配置首页组件。</p>` : null}
    ${!overview && !error ? html`<p role="status">正在读取系统状态…</p>` : null}
  </section>`;
}
render(html`<${App}/>`, document.getElementById('app'));
