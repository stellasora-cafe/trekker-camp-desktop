// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { mkdtempSync, mkdirSync, writeFileSync, existsSync, readdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { tmpdir } from 'node:os';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { assetProvenance, bundledFonts, prepareResources, resolveAssets } from './prepare-release.mjs';

test('discovers local fonts and respects explicit asset directory priority', t => {
	const { desktop, write } = fixture(t);
	assert.equal(resolveAssets(desktop, {}), undefined);
	const local = join(desktop, 'private-assets/public');
	for (const name of bundledFonts) write(join(local, 'fonts/misans-v1', name));
	assert.equal(resolveAssets(desktop, {}), local);
	assert.equal(resolveAssets(desktop, { CAMP_ASSETS_DIR: 'shared/public' }), join(desktop, 'shared/public'));
	assert.equal(resolveAssets(desktop, { CAMP_ASSETS_DIR: 'shared/public', CAMP_DESKTOP_ASSETS_DIR: 'explicit/public' }), join(desktop, 'explicit/public'));
});

function fixture(t) {
	const root = mkdtempSync(join(tmpdir(), 'camp-installer-'));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	const desktop = join(root, 'desktop');
	const write = (path, value = 'fixture') => { mkdirSync(dirname(path), { recursive: true }); writeFileSync(path, value); };
	for (const name of ['LICENSE', 'LICENSING.md', 'THIRD_PARTY_NOTICES.md', 'third_party/mygo/LICENSE']) write(join(desktop, name));
	write(join(desktop, 'LICENSE-AGPL-3.0.txt'));
	write(join(desktop, 'assets/app-icon-512-v1.png'));
	return { root, desktop, write };
}

test('public builds keep legal notices and about artwork while clearing old frontend and font staging', t => {
	const { desktop, write } = fixture(t);
	write(join(desktop, '.release-resources/resources/camp/fonts/private.ttf'));
	write(join(desktop, '.release-resources/resources/frontend.zip'));
	const output = prepareResources(desktop);
	assert.deepEqual(readdirSync(output).sort(), ['about', 'licenses']);
	assert.deepEqual(readdirSync(join(output, 'licenses')).sort(), ['LICENSE', 'LICENSE-AGPL-3.0.txt', 'LICENSING.md', 'MyGo-MIT.txt', 'THIRD_PARTY_NOTICES.md'].sort());
	assert.equal(existsSync(join(output, 'camp')), false);
	assert.equal(existsSync(join(output, 'frontend.zip')), false);
});

test('font build includes only five fonts, font license and optional organization artwork', t => {
	const { root, desktop, write } = fixture(t);
	const assets = join(root, 'private/public');
	for (const name of bundledFonts) write(join(assets, 'fonts/misans-v1', name));
	write(join(assets, '../originals/fonts/misans-v1/MiSans-license.pdf'));
	write(join(assets, 'maintenance/apology-v1.webp'));
	write(join(assets, 'fonts/misans-v1/MiSans-Bold.ttf'));
	write(join(assets, '.env'));
	write(join(desktop, '.release-source/about/organization.png'));
	const output = prepareResources(desktop, assets, true);
	assert.deepEqual(readdirSync(join(output, 'camp/fonts/misans-v1')).sort(), [...bundledFonts].sort());
	assert.ok(existsSync(join(output, 'about/organization.png')));
	assert.ok(existsSync(join(output, 'licenses/MiSans-license.pdf')));
	assert.equal(existsSync(join(output, 'camp/maintenance')), false);
	assert.equal(existsSync(join(output, 'camp/.env')), false);
});

test('official builds require complete fonts and a pinned clean asset checkout', t => {
	const { root, desktop, write } = fixture(t);
	assert.throws(() => prepareResources(desktop, undefined, true), /require the MiSans/);
	const assets = join(root, 'private/public');
	write(join(assets, 'fonts/misans-v1/MiSans-Regular.ttf'));
	assert.throws(() => prepareResources(desktop, assets, true), /ENOENT/);
	const commit = 'a'.repeat(40);
	const dirty = (_, args) => args.includes('rev-parse') ? commit : ' M public/fonts/font.woff2';
	const clean = (_, args) => args.includes('rev-parse') ? commit : '';
	assert.throws(() => assetProvenance(assets, commit, dirty), /clean checkout/);
	assert.throws(() => assetProvenance(assets, 'b'.repeat(40), clean), /pinned commit/);
	assert.deepEqual(assetProvenance(assets, commit, clean), { assetsRevision: commit, assetsDirty: false });
});
