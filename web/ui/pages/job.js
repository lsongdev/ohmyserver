import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, KV, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, RunStatus, TRIGGERS, datetime, duration, relative } from '../cron.js';
import { JobForm } from './jobs.js';

export default function Job({ params }) {
  const { api, href, navigate } = usePanel();
  const job = useApi(`/jobs/${params.id}`, j => (j?.running ? 2000 : 10000));
  const runs = useApi(`/jobs/${params.id}/runs`, () => (job.data?.running ? 2000 : 10000));
  const targets = useApi('/targets');
  const action = useAction();
  const [editing, setEditing] = useState(false);

  if (!job.data) return html`<${Page} title="Job">${job.error ? html`<${Notice} error=${job.error} />` : html`<${Loading} />`}<//>`;
  const j = job.data;
  const target = targets.data?.find(t => t.id === j.target);
  const runNow = () => action.run(async () => {
    const r = await api.post(`/jobs/${j.id}/run`);
    navigate(`/runs/${r.id}`);
  });
  return html`<${Page} title=${j.name} subtitle=${html`<a href=${href('/')}>← Jobs</a>`}
    actions=${html`
      <button class="button button-primary" disabled=${action.busy || j.running} onClick=${runNow}>Run now</button>
      <button class="button" onClick=${() => setEditing(true)}>Edit</button>`}>
    <${Notice} error=${action.error} />
    <div class="card">
      <${KV} items=${[
        ['Schedule', html`<span class="mono">${j.schedule}</span>${j.timezone ? html`<span class="muted" style="margin-left:8px">${j.timezone}</span>` : null}`],
        ['Next run', j.enabled ? (j.next_run ? `${datetime(j.next_run)} (${relative(j.next_run)})` : 'Never') : 'Disabled'],
        ['Runs on', target ? target.name : j.target],
        ['Directory', j.dir],
        ['Timeout', j.timeout ? `${j.timeout}s` : 'None'],
        ['Command', html`<pre class="command">${j.command}</pre>`],
      ]} />
    </div>
    <section class="section">
      <h2>Runs</h2>
      ${runs.data === undefined ? html`<${Loading} />` : !runs.data.length ? html`<${Empty}>No runs yet.<//>` : html`
        <${Table}>
          <thead><tr><th>Started</th><th>Trigger</th><th>Status</th><th class="num">Exit code</th><th class="num">Duration</th></tr></thead>
          <tbody>${runs.data.map(r => html`<tr key=${r.id}>
            <td><a href=${href(`/runs/${r.id}`)}>${datetime(r.started_at)}</a></td>
            <td>${TRIGGERS[r.trigger] || r.trigger}</td>
            <td><${RunStatus} run=${r} />${r.error ? html`<div class="cell-sub">${r.error}</div>` : null}</td>
            <td class="num mono">${r.finished_at && r.status !== 'interrupted' ? r.exit_code : '—'}</td>
            <td class="num">${duration(r)}</td>
          </tr>`)}</tbody>
        <//>`}
    </section>
    ${editing ? html`<${JobForm} job=${j} targets=${targets.data || []}
      onClose=${() => setEditing(false)} onSaved=${() => { setEditing(false); job.reload(); }} />` : null}
  <//>`;
}
