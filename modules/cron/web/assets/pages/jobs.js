import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const blank = { name: '', type: 'cron', schedule: '', command: '', working_dir: '', timeout: 0 };
const stamp = value => value ? new Date(value).toLocaleString() : '—';

export default function CronJobs({ api, navigate }) {
  const [jobs, setJobs] = useState([]), [form, setForm] = useState(blank), [editing, setEditing] = useState(null);
  const [executions, setExecutions] = useState({}), [error, setError] = useState(''), [busy, setBusy] = useState(false);
  const load = async () => { try { setJobs(await api.list() || []); setError(''); } catch (e) { setError(e.message); } };
  useEffect(() => { load(); }, [api]);
  const change = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError('');
    try { const data = { ...form, timeout: Number(form.timeout) || 0 }; await api.update(editing, data); setEditing(null); setForm(blank); await load(); }
    catch (e) { setError(e.message); } finally { setBusy(false); }
  };
  const edit = job => { setEditing(job.id); setForm({ name: job.name, type: job.type, schedule: job.schedule, command: job.command, working_dir: job.working_dir || '', timeout: job.timeout || 0 }); };
  const toggle = async job => { try { await api.enabled(job.id, !job.enabled); await load(); } catch (e) { setError(e.message); } };
  const run = async job => { try { await api.run(job.id); await showRuns(job); } catch (e) { setError(e.message); } };
  const showRuns = async job => { try { setExecutions({ ...executions, [job.id]: await api.executions(job.id) || [] }); } catch (e) { setError(e.message); } };
  const remove = async job => { if (!confirm(`删除定时任务“${job.name}”？`)) return; try { await api.remove(job.id); await load(); } catch (e) { setError(e.message); } };
  return html`<div class="cron-page">
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${editing ? html`<section class="card"><h2>编辑任务</h2><form onSubmit=${submit}>
      <div class="form-group"><label>名称</label><input class="input" required value=${form.name} onInput=${e => change('name', e.currentTarget.value)} /></div>
      <div class="form-group"><label>类型</label><select class="input" value=${form.type} onChange=${e => change('type', e.currentTarget.value)}><option value="cron">Cron 表达式</option><option value="once">一次性</option></select></div>
      <div class="form-group"><label>${form.type === 'cron' ? 'Cron 表达式' : '执行时间（RFC3339）'}</label><input class="input" required value=${form.schedule} placeholder=${form.type === 'cron' ? '*/5 * * * *' : '2026-10-01T12:00:00+08:00'} onInput=${e => change('schedule', e.currentTarget.value)} /></div>
      <div class="form-group"><label>命令</label><textarea class="input" required rows="3" value=${form.command} onInput=${e => change('command', e.currentTarget.value)}></textarea></div>
      <div class="form-group"><label>工作目录（可选）</label><input class="input" value=${form.working_dir} onInput=${e => change('working_dir', e.currentTarget.value)} /></div>
      <div class="form-group"><label>超时（秒，0 表示不限制）</label><input class="input" type="number" min="0" value=${form.timeout} onInput=${e => change('timeout', e.currentTarget.value)} /></div>
      <div class="form-actions"><button class="button button-primary" disabled=${busy}>${editing ? '保存任务' : '创建任务'}</button>${editing ? html`<button class="button" type="button" onClick=${() => { setEditing(null); setForm(blank); }}>取消</button>` : null}</div>
    </form></section>` : null}
    <section class="card"><div class="page-toolbar"><h2>任务列表</h2><div class="button-group"><button class="button button-primary" onClick=${() => navigate('/cron/new')}>新建任务</button><button class="button" onClick=${load}>刷新</button></div></div>
      ${jobs.length ? html`<div class="table-wrap"><table class="table"><thead><tr><th>任务</th><th>计划</th><th>下次运行</th><th>上次运行</th><th>状态</th><th>操作</th></tr></thead><tbody>${jobs.map(job => html`<tr key=${job.id}><td><strong>${job.name}</strong><div class="muted">${job.command}</div></td><td>${job.type}: ${job.schedule}</td><td>${stamp(job.next_run)}</td><td>${stamp(job.last_run)}</td><td>${job.enabled ? '启用' : '停用'}</td><td><div class="button-group"><button class="button" onClick=${() => toggle(job)}>${job.enabled ? '停用' : '启用'}</button><button class="button" onClick=${() => run(job)}>立即运行</button><button class="button" onClick=${() => showRuns(job)}>运行记录</button><button class="button" onClick=${() => edit(job)}>编辑</button><button class="button button-danger" onClick=${() => remove(job)}>删除</button></div></td></tr>${executions[job.id] ? html`<tr><td colspan="6"><strong>最近运行</strong>${executions[job.id].map(x => html`<div class="muted">${stamp(x.started_at)} · ${x.status} · exit ${x.exit_code}${x.error ? ` · ${x.error}` : ''}<pre>${x.output}</pre></div>`)}</td></tr>` : null}`)}</tbody></table></div>` : html`<p class="muted">暂无定时任务。</p>`}
    </section>
  </div>`;
}
