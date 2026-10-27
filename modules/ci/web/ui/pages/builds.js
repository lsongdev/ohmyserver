import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, BuildStatus, EVENTS, active, datetime, short, duration } from '../ci.js';

export default function Builds() {
  const { href } = usePanel();
  const builds = useApi('/builds', list => (list?.some(b => active(b.status)) ? 2000 : 10000));
  const repos = useApi('/repos');
  const name = id => repos.data?.find(r => r.id === id)?.name || `#${id}`;
  return html`<${Page} title="Builds" subtitle="Recent builds across all repositories.">
    <${Notice} error=${builds.error} />
    ${builds.data === undefined ? html`<${Loading} />` : !builds.data.length ? html`<${Empty}>No builds yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Repository</th><th>Build</th><th>Status</th><th>Branch · Commit</th><th>Trigger</th><th>Created</th><th class="num">Duration</th></tr></thead>
        <tbody>${builds.data.map(b => html`<tr key=${b.id}>
          <td><a href=${href(`/repos/${b.repo_id}`)}>${name(b.repo_id)}</a></td>
          <td><a class="cell-main" href=${href(`/builds/${b.id}`)}>#${b.number}</a></td>
          <td><${BuildStatus} status=${b.status} title=${b.error} /></td>
          <td><span class="mono">${b.branch}</span> ${b.commit ? html`<span class="commit muted">${short(b.commit)}</span>` : null}
            <div class="cell-sub message" title=${b.message}>${b.message}</div></td>
          <td>${EVENTS[b.event] || b.event}</td>
          <td class="nowrap muted">${datetime(b.created_at)}</td>
          <td class="num">${duration(b.started_at, b.finished_at)}</td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
