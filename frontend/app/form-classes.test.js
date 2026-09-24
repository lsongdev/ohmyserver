import test from 'node:test';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const frontendDir = resolve(fileURLToPath(new URL('..', import.meta.url)));
const modulesDir = resolve(fileURLToPath(new URL('../../modules', import.meta.url)));

function sources(directory) {
  return readdirSync(directory).flatMap(name => {
    const path = resolve(directory, name);
    if (statSync(path).isDirectory()) return name === 'vendor' ? [] : sources(path);
    return /\.(?:js|html)$/.test(name) && !name.endsWith('.test.js') ? [path] : [];
  });
}

test('frontend inputs and buttons include their base style class', () => {
  const violations = [];
  for (const path of [...sources(frontendDir), ...sources(modulesDir)]) {
    const source = readFileSync(path, 'utf8');
    for (const match of source.matchAll(/<(input|button)\b[^>]*>/gs)) {
      const tag = match[0];
      const expected = match[1] === 'input' ? 'input' : 'button';
      const classes = tag.match(/\bclass\s*=\s*(?:"([^"]*)"|'([^']*)'|\$\{`([^`]*)`)/);
      const value = classes?.[1] ?? classes?.[2] ?? classes?.[3] ?? '';
      if (!value.split(/\s+/).includes(expected)) {
        violations.push(`${path}: <${expected}> lacks class="${expected}"`);
      }
    }
  }
  assert.deepEqual(violations, []);
});
