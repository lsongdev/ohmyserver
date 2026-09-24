import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function ContainerDetail({ api, params, navigate }) {
  const [item, setItem] = useState(null);
  const [error, setError] = useState('');
  const [logs, setLogs] = useState('');
  const [logsError, setLogsError] = useState('');
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [stats, setStats] = useState(null);
  const [statsError, setStatsError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    api.getContainer(params.id, { signal: controller.signal }).then(value => setItem(value))
      .catch(err => { if (err.name !== 'AbortError') setError(err.message || '容器读取失败'); });
    return () => controller.abort();
  }, [api, params.id]);
  useEffect(() => {
    const controller = new AbortController();
    api.streamContainerStats(params.id, setStats, { signal: controller.signal })
      .catch(err => { if (err.name !== 'AbortError') setStatsError(err.message || '容器统计读取失败'); });
    return () => controller.abort();
  }, [api, params.id]);
  async function loadLogs() {
    setLoadingLogs(true);
    setLogsError('');
    try { const result = await api.getContainerLogs(params.id); setLogs(result.logs || ''); }
    catch (err) { setLogsError(err.message || '日志读取失败'); }
    finally { setLoadingLogs(false); }
  }
  return html`<div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${item ? html`<div class="card detail-card"><div class="page-toolbar"><h2>${item.name || item.id}</h2><button class="button" onclick=${() => navigate('/docker/containers')}>返回容器列表</button></div>
      <dl><dt>ID</dt><dd>${item.id}</dd><dt>镜像</dt><dd>${item.image}</dd><dt>状态</dt><dd><span class=${`status-pill ${item.state}`}>${item.status || item.state}</span></dd></dl>
      <section class="section"><h3>实时资源</h3>${statsError ? html`<p class="muted" role="status">${statsError}</p>` : null}${stats ? html`<dl><dt>CPU</dt><dd>${stats.cpu_percent.toFixed(2)}%</dd><dt>内存</dt><dd>${(stats.memory_usage / 1073741824).toFixed(2)} / ${(stats.memory_limit / 1073741824).toFixed(2)} GiB (${stats.memory_percent.toFixed(1)}%)</dd><dt>网络接收</dt><dd>${(stats.network_rx / 1048576).toFixed(2)} MiB</dd><dt>网络发送</dt><dd>${(stats.network_tx / 1048576).toFixed(2)} MiB</dd></dl>` : html`<p class="muted">等待统计数据…</p>`}</section>
      <div class="page-toolbar"><h3>最近日志</h3><div class="button-group"><button class="button" onclick=${() => navigate(`/terminal/docker/${encodeURIComponent(params.id)}`)}>终端</button><button class="button button-secondary" onclick=${loadLogs} disabled=${loadingLogs}>${loadingLogs ? '读取中…' : '读取日志'}</button></div></div>
      ${logsError ? html`<p class="error-text" role="alert">${logsError}</p>` : null}
      ${logs ? html`<pre class="operation-log container-log">${logs}</pre>` : null}
    </div>` : !error ? html`<div class="loading-state">正在读取容器…</div>` : null}
  </div>`;
}
