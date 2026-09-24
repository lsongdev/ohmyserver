import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';

export default function BuildImage({ api, navigate }) {
  const [name, setName] = useState('');
  const [sourceType, setSourceType] = useState('dockerfile');
  const [dockerfile, setDockerfile] = useState('FROM alpine:latest\nCMD ["sh"]');
  const [buildDir, setBuildDir] = useState('');
  const [repository, setRepository] = useState('');
  const [branch, setBranch] = useState('');
  const [logs, setLogs] = useState([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async event => {
    event.preventDefault(); setError(''); setLogs([]); setBusy(true);
    try { await api.buildImage({ name, source_type: sourceType, dockerfile, build_dir: buildDir, repository, branch }, { onLog: line => setLogs(items => [...items.slice(-199), line]) }); }
    catch (err) { setError(err.message || '镜像构建失败'); }
    finally { setBusy(false); }
  };
  const field = (label, value, setValue, props = {}) => html`<div class="form-group"><label>${label}</label><input class="input" value=${value} onInput=${e => setValue(e.currentTarget.value)} ...${props} /></div>`;
  return html`<div><div class="page-toolbar"><h2>构建镜像</h2><button class="button" onclick=${() => navigate('/docker/images')}>返回镜像</button></div>${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}<form class="section" onSubmit=${submit}>
    ${field('镜像名称', name, setName, { required: true, placeholder: 'myapp:latest' })}
    <div class="form-group"><label for="build-source">构建来源</label><select class="input" id="build-source" value=${sourceType} onChange=${e => setSourceType(e.currentTarget.value)}><option value="dockerfile">Dockerfile 内容</option><option value="directory">本地目录</option><option value="repository">Git 仓库</option></select></div>
    ${sourceType === 'dockerfile' ? html`<div class="form-group"><label for="dockerfile">Dockerfile</label><textarea class="input mono" id="dockerfile" rows="12" value=${dockerfile} onInput=${e => setDockerfile(e.currentTarget.value)} required></textarea></div>` : null}
    ${sourceType === 'directory' ? field('构建目录（服务器本地绝对路径）', buildDir, setBuildDir, { required: true, placeholder: '/path/to/context' }) : null}
    ${sourceType === 'repository' ? html`${field('Git 仓库 URL', repository, setRepository, { required: true, placeholder: 'https://example.com/project.git' })}${field('分支（可选）', branch, setBranch, { placeholder: 'main' })}` : null}
    <div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy}>${busy ? '构建中…' : '开始构建'}</button><button class="button" type="button" onclick=${() => navigate('/docker/images')}>取消</button></div>
  </form>${logs.length ? html`<section class="section"><h3>构建日志</h3><pre class="operation-log" aria-live="polite">${logs.join('\n')}</pre></section>` : null}</div>`;
}
