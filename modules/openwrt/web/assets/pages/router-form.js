import { html } from '@panel/shared/ui.js';
import { useEffect, useState } from 'preact/hooks';

export default function RouterForm({ api, params, navigate }) {
  const editing = Boolean(params.id);
  const [form, setForm] = useState({ name: '', host: '', port: 80, username: 'root', password: '', protocol: 'http', description: '' });
  const [error, setError] = useState('');
  useEffect(() => {
    if (!editing) return;
    const controller = new AbortController();
    api.getRouter(params.id, { signal: controller.signal }).then(result => setForm({ ...result, password: '' })).catch(e => { if (e.name !== 'AbortError') setError(e.message); });
    return () => controller.abort();
  }, [api, params.id]);
  const field = (name, label, type = 'text') => html`<label class="form-field"><span>${label}</span><input class="input" type=${type} value=${form[name] ?? ''} required=${name !== 'description' && !(editing && name === 'password')} onInput=${e => setForm({ ...form, [name]: e.currentTarget.value })} /></label>`;
  const submit = async event => {
    event.preventDefault(); setError('');
    try {
      const payload = { ...form, port: Number(form.port) };
      if (editing && !payload.password) delete payload.password;
      const result = editing ? await api.updateRouter(params.id, payload) : await api.createRouter(payload);
      navigate(`/routers/${result.id}`);
    } catch (e) { setError(e.message); }
  };
  return html`<form class="card" onSubmit=${submit}><div class="page-toolbar"><h2>${editing ? '编辑路由器' : '添加 OpenWrt 路由器'}</h2></div>
    ${error ? html`<div class="notice-card error">${error}</div>` : null}
    ${field('name', '名称')}${field('host', '主机地址')}${field('port', '端口', 'number')}${field('username', '用户名')}${field('password', editing ? '新密码（留空保持不变）' : '密码', 'password')}
    <label class="form-field"><span>协议</span><select class="input" value=${form.protocol} onChange=${e => setForm({ ...form, protocol: e.currentTarget.value })}><option value="http">HTTP</option><option value="https">HTTPS</option></select></label>
    ${field('description', '描述')}<div class="form-actions"><button class="button" type="submit">保存</button><a class="button button-secondary" href="#/openwrt">取消</a></div></form>`;
}
