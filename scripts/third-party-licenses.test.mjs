// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { copyDependencyLicenses } from './third-party-licenses.mjs';

function fixture(t) {
	const root = mkdtempSync(join(tmpdir(), 'camp-licenses-'));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	const desktop = join(root, 'desktop');
	const dependency = join(root, 'dependency');
	const go = join(root, 'go');
	for (const directory of [desktop, dependency, go]) mkdirSync(directory);
	writeFileSync(join(go, 'LICENSE'), 'Go license');
	const run = (_command, args) => args[0] === 'env' ? go : `camp/desktop|${desktop}\nexample.test/module|${dependency}\nexample.test/module|${dependency}\n`;
	return { desktop, dependency, output: join(root, 'licenses'), run };
}

test('copies linked module notices once, omits source and local machine paths', t => {
	const f = fixture(t);
	writeFileSync(join(f.dependency, 'LICENSE'), 'Dependency license');
	writeFileSync(join(f.dependency, 'NOTICE.txt'), 'Attribution');
	writeFileSync(join(f.dependency, 'private.go'), 'source');
	const manifest = copyDependencyLicenses(f.desktop, f.output, f.run);
	assert.deepEqual(manifest, [
		{ module: 'example.test/module', files: ['LICENSE', 'NOTICE.txt'] },
		{ module: 'go', files: ['LICENSE'] },
	]);
	assert.equal(readFileSync(join(f.output, 'go/example.test/module/NOTICE.txt'), 'utf8'), 'Attribution');
	assert.doesNotMatch(readFileSync(join(f.output, 'go-dependencies.json'), 'utf8'), /desktop|private.go/);
});

test('release preparation fails if a linked dependency lacks a notice', t => {
	const f = fixture(t);
	assert.throws(() => copyDependencyLicenses(f.desktop, f.output, f.run), /Missing license notice/);
});
