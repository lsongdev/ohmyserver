import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '';
const request = async (path, options = {}) => {
  const headers = new Headers(options.headers || {});
  let body = options.body;
  if (body !== undefined && !(body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
    body = JSON.stringify(body);
  }
  const response = await fetch(`${apiBase}${path}`, { ...options, headers, body });
  if (response.status === 204 || response.status === 201) return null;
  const envelope = await response.json();
  if (!response.ok) throw new Error(envelope.error?.message || `HTTP ${response.status}`);
  return envelope.data;
};
const api = createAPI({ request, apiURL: path => `${apiBase}${path}` });

function App() {
  const [path, setPath] = useState('');
  const [showHidden, setShowHidden] = useState(false);
  const [listing, setListing] = useState(null);
  const [operation, setOperation] = useState('');
  const [name, setName] = useState('');
  const [content, setContent] = useState('');
  const [file, setFile] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.list(path, showHidden, { signal: controller.signal }).then(result => {
      setListing(result);
      setPath(result.current_path);
      setError('');
    }).catch(reason => {
      if (reason.name !== 'AbortError') setError(reason.message || '目录读取失败');
    });
    return () => controller.abort();
  }, [path, showHidden, refresh]);

  const submit = async event => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (operation === 'upload') {
        if (!file) throw new Error('请选择要上传的文件');
        await api.upload(listing.current_path, file);
      } else {
        await api.create(operation, listing.current_path, name, content);
      }
      setOperation('');
      setName('');
      setContent('');
      setFile(null);
      setRefresh(value => value + 1);
    } catch (reason) {
      setError(reason.message || '文件操作失败');
    } finally {
      setBusy(false);
    }
  };
  const remove = async entry => {
    if (!confirm(`确定删除 ${entry.name}${entry.is_dir ? ' 及其中全部内容' : ''}？此操作不可撤销。`)) return;
    setBusy(true);
    setError('');
    try {
      await api.remove(entry.path);
      setRefresh(value => value + 1);
    } catch (reason) {
      setError(reason.message || '删除失败');
    } finally {
      setBusy(false);
    }
  };
  const changeDirectory = nextPath => { setPath(nextPath); setOperation(''); };
  const toggleHidden = event => { setShowHidden(event.currentTarget.checked); setRefresh(value => value + 1); };

  return html`<section class="card">
    <div class="files-standalone-toolbar">
      <div><h2>文件浏览器</h2><div class="files-standalone-path">${listing?.current_path || '读取中…'}</div></div>
      <div class="files-standalone-actions">
        ${listing?.parent ? html`<button class="button" disabled=${busy} onclick=${() => changeDirectory(listing.parent)}>上级目录</button>` : null}
        <button class="button" disabled=${busy} onclick=${() => setRefresh(value => value + 1)}>刷新</button>
        <label><input class="input" type="checkbox" checked=${showHidden} onChange=${toggleHidden} /> 显示隐藏文件</label>
        <button class="button button-primary" disabled=${busy || listing?.is_file} onclick=${() => setOperation(operation === 'create_file' ? '' : 'create_file')}>新建文件</button>
        <button class="button" disabled=${busy || listing?.is_file} onclick=${() => setOperation(operation === 'create_folder' ? '' : 'create_folder')}>新建目录</button>
        <button class="button" disabled=${busy || listing?.is_file} onclick=${() => setOperation(operation === 'upload' ? '' : 'upload')}>上传文件</button>
      </div>
    </div>
    ${error ? html`<p class="files-standalone-error" role="alert">${error}</p>` : null}
    ${operation ? html`<form class="files-standalone-form" onSubmit=${submit}>
      <h3>${operation === 'upload' ? '上传文件' : operation === 'create_folder' ? '新建目录' : '新建文件'}</h3>
      ${operation === 'upload'
        ? html`<label>文件<input class="input" type="file" required onChange=${event => setFile(event.currentTarget.files?.[0] || null)} /></label>`
        : html`<label>名称<input class="input" required value=${name} onInput=${event => setName(event.currentTarget.value)} /></label>`}
      ${operation === 'create_file' ? html`<label>内容<textarea class="input" rows="8" value=${content} onInput=${event => setContent(event.currentTarget.value)}></textarea></label>` : null}
      <div class="files-standalone-actions"><button class="button button-primary" disabled=${busy}>确认</button><button class="button" type="button" onclick=${() => setOperation('')}>取消</button></div>
    </form>` : null}
    ${listing?.is_file
      ? html`<p><a class="button button-primary" href=${api.download(listing.current_path)}>下载 ${listing.file?.name}</a></p>`
      : listing ? html`<div class="table-container"><table class="table"><thead><tr><th>名称</th><th>类型</th><th>大小</th><th>修改时间</th><th>权限</th><th>操作</th></tr></thead><tbody>
        ${listing.entries.map(entry => html`<tr key=${entry.path}><td>${entry.is_dir
          ? html`<button class="button button-link" onclick=${() => changeDirectory(entry.path)}>${entry.name}/</button>`
          : html`<a href=${api.download(entry.path)}>${entry.name}</a>`}</td><td>${entry.is_dir ? '目录' : '文件'}</td><td>${entry.is_dir ? '—' : entry.size.toLocaleString()}</td><td>${new Date(entry.mod_time).toLocaleString()}</td><td>${entry.mode}</td><td><button class="button button-danger" disabled=${busy} onclick=${() => remove(entry)}>删除</button></td></tr>`)}
      </tbody></table>${listing.entries.length ? null : html`<div class="files-standalone-empty">此目录为空。</div>`}</div>` : null}
  </section>`;
}

render(html`<${App} />`, document.getElementById('app'));
