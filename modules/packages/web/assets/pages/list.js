import { useEffect, useState } from 'preact/hooks';
import { html } from '@module/ui';

export default function Packages({ api, view = 'installed' }) {
  const [catalog, setCatalog] = useState(null), [query, setQuery] = useState(''), [error, setError] = useState(''), [busy, setBusy] = useState(false), [message, setMessage] = useState('');
  const load = async (search = query) => { setError(''); try { setCatalog(await api.list(search ? 'search' : view, search)); } catch (e) { setError(e.message || '软件包读取失败'); } };
  useEffect(() => { setQuery(''); setMessage(''); load(''); }, [api, view]);
  const run = async (operation, name = '') => { setBusy(true); setError(''); setMessage(''); try { const result = await operation(name); setMessage(result?.skipped ? '所有软件包均已是最新版本。' : `${operation === api.refresh ? '软件源索引已刷新' : '操作已完成'}${result?.duration ? `（${result.duration}）` : ''}`); await load(); } catch (e) { setError(e.message || '软件包操作失败'); } finally { setBusy(false); } };
  const search = async event => { event.preventDefault(); await load(query.trim()); };
  const items = catalog?.packages || [];
  const caps = catalog?.capabilities || {};
  return html`<div class="packages-page">
    <section><div class="page-toolbar"><div><h2>${view === 'upgradable' ? '可升级软件包' : '软件包'}</h2>${catalog && html`<p class="muted">${catalog.manager} (${catalog.backend}) · ${catalog.executable} · ${catalog.mutation_mode}</p>`}</div><div class="button-group"><button class="button" disabled=${busy || !caps.refresh} onClick=${() => run(api.refresh)}>刷新软件源</button>${view === 'upgradable' ? html`<button class="button button-primary" disabled=${busy || !caps.upgrade || !caps.safe_upgrade_all} onClick=${() => run(api.upgrade)}>全部升级</button>` : null}</div></div>
      <form class="inline-form" onSubmit=${search}><input class="input" aria-label="搜索软件包" placeholder="搜索软件包名称" value=${query} onInput=${e => setQuery(e.currentTarget.value)}/><button class="button button-primary" disabled=${busy || !caps.search}>搜索</button>${query && html`<button class="button" type="button" onClick=${()=>{setQuery('');load('')}}>清除</button>`}</form>
      ${message && html`<div class="notice-card" role="status">${message}</div>`}${error && html`<div class="notice-card error" role="alert">${error}</div>`}
      ${catalog ? html`<div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>版本</th>${view==='upgradable'||query?html`<th>可用版本</th>`:null}<th>架构</th><th>仓库</th><th>描述</th><th>操作</th></tr></thead><tbody>${items.map(item=>html`<tr key=${item.name}><td><strong>${item.name}</strong></td><td class="mono">${item.version||'—'}</td>${view==='upgradable'||query?html`<td class="mono">${item.latest_version||'—'}</td>`:null}<td>${item.architecture||'—'}</td><td>${item.repository||'—'}</td><td>${item.description||'—'}</td><td><div class="button-group">${view==='upgradable'&&caps.upgrade?html`<button class="button button-primary" disabled=${busy} onClick=${()=>run(api.upgrade,item.name)}>升级</button>`:null}${view==='installed'&&caps.remove?html`<button class="button button-danger" disabled=${busy} onClick=${()=>confirm(`移除软件包 ${item.name}？`)&&run(api.remove,item.name)}>移除</button>`:null}${view==='search'&&!item.installed&&caps.install?html`<button class="button button-primary" disabled=${busy} onClick=${()=>run(api.install,item.name)}>安装</button>`:null}</div></td></tr>`)}</tbody></table>${!items.length&&!error?html`<div class="empty-state">${query?'没有匹配的软件包':'没有软件包数据'}</div>`:null}</div>`:null}
    </section>
  </div>`;
}
