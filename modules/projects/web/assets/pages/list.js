import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const empty={name:'',description:'',repo:'',webhook_branch:'main',webhook_secret:''};
export default function Projects({api,navigate}){
  const [items,setItems]=useState([]),[form,setForm]=useState(empty),[editing,setEditing]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=async()=>{setError('');try{setItems(await api.list());}catch(e){setError(e.message);}};
  useEffect(()=>{load();},[api]);
  const submit=async event=>{event.preventDefault();setBusy(true);setError('');try{await api.update(editing,form);setForm(empty);setEditing(null);await load();}catch(e){setError(e.message);}finally{setBusy(false);}};
  const edit=item=>{setEditing(item.id);setForm({name:item.name,description:item.description||'',repo:item.repo||'',webhook_branch:item.webhook_branch||'main',webhook_secret:''});};
  const remove=async item=>{if(!confirm(`删除项目“${item.name}”？其部署记录也会随项目删除。`))return;setBusy(true);try{await api.remove(item.id);await load();}catch(e){setError(e.message);}finally{setBusy(false);}};
  return html`<div class="projects-page">
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${editing ? html`<section class="card"><h2>编辑项目</h2><form onSubmit=${submit}>
      <div class="form-group"><label for="project-name">名称</label><input class="input" id="project-name" required value=${form.name} onInput=${e=>setForm({...form,name:e.currentTarget.value})} /></div>
      <div class="form-group"><label for="project-repo">Git 仓库</label><input class="input" id="project-repo" value=${form.repo} placeholder="https://github.com/owner/repo.git" onInput=${e=>setForm({...form,repo:e.currentTarget.value})} /></div>
      <div class="form-group"><label for="project-branch">Webhook 分支</label><input class="input" id="project-branch" value=${form.webhook_branch} onInput=${e=>setForm({...form,webhook_branch:e.currentTarget.value})} /></div>
      <div class="form-group"><label for="project-secret">Webhook Secret（留空则${editing?'保持不变':'自动生成'}）</label><input class="input" id="project-secret" type="password" autocomplete="new-password" value=${form.webhook_secret} onInput=${e=>setForm({...form,webhook_secret:e.currentTarget.value})} /></div>
      <div class="form-group"><label for="project-description">说明</label><textarea class="input" id="project-description" rows="3" value=${form.description} onInput=${e=>setForm({...form,description:e.currentTarget.value})}></textarea></div>
      <div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy?'保存中…':'保存修改'}</button><button class="button" type="button" onclick=${()=>{setEditing(null);setForm(empty);}}>取消</button></div>
    </form></section>` : null}
    <section class="card"><div class="page-toolbar"><h2>项目列表</h2><button class="button button-primary" onclick=${()=>navigate('/new')}>新建项目</button></div>${!items.length?html`<p class="muted">还没有项目。</p>`:html`<div class="table-wrap"><table class="table"><thead><tr><th>名称</th><th>仓库</th><th>分支</th><th>操作</th></tr></thead><tbody>${items.map(item=>html`<tr key=${item.id}><td><strong>${item.name}</strong>${item.description ? html`<div class="muted">${item.description}</div>` : null}</td><td>${item.repo||'—'}</td><td>${item.webhook_branch||'main'}</td><td><div class="form-actions"><a class="button" href=${`#/projects/${item.id}`}>部署记录</a><button class="button" disabled=${busy} onclick=${()=>edit(item)}>编辑</button><button class="button button-danger" disabled=${busy} onclick=${()=>remove(item)}>删除</button></div></td></tr>`)}</tbody></table></div>`}</section>
  </div>`;
}
