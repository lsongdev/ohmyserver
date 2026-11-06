import { html, useEffect, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, Field, toast } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useUnsavedChanges, routeURL } from '@lsongdev/panel/forms.js';
import { useApi, CLASSES, pinned, ScheduleStatus } from '../zfs.js';

const HINTS = {
  hourly: 'Each hour',
  daily: 'Each day, from midnight',
  weekly: 'Each week, from Monday',
  monthly: 'Each month, from the 1st',
};

const toForm = policy => Object.fromEntries(CLASSES.map(c => [c, String(policy?.[c] ?? 0)]));
const valid = v => /^\d+$/.test(v) && Number(v) <= 1000;

export default function Schedule() {
  const { query, api, navigate, href } = usePanel();
  const name = query.name || '';
  const dataset = useApi(name ? '/dataset' : null, { name });
  const snapshots = useApi(name ? '/snapshots' : null, { dataset: name });
  const schedule = useApi('/schedule');
  const d = dataset.data;
  const [form, setForm] = useState(null);
  const { busy, error, run } = useAction();
  useEffect(() => { if (d && !form) setForm(toForm(d.policy)); }, [d]);

  const original = d ? toForm(d.policy) : null;
  const dirty = !!form && CLASSES.some(c => form[c] !== original[c]);
  const saved = useUnsavedChanges(dirty, { pending: busy });
  const invalid = !form || CLASSES.some(c => !valid(form[c]));
  const leave = () => navigate(routeURL('/dataset', { name, from: query.from }));

  // What saving would remove: retention applies within a minute.
  const removed = form && !invalid && snapshots.data ? CLASSES.reduce((total, c) => {
    const keep = Number(form[c]);
    if (keep === 0) return total; // a class turned off keeps what it has
    // Newest first; held and cloned ones count toward keep but are not removed.
    const excess = snapshots.data.filter(s => s.class === c).slice(keep);
    return total + excess.filter(s => !pinned(s)).length;
  }, 0) : 0;

  const save = e => {
    e.preventDefault();
    if (invalid) return;
    if (removed > 0 && !window.confirm(`Saving removes the oldest ${removed} automatic snapshot${removed === 1 ? '' : 's'} of ${name} within a minute. They cannot be recovered. Continue?`)) return;
    run(async () => {
      await api.put('/policy', { dataset: name, policy: Object.fromEntries(CLASSES.map(c => [c, Number(form[c])])) });
      toast(`Saved the schedule of ${name}`, 'success');
      saved();
      leave();
    });
  };
  const inherit = () => {
    const parent = name.includes('/');
    if (!window.confirm(parent
      ? `Remove ${name}'s own schedule and follow its parent's? Retention then uses the parent's counts.`
      : `Remove the schedule of ${name}? No new automatic snapshots are taken, and existing ones stay until you delete them.`)) return;
    run(async () => {
      await api.put('/policy', { dataset: name, policy: null });
      toast(parent ? `${name} now follows its parent` : `Removed the schedule of ${name}`, 'success');
      saved();
      leave();
    });
  };

  return html`<${PageHeader} title="Schedule" subtitle=${`Automatic snapshots of ${name} and how many of each to keep`}
    back=${{ href: href('/dataset', { name, from: query.from }), label: name || 'Dataset' }} />
    <${Alert} error=${dataset.error} />
    ${!d || !form ? (dataset.error ? null : html`<${Loading} />`) : html`
      <form class="form settings-form" onSubmit=${save}>
        <${Alert} error=${error} />
        <${ScheduleStatus} schedule=${schedule.data} scheduled=${true} />
        ${d.schedule_error ? html`<${Alert} variant="danger">The last run failed: ${d.schedule_error}<//>` : null}
        ${d.policy_invalid ? html`<${Alert} variant="warning">The stored policy <code>${d.policy_invalid}</code> cannot be read, so nothing is taken or removed. Saving replaces it.<//>` : null}
        ${d.policy_source && d.policy_source !== 'local' ? html`<${Alert} variant="info">Follows the schedule of ${d.policy_source}. Saving gives ${name} its own.<//>` : null}
        <div class="card">
          <div class="zfs-keep">
            ${CLASSES.map(c => html`<${Field} key=${c} label=${`Keep ${c}`} hint=${HINTS[c]} error=${valid(form[c]) ? null : 'From 0 to 1000'}>
              <input class="input" type="number" min="0" max="1000" step="1" required disabled=${busy} value=${form[c]}
                onInput=${e => setForm({ ...form, [c]: e.currentTarget.value })} />
            <//>`)}
          </div>
          <p class="muted small">Zero takes none of that class and leaves the ones it already has. Only snapshots named <code>auto-…</code> by a schedule are ever removed, oldest first; manual, held and cloned ones stay. Child datasets without a schedule of their own follow this one.</p>
        </div>
        ${removed > 0 && dirty ? html`<${Alert} variant="warning">Saving removes the oldest ${removed} automatic snapshot${removed === 1 ? '' : 's'} within a minute.<//>` : null}
        <div class="zfs-actions">
          <button class="button button-primary" type="submit" disabled=${busy || invalid || !dirty}>${busy ? 'Saving…' : 'Save'}</button>
          <button class="button" type="button" disabled=${busy} onClick=${leave}>Cancel</button>
          ${d.policy_source === 'local' ? html`<button class="button" type="button" disabled=${busy} onClick=${inherit}>${name.includes('/') ? "Follow parent's schedule" : 'Remove schedule'}</button>` : null}
        </div>
      </form>`}`;
}
