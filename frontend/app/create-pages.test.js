import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const modulesDir = resolve(fileURLToPath(new URL('../../modules', import.meta.url)));

test('resource list pages keep creation forms on standalone routes', () => {
  const lists = [
    'forward/web/assets/pages/listeners.js',
    'widgets/web/assets/pages/list.js',
    'uptime/web/assets/pages/monitors.js',
    'settings/web/assets/pages/list.js',
    'acme/web/assets/pages/providers.js',
    'dns/web/assets/pages/zones.js',
    'dns/web/assets/pages/providers.js',
    'ssh/web/assets/pages/servers.js',
  ];
  for (const relative of lists) {
    const source = readFileSync(resolve(modulesDir, relative), 'utf8');
    assert.doesNotMatch(source, /<form\b/, `${relative} must not embed forms`);
    assert.match(source, /navigate\([^)]*\/new['"]?\)/, `${relative} must link to a standalone create page`);
  }
});

test('SSH create and edit pages are registered as standalone module routes', () => {
  const source = readFileSync(resolve(modulesDir, 'ssh/web/assets/module.js'), 'utf8');
  assert.match(source, /path: '\/servers\/new', load: \(\) => import\('\.\/pages\/server-form\.js'\)/);
  assert.match(source, /path: '\/servers\/:id\/edit', load: \(\) => import\('\.\/pages\/server-form\.js'\)/);
});

test('DNS filter and record forms are standalone module pages', () => {
  const lists = [
    'dns/web/assets/pages/filters.js',
    'dns/web/assets/pages/provider-zone.js',
  ];
  for (const relative of lists) {
    const source = readFileSync(resolve(modulesDir, relative), 'utf8');
    assert.doesNotMatch(source, /<form\b|<details><summary>编辑记录/ , `${relative} must not embed a create or edit form`);
  }
  const dnsModule = readFileSync(resolve(modulesDir, 'dns/web/assets/module.js'), 'utf8');
  assert.match(dnsModule, /path:'\/filters\/subscriptions\/new'/);
  assert.match(dnsModule, /path:'\/filters\/rules\/new'/);
  assert.match(dnsModule, /path:'\/providers\/:id\/zones\/:zone\/records\/:record\/edit'/);
  const zone = readFileSync(resolve(modulesDir, 'dns/web/assets/pages/provider-zone.js'), 'utf8');
  assert.match(zone, /records\/\$\{encodeURIComponent\(item\.id\)\}\/edit/);
});

test('mixed edit/list pages do not create resources inline', () => {
  for (const relative of ['cron/web/assets/pages/jobs.js', 'projects/web/assets/pages/list.js']) {
    const source = readFileSync(resolve(modulesDir, relative), 'utf8');
    assert.doesNotMatch(source, /api\.create\(/, `${relative} must not call create from its list/edit page`);
    assert.match(source, /navigate\([^)]*\/new['"]?\)/, `${relative} must link to a standalone create page`);
  }
});
