import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

const loaders = {
  images: api => api.listImages(),
  volumes: api => api.listVolumes(),
  networks: api => api.listNetworks(),
};
const removers = {
  images: (api, item) => api.deleteImage(item.id),
  volumes: (api, item) => api.deleteVolume(item.name),
  networks: (api, item) => api.deleteNetwork(item.id),
};

export default function DockerResources({ api, kind, title, navigate }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState('');
  const [reload, setReload] = useState(0);
  const [imageName, setImageName] = useState('');
  const [pulling, setPulling] = useState(false);
  const [pullLog, setPullLog] = useState([]);
  useEffect(() => {
    let active = true;
    loaders[kind](api).then(data => { if (active) setItems(data || []); })
      .catch(err => { if (active) setError(err.message || `${title}读取失败`); });
    return () => { active = false; };
  }, [api, kind, reload]);
  async function remove(item) {
    const label = item.name || item.repo_tags?.[0] || item.id;
    if (!confirm(`确定删除 ${label} 吗？`)) return;
    setError('');
    try { await removers[kind](api, item); setReload(value => value + 1); }
    catch (err) { setError(err.message || '删除失败'); }
  }
  async function pull(event) {
    event.preventDefault();
    if (!imageName.trim()) return;
    setPulling(true);
    setPullLog([]);
    setError('');
    try {
      await api.pullImage(imageName.trim(), { onProgress: message => setPullLog(items => [...items.slice(-39), message]) });
      setImageName('');
      setReload(value => value + 1);
    } catch (err) { setError(err.message || '镜像拉取失败'); }
    finally { setPulling(false); }
  }
  const columns = kind === 'images'
    ? ['镜像', '大小', '创建时间']
    : kind === 'volumes' ? ['名称', '驱动', '范围'] : ['名称', '驱动', '范围'];
  return html`<div>
    <div class="page-toolbar"><h2>${title}</h2><div class="toolbar-actions">${kind === 'images' ? html`<button class="button button-secondary" onclick=${() => navigate('/images/build')}>构建镜像</button>` : null}${kind !== 'images' ? html`<button class="button button-secondary" onclick=${() => navigate(`/${kind}/new`)}>创建${title}</button>` : null}<button class="button" onclick=${() => setReload(value => value + 1)}>刷新</button></div></div>
    ${kind === 'images' ? html`<form class="inline-form" onSubmit=${pull}><input class="input" aria-label="镜像名称" placeholder="nginx:latest" value=${imageName} onInput=${event => setImageName(event.currentTarget.value)} /><button class="button" type="submit" disabled=${pulling || !imageName.trim()}>${pulling ? '正在拉取…' : '拉取镜像'}</button></form>${pullLog.length ? html`<pre class="operation-log" aria-live="polite">${pullLog.join('\n')}</pre>` : null}` : null}
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <div class="table-wrap"><table class="table"><thead><tr>${columns.map(column => html`<th>${column}</th>`)}<th>操作</th></tr></thead>
      <tbody>${items.map(item => html`<tr key=${item.id || item.name}>
        ${kind === 'images' ? html`<td>${(item.repo_tags || []).join(', ') || item.id}<div class="muted mono">${item.id}</div></td><td>${(item.size / 1024 / 1024).toFixed(1)} MB</td><td>${item.created ? new Date(item.created * 1000).toLocaleString() : '—'}</td>` : null}
        ${kind === 'volumes' ? html`<td>${item.name}</td><td>${item.driver}</td><td>${item.scope}</td>` : null}
        ${kind === 'networks' ? html`<td>${item.name}<div class="muted mono">${item.id}</div></td><td>${item.driver}</td><td>${item.scope}${item.internal ? ' · 内部' : ''}</td>` : null}
        <td><button class="button button-link button-danger" onclick=${() => remove(item)}>删除</button></td></tr>`)}</tbody>
      </table>${!items.length && !error ? html`<div class="empty-state">没有${title}</div>` : null}</div>
  </div>`;
}
