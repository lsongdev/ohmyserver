import { html } from '@panel/shared/ui.js';
import { useEffect,useState } from 'preact/hooks';

export default function Filters({api,navigate}){
  const [subscriptions,setSubscriptions]=useState([]),[rules,setRules]=useState([]),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>Promise.all([api.listSubscriptions(),api.listRules()]).then(([subs,filters])=>{setSubscriptions(subs);setRules(filters)}).catch(err=>setError(err.message||'过滤配置读取失败'));
  useEffect(()=>{const controller=new AbortController();Promise.all([api.listSubscriptions({signal:controller.signal}),api.listRules({signal:controller.signal})]).then(([subs,filters])=>{setSubscriptions(subs);setRules(filters)}).catch(err=>{if(err.name!=='AbortError')setError(err.message||'过滤配置读取失败')});return()=>controller.abort()},[api]);
  const run=async action=>{setBusy(true);setError('');try{await action();await load()}catch(err){setError(err.message||'操作失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>DNS 过滤与订阅</h2><button class="button" disabled=${busy} onClick=${load}>刷新</button></div>${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}
    <div class="page-toolbar"><h3>订阅源</h3><button class="button button-primary" onClick=${()=>navigate('/dns/filters/subscriptions/new')}>添加订阅</button></div>
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>名称</th><th>类型</th><th>状态</th><th>最近更新</th><th>错误</th><th>操作</th></tr></thead><tbody>${subscriptions.map(item=>html`<tr key=${item.id}><td>${item.name}</td><td>${item.kind}</td><td>${item.enabled?'启用':'停用'}</td><td>${item.last_updated||'—'}</td><td>${item.last_error}</td><td class="table-actions"><button class="button" disabled=${busy} onClick=${()=>run(()=>api.subscriptionAction(item.id,'refresh'))}>刷新</button><button class="button" disabled=${busy} onClick=${()=>run(()=>api.subscriptionAction(item.id,'toggle'))}>${item.enabled?'停用':'启用'}</button><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.subscriptionAction(item.id,'delete'))}>删除</button></td></tr>`)}</tbody></table>${!subscriptions.length?html`<div class="empty-state">暂无订阅源</div>`:null}</div>
    <div class="page-toolbar"><h3>自定义规则</h3><button class="button button-primary" onClick=${()=>navigate('/dns/filters/rules/new')}>添加规则</button></div>
    <div class="data-table-wrap"><table class="data-table"><thead><tr><th>规则</th><th>状态</th><th>操作</th></tr></thead><tbody>${rules.map(item=>html`<tr key=${item.id}><td class="mono">${item.rule}</td><td>${item.enabled?'启用':'停用'}</td><td class="table-actions"><button class="button" disabled=${busy} onClick=${()=>run(()=>api.ruleAction(item.id,'toggle'))}>${item.enabled?'停用':'启用'}</button><button class="button button-danger" disabled=${busy} onClick=${()=>run(()=>api.ruleAction(item.id,'delete'))}>删除</button></td></tr>`)}</tbody></table>${!rules.length?html`<div class="empty-state">暂无自定义规则</div>`:null}</div>
  </section>`;
}
