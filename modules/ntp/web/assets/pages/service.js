import { html } from '@module/ui';
import { useEffect, useState } from 'preact/hooks';

export default function NTPService({api}) {
  const [settings,setSettings]=useState(null),[status,setStatus]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>Promise.all([api.getSettings(),api.getStatus()]).then(([config,runtime])=>{setSettings(config);setStatus(runtime);}).catch(err=>setError(err.message||'NTP 信息读取失败'));
  useEffect(()=>{const controller=new AbortController();Promise.all([api.getSettings({signal:controller.signal}),api.getStatus({signal:controller.signal})]).then(([config,runtime])=>{setSettings(config);setStatus(runtime);}).catch(err=>{if(err.name!=='AbortError')setError(err.message||'NTP 信息读取失败')});return()=>controller.abort()},[api]);
  const change=(key,value)=>setSettings({...settings,[key]:value});
  const save=async event=>{event.preventDefault();setBusy(true);setError('');try{await api.updateSettings(settings);await load()}catch(err){setError(err.message||'保存失败')}finally{setBusy(false)}};
  const act=async action=>{setBusy(true);setError('');try{await api.action(action);await load()}catch(err){setError(err.message||'服务操作失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>NTP 服务</h2><span class=${`status-pill ${status?.runtime?.running?'running':''}`}>${status?.runtime?.running?'运行中':'已停止'}</span><button class="button" disabled=${busy} onClick=${()=>act(status?.runtime?.running?'stop':'start')}>${status?.runtime?.running?'停止':'启动'}</button><button class="button" disabled=${busy} onClick=${()=>act('reload')}>重载</button></div>
    ${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}${status?.last_error?html`<div class="notice-card error">${status.last_error}</div>`:null}
    ${settings?html`<form class="ntp-settings" onSubmit=${save}><label class="form-field">监听地址<input class="input" value=${settings.listen_addr} onInput=${e=>change('listen_addr',e.currentTarget.value)}/></label><label class="form-field">Stratum<input class="input" type="number" min="1" max="15" value=${settings.stratum} onInput=${e=>change('stratum',Number(e.currentTarget.value))}/></label><label class="form-field">Reference ID<input class="input" maxlength="4" value=${settings.reference_id} onInput=${e=>change('reference_id',e.currentTarget.value)}/></label><label class="form-field ntp-enabled"><span>启用 NTP</span><input class="input ntp-checkbox" type="checkbox" checked=${settings.enabled} onChange=${e=>change('enabled',e.currentTarget.checked)}/></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>保存配置</button></div></form>`:null}
    ${status?.runtime?html`<p class="muted">监听：${status.runtime.listen_addr||'—'} · 请求数：${status.runtime.requests} · 启动时间：${status.runtime.started_at||'—'}</p>`:null}
  </section>`;
}
