import { html } from './html.js';

// The shell chooses its icon set; module manifests stay independent of it.
const icons = {
  home: { '/': 'house' },
  docker: {
    '/': 'gauge-high', '/containers': 'cubes', '/images': 'image',
    '/volumes': 'database', '/networks': 'network-wired', '/stacks': 'layer-group',
    '/store': 'store', '/apps': 'boxes-stacked', '/jobs': 'list-check',
  },
  acme: {
    '/certificates': 'certificate', '/issuances': 'rotate', '/accounts': 'user-shield',
    '/providers': 'cloud', '/orders': 'file-lines',
  },
  gateway: {
    '/': 'gauge-high', '/sites': 'globe', '/certificates': 'certificate',
    '/events': 'list-ul', '/config': 'gear',
  },
  dns: {
    '/': 'gauge-high', '/querylog': 'magnifying-glass', '/zones': 'globe',
    '/filters': 'filter', '/clients': 'desktop', '/tools': 'screwdriver-wrench', '/settings': 'gear',
  },
  domains: { '/': 'globe', '/accounts': 'user-shield' },
  dhcp: { '/': 'network-wired', '/reservations': 'thumbtack', '/activity': 'wave-square', '/settings': 'gear' },
  forward: { '/': 'right-left' },
  ssh: { '/': 'terminal', '/key': 'key' },
  cron: { '/': 'clock' },
  monitor: { '/': 'chart-line' },
  ci: { '/': 'code-branch', '/builds': 'hammer' },
  packages: { '/': 'box', '/recommended': 'star', '/jobs': 'list-check' },
  services: { '/': 'gears', '/logs': 'file-lines' },
  disks: { '/': 'gauge-high' },
  samba: { '/': 'share-nodes', '/users': 'users', '/sessions': 'plug', '/logs': 'file-lines', '/global': 'gear' },
  notify: { '/': 'bell' },
  zfs: { '/': 'hard-drive', '/snapshots': 'camera' },
  files: { '/': 'folder', '/trash': 'trash-can' },
};

export const faIcon = name => html`<span class=${`fa-solid fa-${name}`} aria-hidden="true"></span>`;

export function navigationIcon(module, item) {
  const name = icons[module.id]?.[item.path];
  return name ? faIcon(name) : item.icon || faIcon('circle');
}
