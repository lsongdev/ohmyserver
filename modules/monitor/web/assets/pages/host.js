import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

const charts = [
  { key: 'cpu_usage', label: 'CPU 使用率', max: 100, suffix: '%' },
  { key: 'memory_usage', label: '内存使用率', max: 100, suffix: '%' },
  { key: 'disk_usage', label: '磁盘使用率', max: 100, suffix: '%' },
  { key: 'net_bytes_sent', label: '网络发送累计', max: null, suffix: ' B' },
];
function Chart({ label, values, keyName, fixedMax, suffix = '' }) {
  const nums = values.map(v => Number(v[keyName] || 0)); const max = fixedMax || Math.max(...nums, 1); const min = fixedMax ? 0 : Math.min(...nums, 0); const range = max - min || 1;
  const points = nums.map((value, index) => `${nums.length < 2 ? 300 : 12 + index * 576 / (nums.length - 1)},${148 - (value - min) / range * 132}`).join(' ');
  return html`<section class="card monitor-chart"><h3>${label}</h3><svg viewBox="0 0 600 160" role="img" aria-label=${label}><line x1="12" y1="148" x2="588" y2="148"/><polyline points=${points}/></svg><div class="monitor-chart-endpoints"><span>${values.length ? new Date(values[0].timestamp).toLocaleTimeString() : '—'}</span><b>${nums.length ? `${nums[nums.length - 1].toFixed(1)}${suffix}` : '暂无数据'}</b><span>${values.length ? new Date(values[values.length - 1].timestamp).toLocaleTimeString() : '—'}</span></div></section>`;
}
const prettyBytes = n => { const units = ['B', 'KB', 'MB', 'GB', 'TB']; let i = 0, x = Number(n || 0); while (x >= 1024 && i < units.length - 1) { x /= 1024; i++; } return `${x.toFixed(i ? 1 : 0)} ${units[i]}`; };

export default function Host({ api, params, search, navigate }) {
  const hostname = params.hostname; const [duration, setDuration] = useState(search.get('duration') || '1h'); const [result, setResult] = useState(null); const [error, setError] = useState('');
  useEffect(() => { setDuration(search.get('duration') || '1h'); }, [search.toString()]);
  useEffect(() => {
    let active = true, controller;
    const refresh = () => { controller?.abort(); controller = new AbortController(); api.history(hostname, duration, { signal: controller.signal }).then(v => { if (active) { setResult(v); setError(''); } }).catch(e => { if (active && e.name !== 'AbortError') setError(e.message); }); };
    refresh(); const timer = setInterval(refresh, 10000);
    return () => { active = false; clearInterval(timer); controller?.abort(); };
  }, [api, hostname, duration]);
  const current = result?.current; const history = result?.history || [];
  const chooseDuration = value => { setDuration(value); navigate(`/monitor/hosts/${encodeURIComponent(hostname)}?duration=${encodeURIComponent(value)}`); };
  return html`<div class="page-toolbar"><div><h2>${hostname}</h2><span class="muted">${current ? new Date(current.timestamp).toLocaleString() : ''}</span></div><a class="button button-secondary" href="#/monitor/hosts">返回主机列表</a></div>
    ${error ? html`<div class="notice-card error">${error}</div>` : null}
    ${current ? html`<div class="monitor-stats"><div class="card"><span>CPU</span><b>${current.cpu_usage.toFixed(1)}%</b><small>${current.cpu_cores} cores</small></div><div class="card"><span>内存</span><b>${current.memory_usage.toFixed(1)}%</b><small>${prettyBytes(current.memory_used)} / ${prettyBytes(current.memory_total)}</small></div><div class="card"><span>磁盘</span><b>${current.disk_usage.toFixed(1)}%</b><small>${prettyBytes(current.disk_used)} / ${prettyBytes(current.disk_total)}</small></div><div class="card"><span>网络累计</span><b>${prettyBytes(current.net_bytes_sent + current.net_bytes_recv)}</b><small>发送 ${prettyBytes(current.net_bytes_sent)} · 接收 ${prettyBytes(current.net_bytes_recv)}</small></div><div class="card"><span>系统负载</span><b>${current.load_1.toFixed(2)}</b><small>1 / 5 / 15 分钟：${current.load_1.toFixed(2)} / ${current.load_5.toFixed(2)} / ${current.load_15.toFixed(2)}</small></div><div class="card"><span>运行时间</span><b>${Math.floor(current.uptime / 3600)} 小时</b><small>${current.timestamp}</small></div></div>` : null}
    <div class="monitor-duration">${[['1h','1 小时'],['6h','6 小时'],['24h','24 小时'],['168h','7 天']].map(([value,label]) => html`<button class=${`button ${duration === value ? '' : 'button-secondary'}`} onClick=${() => chooseDuration(value)}>${label}</button>`)}</div>
    <div class="monitor-charts">${charts.map(chart => html`<${Chart} key=${chart.key} label=${chart.label} values=${history} keyName=${chart.key} fixedMax=${chart.max} suffix=${chart.suffix}/>`)}<${Chart} label="网络接收累计" values=${history} keyName="net_bytes_recv" suffix=" B"/></div>`;
}
