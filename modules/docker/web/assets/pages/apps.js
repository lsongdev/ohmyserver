import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Apps({ api, navigate, href }) {
  const [items, setItems] = useState([]); const [categories, setCategories] = useState([]); const [filter, setFilter] = useState(''); const [category, setCategory] = useState(''); const [query, setQuery] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false); const [reload, setReload] = useState(0);
  useEffect(() => { const controller = new AbortController(); Promise.all([api.listApps({ filter, category, signal: controller.signal }), api.appCategories({ signal: controller.signal })]).then(([apps, groups]) => { setItems(apps || []); setCategories(groups || []); setError(''); }).catch(err => { if (err.name !== 'AbortError') setError(err.message || '应用目录读取失败'); }); return () => controller.abort(); }, [api, filter, category, reload]);
  const sync = async () => { setBusy(true); setError(''); try { await api.syncApps(); setReload(value => value + 1); } catch (err) { setError(err.message || '同步应用目录失败'); } finally { setBusy(false); } };
  const visible = items.filter(item => `${item.name} ${item.id} ${item.short_desc || ''}`.toLowerCase().includes(query.toLowerCase()));
  return html`<div><div class="page-toolbar"><h2>应用商店</h2><button class="button" disabled=${busy} onclick=${sync}>${busy ? '同步中…' : '同步目录'}</button></div>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="filter-bar"><div class="button-group">${[['','全部'],['running','运行中'],['installed','已安装']].map(([key,label]) => html`<button class=${`button ${filter === key ? 'button-primary' : ''}`} onclick=${() => setFilter(key)}>${label}</button>`)}</div><select class="input" value=${category} onChange=${e => setCategory(e.currentTarget.value)}><option value="">所有类别</option>${categories.map(value => html`<option value=${value}>${value}</option>`)}</select><input class="input" value=${query} onInput=${e => setQuery(e.currentTarget.value)} placeholder="搜索应用" /></div>
    <div class="app-grid">${visible.map(item => html`<article class="card app-card" key=${item.id}><div class="app-heading"><img class="app-icon" src=${api.appIconURL(item.id)} alt="" /><div><h3><a href=${href(`/apps/${encodeURIComponent(item.id)}`)}>${item.name}</a></h3><span class=${`status-pill ${item.status.state === 'running' ? 'running' : ''}`}>${item.status.state}</span></div></div><p>${item.short_desc || item.description || ''}</p><div class="button-group">${item.categories?.map(value => html`<span class="status-pill">${value}</span>`)}</div><button class="button button-primary" onclick=${() => navigate(`/apps/${encodeURIComponent(item.id)}`)}>详情</button></article>`)}</div>
    ${!visible.length && !error ? html`<div class="empty-state">没有匹配的应用</div>` : null}
  </div>`;
}
