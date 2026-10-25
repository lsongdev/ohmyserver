import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, KV } from '../lib/ui.js';
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

  if (!job.data) return html`<${Page} title="任务">${job.error ? html`<${Notice} error=${job.error} />` : html`<${Loading} />`}<//>`;
  const j = job.data;
  const target = targets.data?.find(t => t.id === j.target);
  const runNow = () => action.run(async () => {
    const r = await api.post(`/jobs/${j.id}/run`);
    navigate(`/runs/${r.id}`);
  });
  return html`<${Page} title=${j.name} subtitle=${html`<a href=${href('/')}>← 任务</a>`}
    actions=${html`
      <button class="btn btn-primary" disabled=${action.busy || j.running} onClick=${runNow}>立即运行</button>
      <button class="btn" onClick=${() => setEditing(true)}>编辑</button>`}>
    <${Notice} error=${action.error} />
    <div class="card">
      <${KV} items=${[
        ['计划', html`<span class="mono">${j.schedule}</span>${j.timezone ? html`<span class="muted" style="margin-left:8px">${j.timezone}</span>` : null}`],
        ['下次执行', j.enabled ? (j.next_run ? `${datetime(j.next_run)}（${relative(j.next_run)}）` : '不会再执行') : '已停用'],
        ['执行位置', target ? target.name : j.target],
        ['工作目录', j.dir],
        ['超时', j.timeout ? `${j.timeout} 秒` : '不限制'],
        ['命令', html`<pre class="command">${j.command}</pre>`],
      ]} />
    </div>
    <section class="section">
      <h2>执行记录</h2>
      ${runs.data === undefined ? html`<${Loading} />` : !runs.data.length ? html`<${Empty}>还没有执行过。<//>` : html`
        <div class="table-wrap"><table class="table">
          <thead><tr><th>开始时间</th><th>触发</th><th>状态</th><th class="num">退出码</th><th class="num">耗时</th></tr></thead>
          <tbody>${runs.data.map(r => html`<tr key=${r.id}>
            <td><a href=${href(`/runs/${r.id}`)}>${datetime(r.started_at)}</a></td>
            <td>${TRIGGERS[r.trigger] || r.trigger}</td>
            <td><${RunStatus} run=${r} />${r.error ? html`<div class="cell-sub">${r.error}</div>` : null}</td>
            <td class="num mono">${r.finished_at && r.status !== 'interrupted' ? r.exit_code : '—'}</td>
            <td class="num">${duration(r)}</td>
          </tr>`)}</tbody>
        </table></div>`}
    </section>
    ${editing ? html`<${JobForm} job=${j} targets=${targets.data || []}
      onClose=${() => setEditing(false)} onSaved=${() => { setEditing(false); job.reload(); }} />` : null}
  <//>`;
}
