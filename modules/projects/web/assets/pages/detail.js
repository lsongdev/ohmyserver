import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

export default function ProjectDeployments({api,params,navigate,href,moduleHref}){
  const id=Number(params.id),[project,setProject]=useState(null),[items,setItems]=useState([]),[runtime,setRuntime]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false),[secret,setSecret]=useState('');
  const load=async()=>{setError('');try{const [p,d,r]=await Promise.all([api.get(id),api.deployments(id),api.runtime(id)]);setProject(p);setItems(d);setRuntime(r);}catch(e){setError(e.message);}};
  useEffect(()=>{load();},[id,api]);
  const start=async()=>{setBusy(true);setError('');try{const item=await api.start(id);navigate(`/${id}/deployments/${item.id}`);}catch(e){setError(e.message);}finally{setBusy(false);}};
  const rotate=async()=>{if(!confirm('轮换 Webhook Secret？GitHub 中配置的旧 Secret 将立即失效。'))return;setBusy(true);try{const result=await api.rotateSecret(id);setSecret(result.webhook_secret_once);}catch(e){setError(e.message);}finally{setBusy(false);}};
  return html`<div class="projects-page">
    <a href=${href("/")}>← 项目列表</a>${error&&html`<div class="notice-card error" role="alert">${error}</div>`}
    <section class="card"><h2>${project?.name||'项目部署'}</h2><p>${project?.repo||'尚未配置 Git 仓库'}</p><button class="button button-primary" disabled=${busy||!project?.repo} onclick=${start}>${busy?'正在创建部署…':'立即部署'}</button><h3>运行状态</h3><p>${runtime?.status||'读取中…'} ${runtime?.container_name||''}</p>${runtime?.container_id&&html`<p><a href=${moduleHref('docker', `/containers/${runtime.container_id}`)}>查看容器 ${runtime.container_id}</a></p>`}${runtime?.error&&html`<p class="error">${runtime.error}</p>`}<h3>GitHub Webhook</h3><p>Payload URL: <code>${api.webhookURL(id)}</code></p><p>分支：${project?.webhook_branch||'main'}</p><button class="button" disabled=${busy} onclick=${rotate}>轮换 Secret</button>${secret&&html`<div class="notice-card"><strong>新 Secret（仅显示一次）</strong><p><code>${secret}</code></p><p>请立即更新 GitHub Webhook 配置。</p></div>`}</section>
    <section class="card"><h2>部署历史</h2>${!items.length?html`<p class="muted">暂无部署记录。</p>`:html`<div class="table-wrap"><table class="table"><thead><tr><th>#</th><th>状态</th><th>提交</th><th>触发方式</th><th>创建时间</th><th></th></tr></thead><tbody>${items.map(x=>html`<tr key=${x.id}><td>${x.id}</td><td>${x.status}</td><td>${x.commit||'—'} ${x.commit_msg||''}</td><td>${x.triggered_by}</td><td>${x.created_at?new Date(x.created_at).toLocaleString():'—'}</td><td><a class="button" href=${href(`/${id}/deployments/${x.id}`)}>查看</a></td></tr>`)}</tbody></table></div>`}</section>
  </div>`;
}
