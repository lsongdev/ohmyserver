import { useEffect, useState } from 'preact/hooks';
import { html } from '@module/ui';

export default function DeploymentDetail({api,params,href}){
  const id=Number(params.id),deploymentId=Number(params.deploymentId),[deployment,setDeployment]=useState(null),[logs,setLogs]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=async()=>{try{const [d,l]=await Promise.all([api.deployment(deploymentId),api.logs(deploymentId)]);setDeployment(d);setLogs(l);return l;}catch(e){setError(e.message);return [];}};
  useEffect(()=>{let source;let active=true;load().then(initial=>{if(!active)return;const lastID=initial.reduce((max,item)=>Math.max(max,item.id),0);source=new EventSource(api.eventsURL(id,deploymentId,lastID));source.addEventListener('status',event=>{const value=JSON.parse(event.data);setDeployment(current=>current?{...current,status:value.status}:current);});source.addEventListener('log',event=>{const item=JSON.parse(event.data);setLogs(current=>current.some(x=>x.id===item.id)?current:[...current,{...item,created_at:new Date().toISOString()}]);});source.addEventListener('complete',event=>{const value=JSON.parse(event.data);setDeployment(current=>current?{...current,status:value.status}:current);source.close();});source.addEventListener('problem',event=>{try{setError(JSON.parse(event.data).message);}catch{setError('部署日志流中断');}source.close();});});return()=>{active=false;source?.close();};},[deploymentId,api,id]);
  const cancel=async()=>{if(!confirm('确定取消这个部署？'))return;setBusy(true);try{await api.cancel(id,deploymentId);await load();}catch(e){setError(e.message);}finally{setBusy(false);}};
  return html`<div class="projects-page"><a href=${href(`/${id}`)}>← 返回部署列表</a>
    ${error&&html`<div class="notice-card error" role="alert">${error}</div>`}
    <section class="card"><h2>部署 #${deploymentId}</h2><dl class="about-grid"><div><dt>状态</dt><dd>${deployment?.status||'读取中…'}</dd></div><div><dt>提交</dt><dd>${deployment?.commit||'—'} ${deployment?.commit_msg||''}</dd></div><div><dt>触发方式</dt><dd>${deployment?.triggered_by||'—'}</dd></div><div><dt>镜像</dt><dd>${deployment?.docker_image||'—'}</dd></div></dl>${deployment&&['pending','running'].includes(deployment.status)&&html`<button class="button button-danger" disabled=${busy} onclick=${cancel}>取消部署</button>`}</section>
    <section class="card"><h2>构建日志</h2><pre class="deployment-log">${logs.map(item=>`[${new Date(item.created_at).toLocaleTimeString()}] ${item.message}`).join('\n')||'等待日志…'}</pre></section>
  </div>`;
}
