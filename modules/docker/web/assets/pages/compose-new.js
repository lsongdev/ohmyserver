import { html } from '@module/ui';
import { useState } from 'preact/hooks';

export default function ComposeNew({ api, navigate }) {
  const [name, setName] = useState(''); const [source, setSource] = useState('file'); const [compose, setCompose] = useState(''); const [directory, setDirectory] = useState(''); const [repository, setRepository] = useState(''); const [branch, setBranch] = useState(''); const [error, setError] = useState(''); const [busy, setBusy] = useState(false);
  const submit = async event => { event.preventDefault(); setBusy(true); setError(''); try { const project = await api.createComposeProject({ name, source, compose, directory, repository, branch }); navigate(`/compose/${encodeURIComponent(project.name)}`); } catch (err) { setError(err.message || 'Compose 项目创建失败'); } finally { setBusy(false); } };
  return html`<div><button class="button button-link" onclick=${() => navigate('/compose')}>← Compose 项目</button><h2>新建 Compose 项目</h2>${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <form class="section" onSubmit=${submit}><div class="form-group"><label for="compose-name">项目名称</label><input class="input" id="compose-name" value=${name} onInput=${e => setName(e.currentTarget.value)} required /></div>
      <div class="form-group"><label for="compose-source">配置来源</label><select class="input" id="compose-source" value=${source} onChange=${e => setSource(e.currentTarget.value)}><option value="file">Compose YAML</option><option value="directory">服务器目录</option><option value="repository">Git 仓库</option></select></div>
      ${source === 'file' ? html`<div class="form-group"><label for="compose-yaml">Compose YAML</label><textarea class="input" id="compose-yaml" rows="14" value=${compose} onInput=${e => setCompose(e.currentTarget.value)} placeholder="services:\n  web:\n    image: nginx:latest\n    ports:\n      - '8080:80'" required></textarea></div>` : null}
      ${source === 'directory' ? html`<div class="form-group"><label for="compose-dir">绝对目录路径</label><input class="input" id="compose-dir" value=${directory} onInput=${e => setDirectory(e.currentTarget.value)} required /></div>` : null}
      ${source === 'repository' ? html`<div class="form-group"><label for="compose-repo">仓库 URL</label><input class="input" id="compose-repo" value=${repository} onInput=${e => setRepository(e.currentTarget.value)} required /></div><div class="form-group"><label for="compose-branch">分支（可选）</label><input class="input" id="compose-branch" value=${branch} onInput=${e => setBranch(e.currentTarget.value)} /></div>` : null}
      <div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy}>${busy ? '正在创建并启动…' : '创建并启动'}</button><button class="button" type="button" onclick=${() => navigate('/compose')}>取消</button></div>
    </form></div>`;
}
