import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { PageHeader, Alert, toast } from '@lsongdev/scripts/react/components/index.js';
import { WithConfig, sectionFor, entries, withEntries } from '../xray.js';

export default function List({ params }) {
  const { href } = usePanel();
  const key = params.section;
  const section = sectionFor(key);
  if (!section) return html`<${Alert} variant="warning">Page not found<//>`;
  return html`<${PageHeader} title=${section.title} subtitle=${section.description}
    actions=${html`<a class="button button-primary" href=${href('/' + key + '/new')}>Add ${section.item}</a>`} />
    <${WithConfig} render=${(state, reload) => html`<${Rows} sectionKey=${key} section=${section} state=${state} reload=${reload} />`} />`;
}

function Rows({ sectionKey, section, state, reload }) {
  const { href, api } = usePanel();
  const { busy, error, run } = useAction();
  const list = entries(state.config, sectionKey);
  const update = (index, direction) => {
    const next = [...list];
    if (direction === 0) {
      if (!window.confirm('Remove this ' + section.item + ' from the saved configuration?')) return;
      next.splice(index, 1);
    } else {
      [next[index], next[index + direction]] = [next[index + direction], next[index]];
    }
    run(async () => {
      await api.put('/config', { revision: state.revision, config: withEntries(state.config, sectionKey, next) });
      toast('Configuration saved', 'success');
      reload();
    });
  };
  return html`<${Alert} error=${error} />
    ${list.length === 0 ? html`<p>No ${section.title.toLowerCase()} configured.</p>` : html`
      <ol class="xray-rows">${list.map((item, index) => html`
        <li key=${index} class="xray-row">
          <div>
            <a href=${href('/' + sectionKey + '/' + index + '/edit')}>
              ${item.tag || item.ruleTag || (sectionKey === 'rules' ? 'Rule ' + (index + 1) : item.protocol)}
            </a>
            <small>${sectionKey === 'rules' ? 'Target: ' + (item.outboundTag || item.balancerTag || '—')
              : [item.protocol, item.port == null ? null : (item.listen || 'all addresses') + ':' + item.port].filter(Boolean).join(' · ')}</small>
          </div>
          <div class="xray-actions">
            ${sectionKey !== 'inbounds' && html`
              <button type="button" class="button" disabled=${busy || index === 0} aria-label=${'Move ' + section.item + ' ' + (index + 1) + ' up'} onClick=${() => update(index, -1)}>↑</button>
              <button type="button" class="button" disabled=${busy || index === list.length - 1} aria-label=${'Move ' + section.item + ' ' + (index + 1) + ' down'} onClick=${() => update(index, 1)}>↓</button>`}
            <button type="button" class="button" disabled=${busy} onClick=${() => update(index, 0)}>Remove</button>
          </div>
        </li>`)}
      </ol>`}`;
}
