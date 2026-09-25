import { useEffect, useState } from 'preact/hooks';
import { html } from '@panel/shared/ui.js';

const blank = { name: '', host: '', port: 22, username: '', auth_type: 'password', password: '', private_key: '', description: '', enabled: true };
export default function ServerForm({ api, params, navigate }) {
  const editing = Boolean(params.id);
  const [form, setForm] = useState(blank), [error, setError] = useState(''), [message, setMessage] = useState(''), [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!editing) return;
    const controller = new AbortController();
    api.get(params.id, { signal: controller.signal }).then(value => setForm({ ...blank, ...value, password: '', private_key: '' }))
      .catch(e => { if (e.name !== 'AbortError') setError(e.message || '读取 SSH 配置失败'); });
    return () => controller.abort();
  }, [api, editing, params.id]);
  const change = (key, value) => setForm(current => ({ ...current, [key]: value }));
  const submit = async event => {
    event.preventDefault(); setBusy(true); setError(''); setMessage('');
    try {
      const payload = { ...form, port: Number(form.port) };
      if (editing) await api.update(params.id, payload); else await api.create(payload);
      navigate('/servers');
    } catch (e) { setError(e.message || '保存失败'); } finally { setBusy(false); }
  };
  const test = async () => { setBusy(true); setError(''); setMessage(''); try { const result = await api.test({ ...form, port: Number(form.port) }); setMessage(result.message || '连接成功'); } catch (e) { setError(e.message || '连接测试失败'); } finally { setBusy(false); } };
  return html`<section class="card"><div class="page-toolbar"><h2>${editing ? '编辑 SSH 服务器' : '添加 SSH 服务器'}</h2><button class="button" onClick=${() => navigate('/servers')}>返回列表</button></div>
    ${message && html`<div class="notice-card" role="status">${message}</div>`}${error && html`<div class="notice-card error" role="alert">${error}</div>`}
    <form class="form-grid" onSubmit=${submit}><label class="form-group">名称<input class="input" required value=${form.name} onInput=${e => change('name', e.currentTarget.value)}/></label><label class="form-group">主机<input class="input" required value=${form.host} onInput=${e => change('host', e.currentTarget.value)}/></label><label class="form-group">端口<input class="input" type="number" min="1" max="65535" value=${form.port} onInput=${e => change('port', e.currentTarget.value)}/></label><label class="form-group">用户名<input class="input" required value=${form.username} onInput=${e => change('username', e.currentTarget.value)}/></label><label class="form-group">认证方式<select class="input" value=${form.auth_type} onChange=${e => change('auth_type', e.currentTarget.value)}><option value="password">密码</option><option value="key">私钥</option></select></label>
      ${form.auth_type === 'password' ? html`<label class="form-group">密码<input class="input" type="password" required=${!editing} placeholder=${editing ? '留空保留当前密码' : ''} value=${form.password} onInput=${e => change('password', e.currentTarget.value)}/></label>` : html`<label class="form-group">私钥<textarea class="input" required=${!editing} placeholder=${editing ? '留空保留当前私钥' : ''} value=${form.private_key} onInput=${e => change('private_key', e.currentTarget.value)}></textarea></label>`}
      <label class="form-group">描述<input class="input" value=${form.description} onInput=${e => change('description', e.currentTarget.value)}/></label><label class="form-group"><span>启用</span><input class="input" type="checkbox" checked=${form.enabled} onChange=${e => change('enabled', e.currentTarget.checked)}/></label><div class="form-actions"><button class="button button-primary" disabled=${busy}>${busy ? '保存中…' : editing ? '保存更改' : '添加服务器'}</button><button class="button" type="button" disabled=${busy} onClick=${test}>测试连接</button><button class="button" type="button" disabled=${busy} onClick=${() => navigate('/servers')}>取消</button></div>
    </form>
  </section>`;
}
