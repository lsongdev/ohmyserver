import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, LogView } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, RunStatus, TRIGGERS, datetime, duration } from '../cron.js';

export default function Run({ params }) {
  const { api, href } = usePanel();
  // Poll while the run is in progress: its output grows.
  const run = useApi(`/runs/${params.id}`, r => (r?.status === 'running' ? 1000 : 0));
  const job = useApi(run.data ? `/jobs/${run.data.job_id}` : null);
  const cancel = useAction();

  if (!run.data) return html`<${Page} title="执行记录">${run.error ? html`<${Notice} error=${run.error} />` : html`<${Loading} />`}<//>`;
  const r = run.data;
  const lines = r.output ? r.output.replace(/\n$/, '').split('\n') : [];
  return html`<${Page} title=${job.data ? `${job.data.name} · #${r.id}` : `#${r.id}`}
    subtitle=${html`<a href=${href(`/jobs/${r.job_id}`)}>← 任务</a>`}
    actions=${r.status === 'running' ? html`<button class="btn btn-danger" disabled=${cancel.busy}
      onClick=${() => cancel.run(async () => { await api.post(`/runs/${r.id}/cancel`); run.reload(); })}>取消运行</button>` : null}>
    <${Notice} error=${cancel.error} />
    <div class="card">
      <${KV} items=${[
        ['状态', html`<${RunStatus} run=${r} />`],
        ['触发', TRIGGERS[r.trigger] || r.trigger],
        ['开始', datetime(r.started_at)],
        ['结束', r.finished_at ? datetime(r.finished_at) : null],
        ['耗时', r.finished_at ? duration(r) : null],
        ['退出码', r.finished_at && r.status !== 'interrupted' ? String(r.exit_code) : null],
        ['错误', r.error],
      ]} />
    </div>
    <section class="section">
      <h2>输出</h2>
      <${LogView} lines=${lines} empty=${r.status === 'running' ? '等待输出…' : '没有输出'} />
    </section>
  <//>`;
}
