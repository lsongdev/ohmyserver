import { html, useState } from '@lsongdev/panel/html.js';
import { Field } from '@lsongdev/scripts/react/components/index.js';
import { FormSection } from '@lsongdev/panel/forms.js';
import { format } from '../xray.js';
import { Fields, InputField, ObjectList, protocols, setField } from './proxy-fields.js';

const tag = { key: 'tag', label: 'Tag' };
const listener = [
  { key: 'listen', label: 'Listen address', hint: 'Use 127.0.0.1 for local access only.' },
  { key: 'port', label: 'Listen port', hint: 'Accepts a port, range, or environment reference. Leave empty for Unix sockets.' },
];
const transportFields = [
  { key: 'network', label: 'Transport', choices: [['raw', 'Raw TCP'], ['ws', 'WebSocket'], ['grpc', 'gRPC'], ['xhttp', 'XHTTP'], ['httpupgrade', 'HTTP Upgrade'], ['kcp', 'mKCP']] },
  { key: 'security', label: 'Transport security', choices: [['none', 'None'], ['tls', 'TLS'], ['reality', 'REALITY']] },
];

const isObject = value => value != null && typeof value === 'object' && !Array.isArray(value);

export function parseProxy(text) {
  const data = JSON.parse(text);
  if (!isObject(data)) throw new Error('Enter a JSON object.');
  return data;
}

function Transport({ value, change, inbound }) {
  const stream = value.streamSettings || {};
  const update = next => change(setField(value, 'streamSettings', next));
  const ws = stream.network === 'ws';
  const tls = stream.security === 'tls';
  const nested = (section, field) => html`<${InputField} field=${field} value=${stream[section]?.[field.key]}
    onChange=${next => update(setField(stream, section, setField(stream[section], field.key, next)))} />`;
  return html`<${FormSection} title="Transport">
    <${Fields} fields=${transportFields} value=${stream} onChange=${update} />
    <div class="xray-fields">
      ${ws && nested('wsSettings', { key: 'path', label: 'WebSocket path' })}
      ${stream.network === 'grpc' && nested('grpcSettings', { key: 'serviceName', label: 'gRPC service name' })}
      ${tls && !inbound && nested('tlsSettings', { key: 'serverName', label: 'TLS server name' })}
    </div>
    ${(stream.security === 'reality' || tls && inbound) && html`<p>Use JSON to configure ${tls ? 'TLS certificates' : 'REALITY settings'}.</p>`}
  <//>`;
}

export default function ProxyFields({ sectionKey, value, change, raw, changeRaw, error, mode, setMode }) {
  const [protocolSettings, setProtocolSettings] = useState({});
  const inbound = sectionKey === 'inbounds';
  const options = protocols[sectionKey];
  const spec = Object.hasOwn(options, value.protocol) ? options[value.protocol] : undefined;
  const settings = value.settings || {};
  const objectList = spec?.list?.(settings);
  const legacy = spec?.legacy && settings[spec.legacy] != null && settings.address == null;
  const editSettings = next => change(setField(value, 'settings', next));
  const editProtocol = protocol => {
    setProtocolSettings({ ...protocolSettings, [value.protocol]: settings });
    change({ ...value, protocol, settings: Object.hasOwn(protocolSettings, protocol) ? protocolSettings[protocol] : { ...options[protocol]?.defaults } });
  };
  const editResponse = type => editSettings(setField(settings, 'response',
    Object.keys(settings.response || {}).length || type ? setField(settings.response, 'type', type) : ''));
  const choices = Object.entries(options).map(([key, spec]) => [key, spec.label]);

  return html`
    ${mode === 'json' ? html`
      <${Field} label="Entry configuration (JSON)" error=${error} required>
        <textarea class="input xray-json" rows="18" required spellcheck="false" value=${raw}
          onInput=${e => changeRaw(e.currentTarget.value)} />
      <//>` : html`
      <${FormSection} title=${inbound ? 'Listener' : 'Outbound'}>
        <div class="xray-fields">
          <${InputField} field=${{ label: 'Protocol', choices, required: true }} value=${value.protocol} onChange=${editProtocol} />
          <${InputField} field=${tag} value=${value.tag} onChange=${next => change(setField(value, 'tag', next))} />
        </div>
        ${inbound && value.protocol !== 'tun' && html`<div class="xray-fields">
          <${InputField} field=${listener[0]} value=${value.listen} onChange=${next => change(setField(value, 'listen', next))} />
          <${InputField} field=${listener[1]} value=${Array.isArray(value.port) ? value.port.join(',') : value.port}
            onChange=${next => change(setField(value, 'port', /^\d+$/.test(next) ? Number(next) : next))} />
        </div>`}
      <//>
      ${!spec || spec.json || legacy ? html`<p>${legacy ? 'This entry uses an older settings format.' : 'Protocol settings are editable in JSON.'}
        ${legacy && html` <button type="button" class="button" onClick=${() => setMode('json')}>Edit settings in JSON</button>`}</p>`
        : (spec.fields || objectList || spec.response) && html`<${FormSection} title="Protocol settings">
          ${spec.fields && html`<${Fields} fields=${spec.fields} value=${settings} onChange=${editSettings} />`}
          ${objectList && html`<${ObjectList} spec=${objectList} items=${settings[objectList.key] || []}
            onChange=${items => editSettings(setField(settings, objectList.key, items))} />`}
          ${spec.response && html`<${InputField} field=${{ label: 'Response', choices: [['none', 'Close connection'], ['http', 'HTTP 403']] }}
            value=${settings.response?.type} onChange=${editResponse} />`}
        <//>`}
      ${value.protocol !== 'tun' && html`<${Transport} value=${value} change=${change} inbound=${inbound} />`}
    `}
  `;
}
