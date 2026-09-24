import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function FileOperation({ api, params, search, navigate }) {
  const operation = ['create_file', 'create_folder', 'upload'].includes(params.operation) ? params.operation : '';
  const path = search?.get('path') || '';
  const showHidden = search?.get('show_hidden') === 'true';
  const [listing, setListing] = useState(null), [name, setName] = useState(''), [content, setContent] = useState(''), [file, setFile] = useState(null), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const back = `/files?path=${encodeURIComponent(path)}&show_hidden=${showHidden}`;
  useEffect(() => { const controller = new AbortController(); api.list(path, showHidden, { signal: controller.signal }).then(setListing).catch(e => { if (e.name !== 'AbortError') setError(e.message || '目录读取失败'); }); return () => controller.abort(); }, [api, path, showHidden]);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { if (!operation) throw new Error('文件操作类型无效'); if (listing?.is_file) throw new Error('不能在文件路径下创建内容'); if (operation === 'upload') { if (!file) throw new Error('请选择一个文件'); await api.upload(listing?.current_path || path, file); } else await api.create(operation, listing?.current_path || path, name, content); navigate(back); } catch (e) { setError(e.message || '文件操作失败'); } finally { setBusy(false); } };
  const title = operation === 'upload' ? '上传文件' : operation === 'create_folder' ? '新建目录' : '新建文件';
  return html`<section class="card"><div class="page-toolbar"><h2>${title}</h2><button class="button" onClick=${() => navigate(back)}>返回目录</button></div><p class="mono">${listing?.current_path || path || '读取目录中…'}</p>${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}<form class="files-create-form" onSubmit=${submit}>${operation === 'upload' ? html`<label>文件<input class="input" type="file" onChange=${e => setFile(e.currentTarget.files?.[0] || null)} required /></label>` : html`<label>名称<input class="input" aria-label="名称" placeholder="名称" value=${name} onInput=${e => setName(e.currentTarget.value)} required /></label>`}${operation === 'create_file' ? html`<label>内容<textarea class="input" aria-label="文件内容" rows="8" value=${content} onInput=${e => setContent(e.currentTarget.value)}></textarea></label>` : null}<div class="form-actions"><button class="button button-primary" disabled=${busy || !listing || listing.is_file}>${busy ? '处理中…' : '确认'}</button><button class="button" type="button" onClick=${() => navigate(back)}>取消</button></div></form></section>`;
}
