import { html } from '../lib/html.js';
import { Page, Notice, Loading, KV, LogView } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { datetime } from '../lib/format.js';
import { useApi, JobStatus, duration } from '../packages.js';

export default function Job({ params }) {
  const { href, link } = usePanel();
  // Poll while the job runs: its output grows.
  const job = useApi(`/jobs/${params.id}`, j => (j?.status === 'running' ? 1000 : 0));
  if (!job.data) return html`<${Page} title="Job" back=${{ href: href('/jobs'), label: 'Jobs' }}>${job.error ? html`<${Notice} error=${job.error} />` : html`<${Loading} />`}<//>`;
  const j = job.data;
  const docker = j.action === 'docker' && j.status === 'succeeded' && link?.('docker', '/');
  return html`<${Page} title=${j.title} back=${{ href: href('/jobs'), label: 'Jobs' }}
    actions=${docker ? html`<a class="button button-primary" href=${docker}>Open Docker</a>` : null}>
    <div class="card">
      <${KV} items=${[
        ['Status', html`<${JobStatus} job=${j} />`],
        ['Started', datetime(j.started_at)],
        ['Duration', duration(j)],
        ['Error', j.error],
      ]} />
    </div>
    <section class="section">
      <h2>Output</h2>
      <${LogView} lines=${j.output || []} empty=${j.status === 'running' ? 'Waiting for output…' : 'No output'} />
    </section>
  <//>`;
}
