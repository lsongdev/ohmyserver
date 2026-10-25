import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, Modal, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, BuildStatus, active, datetime, short, duration } from '../ci.js';

export default function Repos() {
  const { api, href, navigate } = usePanel();
  const repos = useApi('/repos', list => (list?.some(r => r.last_build && active(r.last_build.status)) ? 2000 : 10000));
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const run = r => action.run(async () => {
    const b = await api.post(`/repos/${r.id}/builds`);
    navigate(`/builds/${b.id}`);
  });
  return html`<${Page} title="仓库" subtitle="推送代码或手动运行，按仓库里的 .ci.yml 在容器中构建和测试。"
    actions=${html`<button class="btn btn-primary" onClick=${() => setAdding(true)}>添加仓库</button>`}>
    <${Notice} error=${repos.error || action.error} />
    ${repos.data === undefined ? (repos.error ? null : html`<${Loading} />`) : !repos.data.length ? html`<${Empty}>还没有仓库。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>仓库</th><th>最近构建</th><th>提交</th><th class="num">耗时</th><th class="actions"></th></tr></thead>
        <tbody>${repos.data.map(r => {
          const b = r.last_build;
          return html`<tr key=${r.id}>
            <td><a class="cell-main" href=${href(`/repos/${r.id}`)}>${r.name}</a><div class="cell-sub mono">${r.url}</div></td>
            <td>${b ? html`<a href=${href(`/builds/${b.id}`)}><${BuildStatus} status=${b.status} title=${b.error} /></a>
              <div class="cell-sub">#${b.number} · ${b.branch} · ${datetime(b.created_at)}</div>` : html`<span class="muted">还没有构建</span>`}</td>
            <td>${b?.commit ? html`<span class="commit">${short(b.commit)}</span><div class="cell-sub message" title=${b.message}>${b.message}</div>` : '—'}</td>
            <td class="num">${b ? duration(b.started_at, b.finished_at) : '—'}</td>
            <td class="actions"><span class="btn-group">
              <button class="btn btn-sm btn-primary" disabled=${action.busy} onClick=${() => run(r)}>运行</button>
              <a class="btn btn-sm" href=${href(`/repos/${r.id}`)}>详情</a>
            </span></td>
          </tr>`;
        })}</tbody>
      </table></div>`}
    ${adding ? html`<${RepoForm} onClose=${() => setAdding(false)} onSaved=${r => { setAdding(false); navigate(`/repos/${r.id}`); }} />` : null}
  <//>`;
}

// RepoForm adds or edits a repository. Secrets are edited as rows; a
// stored secret shows no value, and leaving it empty keeps it.
export function RepoForm({ repo, onClose, onSaved }) {
  const { api } = usePanel();
  const isNew = !repo;
  const [r, setR] = useState({
    name: repo?.name || '', url: repo?.url || '', branch: repo?.branch || 'main',
    config_path: repo?.config_path || '.ci.yml', timeout: repo?.timeout || 60, token: '',
  });
  const [secrets, setSecrets] = useState(() => (repo?.secret_names || []).map(name => ({ name, value: '', stored: true })));
  const { busy, error, run } = useAction();
  const set = patch => setR(prev => ({ ...prev, ...patch }));
  const setSecret = (i, patch) => setSecrets(list => list.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const submit = e => {
    e.preventDefault();
    run(async () => {
      const body = { ...r, timeout: Number(r.timeout) || 60, secrets: Object.fromEntries(secrets.filter(s => s.name.trim()).map(s => [s.name.trim(), s.value])) };
      const saved = isNew ? await api.post('/repos', body) : await api.put(`/repos/${repo.id}`, body);
      toast(isNew ? '仓库已添加' : '仓库已保存', 'success');
      onSaved(saved);
    });
  };
  return html`<${Modal} wide title=${isNew ? '添加仓库' : `编辑 ${repo.name}`} onClose=${onClose} footer=${html`
    <span class="spacer"></span>
    <button class="btn" type="button" onClick=${onClose}>取消</button>
    <button class="btn btn-primary" type="submit" form="repo-form" disabled=${busy}>${isNew ? '添加' : '保存'}</button>`}>
    <form id="repo-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <div class="form-grid">
        <${Field} label="名称" required>
          <input class="input" required value=${r.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="ohmyserver" autofocus />
        <//>
        <${Field} label="默认分支" hint="手动运行时构建这个分支">
          <input class="input mono" value=${r.branch} onInput=${e => set({ branch: e.currentTarget.value })} />
        <//>
      </div>
      <${Field} label="Git 地址" required hint="HTTPS 地址；私有仓库填写下面的访问令牌">
        <input class="input mono" required value=${r.url} onInput=${e => set({ url: e.currentTarget.value })} placeholder="https://github.com/you/app.git" />
      <//>
      <div class="form-grid">
        <${Field} label="访问令牌" hint=${!isNew && repo.has_token ? '已保存；留空则保留' : 'GitHub、Gitea 的个人访问令牌，公开仓库留空'}>
          <input class="input" type="password" autocomplete="new-password" value=${r.token} onInput=${e => set({ token: e.currentTarget.value })} />
        <//>
        <${Field} label="配置文件">
          <input class="input mono" value=${r.config_path} onInput=${e => set({ config_path: e.currentTarget.value })} />
        <//>
        <${Field} label="超时（分钟）">
          <input class="input" type="number" min="1" max="1440" value=${r.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
        <//>
      </div>
      <fieldset class="field" style="border:0;padding:0;margin:0;display:grid;gap:8px">
        <span class="field-label">密钥</span>
        <span class="field-hint">步骤在 secrets 里列出名字才能拿到，日志里会打码。已保存的密钥不显示值，留空则保留。</span>
        ${secrets.map((s, i) => html`<div class="secret-row" key=${i}>
          <input class="input mono" placeholder="NAME" value=${s.name} disabled=${s.stored} onInput=${e => setSecret(i, { name: e.currentTarget.value })} />
          <input class="input" type="password" autocomplete="new-password" placeholder=${s.stored ? '已保存' : '值'} value=${s.value} onInput=${e => setSecret(i, { value: e.currentTarget.value })} />
          <button type="button" class="btn btn-ghost btn-icon" aria-label="删除" onClick=${() => setSecrets(list => list.filter((_, j) => j !== i))}>✕</button>
        </div>`)}
        <div><button type="button" class="btn btn-sm" onClick=${() => setSecrets(list => [...list, { name: '', value: '' }])}>添加密钥</button></div>
      </fieldset>
    </form>
  <//>`;
}
