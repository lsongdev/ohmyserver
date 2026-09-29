import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
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

test('module pages never embed their host page prefix in navigation', () => {
  for (const moduleEntry of readdirSync(modulesDir, { withFileTypes: true })) {
    if (!moduleEntry.isDirectory()) continue;
    const pagesDir = resolve(modulesDir, moduleEntry.name, 'web/assets/pages');
    let pages;
    try {
      pages = readdirSync(pagesDir, { withFileTypes: true });
    } catch {
      continue;
    }
    const escaped = moduleEntry.name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const hostPath = new RegExp("\\b(?:navigate|href)\\(\\s*['\"`]\\/" + escaped + "(?:/|['\"`])");
    for (const page of pages) {
      if (!page.isFile() || !page.name.endsWith('.js')) continue;
      const source = readFileSync(resolve(pagesDir, page.name), 'utf8');
      assert.doesNotMatch(source, hostPath, moduleEntry.name + '/' + page.name + ' must navigate relative to its module root');
      const rawHash = new RegExp("#/" + escaped + "(?:/|['\"`])");
      assert.doesNotMatch(source, rawHash, moduleEntry.name + '/' + page.name + ' must not hard-code the host hash prefix');
    }
  }
});


test('cross-module links use the explicit host capability', () => {
  const moduleNames = readdirSync(modulesDir, { withFileTypes: true }).filter(entry => entry.isDirectory()).map(entry => entry.name);
  for (const moduleEntry of readdirSync(modulesDir, { withFileTypes: true })) {
    if (!moduleEntry.isDirectory()) continue;
    const pagesDir = resolve(modulesDir, moduleEntry.name, 'web/assets/pages');
    let pages;
    try { pages = readdirSync(pagesDir, { withFileTypes: true }); } catch { continue; }
    for (const page of pages) {
      if (!page.isFile() || !page.name.endsWith('.js')) continue;
      const source = readFileSync(resolve(pagesDir, page.name), 'utf8');
      for (const target of moduleNames) {
        if (target === moduleEntry.name) continue;
        for (const fn of ['navigate', 'href']) {
          for (const quote of ["'", '"', '`']) {
            assert.equal(source.includes(fn + '(' + quote + '/' + target + '/'), false, moduleEntry.name + '/' + page.name + ' must use hrefTo() for cross-module navigation');
            assert.equal(source.includes('#/' + target + '/'), false, moduleEntry.name + '/' + page.name + ' must not hard-code a cross-module hash path');
          }
        }
      }
    }
  }
});


test('pages use the single cross-module capability name', () => {
  for (const moduleEntry of readdirSync(modulesDir, { withFileTypes: true })) {
    if (!moduleEntry.isDirectory()) continue;
    const pagesDir = resolve(modulesDir, moduleEntry.name, 'web/assets/pages');
    let pages;
    try { pages = readdirSync(pagesDir, { withFileTypes: true }); } catch { continue; }
    for (const page of pages) {
      if (!page.isFile() || !page.name.endsWith('.js')) continue;
      const source = readFileSync(resolve(pagesDir, page.name), 'utf8');
      assert.doesNotMatch(source, /\bmoduleHref\b/, moduleEntry.name + '/' + page.name + ' must use hrefTo');
    }
  }
});
