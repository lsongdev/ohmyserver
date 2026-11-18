import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, Badge, KeyValue, Loading, PageHeader } from '@lsongdev/scripts/react/components/index.js';

const registration = {
  0: 'Not registered', 1: 'Registered (home)', 2: 'Searching',
  3: 'Denied', 4: 'Unknown', 5: 'Registered (roaming)',
};

const sim = { READY: 'Ready', 'SIM PIN': 'SIM PIN required', 'SIM PUK': 'SIM PUK required' };

const connection = {
  connecting: 'Connecting', not_found: 'Modem not detected',
  disconnected: 'Modem disconnected', connected: 'Modem connected',
};

const cellStates = {
  SEARCH: 'Searching', LIMSRV: 'Limited service',
  NOCONN: 'Camped (idle)', CONNECT: 'Active radio connection',
};

function metric(value, unit) {
  return value == null ? '—' : [value, unit].filter(Boolean).join(' ');
}

function Registration({ label, value, error }) {
  const variant = value === 1 || value === 5 ? 'success' : value === 3 ? 'danger' : value === 2 ? 'warning' : undefined;
  return html`<div>
    <dt>${label}</dt>
    <dd><${Badge} variant=${variant}>${value == null ? 'Unavailable' : registration[value] ?? `Status ${value}`}<//></dd>
    ${error ? html`<dd class="muted small">${error}</dd>` : null}
  </div>`;
}

// Use one display scale so LTE and 5G power readings can be compared directly.
const rsrpScale = { min: -140, max: -60 };

function Signal({ cell }) {
  const lte = cell.radio === 'LTE';
  const band = cell.band == null ? null : `${lte ? 'B' : 'n'}${cell.band}`;
  const power = metric(cell.rsrp, 'dBm');
  return html`<article class="cellular-signal">
    <div class="cellular-heading">
      <h3>${cell.radio}</h3>
      ${band ? html`<${Badge}>${band}<//>` : null}
    </div>
    <div class="cellular-power">
      <strong>${power}</strong>
      <span class="muted small">RSRP</span>
    </div>
    ${cell.rsrp != null ? html`
      <meter class="cellular-meter" min=${rsrpScale.min} max=${rsrpScale.max} value=${cell.rsrp}
        aria-label=${`${cell.radio} RSRP`} aria-valuetext=${power}></meter>
      <div class="cellular-scale muted small" aria-hidden="true"><span>−140 dBm</span><span>−60 dBm</span></div>
    ` : null}
    <dl class="cellular-metrics">
      <div><dt>RSRQ</dt><dd>${metric(cell.rsrq, 'dB')}</dd></div>
      <div><dt>${lte ? 'SINR (modem index)' : 'SINR'}</dt><dd>${metric(cell.sinr, lte ? '' : 'dB')}</dd></div>
    </dl>
  </article>`;
}

export default function Cellular() {
  const { api } = usePanel();
  const { data, error } = useResource(signal => api.get('/status', { signal }), [], { interval: 5000 });
  const connected = data?.state === 'connected';
  const networkCode = data?.cells?.find(cell => cell.plmn)?.plmn;
  const operator = data?.operator?.name || (networkCode ? `PLMN ${networkCode}` : 'Operator unavailable');
  const show = (key, value) => data.errors?.[key] ?? value ?? '—';
  return html`
    <${PageHeader} title="Cellular" />
    <${Alert} error=${error} />
    ${!data && !error ? html`<${Loading} />` : null}
    ${data ? html`
      ${data.error ? html`<${Alert} variant="warning">${data.error}<//>` : null}
      <div class="cellular-overview">
        <section class="card">
          <div class="cellular-heading">
            <h2>Network registration</h2>
            <${Badge} variant=${connected ? 'success' : 'warning'} dot>${connection[data.state] ?? 'Unknown modem state'}<//>
          </div>
          ${connected ? html`
            <p class="cellular-operator">${operator}</p>
            <dl class="cellular-metrics">
              <${Registration} label="LTE" value=${data.lte_registration} error=${data.errors?.lte_registration} />
              <${Registration} label="5G" value=${data.nr_registration} error=${data.errors?.nr_registration} />
              <div><dt>SIM</dt><dd>${show('sim', sim[data.sim] ?? data.sim)}</dd></div>
              <div>
                <dt>Packet service</dt>
                <dd>${show('packet_attached', data.packet_attached == null ? null : data.packet_attached ? 'Attached' : 'Detached')}</dd>
              </div>
            </dl>
          ` : null}
        </section>
        <section class="card">
          <div class="cellular-heading">
            <h2>Serving cell</h2>
            ${data.cell_state ? html`<${Badge}>${cellStates[data.cell_state] ?? data.cell_state}<//>` : null}
          </div>
          ${data.cells?.length
            ? data.cells.map(cell => html`<${Signal} key=${cell.radio} cell=${cell} />`)
            : html`<p class="muted">${data.errors?.cells || (connected ? 'No serving cell reported' : 'Waiting for the modem')}</p>`}
        </section>
      </div>
      <section class="card cellular-details">
        <h2>Modem details</h2>
        <${KeyValue} items=${[
          ['Model', show('model', data.model)],
          ['Firmware', show('firmware', data.firmware)],
          ['Serial device', data.port ?? '—'],
          ['Data session', 'Not measured'],
          ['Last read', data.updated_at ? new Date(data.updated_at).toLocaleTimeString() : '—'],
          ...(data.errors?.operator ? [['Operator query', data.errors.operator]] : []),
        ]} />
      </section>
    ` : null}
  `;
}
