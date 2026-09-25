import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const index = readFileSync(resolve(frontendDir, 'index.html'), 'utf8');

test('frontend assets resolve beneath a host-provided document base', () => {
  assert.match(index, /<base href="\/">/);
  assert.doesNotMatch(index, /(?:href|src)="\/frontend\//);
  assert.match(index, /"preact":\s*"\.\/frontend\/vendor\/preact\.module\.js"/);
  assert.match(index, /"preact\/hooks":\s*"\.\/frontend\/vendor\/hooks\.module\.js"/);
  assert.doesNotMatch(index, /"(?:preact|preact\/hooks|htm|@xterm\/xterm|@xterm\/addon-fit)":\s*"(?:frontend|\/frontend)\//);

  const path = 'frontend/app/main.js';
  assert.equal(new URL(path, 'https://example.test/panel/').pathname, '/panel/frontend/app/main.js');
  assert.equal(new URL(path, 'https://example.test/').pathname, '/frontend/app/main.js');
});
