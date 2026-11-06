import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, KeyValue, LogView } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi, RunStatus, TRIGGERS, datetime, duration } from '../cron.js';

export default function Run({ params }) {
  const { api, href } = usePanel();
  // Poll while the run is in progress: its output grows.
  const run = useApi(`/runs/${params.id}`, r => (r?.status === 'running' ? 1000 : 0));
  const job = useApi(run.data ? `/jobs/${run.data.job_id}` : null);
  const cancel = useAction();

  if (!run.data) return html`<${PageHeader} title="Run" back=${{ href: href('/'), label: 'Jobs' }} />${run.error ? html`<${Alert} error=${run.error} />` : html`<${Loading} />`}`;
  const r = run.data;
  const lines = r.output ? r.output.replace(/\n$/, '').split('\n') : [];
  return html`<${PageHeader} title=${job.data ? `${job.data.name} · #${r.id}` : `#${r.id}`}
    back=${{ href: href(`/jobs/${r.job_id}`), label: 'Job' }}
    actions=${r.status === 'running' ? html`<button class="button button-danger" disabled=${cancel.busy}
      onClick=${() => cancel.run(async () => { await api.post(`/runs/${r.id}/cancel`); run.reload(); })}>Cancel run</button>` : null} />
    <${Alert} error=${cancel.error} />
    <div class="card">
      <${KeyValue} items=${[
        ['Status', html`<${RunStatus} run=${r} />`],
        ['Trigger', TRIGGERS[r.trigger] || r.trigger],
        ['Started', datetime(r.started_at)],
        ['Finished', r.finished_at ? datetime(r.finished_at) : null],
        ['Duration', r.finished_at ? duration(r) : null],
        ['Exit code', r.finished_at && r.status !== 'interrupted' ? String(r.exit_code) : null],
        ['Error', r.error],
      ]} />
    </div>
    <section class="section">
      <h2>Output</h2>
      <${LogView} lines=${lines} empty=${r.status === 'running' ? 'Waiting for output…' : 'No output'} />
    </section>`;
}
