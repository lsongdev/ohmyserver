import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function Overview({ api }) {
  const [settings, setSettings] = useState(null);
  const [status, setStatus] = useState(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const load = () => Promise.all([api.getSettings(), api.getStatus()]).then(([config, runtime]) => { setSettings(config); setStatus(runtime); }).catch(err => setError(err.message || 'DHCP 状态读取失败'));
  useEffect(() => {
    const controller = new AbortController();
    Promise.all([api.getSettings({signal:controller.signal}),api.getStatus({signal:controller.signal})]).then(([config,runtime])=>{setSettings(config);setStatus(runtime);}).catch(err => { if(err.name !== 'AbortError') setError(err.message || 'DHCP 状态读取失败'); });
    return () => controller.abort();
  }, [api]);
  const save = async event => { event.preventDefault(); setBusy(true); setError(''); try { await api.updateSettings(settings); await load(); } catch(err) { setError(err.message || '保存失败'); } finally { setBusy(false); } };
  const action = async name => { setBusy(true); setError(''); try { setStatus(await api.action(name)); await load(); } catch(err) { setError(err.message || '操作失败'); } finally { setBusy(false); } };
  const change = (key,value) => setSettings({...settings,[key]:value});
  return html`<section><div class="page-toolbar"><h2>DHCP 配置</h2><span class=${`status-pill ${status?.runtime?.running ? 'running' : ''}`}>${status?.runtime?.running ? '运行中' : '已停止'}</span>
    <button class="button" disabled=${busy} onClick=${()=>action(status?.runtime?.running?'stop':'start')}>${status?.runtime?.running?'停止':'启动'}</button><button class="button" disabled=${busy} onClick=${()=>action('reload')}>重载</button></div>
    ${error ? html`<div class="notice-card error" role="alert">${error}</div>` : null}
    ${status?.last_error ? html`<div class="notice-card error">${status.last_error}</div>` : null}
    ${settings ? html`<form class="dhcp-settings" onSubmit=${save}><div class="dhcp-settings-grid">
      <label class="form-field">监听地址<input class="input" value=${settings.listen_addr} onInput=${e=>change('listen_addr',e.currentTarget.value)}/></label>
      <label class="form-field">服务端地址<input class="input" value=${settings.server_ip} onInput=${e=>change('server_ip',e.currentTarget.value)}/></label>
      <label class="form-field">子网掩码<input class="input" value=${settings.subnet_mask} onInput=${e=>change('subnet_mask',e.currentTarget.value)}/></label>
      <label class="form-field">地址池起始<input class="input" value=${settings.pool_start} onInput=${e=>change('pool_start',e.currentTarget.value)}/></label>
      <label class="form-field">地址池结束<input class="input" value=${settings.pool_end} onInput=${e=>change('pool_end',e.currentTarget.value)}/></label>
      <label class="form-field">网关<input class="input" value=${settings.router} onInput=${e=>change('router',e.currentTarget.value)}/></label>
      <label class="form-field">DNS 服务器<textarea class="input" value=${settings.dns_servers} onInput=${e=>change('dns_servers',e.currentTarget.value)}></textarea></label>
      <label class="form-field">NTP 服务器<textarea class="input" value=${settings.ntp_servers} onInput=${e=>change('ntp_servers',e.currentTarget.value)}></textarea></label>
      <label class="form-field">域名<input class="input" value=${settings.domain_name} onInput=${e=>change('domain_name',e.currentTarget.value)}/></label>
      <label class="form-field">租约秒数<input class="input" type="number" value=${settings.lease_seconds} onInput=${e=>change('lease_seconds',Number(e.currentTarget.value))}/></label>
      <label class="form-field">续租秒数<input class="input" type="number" value=${settings.renewal_seconds} onInput=${e=>change('renewal_seconds',Number(e.currentTarget.value))}/></label>
      <label class="form-field">重绑定秒数<input class="input" type="number" value=${settings.rebinding_seconds} onInput=${e=>change('rebinding_seconds',Number(e.currentTarget.value))}/></label>
      <label class="form-field dhcp-enabled"><span>启用 DHCP</span><input class="input dhcp-checkbox" type="checkbox" checked=${settings.enabled} onChange=${e=>change('enabled',e.currentTarget.checked)}/></label>
      </div><div class="form-actions"><button class="button button-primary" type="submit" disabled=${busy}>保存配置</button></div></form>` : null}
  </section>`;
}
