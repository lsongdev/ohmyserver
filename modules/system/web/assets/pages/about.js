import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function About({ api }) {
  const [info, setInfo] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const load = async check => {
    setBusy(check ? 'check' : 'load'); setError('');
    try { setInfo(await api.getInfo(check)); } catch (e) { setError(e.message); }
    finally { setBusy(''); }
  };
  useEffect(() => { load(false); }, [api]);
  const check = () => load(true);
  const update = async () => {
    if (!confirm('确认排队安装已校验的新版本？服务将重启。')) return;
    setBusy('update'); setError('');
    try { const result = await api.update(); setInfo(current => ({ ...current, update: { ...current?.update, checked: true, message: result?.message || '更新任务已排队' } })); }
    catch (e) { setError(e.message); }
    finally { setBusy(''); }
  };
  if (!info && busy) return html`<div class="loading-state" role="status">正在读取系统信息…</div>`;
  if (!info) return html`<div class="notice-card error"><h2>无法读取系统信息</h2><p>${error}</p><button class="button" onclick=${() => load(false)}>重试</button></div>`;
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
    <section class="card"><h2>软件更新</h2>
      ${info.update?.checked ? html`<p>${info.update.message || (info.update.available ? `发现新版本 ${info.update.latest_version}` : '已是最新版本')}</p>
        ${info.update.release_url && html`<p><a href=${info.update.release_url} target="_blank" rel="noreferrer">查看发行说明</a></p>`}
        ${info.update.available && html`<p>目标版本：${info.update.latest_version} · ${info.update.asset_name}</p>`}` : html`<p class="muted">检查上游发行版本和当前平台安装包。</p>`}
      <div class="form-actions"><button class="button" disabled=${!!busy} onclick=${check}>${busy === 'check' ? '检查中…' : '检查更新'}</button>
        ${info.update?.available && info.update?.installable && html`<button class="button button-primary" disabled=${!!busy} onclick=${update}>${busy === 'update' ? '正在排队…' : '安装更新'}</button>`}
      </div>
    </section>
    <section class="card"><h2>开源组件</h2><ul>${info.credits.map(item => html`<li><a href=${item.url} target="_blank" rel="noreferrer">${item.name}</a> · ${item.license}</li>`)}</ul></section>
  </div>`;
}
