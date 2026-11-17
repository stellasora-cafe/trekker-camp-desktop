// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { validatePackageFiles } from './package-files.mjs';
import { bundledFonts } from './prepare-release.mjs';

function files() {
	return new Set(['camp.exe', '.mygo-files', 'resources/release.json', 'resources/about/camp-icon.png', ...['LICENSE', 'LICENSE-AGPL-3.0.txt', 'LICENSING.md', 'THIRD_PARTY_NOTICES.md', 'go-dependencies.json'].map(name => `resources/licenses/${name}`)]);
}

test('official packages require all fonts and their license', () => {
	const entries = files();
	assert.doesNotThrow(() => validatePackageFiles(entries, 'camp'));
	assert.throws(() => validatePackageFiles(entries, 'camp', true), /missing.*MiSans/);
	for (const font of bundledFonts) entries.add(`resources/camp/fonts/misans-v1/${font}`);
	assert.throws(() => validatePackageFiles(entries, 'camp', true), /missing.*license.pdf/);
	entries.add('resources/licenses/MiSans-license.pdf');
	entries.add('resources/about/organization.png');
	assert.doesNotThrow(() => validatePackageFiles(entries, 'camp', true));
	entries.delete('resources/camp/fonts/misans-v1/MiSans-Bold.woff2');
	assert.throws(() => validatePackageFiles(entries, 'camp'), /missing.*Bold/);
});

test('packages reject frontend, maintenance artwork and non-whitelisted fonts', () => {
	for (const extra of ['resources/frontend.zip', 'resources/camp/maintenance/apology.webp', 'resources/camp/fonts/misans-v1/MiSans-Bold.ttf', 'resources/about/other.png']) {
		const entries = files();
		entries.add(extra);
		assert.throws(() => validatePackageFiles(entries, 'camp'), /Unapproved/);
	}
});
