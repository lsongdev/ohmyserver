import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, KV, CopyButton, confirmDanger, toast, Table } from '../lib/ui.js';
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

  if (!repo.data) return html`<${Page} title="Repository">${repo.error ? html`<${Notice} error=${repo.error} />` : html`<${Loading} />`}<//>`;
  const r = repo.data;
  const hook = client.url(`hooks/${r.id}`).href;
  const run = () => action.run(async () => {
    const b = await api.post(`/repos/${r.id}/builds`, { branch: branch.trim() });
    navigate(`/builds/${b.id}`);
  });
  const rotate = () => {
    if (!confirmDanger('Replace the webhook secret? The old one stops working at once; update it on the code host.')) return;
    action.run(async () => { await api.post(`/repos/${r.id}/webhook-secret`); repo.reload(); toast('Webhook secret replaced', 'success'); });
  };
  const remove = () => {
    if (!confirmDanger(`Delete ${r.name} and all its builds?`)) return;
    action.run(async () => { await api.del(`/repos/${r.id}`); navigate('/'); });
  };

  return html`<${Page} title=${r.name} subtitle=${html`<a href=${href('/')}>← Repositories</a> · <span class="mono">${r.url}</span>`}
    actions=${html`
      <input class="input mono" style="width:160px" placeholder=${r.branch} value=${branch} onInput=${e => setBranch(e.currentTarget.value)} aria-label="Branch" />
      <button class="button button-primary" disabled=${action.busy} onClick=${run}>Run</button>
      <button class="button" onClick=${() => setEditing(true)}>Edit</button>
      <button class="button button-danger" onClick=${remove}>Delete</button>`}>
    <${Notice} error=${action.error || builds.error} />
    <section>
      ${builds.data === undefined ? html`<${Loading} />` : !builds.data.length ? html`<${Empty}>No builds yet. Click Run, or set up the webhook below and push.<//>` : html`
        <${Table}>
          <thead><tr><th>Build</th><th>Status</th><th>Branch · Commit</th><th>Trigger</th><th>Created</th><th class="num">Duration</th></tr></thead>
          <tbody>${builds.data.map(b => html`<tr key=${b.id}>
            <td><a class="cell-main" href=${href(`/builds/${b.id}`)}>#${b.number}</a></td>
            <td><${BuildStatus} status=${b.status} title=${b.error} /></td>
            <td><span class="mono">${b.branch}</span> ${b.commit ? html`<span class="commit muted">${short(b.commit)}</span>` : null}
              <div class="cell-sub message" title=${b.message}>${b.message}</div></td>
            <td>${EVENTS[b.event] || b.event}${b.author ? html`<div class="cell-sub">${b.author}</div>` : null}</td>
            <td class="nowrap muted">${datetime(b.created_at)}</td>
            <td class="num">${duration(b.started_at, b.finished_at)}</td>
          </tr>`)}</tbody>
        <//>`}
    </section>
    <div class="grid-2 section">
      <div class="card">
        <h2>Webhook</h2>
        <p class="muted" style="margin-top:0">In the repository settings on GitHub, Gitea or GitLab, add a webhook for push events with content type application/json.</p>
        <${KV} items=${[
          ['URL', html`<span class="copy-row"><code>${hook}</code><${CopyButton} text=${hook} /></span>`],
          ['Secret', html`<span class="copy-row"><code>${r.webhook_secret}</code><${CopyButton} text=${r.webhook_secret} />
            <button class="button button-small" onClick=${rotate}>Replace</button></span>`],
        ]} />
        <p class="field-hint" style="margin-bottom:0">The code host must be able to reach this URL. Behind a private network, expose it through a gateway.</p>
      </div>
      <div class="card">
        <h2>Example ${r.config_path}</h2>
        <pre class="example">${EXAMPLE}</pre>
      </div>
    </div>
    ${editing ? html`<${RepoForm} repo=${r} onClose=${() => setEditing(false)} onSaved=${() => { setEditing(false); repo.reload(); }} />` : null}
  <//>`;
}
