import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';
import { createAPI } from './api.js';

const apiBase = document.querySelector('meta[name="api-base"]')?.content || '/v1/cron';
const api = createAPI({ request: async (path, options = {}) => {
  const response = await fetch(`${apiBase}${path}`, {
    ...options,
    body: options.body == null ? undefined : JSON.stringify(options.body),
    headers: options.body ? { 'Content-Type': 'application/json', ...options.headers } : options.headers,
  });
  const body = response.status === 204 ? null : await response.json();
  if (!response.ok) throw new Error(body?.error?.message || `HTTP ${response.status}`);
  return body?.data ?? body;
} });

function App() {
  const [jobs, setJobs] = useState([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [executions, setExecutions] = useState({});
  const refresh = async () => {
    try { setJobs(await api.list() || []); setError(''); }
    catch (reason) { setError(reason.message); }
  };
  useEffect(() => { refresh(); }, []);
  const field = (name, label, placeholder = '') => html`
    <label>${label}<input class="input" name=${name} placeholder=${placeholder} required /></label>`;
  const create = async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    setBusy(true); setError('');
    try {
      await api.create({ name: values.get('name'), type: values.get('type'), schedule: values.get('schedule'), command: values.get('command'), working_dir: values.get('working_dir'), timeout: Number(values.get('timeout') || 0) });
      form.reset(); await refresh();
    } catch (reason) { setError(reason.message); }
    finally { setBusy(false); }
  };
  const run = async id => {
    setBusy(true); setError('');
    try { await api.run(id); setExecutions({ ...executions, [id]: await api.executions(id) }); }
    catch (reason) { setError(reason.message); }
    finally { setBusy(false); }
  };
  const remove = async id => {
    if (!confirm('删除这个定时任务？')) return;
    setBusy(true); setError('');
    try { await api.remove(id); await refresh(); }
    catch (reason) { setError(reason.message); }
    finally { setBusy(false); }
  };
  const toggle = async job => {
    setBusy(true); setError('');
    try { await api.enabled(job.id, !job.enabled); await refresh(); }
    catch (reason) { setError(reason.message); }
    finally { setBusy(false); }
  };
  return html`<div>
    ${error && html`<p class="cron-error" role="alert">${error}</p>`}
    <div class="cron-grid">
      <section class="cron-card"><h1>任务列表</h1>
        ${jobs.length ? jobs.map(job => html`<article class="cron-job">
          <strong>${job.name}</strong> · ${job.enabled ? '已启用' : '已停用'}<br />
          <code>${job.type}: ${job.schedule}</code><br />
          <code>${job.command}</code>
          <div class="cron-actions">
            <button class="button" disabled=${busy} onClick=${() => run(job.id)}>立即运行</button>
            <button class="button" disabled=${busy} onClick=${() => toggle(job)}>${job.enabled ? '停用' : '启用'}</button>
            <button class="button" disabled=${busy} onClick=${() => remove(job.id)}>删除</button>
          </div>
          ${(executions[job.id] || []).map(run => html`<small>${run.status}: ${run.output || run.error}</small>`)}
        </article>`) : html`<p>暂无任务</p>`}
      </section>
      <section class="cron-card"><h2>新建任务</h2>
        <form class="cron-form" onSubmit=${create}>
          ${field('name', '名称')}
          <label>类型<select class="input" name="type"><option value="cron">Cron</option><option value="once">单次</option></select></label>
          ${field('schedule', '表达式或 RFC3339 时间', '0 1 * * *')}
          ${field('command', '命令')}<label>工作目录（可选）<input class="input" name="working_dir" /></label>
          <label>超时秒数<input class="input" type="number" name="timeout" min="0" value="0" /></label>
          <button class="button" disabled=${busy}>创建任务</button>
        </form>
      </section>
    </div>
  </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
