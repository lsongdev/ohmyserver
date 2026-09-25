import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

const blank = () => ({ name: '', type: 'http', target: '', port: 443, interval: 60, timeout: 10, expected_status: 200, enabled: true });

export default function Monitors({ api, navigate }) {
  const [items, setItems] = useState([]), [checks, setChecks] = useState({});
  const [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const load = async (signal) => { const data = await api.list({ signal }); setItems(data); };
  useEffect(() => { const controller = new AbortController(); load(controller.signal).catch(e => { if (e.name !== 'AbortError') setError(e.message); }); return () => controller.abort(); }, [api]);
  const act = async (fn) => { setBusy(true); setError(''); try { await fn(); await load(); } catch (e) { setError(e.message || '操作失败'); } finally { setBusy(false); } };
  return html`<section>
    <div class="page-toolbar"><h2>在线监测</h2><div class="button-group"><button class="button button-primary" onClick=${() => navigate('/new')}>添加目标</button><button class="button" disabled=${busy} onClick=${() => act(() => load())}>刷新</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>类型</th><th>目标</th><th>状态</th><th>24 小时在线率</th><th>最近检查</th><th>操作</th></tr></thead>
    <tbody>${items.map(item => html`<tr key=${item.id}><td>${item.name}</td><td>${item.type}</td><td class="mono">${item.target}${item.type === 'tcp' ? `:${item.port}` : ''}</td><td>${item.last_check?.status || '待检查'}</td><td>${Number(item.uptime_pct).toFixed(2)}%</td><td>${item.last_check?.checked_at || '—'}</td><td class="table-actions"><button class="button" disabled=${busy} onClick=${() => act(() => api.action(item.id, 'check'))}>立即检查</button><button class="button" disabled=${busy} onClick=${() => act(() => api.action(item.id, 'toggle'))}>${item.enabled ? '停用' : '启用'}</button><button class="button" disabled=${busy} onClick=${async () => { try { const rows = await api.checks(item.id); setChecks({ ...checks, [item.id]: rows }); } catch (e) { setError(e.message); } }}>历史</button><button class="button button-danger" disabled=${busy} onClick=${() => act(() => api.remove(item.id))}>删除</button></td></tr>${checks[item.id]?.length ? html`<tr><td colspan="7"><div class="uptime-history">${checks[item.id].map(check => html`<span>${check.checked_at} · ${check.status} · ${check.response_time} ms</span>`)}</div></td></tr>` : null}`)}</tbody>
    </table>${!items.length ? html`<div class="empty-state">暂无监测目标</div>` : null}</div>
  </section>`;
}
