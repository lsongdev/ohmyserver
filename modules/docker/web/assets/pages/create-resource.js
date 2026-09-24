import { html } from '@panel/shared/ui.js';
import { useState } from 'preact/hooks';

const returnPath = { container: '/docker/containers', volume: '/docker/volumes', network: '/docker/networks' };

export default function CreateDockerResource({ api, kind, title, navigate }) {
  const [values, setValues] = useState({ name: '', image: '', command: '', driver: '', subnet: '', gateway: '', ip_range: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  function update(field, event) { setValues(current => ({ ...current, [field]: event.currentTarget.value })); }
  async function submit(event) {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      if (kind === 'container') await api.createContainer({ name: values.name, image: values.image, command: values.command });
      if (kind === 'volume') await api.createVolume({ name: values.name, driver: values.driver });
      if (kind === 'network') await api.createNetwork({ name: values.name, driver: values.driver, subnet: values.subnet, gateway: values.gateway, ip_range: values.ip_range });
      navigate(returnPath[kind]);
    } catch (err) { setError(err.message || `${title}失败`); }
    finally { setBusy(false); }
  }
  const field = (key, label, placeholder = '', required = false) => html`<div class="form-field">
    <label for=${`docker-${key}`}>${label}</label>
    <input class="input" id=${`docker-${key}`} name=${key} value=${values[key]} placeholder=${placeholder} required=${required} onInput=${event => update(key, event)} />
  </div>`;
  return html`<div class="card"><h2>${title}</h2>
    ${error ? html`<div class="notice-card error" role="alert"><p>${error}</p></div>` : null}
    <form onSubmit=${submit}>
      ${field('name', '名称', '', true)}
      ${kind === 'container' ? html`${field('image', '镜像', 'alpine:latest', true)}${field('command', '命令（可选）', 'tail -f /dev/null')}` : null}
      ${kind === 'volume' ? field('driver', '驱动（留空使用默认）', 'local') : null}
      ${kind === 'network' ? html`${field('driver', '驱动', 'bridge')}${field('subnet', '子网（可选）', '172.20.0.0/16')}${field('gateway', '网关（可选）', '172.20.0.1')}${field('ip_range', 'IP 范围（可选）', '172.20.10.0/24')}` : null}
      <div class="form-actions"><button class="button" type="submit" disabled=${busy}>${busy ? '正在创建…' : '创建'}</button><button class="button button-secondary" type="button" onclick=${() => navigate(returnPath[kind])}>取消</button></div>
    </form>
  </div>`;
}
