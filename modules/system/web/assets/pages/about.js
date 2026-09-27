import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function About({ api }) {
  const [info, setInfo] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = async () => {
    setBusy(true); setError('');
    try { setInfo(await api.getInfo()); } catch (e) { setError(e.message); }
    finally { setBusy(false); }
  };
  useEffect(() => { load(); }, [api]);
  if (!info && busy) return html`<div class="loading-state" role="status">正在读取系统信息…</div>`;
  if (!info) return html`<div class="notice-card error"><h2>无法读取系统信息</h2><p>${error}</p><button class="button" onclick=${load}>重试</button></div>`;
  const rows = [
    ['当前版本', info.program_version], ['提交', info.program_commit], ['构建时间', info.build_time],
    ['运行环境', `${info.os} / ${info.arch}`], ['Go 版本', info.go_version], ['CPU 核心', info.num_cpu],
    ['监听地址', info.listen], ['数据库', info.database], ['Docker 镜像源', info.docker_mirror || '未配置'], ['运行时间', info.uptime],
  ];
  return html`<div class="system-about">
    ${error && html`<div class="notice-card error" role="alert">${error}</div>`}
    <section class="card"><h2>${info.program_name}</h2><p>${info.program_desc}</p>
      <dl class="about-grid">${rows.map(([label, value]) => html`<div><dt>${label}</dt><dd>${value || '—'}</dd></div>`)}</dl>
    </section>
    <section class="card"><h2>开源组件</h2><ul>${info.credits.map(item => html`<li><a href=${item.url} target="_blank" rel="noreferrer">${item.name}</a> · ${item.license}</li>`)}</ul></section>
  </div>`;
}
