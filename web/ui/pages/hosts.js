import { html, useState } from '../lib/html.js';
import { useAction, useResource } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Badge, Field, Modal, confirmDanger, toast } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { ago } from '../lib/format.js';

export const AUTH = { password: '密码', key: '私钥', panel: '面板公钥' };

export const target = h => `${h.user}@${h.address}${h.port === 22 ? '' : `:${h.port}`}`;

export default function Hosts() {
  const { api, href } = usePanel();
  const list = useResource(signal => api.get('/hosts', { signal }), []);
  const action = useAction();
  const [editing, setEditing] = useState(null); // a host, or {} for a new one

  const remove = h => {
    if (!confirmDanger(`删除主机 ${h.name}？`)) return;
    action.run(async () => { await api.del(`/hosts/${h.id}`); list.reload(); });
  };

  return html`<${Page} title="主机" subtitle="保存 SSH 主机，在浏览器里打开终端。"
    actions=${html`<button class="btn btn-primary" onClick=${() => setEditing({})}>添加主机</button>`}>
    <${Notice} error=${list.error || action.error} />
    ${list.data === undefined ? (list.error ? null : html`<${Loading} />`) : !list.data.length ? html`<${Empty}>还没有主机。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>名称</th><th>地址</th><th>认证</th><th>主机密钥</th><th>上次连接</th><th class="actions"></th></tr></thead>
        <tbody>${list.data.map(h => html`<tr key=${h.id}>
          <td><div class="cell-main">${h.name}</div>${h.description ? html`<div class="cell-sub">${h.description}</div>` : null}</td>
          <td class="mono">${target(h)}</td>
          <td><${Badge}>${AUTH[h.auth] || h.auth}<//></td>
          <td>${h.host_key_sha256
            ? html`<span class="mono cell-sub" title=${`${h.host_key_type} ${h.host_key_sha256}`}>${h.host_key_sha256.slice(0, 19)}…</span>`
            : html`<span class="muted">首次连接时记录</span>`}</td>
          <td class="nowrap muted">${h.last_connected_at ? ago(h.last_connected_at) : '—'}</td>
          <td class="actions"><span class="btn-group">
            <a class="btn btn-sm btn-primary" href=${href(`/hosts/${h.id}/terminal`)}>终端</a>
            <button class="btn btn-sm" onClick=${() => setEditing(h)}>编辑</button>
            <button class="btn btn-sm btn-danger" disabled=${action.busy} onClick=${() => remove(h)}>删除</button>
          </span></td>
        </tr>`)}</tbody>
      </table></div>`}
    ${editing ? html`<${HostForm} host=${editing} onClose=${() => setEditing(null)} onSaved=${() => { setEditing(null); list.reload(); }} />` : null}
  <//>`;
}

function HostForm({ host, onClose, onSaved }) {
  const { api, href } = usePanel();
  const isNew = !host.id;
  const [h, setH] = useState({
    name: host.name || '', address: host.address || '', port: host.port || 22, user: host.user || 'root',
    auth: host.auth || 'password', password: '', private_key: '', passphrase: '', description: host.description || '',
  });
  const save = useAction();
  const test = useAction();
  const [tested, setTested] = useState(null);
  const set = patch => { setH(prev => ({ ...prev, ...patch })); setTested(null); };
  const submit = e => {
    e.preventDefault();
    save.run(async () => {
      if (isNew) await api.post('/hosts', h);
      else await api.put(`/hosts/${host.id}`, h);
      onSaved();
    });
  };
  const runTest = () => test.run(async () => {
    setTested(await api.post('/test', { id: host.id || 0, ...h }));
    toast('连接成功', 'success');
  });
  // Stored secrets are not sent back: leaving a field empty keeps them.
  const keep = !isNew && host.auth === h.auth;
  return html`<${Modal} wide title=${isNew ? '添加主机' : `编辑 ${host.name}`} onClose=${onClose} footer=${html`
    <button class="btn" type="button" disabled=${test.busy} onClick=${runTest}>${test.busy ? '连接中…' : '测试连接'}</button>
    <span class="spacer"></span>
    <button class="btn" type="button" onClick=${onClose}>取消</button>
    <button class="btn btn-primary" type="submit" form="host-form" disabled=${save.busy}>${isNew ? '添加' : '保存'}</button>`}>
    <form id="host-form" class="form" onSubmit=${submit}>
      <${Notice} error=${save.error || test.error} />
      ${tested ? html`<${Notice} tone="info"><span class="test-result">连接成功 · ${tested.host_key_type}
        <span class="mono">${tested.host_key_sha256}</span>${tested.pinned ? '' : '（保存后首次连接时记录）'}</span><//>` : null}
      <div class="form-grid">
        <${Field} label="名称" required>
          <input class="input" required value=${h.name} onInput=${e => set({ name: e.currentTarget.value })} placeholder="nas" autofocus />
        <//>
        <${Field} label="说明">
          <input class="input" value=${h.description} onInput=${e => set({ description: e.currentTarget.value })} />
        <//>
        <${Field} label="地址" required>
          <input class="input mono" required value=${h.address} onInput=${e => set({ address: e.currentTarget.value })} placeholder="192.168.1.10" />
        <//>
        <${Field} label="端口">
          <input class="input" type="number" min="1" max="65535" value=${h.port} onInput=${e => set({ port: Number(e.currentTarget.value) })} />
        <//>
        <${Field} label="用户" required>
          <input class="input mono" required value=${h.user} onInput=${e => set({ user: e.currentTarget.value })} />
        <//>
      </div>
      <fieldset class="field"><span class="field-label">认证方式</span><div class="choices">
        ${Object.entries(AUTH).map(([k, label]) => html`<label class="check" key=${k}>
          <input type="radio" name="auth" checked=${h.auth === k} onChange=${() => set({ auth: k })} />${label}</label>`)}
      </div></fieldset>
      ${h.auth === 'password' ? html`<${Field} label="密码" required=${!keep} hint=${keep && host.has_password ? '留空则保留已保存的密码' : null}>
        <input class="input" type="password" autocomplete="new-password" required=${!keep} value=${h.password} onInput=${e => set({ password: e.currentTarget.value })} />
      <//>` : null}
      ${h.auth === 'key' ? html`
        <${Field} label="私钥" required=${!keep} hint=${keep && host.has_key ? '留空则保留已保存的私钥' : 'OpenSSH 或 PEM 格式'}>
          <textarea class="input" rows="6" spellcheck="false" required=${!keep} value=${h.private_key}
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" onInput=${e => set({ private_key: e.currentTarget.value })} />
        <//>
        <${Field} label="私钥密码" hint="私钥加密时填写">
          <input class="input" type="password" autocomplete="new-password" value=${h.passphrase} onInput=${e => set({ passphrase: e.currentTarget.value })} />
        <//>` : null}
      ${h.auth === 'panel' ? html`<${Notice} tone="info">把<a href=${href('/key')}>面板公钥</a>加入目标主机的 <code>~/.ssh/authorized_keys</code>。<//>` : null}
    </form>
  <//>`;
}
