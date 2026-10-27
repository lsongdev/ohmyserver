import { html } from '../lib/html.js';
import { Page, Resource, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { datetime } from '../lib/format.js';
import { useApi, JobStatus, duration } from '../packages.js';

export default function Jobs() {
  const { href } = usePanel();
  const jobs = useApi('/jobs', list => (list?.some(j => j.status === 'running') ? 2000 : 10000));
  return html`<${Page} title="Jobs" subtitle="Changes to the system's packages since the panel started, newest first.">
    <${Resource} resource=${jobs} empty="No jobs yet. Installing, removing or upgrading packages runs a job.">${list => html`
      <${Table}>
        <thead><tr><th>Job</th><th>Status</th><th>Started</th><th class="num">Duration</th></tr></thead>
        <tbody>${list.map(j => html`<tr key=${j.id}>
          <td><a class="cell-main" href=${href(`/jobs/${j.id}`)}>${j.title}</a></td>
          <td><${JobStatus} job=${j} /></td>
          <td class="nowrap muted">${datetime(j.started_at)}</td>
          <td class="num muted">${duration(j)}</td>
        </tr>`)}</tbody>
      <//>`}<//>
  <//>`;
}
