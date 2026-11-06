import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { FormSection, returnPath, useUnsavedChanges } from '@lsongdev/panel/forms.js';
import { useApi } from '../cron.js';

export default function JobEditor({ params }) {
  const { api, href, query } = usePanel();
  const isNew = !params.id;
  const job = useResource(signal => isNew ? Promise.resolve(null) : api.get(`/jobs/${params.id}`, { signal }), [params.id]);
  const targets = useApi('/targets');
  const back = returnPath(query, isNew ? '/' : `/jobs/${params.id}`);
  const title = isNew ? 'Add job' : job.data ? `Edit ${job.data.name}` : 'Edit job';
  return html`<${PageHeader} title=${title} back=${{ href: href(back), label: back.startsWith('/jobs/') ? 'Job' : 'Jobs' }} />
    ${targets.error ? html`<${Alert} error=${targets.error} />` : null}
    ${!isNew && !job.data ? (job.error ? html`<${Alert} error=${job.error} />` : html`<${Loading} />`)
      : !targets.data && !targets.error ? html`<${Loading} />`
      : html`<${JobForm} job=${job.data || {}} targets=${targets.data || []} back=${back} />`}`;
}

const EXAMPLES = [
  ['Every 5 minutes', '*/5 * * * *'], ['Hourly', '@hourly'], ['Daily at 3:00', '0 3 * * *'],
  ['Weekdays at 9:30', '30 9 * * mon-fri'], ['Sundays at 4:00', '0 4 * * sun'], ['Monthly on the 1st', '0 0 1 * *'],
];

function JobForm({ job, targets, back }) {
  const { api, href, navigate } = usePanel();
  const isNew = !job.id;
  const [j, setJ] = useState({
    name: job.name || '', schedule: job.schedule || '0 3 * * *', timezone: job.timezone || '',
    target: job.target || 'local', command: job.command || '', dir: job.dir || '',
    timeout: String(job.timeout ?? 3600), enabled: job.enabled ?? true,
  });
  const [initial] = useState(() => JSON.stringify(j));
  const { busy, error, run } = useAction();
  const saved = useUnsavedChanges(JSON.stringify(j) !== initial, { pending: busy });
  const [preview, setPreview] = useState({});
  const set = patch => {
    setJ(prev => ({ ...prev, ...patch }));
    if ('schedule' in patch || 'timezone' in patch) setPreview({});
  };

  // Show when the schedule fires, after typing pauses.
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      api.get('/preview', { query: { schedule: j.schedule, timezone: j.timezone }, signal: controller.signal })
        .then(p => setPreview({ next: p.next || [] }), err => { if (err.name !== 'AbortError') setPreview({ error: err.message }); });
    }, 300);
    return () => { clearTimeout(timer); controller.abort(); };
  }, [j.schedule, j.timezone]);

  const submit = e => {
    e.preventDefault();
    run(async () => {
      const body = { ...j, timeout: Number(j.timeout) || 0 };
      if (isNew) await api.post('/jobs', body);
      else await api.put(`/jobs/${job.id}`, body);
      toast(isNew ? 'Job added' : 'Job saved', 'success');
      saved();
      navigate(back, { replace: true });
    });
  };
  const local = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return html`<form class="form settings-form" onSubmit=${submit}>
      <fieldset class="form-fields" disabled=${busy}>
      <${Alert} error=${error} />
      <${FormSection} title="Execution">
      <div class="cron-job-fields">
        <${Field} label="Name" required>
          <input class="input" required value=${j.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="backup" autofocus />
        <//>
        <${Field} label="Runs on">
          <select class="input" value=${j.target} onChange=${e => set({ target: e.currentTarget.value })}>
            ${targets.filter(t => !t.internal || t.id === j.target).map(t => html`<option value=${t.id}>${t.kind === 'local' ? `Local (${t.name})` : `${t.name} (${t.kind.toUpperCase()})`}</option>`)}
            ${targets.some(t => t.id === j.target) ? null : html`<option value=${j.target}>${j.target} (unavailable)</option>`}
          </select>
        <//>
      </div>
      <${Field} label="Command" required hint="Run by /bin/sh; its output and exit code are recorded">
        <textarea class="input" rows="5" spellcheck="false" required value=${j.command}
          placeholder="tar czf /backup/data-$(date +%F).tgz /srv/data" onInput=${e => set({ command: e.currentTarget.value })} />
      <//>
      <div class="cron-job-fields">
        <${Field} label="Directory" hint="Empty uses the default">
          <input class="input mono" value=${j.dir} onInput=${e => set({ dir: e.currentTarget.value })} />
        <//>
        <${Field} label="Timeout (seconds)" hint="0 for none">
          <input class="input" type="number" min="0" value=${j.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
        <//>
      </div>
      <//>
      <${FormSection} title="Schedule">
      <div class="cron-job-fields">
        <${Field} label="Schedule" required error=${preview.error}>
          <input class="input mono" required value=${j.schedule} onInput=${e => set({ schedule: e.currentTarget.value })} />
          <div class="examples">${EXAMPLES.map(([label, expr]) => html`<button type="button" class="button button-small" key=${expr} onClick=${() => set({ schedule: expr })}>${label}</button>`)}</div>
          ${preview.next?.length ? html`<ul class="preview">${preview.next.map(t => html`<li key=${t}>${new Date(t).toLocaleString('zh-CN', { hour12: false, timeZone: j.timezone || undefined })}</li>`)}</ul>` : null}
        <//>
        <${Field} label="Time zone" hint=${`Empty uses the server's; your browser's is ${local}`}>
          <input class="input mono" value=${j.timezone} placeholder="Asia/Shanghai" onInput=${e => set({ timezone: e.currentTarget.value })} />
        <//>
      </div>
      <label class="field-check"><input type="checkbox" checked=${j.enabled} onChange=${e => set({ enabled: e.currentTarget.checked })} />Run on schedule</label>
      <//>
      </fieldset>
      <div class="button-actions">
        <button class="button button-primary" disabled=${busy}>${busy ? 'Saving…' : isNew ? 'Add job' : 'Save changes'}</button>
        <a class="button" href=${href(back)}>Cancel</a>
      </div>
    </form>`;
}
