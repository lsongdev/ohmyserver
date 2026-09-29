import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function Settings({ api, navigate }) {
  const [items, setItems] = useState([]);
  const [drafts, setDrafts] = useState({});
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = () => api.list().then(setItems).catch(err => setError(err.message || '设置读取失败'));
  useEffect(() => { load(); }, [api]);
  const save = async (item) => {
    setBusy(true); setError('');
    try { await api.update(item.id, drafts[item.id] ?? item.value); await load(); }
    catch (err) { setError(err.message || '保存失败'); }
    finally { setBusy(false); }
  };
  const remove = async (item) => {
    if (!confirm(`删除设置 ${item.name}？`)) return;
    setBusy(true); setError('');
    try { await api.delete(item.id); await load(); }
    catch (err) { setError(err.message || '删除失败'); }
    finally { setBusy(false); }
  };
  return html`<section>
    <div class="page-toolbar"><h2>设置</h2><div class="button-group"><button class="button button-secondary" disabled=${busy} onClick=${load}>刷新</button><button class="button button-primary" onClick=${() => navigate('/new')}>新增设置</button></div></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>类型</th><th>值</th><th>说明</th><th>操作</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id}><td class="mono">${item.name}</td><td>${item.type}</td>
        <td><input class="input" aria-label=${`设置 ${item.name}`} value=${drafts[item.id] ?? item.value} onInput=${event => setDrafts({...drafts,[item.id]:event.currentTarget.value})}/></td>
        <td>${item.description}</td><td><div class="button-group"><button class="button button-primary" disabled=${busy} onClick=${() => save(item)}>保存</button><button class="button button-danger" disabled=${busy} onClick=${() => remove(item)}>删除</button></div></td></tr>`)}</tbody>
    </table>${!items.length && !error ? html`<div class="empty-state">暂无设置</div>` : null}</div>
  </section>`;
}
