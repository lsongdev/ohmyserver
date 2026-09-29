import { html } from '@module/ui';
import { useEffect,useState } from 'preact/hooks';

export default function DNSService({api}){
  const [settings,setSettings]=useState(null),[status,setStatus]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false);
  const load=()=>Promise.all([api.getSettings(),api.getStatus()]).then(([config,runtime])=>{setSettings(config);setStatus(runtime)}).catch(err=>setError(err.message||'DNS 服务信息读取失败'));
  useEffect(()=>{const controller=new AbortController();Promise.all([api.getSettings({signal:controller.signal}),api.getStatus({signal:controller.signal})]).then(([config,runtime])=>{setSettings(config);setStatus(runtime)}).catch(err=>{if(err.name!=='AbortError')setError(err.message||'DNS 服务信息读取失败')});return()=>controller.abort()},[api]);
  const change=(key,value)=>setSettings({...settings,[key]:value});
  const save=async event=>{event.preventDefault();setBusy(true);setError('');try{await api.updateSettings(settings);await load()}catch(err){setError(err.message||'保存失败')}finally{setBusy(false)}};
  const action=async name=>{setBusy(true);setError('');try{await api.action(name);await load()}catch(err){setError(err.message||'服务操作失败')}finally{setBusy(false)}};
  return html`<section><div class="page-toolbar"><h2>DNS 服务配置</h2><span class=${`status-pill ${status?.runtime?.running?'running':''}`}>${status?.runtime?.running?'运行中':'已停止'}</span><button class="button" disabled=${busy} onClick=${()=>action(status?.runtime?.running?'stop':'start')}>${status?.runtime?.running?'停止':'启动'}</button><button class="button" disabled=${busy} onClick=${()=>action('reload')}>重载</button></div>
    ${error?html`<div class="notice-card error" role="alert">${error}</div>`:null}${status?.last_error?html`<div class="notice-card error">${status.last_error}</div>`:null}
    ${settings?html`<form class="dns-settings" onSubmit=${save}><div class="dns-settings-grid">
      <label class="form-field">UDP 监听<input class="input" value=${settings.udp_addr} onInput=${e=>change('udp_addr',e.currentTarget.value)}/></label><label class="form-field">TCP 监听<input class="input" value=${settings.tcp_addr} onInput=${e=>change('tcp_addr',e.currentTarget.value)}/></label>
      <label class="form-field">DoT 监听<input class="input" value=${settings.dot_addr} onInput=${e=>change('dot_addr',e.currentTarget.value)}/></label><label class="form-field">DoH 监听<input class="input" value=${settings.doh_addr} onInput=${e=>change('doh_addr',e.currentTarget.value)}/></label>
      <label class="form-field">TLS 证书文件<input class="input" value=${settings.cert_file} onInput=${e=>change('cert_file',e.currentTarget.value)}/></label><label class="form-field">TLS 私钥文件<input class="input" value=${settings.key_file} onInput=${e=>change('key_file',e.currentTarget.value)}/></label>
      <label class="form-field">上游解析器<textarea class="input" value=${settings.upstreams} onInput=${e=>change('upstreams',e.currentTarget.value)}></textarea></label>
      <label class="form-field">缓存最短 TTL<input class="input" type="number" value=${settings.cache_min_ttl} onInput=${e=>change('cache_min_ttl',Number(e.currentTarget.value))}/></label><label class="form-field">缓存最长 TTL<input class="input" type="number" value=${settings.cache_max_ttl} onInput=${e=>change('cache_max_ttl',Number(e.currentTarget.value))}/></label><label class="form-field">缓存条目上限<input class="input" type="number" value=${settings.cache_entries} onInput=${e=>change('cache_entries',Number(e.currentTarget.value))}/></label>
      <label class="form-field dns-switch"><span>启用缓存</span><input class="input dns-checkbox" type="checkbox" checked=${settings.cache_enabled} onChange=${e=>change('cache_enabled',e.currentTarget.checked)}/></label><label class="form-field dns-switch"><span>启用 DNS 服务</span><input class="input dns-checkbox" type="checkbox" checked=${settings.enabled} onChange=${e=>change('enabled',e.currentTarget.checked)}/></label>
      </div><div class="form-actions"><button class="button button-primary" disabled=${busy}>保存配置</button></div></form>`:null}
    ${status?.runtime?html`<p class="muted">UDP ${status.runtime.udp_addr||'—'} · TCP ${status.runtime.tcp_addr||'—'} · DoT ${status.runtime.dot_addr||'—'} · DoH ${status.runtime.doh_addr||'—'}</p>`:null}
  </section>`;
}
