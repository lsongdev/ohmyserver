import { html, useEffect, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, Modal, confirmDanger, toast, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, RunStatus, datetime, relative } from '../cron.js';

export default function Jobs() {
  const { api, href, navigate } = usePanel();
  const jobs = useApi('/jobs', 5000);
  const targets = useApi('/targets');
  const action = useAction();
  const [editing, setEditing] = useState(null); // a job, or {} for a new one
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
    actions=${html`<button class="button button-primary" onClick=${() => setEditing({})}>Add job</button>`}>
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
            <button class="button button-small" onClick=${() => setEditing(j)}>Edit</button>
            <button class="button button-small button-danger" disabled=${action.busy} onClick=${() => remove(j)}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//>`}
    ${editing ? html`<${JobForm} job=${editing} targets=${targets.data || []}
      onClose=${() => setEditing(null)} onSaved=${() => { setEditing(null); jobs.reload(); }} />` : null}
  <//>`;
}

const EXAMPLES = [
  ['Every 5 minutes', '*/5 * * * *'], ['Hourly', '@hourly'], ['Daily at 3:00', '0 3 * * *'],
  ['Weekdays at 9:30', '30 9 * * mon-fri'], ['Sundays at 4:00', '0 4 * * sun'], ['Monthly on the 1st', '0 0 1 * *'],
];

export function JobForm({ job, targets, onClose, onSaved }) {
  const { api } = usePanel();
  const isNew = !job.id;
  const [j, setJ] = useState({
    name: job.name || '', schedule: job.schedule || '0 3 * * *', timezone: job.timezone || '',
    target: job.target || 'local', command: job.command || '', dir: job.dir || '',
    timeout: job.timeout ?? 3600, enabled: job.enabled ?? true,
  });
  const { busy, error, run } = useAction();
  const [preview, setPreview] = useState({});
  const set = patch => setJ(prev => ({ ...prev, ...patch }));

  // Show when the schedule fires, after typing pauses.
  useEffect(() => {
    const timer = setTimeout(() => {
      api.get('/preview', { query: { schedule: j.schedule, timezone: j.timezone } })
        .then(p => setPreview({ next: p.next || [] }), err => setPreview({ error: err.message }));
    }, 300);
    return () => clearTimeout(timer);
  }, [j.schedule, j.timezone]);

  const submit = e => {
    e.preventDefault();
    run(async () => {
      const body = { ...j, timeout: Number(j.timeout) || 0 };
      if (isNew) await api.post('/jobs', body);
      else await api.put(`/jobs/${job.id}`, body);
      toast(isNew ? 'Job added' : 'Job saved', 'success');
      onSaved();
    });
  };
  const local = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return html`<${Modal} wide title=${isNew ? 'Add job' : `Edit ${job.name}`} onClose=${onClose} footer=${html`
    <span class="spacer"></span>
    <button class="button" type="button" onClick=${onClose}>Cancel</button>
    <button class="button button-primary" type="submit" form="job-form" disabled=${busy}>${isNew ? 'Add' : 'Save'}</button>`}>
    <form id="job-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <div class="form-grid">
        <${Field} label="Name" required>
          <input class="input" required value=${j.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="backup" autofocus />
        <//>
        <${Field} label="Runs on">
          <select class="input" value=${j.target} onChange=${e => set({ target: e.currentTarget.value })}>
            ${targets.map(t => html`<option value=${t.id}>${t.kind === 'local' ? `Local (${t.name})` : `${t.name} (${t.kind.toUpperCase()})`}</option>`)}
            ${targets.some(t => t.id === j.target) ? null : html`<option value=${j.target}>${j.target} (unavailable)</option>`}
          </select>
        <//>
      </div>
      <div class="form-grid">
        <${Field} label="Schedule" required error=${preview.error}>
          <input class="input mono" required value=${j.schedule} onInput=${e => set({ schedule: e.currentTarget.value })} />
          <div class="examples">${EXAMPLES.map(([label, expr]) => html`<button type="button" class="button button-small" key=${expr} onClick=${() => set({ schedule: expr })}>${label}</button>`)}</div>
          ${preview.next?.length ? html`<ul class="preview">${preview.next.map(t => html`<li key=${t}>${new Date(t).toLocaleString('zh-CN', { hour12: false, timeZone: j.timezone || undefined })}</li>`)}</ul>` : null}
        <//>
        <${Field} label="Time zone" hint=${`Empty uses the server's; your browser's is ${local}`}>
          <input class="input mono" value=${j.timezone} placeholder="Asia/Shanghai" onInput=${e => set({ timezone: e.currentTarget.value })} />
        <//>
      </div>
      <${Field} label="Command" required hint="Run by /bin/sh; its output and exit code are recorded">
        <textarea class="input" rows="5" spellcheck="false" required value=${j.command}
          placeholder="tar czf /backup/data-$(date +%F).tgz /srv/data" onInput=${e => set({ command: e.currentTarget.value })} />
      <//>
      <div class="form-grid">
        <${Field} label="Directory" hint="Empty uses the default">
          <input class="input mono" value=${j.dir} onInput=${e => set({ dir: e.currentTarget.value })} />
        <//>
        <${Field} label="Timeout (seconds)" hint="0 for none">
          <input class="input" type="number" min="0" value=${j.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
        <//>
      </div>
      <label class="field-check"><input type="checkbox" checked=${j.enabled} onChange=${e => set({ enabled: e.currentTarget.checked })} />Run on schedule</label>
    </form>
  <//>`;
}
