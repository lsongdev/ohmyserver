import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, KV, CopyButton, confirmDanger, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, BuildStatus, EVENTS, active, datetime, short, duration } from '../ci.js';
import { RepoForm } from './repos.js';

const EXAMPLE = `when:
  branch: [main]
steps:
  test:
    image: golang:1.26
    commands:
      - go vet ./...
      - go test ./...
    cache: [/root/go/pkg/mod, /root/.cache/go-build]
  image:
    build:
      tag: myapp:latest`;

export default function Repo({ params }) {
  const { api, client, href, navigate } = usePanel();
  const repo = useApi(`/repos/${params.id}`);
  const builds = useApi(`/repos/${params.id}/builds`, list => (list?.some(b => active(b.status)) ? 2000 : 10000));
  const action = useAction();
  const [editing, setEditing] = useState(false);
  const [branch, setBranch] = useState('');

  if (!repo.data) return html`<${Page} title="仓库">${repo.error ? html`<${Notice} error=${repo.error} />` : html`<${Loading} />`}<//>`;
  const r = repo.data;
  const hook = client.url(`hooks/${r.id}`).href;
  const run = () => action.run(async () => {
    const b = await api.post(`/repos/${r.id}/builds`, { branch: branch.trim() });
    navigate(`/builds/${b.id}`);
  });
  const rotate = () => {
    if (!confirmDanger('换一个新的 Webhook 密钥？旧密钥会立即失效，需要到代码托管平台更新。')) return;
    action.run(async () => { await api.post(`/repos/${r.id}/webhook-secret`); repo.reload(); toast('Webhook 密钥已更换', 'success'); });
  };
  const remove = () => {
    if (!confirmDanger(`删除仓库 ${r.name} 及其全部构建记录？`)) return;
    action.run(async () => { await api.del(`/repos/${r.id}`); navigate('/'); });
  };

  return html`<${Page} title=${r.name} subtitle=${html`<a href=${href('/')}>← 仓库</a> · <span class="mono">${r.url}</span>`}
    actions=${html`
      <input class="input mono" style="width:160px" placeholder=${r.branch} value=${branch} onInput=${e => setBranch(e.currentTarget.value)} aria-label="分支" />
      <button class="btn btn-primary" disabled=${action.busy} onClick=${run}>运行</button>
      <button class="btn" onClick=${() => setEditing(true)}>编辑</button>
      <button class="btn btn-danger" onClick=${remove}>删除</button>`}>
    <${Notice} error=${action.error || builds.error} />
    <section>
      ${builds.data === undefined ? html`<${Loading} />` : !builds.data.length ? html`<${Empty}>还没有构建。点击「运行」，或配置下面的 Webhook 后推送代码。<//>` : html`
        <div class="table-wrap"><table class="table">
          <thead><tr><th>构建</th><th>状态</th><th>分支 · 提交</th><th>触发</th><th>时间</th><th class="num">耗时</th></tr></thead>
          <tbody>${builds.data.map(b => html`<tr key=${b.id}>
            <td><a class="cell-main" href=${href(`/builds/${b.id}`)}>#${b.number}</a></td>
            <td><${BuildStatus} status=${b.status} title=${b.error} /></td>
            <td><span class="mono">${b.branch}</span> ${b.commit ? html`<span class="commit muted">${short(b.commit)}</span>` : null}
              <div class="cell-sub message" title=${b.message}>${b.message}</div></td>
            <td>${EVENTS[b.event] || b.event}${b.author ? html`<div class="cell-sub">${b.author}</div>` : null}</td>
            <td class="nowrap muted">${datetime(b.created_at)}</td>
            <td class="num">${duration(b.started_at, b.finished_at)}</td>
          </tr>`)}</tbody>
        </table></div>`}
    </section>
    <div class="grid-2 section">
      <div class="card">
        <h2>Webhook</h2>
        <p class="muted" style="margin-top:0">在 GitHub、Gitea 或 GitLab 的仓库设置里添加 Webhook，选择 push 事件，内容类型 application/json。</p>
        <${KV} items=${[
          ['地址', html`<span class="copy-row"><code>${hook}</code><${CopyButton} text=${hook} /></span>`],
          ['密钥', html`<span class="copy-row"><code>${r.webhook_secret}</code><${CopyButton} text=${r.webhook_secret} />
            <button class="btn btn-sm" onClick=${rotate}>更换</button></span>`],
        ]} />
        <p class="field-hint" style="margin-bottom:0">代码托管平台需要能访问这个地址；面板在内网时，可以通过网关模块暴露到公网。</p>
      </div>
      <div class="card">
        <h2>${r.config_path} 示例</h2>
        <pre class="example">${EXAMPLE}</pre>
      </div>
    </div>
    ${editing ? html`<${RepoForm} repo=${r} onClose=${() => setEditing(false)} onSaved=${() => { setEditing(false); repo.reload(); }} />` : null}
  <//>`;
}
