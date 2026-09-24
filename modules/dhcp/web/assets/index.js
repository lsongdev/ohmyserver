import { html, render, useEffect, useState } from 'https://lsong.org/scripts/react/index.js';

const base = document.querySelector('meta[name="api-base"]')?.content || '/v1/dhcp';
async function request(path, options = {}) {
  const response = await fetch(`${base}${path}`, { ...options, headers: { 'Content-Type': 'application/json', ...options.headers } });
  const body = response.status === 204 ? {} : await response.json();
  if (!response.ok) throw new Error(body.error?.message || `HTTP ${response.status}`);
  return body.data;
}

function App() {
  const [tab, setTab] = useState('settings');
  const [settings, setSettings] = useState(null);
  const [status, setStatus] = useState(null);
  const [leases, setLeases] = useState([]);
  const [staticLeases, setStaticLeases] = useState([]);
  const [draft, setDraft] = useState({ mac: '', ip: '', hostname: '', description: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  async function load() {
    try {
      const [config, runtime, dynamic, fixed] = await Promise.all([
        request('/settings'), request('/status'), request('/leases'), request('/static-leases'),
      ]);
      setSettings(config); setStatus(runtime); setLeases(dynamic); setStaticLeases(fixed); setError('');
    } catch (err) { setError(err.message || '读取 DHCP 数据失败'); }
  }
  useEffect(() => { load(); }, []);
  const set = (key, value) => setSettings(current => ({ ...current, [key]: value }));
  async function save(event) {
    event.preventDefault(); setBusy(true); setError('');
    try { await request('/settings', { method: 'PUT', body: JSON.stringify(settings) }); await load(); }
    catch (err) { setError(err.message || '保存配置失败'); } finally { setBusy(false); }
  }
  async function action(name) {
    setBusy(true); setError('');
    try { await request('/action', { method: 'POST', body: JSON.stringify({ action: name }) }); await load(); }
    catch (err) { setError(err.message || '服务操作失败'); } finally { setBusy(false); }
  }
  async function saveLease(event) {
    event.preventDefault(); setBusy(true); setError('');
    try { await request('/static-leases', { method: 'POST', body: JSON.stringify(draft) }); setDraft({ mac: '', ip: '', hostname: '', description: '' }); await load(); }
    catch (err) { setError(err.message || '添加静态租约失败'); } finally { setBusy(false); }
  }
  async function remove(path) {
    setBusy(true); setError('');
    try { await request(path, { method: 'DELETE' }); await load(); }
    catch (err) { setError(err.message || '删除失败'); } finally { setBusy(false); }
  }
  const field = (label, key, type = 'text') => html`<label class="form-group">${label}<input class="input" type=${type} value=${settings?.[key] ?? ''} onInput=${event => set(key, type === 'number' ? Number(event.currentTarget.value) : event.currentTarget.value)} /></label>`;
  return html`<section class="section">
    <h1>DHCP 服务</h1>
    ${error && html`<p role="alert">${error}</p>`}
    <nav class="form-actions">${[['settings','配置'],['leases','动态租约'],['static','静态租约']].map(([id,label]) => html`<button class="button" onClick=${() => setTab(id)}>${label}</button>`)}</nav>
    ${status && html`<p>运行状态：${status.runtime?.running ? '运行中' : '已停止'} · ${status.runtime?.listen_addr || '未监听'} ${status.last_error || ''}</p>`}
    ${tab === 'settings' && settings && html`<form onSubmit=${save}>
      <label class="form-group"><input class="input" type="checkbox" checked=${settings.enabled} onChange=${event => set('enabled', event.currentTarget.checked)} />启用 DHCP</label>
      ${field('监听地址', 'listen_addr')}${field('服务器 IPv4', 'server_ip')}${field('子网掩码', 'subnet_mask')}
      ${field('地址池起始', 'pool_start')}${field('地址池结束', 'pool_end')}${field('网关', 'router')}
      ${field('DNS 服务器', 'dns_servers')}${field('NTP 服务器', 'ntp_servers')}${field('域名', 'domain_name')}
      ${field('租约秒数', 'lease_seconds', 'number')}${field('续租秒数', 'renewal_seconds', 'number')}${field('重绑定秒数', 'rebinding_seconds', 'number')}
      <div class="form-actions"><button class="button button-primary" disabled=${busy}>保存配置</button><button class="button" type="button" disabled=${busy} onClick=${() => action('start')}>启动</button><button class="button" type="button" disabled=${busy} onClick=${() => action('stop')}>停止</button><button class="button" type="button" disabled=${busy} onClick=${() => action('reload')}>重载</button></div>
    </form>`}
    ${tab === 'leases' && html`<table class="table"><thead><tr><th>IP</th><th>MAC</th><th>主机名</th><th>状态</th><th>过期时间</th><th></th></tr></thead><tbody>${leases.map(item => html`<tr><td>${item.ip}</td><td>${item.mac}</td><td>${item.hostname}</td><td>${item.state}</td><td>${item.expires_at}</td><td><button class="button button-danger" onClick=${() => remove(`/leases/${item.id}`)}>删除</button></td></tr>`)}</tbody></table>`}
    ${tab === 'static' && html`<form onSubmit=${saveLease}><h2>添加静态租约</h2><input class="input" placeholder="MAC 地址" value=${draft.mac} onInput=${event => setDraft({...draft,mac:event.currentTarget.value})} required /><input class="input" placeholder="IPv4 地址" value=${draft.ip} onInput=${event => setDraft({...draft,ip:event.currentTarget.value})} required /><input class="input" placeholder="主机名" value=${draft.hostname} onInput=${event => setDraft({...draft,hostname:event.currentTarget.value})} /><input class="input" placeholder="描述" value=${draft.description} onInput=${event => setDraft({...draft,description:event.currentTarget.value})} /><button class="button button-primary" disabled=${busy}>添加租约</button></form><table class="table"><thead><tr><th>IP</th><th>MAC</th><th>主机名</th><th>描述</th><th></th></tr></thead><tbody>${staticLeases.map(item => html`<tr><td>${item.ip}</td><td>${item.mac}</td><td>${item.hostname}</td><td>${item.description}</td><td><button class="button button-danger" onClick=${() => remove(`/static-leases/${item.id}`)}>删除</button></td></tr>`)}</tbody></table>`}
  </section>`;
}
render(html`<${App} />`, document.getElementById('app'));
