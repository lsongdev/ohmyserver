import { html, useEffect, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, Modal, confirmDanger, toast } from '../lib/ui.js';
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
    if (!confirmDanger(`删除任务 ${j.name} 及其执行记录？`)) return;
    action.run(async () => { await api.del(`/jobs/${j.id}`); jobs.reload(); });
  };

  return html`<${Page} title="定时任务" subtitle="按计划运行命令，在本机或已保存的主机上。"
    actions=${html`<button class="btn btn-primary" onClick=${() => setEditing({})}>添加任务</button>`}>
    <${Notice} error=${jobs.error || action.error} />
    ${jobs.data === undefined ? (jobs.error ? null : html`<${Loading} />`) : !jobs.data.length ? html`<${Empty}>还没有定时任务。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>任务</th><th>计划</th><th>执行位置</th><th>上次执行</th><th class="actions"></th></tr></thead>
        <tbody>${jobs.data.map(j => html`<tr key=${j.id}>
          <td><a class="cell-main" href=${href(`/jobs/${j.id}`)}>${j.name}</a><div class="cell-command" title=${j.command}>${j.command}</div></td>
          <td><span class="mono">${j.schedule}</span>
            <div class="cell-sub">${j.enabled ? (j.next_run ? `下次 ${relative(j.next_run)}` : '不会再执行') : '已停用'}</div></td>
          <td>${targetName(j.target)}</td>
          <td>${j.last_run ? html`<a href=${href(`/runs/${j.last_run.id}`)}><${RunStatus} run=${j.last_run} /></a>
            <div class="cell-sub">${datetime(j.last_run.started_at)}</div>` : html`<span class="muted">—</span>`}</td>
          <td class="actions"><span class="btn-group">
            <button class="btn btn-sm" disabled=${action.busy || j.running} onClick=${() => run(j)}>立即运行</button>
            <button class="btn btn-sm" disabled=${action.busy} onClick=${() => toggle(j)}>${j.enabled ? '停用' : '启用'}</button>
            <button class="btn btn-sm" onClick=${() => setEditing(j)}>编辑</button>
            <button class="btn btn-sm btn-danger" disabled=${action.busy} onClick=${() => remove(j)}>删除</button>
          </span></td>
        </tr>`)}</tbody>
      </table></div>`}
    ${editing ? html`<${JobForm} job=${editing} targets=${targets.data || []}
      onClose=${() => setEditing(null)} onSaved=${() => { setEditing(null); jobs.reload(); }} />` : null}
  <//>`;
}

const EXAMPLES = [
  ['每 5 分钟', '*/5 * * * *'], ['每小时', '@hourly'], ['每天 3:00', '0 3 * * *'],
  ['工作日 9:30', '30 9 * * mon-fri'], ['每周日 4:00', '0 4 * * sun'], ['每月 1 号', '0 0 1 * *'],
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
      toast(isNew ? '任务已添加' : '任务已保存', 'success');
      onSaved();
    });
  };
  const local = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return html`<${Modal} wide title=${isNew ? '添加定时任务' : `编辑 ${job.name}`} onClose=${onClose} footer=${html`
    <span class="spacer"></span>
    <button class="btn" type="button" onClick=${onClose}>取消</button>
    <button class="btn btn-primary" type="submit" form="job-form" disabled=${busy}>${isNew ? '添加' : '保存'}</button>`}>
    <form id="job-form" class="form" onSubmit=${submit}>
      <${Notice} error=${error} />
      <div class="form-grid">
        <${Field} label="名称" required>
          <input class="input" required value=${j.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="backup" autofocus />
        <//>
        <${Field} label="执行位置">
          <select class="input" value=${j.target} onChange=${e => set({ target: e.currentTarget.value })}>
            ${targets.map(t => html`<option value=${t.id}>${t.kind === 'local' ? `本机（${t.name}）` : `${t.name}（${t.kind.toUpperCase()}）`}</option>`)}
            ${targets.some(t => t.id === j.target) ? null : html`<option value=${j.target}>${j.target}（不可用）</option>`}
          </select>
        <//>
      </div>
      <div class="form-grid">
        <${Field} label="计划" required error=${preview.error}>
          <input class="input mono" required value=${j.schedule} onInput=${e => set({ schedule: e.currentTarget.value })} />
          <div class="examples">${EXAMPLES.map(([label, expr]) => html`<button type="button" class="btn btn-sm" key=${expr} onClick=${() => set({ schedule: expr })}>${label}</button>`)}</div>
          ${preview.next?.length ? html`<ul class="preview">${preview.next.map(t => html`<li key=${t}>${new Date(t).toLocaleString('zh-CN', { hour12: false, timeZone: j.timezone || undefined })}</li>`)}</ul>` : null}
        <//>
        <${Field} label="时区" hint=${`留空使用服务器时区；你的浏览器是 ${local}`}>
          <input class="input mono" value=${j.timezone} placeholder="Asia/Shanghai" onInput=${e => set({ timezone: e.currentTarget.value })} />
        <//>
      </div>
      <${Field} label="命令" required hint="由 /bin/sh 执行；输出和退出码会记录下来">
        <textarea class="input" rows="5" spellcheck="false" required value=${j.command}
          placeholder="tar czf /backup/data-$(date +%F).tgz /srv/data" onInput=${e => set({ command: e.currentTarget.value })} />
      <//>
      <div class="form-grid">
        <${Field} label="工作目录" hint="留空使用默认目录">
          <input class="input mono" value=${j.dir} onInput=${e => set({ dir: e.currentTarget.value })} />
        <//>
        <${Field} label="超时（秒）" hint="0 表示不限制">
          <input class="input" type="number" min="0" value=${j.timeout} onInput=${e => set({ timeout: e.currentTarget.value })} />
        <//>
      </div>
      <label class="check"><input type="checkbox" checked=${j.enabled} onChange=${e => set({ enabled: e.currentTarget.checked })} />按计划执行</label>
    </form>
  <//>`;
}
