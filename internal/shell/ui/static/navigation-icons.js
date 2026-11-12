import { html } from '@lsongdev/panel/html.js';

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
  packages: { '/': 'box', '/recommended': 'star', '/repositories': 'database', '/jobs': 'list-check' },
  services: { '/': 'gears', '/logs': 'file-lines' },
  network: { '/': 'ethernet', '/routes': 'route', '/ports': 'plug', '/tools': 'stethoscope' },
  disks: { '/': 'gauge-high' },
  samba: { '/': 'share-nodes', '/users': 'users', '/sessions': 'plug', '/logs': 'file-lines', '/global': 'gear' },
  notify: { '/': 'bell', '/channels': 'paper-plane' },
  zfs: { '/': 'hard-drive', '/snapshots': 'camera' },
  files: { '/': 'folder', '/trash': 'trash-can' },
};

export const faIcon = name => html`<span class=${`fa-solid fa-${name}`} aria-hidden="true"></span>`;

export function navigationIcon(module, item) {
  const name = icons[module.id]?.[item.path];
  return name ? faIcon(name) : item.icon || faIcon('circle');
}

// A navigation group's icon, shown when the sidebar collapses to icons; by
// shared group title or module id. Groups not listed take their first link's.
const groupIcons = {
  Home: 'house', Storage: 'hard-drive',
  monitor: 'chart-line', docker: 'cubes', acme: 'certificate', gateway: 'tower-broadcast',
  dns: 'globe', domains: 'at', dhcp: 'network-wired', forward: 'right-left', ssh: 'terminal',
  cron: 'clock', ci: 'code-branch', packages: 'box', services: 'gears', network: 'ethernet',
  samba: 'share-nodes', files: 'folder',
};

export function groupIcon(id) {
  const name = groupIcons[id];
  return name ? faIcon(name) : undefined;
}
