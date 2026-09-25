import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const formatBytes = value => {
  if (!value) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let amount = value, index = 0;
  while (amount >= 1024 && index < units.length - 1) { amount /= 1024; index += 1; }
  return `${amount.toFixed(index ? 1 : 0)} ${units[index]}`;
};

export default function SSHStats({ api, params, navigate, hrefTo }) {
  const [server, setServer] = useState(null), [stats, setStats] = useState(null), [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    const source = new EventSource(api.statsURL(params.id), { withCredentials: true });
    api.get(params.id).then(value => { if (active) setServer(value); }).catch(e => { if (active) { setError(e.message || 'SSH 配置读取失败'); source.close(); } });
    source.addEventListener('stats', event => { try { setStats(JSON.parse(event.data)); setError(''); } catch { setError('统计数据格式错误'); } });
    source.addEventListener('error', event => {
      if (event.data) { try { const item = JSON.parse(event.data); setError(item.message || '远程统计暂不可用'); } catch { setError('远程统计暂不可用'); } }
    });
    source.onerror = () => { if (source.readyState === EventSource.CLOSED) setError('统计连接已关闭'); };
    return () => { active = false; source.close(); };
  }, [api, params.id]);
  const terminalHref = hrefTo?.('terminal', `/ssh/${params.id}`);
  return html`<div class="ssh-stats-page">
    ${error && html`<div class="notice-card error" role="alert">${error}</div>`}
    <section class="card"><div class="page-toolbar"><div><h2>${server?.name || `SSH 服务器 ${params.id}`}</h2><p class="muted">${server ? `${server.username}@${server.host}:${server.port}` : ''}</p></div><div class="button-group"><button class="button" onClick=${() => navigate('/servers')}>返回列表</button><span class="status-pill ${stats?.connected ? 'running' : ''}">${stats?.connected ? '已连接' : '等待数据'}</span><a class="button" href=${api.metricsURL(params.id)} target="_blank" rel="noreferrer">Prometheus 指标</a>${terminalHref && server?.enabled && server?.has_credential ? html`<a class="button" href=${terminalHref}>打开终端</a>` : null}</div></div>
      ${stats ? html`<div class="ssh-stat-grid"><article><span>CPU</span><strong>${stats.cpu_percent.toFixed(1)}%</strong></article><article><span>内存</span><strong>${stats.memory_percent.toFixed(1)}%</strong><small>${formatBytes(stats.memory_usage)} / ${formatBytes(stats.memory_total)}</small></article><article><span>网络接收</span><strong>${formatBytes(stats.network_rx)}</strong></article><article><span>网络发送</span><strong>${formatBytes(stats.network_tx)}</strong></article><article><span>负载均值</span><strong>${(stats.load_average || []).map(value => Number(value).toFixed(2)).join(' · ') || '—'}</strong></article></div><p class="muted">最近更新：${new Date(stats.connected_at).toLocaleTimeString()}</p>` : !error ? html`<div class="loading-state">正在收集 SSH 主机指标…</div>` : null}
    </section>
  </div>`;
}
