import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, confirmDanger, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { routeURL } from '../lib/forms.js';
import { useApi, RunStatus, datetime, relative } from '../cron.js';

export default function Jobs() {
  const { api, href, navigate, query } = usePanel();
  const jobs = useApi('/jobs', 5000);
  const targets = useApi('/targets');
  const action = useAction();
  const targetName = id => targets.data?.find(t => t.id === id)?.name || id;

  const run = j => action.run(async () => {
    const r = await api.post(`/jobs/${j.id}/run`);
    navigate(`/runs/${r.id}`);
  });
  const toggle = j => action.run(async () => { await api.post(`/jobs/${j.id}/${j.enabled ? 'disable' : 'enable'}`); jobs.reload(); });
  const remove = j => {
    if (!confirmDanger(`Delete the job ${j.name} and its runs?`)) return;
    action.run(async () => { await api.del(`/jobs/${j.id}`); jobs.reload(); });
  };

  return html`<${Page} title="Jobs" subtitle="Run commands on a schedule, here or on a saved host."
    actions=${html`<a class="button button-primary" href=${href('/jobs/new', { return: routeURL('/', query) })}>Add job</a>`}>
    <${Notice} error=${jobs.error || action.error} />
    ${jobs.data === undefined ? (jobs.error ? null : html`<${Loading} />`) : !jobs.data.length ? html`<${Empty}>No jobs yet.<//>` : html`
      <${Table}>
        <thead><tr><th>Job</th><th>Schedule</th><th>Runs on</th><th>Last run</th><th class="actions"></th></tr></thead>
        <tbody>${jobs.data.map(j => html`<tr key=${j.id}>
          <td><a class="cell-main" href=${href(`/jobs/${j.id}`)}>${j.name}</a><div class="cell-command" title=${j.command}>${j.command}</div></td>
          <td><span class="mono">${j.schedule}</span>
            <div class="cell-sub">${j.enabled ? (j.next_run ? `Next ${relative(j.next_run)}` : 'Never runs again') : 'Disabled'}</div></td>
          <td>${targetName(j.target)}</td>
          <td>${j.last_run ? html`<a href=${href(`/runs/${j.last_run.id}`)}><${RunStatus} run=${j.last_run} /></a>
            <div class="cell-sub">${datetime(j.last_run.started_at)}</div>` : html`<span class="muted">—</span>`}</td>
          <td class="actions"><span class="button-row">
            <button class="button button-small" disabled=${action.busy || j.running} onClick=${() => run(j)}>Run now</button>
            <button class="button button-small" disabled=${action.busy} onClick=${() => toggle(j)}>${j.enabled ? 'Disable' : 'Enable'}</button>
            <a class="button button-small" href=${href(`/jobs/${j.id}/edit`, { return: routeURL('/', query) })}>Edit</a>
            <button class="button button-small button-danger" disabled=${action.busy} onClick=${() => remove(j)}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}
