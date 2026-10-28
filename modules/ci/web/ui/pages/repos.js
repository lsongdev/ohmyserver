import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { routeURL } from '../lib/forms.js';
import { useApi, BuildStatus, active, datetime, short, duration } from '../ci.js';

export default function Repos() {
  const { api, href, navigate, query } = usePanel();
  const repos = useApi('/repos', list => (list?.some(r => r.last_build && active(r.last_build.status)) ? 2000 : 10000));
  const action = useAction();
  const run = r => action.run(async () => {
    const b = await api.post(`/repos/${r.id}/builds`);
    navigate(`/builds/${b.id}`);
  });
  return html`<${Page} title="Repositories" subtitle="Build and test in containers, as each repository's .ci.yml says, on push or on demand."
    actions=${html`<a class="button button-primary" href=${href('/repos/new', { return: routeURL('/', query) })}>Add repository</a>`}>
    <${Notice} error=${repos.error || action.error} />
    ${repos.data === undefined ? (repos.error ? null : html`<${Loading} />`) : !repos.data.length ? html`<${Empty}>No repositories yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Repository</th><th>Last build</th><th>Commit</th><th class="num">Duration</th><th class="actions"></th></tr></thead>
        <tbody>${repos.data.map(r => {
          const b = r.last_build;
          return html`<tr key=${r.id}>
            <td><a class="cell-main" href=${href(`/repos/${r.id}`)}>${r.name}</a><div class="cell-sub mono">${r.url}</div></td>
            <td>${b ? html`<a href=${href(`/builds/${b.id}`)}><${BuildStatus} status=${b.status} title=${b.error} /></a>
              <div class="cell-sub">#${b.number} · ${b.branch} · ${datetime(b.created_at)}</div>` : html`<span class="muted">No builds yet</span>`}</td>
            <td>${b?.commit ? html`<span class="commit">${short(b.commit)}</span><div class="cell-sub message" title=${b.message}>${b.message}</div>` : '—'}</td>
            <td class="num">${b ? duration(b.started_at, b.finished_at) : '—'}</td>
            <td class="actions"><span class="button-row">
              <button class="button button-small button-primary" disabled=${action.busy} onClick=${() => run(r)}>Run</button>
              <a class="button button-small" href=${href(`/repos/${r.id}`)}>Details</a>
            </span></td>
          </tr>`;
        })}</tbody>
      <//>`}
  <//>`;
}
