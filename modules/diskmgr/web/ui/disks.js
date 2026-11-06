// Disk module UI helpers shared by the pages.
import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Badge } from '@lsongdev/scripts/react/components/index.js';
import { Status } from '@lsongdev/panel/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

// kind names what a disk is: NVMe, SSD, HDD, USB.
export function kind(disk) {
  if (disk.transport === 'nvme' || disk.name.startsWith('nvme')) return 'NVMe SSD';
  if (disk.transport === 'usb') return 'USB';
  if (disk.name.startsWith('vd') || disk.name.startsWith('xvd')) return 'Virtual';
  return disk.rotational ? 'HDD' : 'SSD';
}

// uptime formats power-on hours as days or years.
export function uptime(hours) {
  if (!hours) return '—';
  if (hours < 48) return `${hours} h`;
  const days = hours / 24;
  return days < 365 ? `${Math.round(days)} days` : `${(days / 365).toFixed(1)} years`;
}

// zfsPools names the ZFS pools a disk's labels say it is in.
export function zfsPools(disk) {
  const found = [];
  const walk = b => {
    if (b.fstype === 'zfs_member' && b.label) found.push(b.label);
    (b.children || []).forEach(walk);
  };
  walk(disk);
  return [...new Set(found)];
}

// HealthStatus is a disk's health in one badge.
export function HealthStatus({ health }) {
  if (!health) return html`<span class="muted">—</span>`;
  if (health.error) return html`<${Badge} title=${health.error}>No data<//>`;
  if (health.passed === false) return html`<${Status} state="failed" label="Failing" />`;
  if (health.warnings?.length) return html`<${Status} state="waiting" variant="warning" label=${`${health.warnings.length} warning${health.warnings.length > 1 ? 's' : ''}`} title=${health.warnings.join('\n')} />`;
  if (health.testing != null) return html`<${Status} state="processing" label=${`Testing, ${health.testing}% left`} />`;
  if (health.asleep && health.passed == null) return html`<${Badge}>Asleep<//>`;
  return html`<${Status} state="ok" label=${health.asleep ? 'Healthy (asleep)' : 'Healthy'} />`;
}

// usage describes what is on a disk: arrays it belongs to and mount points.
export function usage(disk) {
  const found = [];
  const walk = b => {
    if (b.type?.startsWith('raid')) found.push(`${b.name} (${b.type})${b.mountpoint ? ` ${b.mountpoint}` : ''}`);
    else if (b.mountpoint) found.push(b.mountpoint);
    else if (b.fstype === 'swap') found.push('swap');
    (b.children || []).forEach(walk);
  };
  walk(disk);
  return [...new Set(found)];
}
