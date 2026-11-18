import { html } from '@lsongdev/panel/html.js';
import { Field } from '@lsongdev/scripts/react/components/index.js';

const text = (key, label, options = {}) => ({ key, label, ...options });
const select = (key, label, choices) => text(key, label, { choices });
const flag = (key, label) => text(key, label, { type: 'checkbox' });
const secret = (key = 'password') => text(key, 'Password', { type: 'password' });
const port = (key, label, required = false) => text(key, label, { type: 'number', required });
const account = [text('user', 'Username', { required: true }), { ...secret('pass'), required: true }];
const email = text('email', 'Email');
const id = text('id', 'Client ID', { required: true });
const flow = text('flow', 'Flow');
const endpoint = [text('address', 'Server address', { required: true }), port('port', 'Server port', true)];
const network = select('network', 'Network', [['tcp', 'TCP'], ['udp', 'UDP'], ['tcp,udp', 'TCP and UDP']]);
const clients = fields => ({ key: 'clients', label: 'Client', fields });
const accounts = { key: 'accounts', label: 'Account', fields: account };

const socks = {
  label: 'SOCKS',
  fields: [select('auth', 'Authentication', [['noauth', 'None'], ['password', 'Username and password']]), flag('udp', 'Enable UDP')],
  list: settings => settings.auth === 'password' || settings.accounts?.length ? accounts : null,
};
const tunnel = { label: 'Tunnel', fields: [text('address', 'Destination address'), port('port', 'Destination port'), network, flag('followRedirect', 'Follow redirect')] };
const strategy = ['AsIs', 'UseIP', 'UseIPv4', 'UseIPv6', 'ForceIP', 'ForceIPv4', 'ForceIPv6'].map(value => [value, value]);

export const protocols = {
  inbounds: {
    socks,
    mixed: { ...socks, label: 'Mixed (HTTP and SOCKS)' },
    http: { label: 'HTTP', list: () => accounts },
    vless: { label: 'VLESS', defaults: { decryption: 'none' }, fields: [text('decryption', 'Decryption', { required: true })], list: () => clients([id, email, flow]) },
    vmess: { label: 'VMess', list: () => clients([id, email]) },
    trojan: { label: 'Trojan', list: () => clients([{ ...secret(), required: true }, email]) },
    shadowsocks: { label: 'Shadowsocks', fields: [text('method', 'Cipher'), secret(), network], list: () => clients([text('method', 'Cipher'), secret(), email]) },
    tunnel,
    'dokodemo-door': { ...tunnel, label: 'Dokodemo-door' },
    wireguard: { label: 'WireGuard', json: true },
    hysteria: { label: 'Hysteria', json: true },
    tun: { label: 'TUN', json: true },
  },
  outbounds: {
    freedom: { label: 'Direct (Freedom)', fields: [select('targetStrategy', 'Target strategy', strategy), text('redirect', 'Redirect address')] },
    direct: { label: 'Direct', fields: [select('targetStrategy', 'Target strategy', strategy), text('redirect', 'Redirect address')] },
    blackhole: { label: 'Block (Blackhole)', response: true },
    block: { label: 'Block', response: true },
    http: { label: 'HTTP', fields: [...endpoint, text('user', 'Username'), secret('pass')], legacy: 'servers' },
    socks: { label: 'SOCKS', fields: [...endpoint, text('user', 'Username'), secret('pass')], legacy: 'servers' },
    vless: { label: 'VLESS', defaults: { encryption: 'none' }, fields: [...endpoint, id, flow, text('encryption', 'Encryption', { required: true })], legacy: 'vnext' },
    vmess: { label: 'VMess', fields: [...endpoint, id, text('security', 'Encryption')], legacy: 'vnext' },
    trojan: { label: 'Trojan', fields: [...endpoint, { ...secret(), required: true }], legacy: 'servers' },
    shadowsocks: { label: 'Shadowsocks', fields: [...endpoint, text('method', 'Cipher', { required: true }), { ...secret(), required: true }], legacy: 'servers' },
    loopback: { label: 'Loopback', fields: [text('inboundTag', 'Inbound tag', { required: true })] },
    dns: { label: 'DNS', fields: [text('address', 'DNS server address'), port('port', 'DNS server port'), select('network', 'Network', [['tcp', 'TCP'], ['udp', 'UDP']])] },
    wireguard: { label: 'WireGuard', json: true },
    hysteria: { label: 'Hysteria', json: true },
  },
};

export function setField(object, key, value) {
  const next = { ...object };
  if (value === '') delete next[key];
  else next[key] = value;
  return next;
}

export function InputField({ field, value, onChange }) {
  const { label, required, choices, hint, type = 'text' } = field;
  const options = choices && value != null && value !== '' && !choices.some(([key]) => key === value)
    ? [...choices, [value, String(value)]] : choices;
  return html`<${Field} label=${label} required=${required} hint=${hint}>
    ${options ? html`<select class="input" value=${value ?? ''} required=${required}
      onChange=${e => onChange(e.currentTarget.value)}>
        <option value="">Default</option>
        ${options.map(([key, name]) => html`<option key=${String(key)} value=${key}>${name}</option>`)}
      </select>` : type === 'checkbox' ? html`<input type="checkbox" checked=${!!value}
        onChange=${e => onChange(e.currentTarget.checked)} />`
      : html`<input class="input" type=${type} value=${value ?? ''} required=${required}
          min=${type === 'number' ? 1 : undefined} max=${type === 'number' ? 65535 : undefined}
          autocomplete=${type === 'password' ? 'new-password' : 'off'}
          onInput=${e => {
            const input = e.currentTarget;
            onChange(type === 'number' && input.value !== '' ? input.valueAsNumber : input.value);
          }} />`}
  <//>`;
}

export function Fields({ fields, value, onChange }) {
  return html`<div class="xray-fields">${fields.map(field => html`
    <${InputField} key=${field.key} field=${field} value=${value[field.key]}
      onChange=${next => onChange(setField(value, field.key, next))} />`)}</div>`;
}

export function ObjectList({ spec, items, onChange }) {
  const { label, fields } = spec;
  return html`<div class="xray-object-list">
    ${items.map((item, index) => html`<fieldset class="xray-object" key=${index}>
      <legend>${label} ${index + 1}</legend>
      <${Fields} fields=${fields} value=${item}
        onChange=${next => onChange(items.map((entry, i) => i === index ? next : entry))} />
      <button class="button" type="button" onClick=${() => onChange(items.filter((_, i) => i !== index))}>Remove ${label.toLowerCase()} ${index + 1}</button>
    </fieldset>`)}
    <button class="button" type="button" onClick=${() => onChange([...items, {}])}>Add ${label.toLowerCase()}</button>
  </div>`;
}
