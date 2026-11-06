import { html } from '@lsongdev/panel/html.js';
import { PageHeader, Alert, Loading, KeyValue, LogView } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { datetime } from '@lsongdev/panel/format.js';
import { useApi, JobStatus, duration } from '../packages.js';

export default function Job({ params }) {
  const { href, link } = usePanel();
  // Poll while the job runs: its output grows.
  const job = useApi(`/jobs/${params.id}`, j => (j?.status === 'running' ? 1000 : 0));
  if (!job.data) return html`<${PageHeader} title="Job" back=${{ href: href('/jobs'), label: 'Jobs' }} />${job.error ? html`<${Alert} error=${job.error} />` : html`<${Loading} />`}`;
  const j = job.data;
  const docker = j.action === 'docker' && j.status === 'succeeded' && link?.('docker', '/');
  return html`<${PageHeader} title=${j.title} back=${{ href: href('/jobs'), label: 'Jobs' }}
    actions=${docker ? html`<a class="button button-primary" href=${docker}>Open Docker</a>` : null} />
    <div class="card">
      <${KeyValue} items=${[
        ['Status', html`<${JobStatus} job=${j} />`],
        ['Started', datetime(j.started_at)],
        ['Duration', duration(j)],
        ['Error', j.error],
      ]} />
    </div>
    <section class="section">
      <h2>Output</h2>
      <${LogView} lines=${j.output || []} empty=${j.status === 'running' ? 'Waiting for output…' : 'No output'} />
    </section>`;
}
