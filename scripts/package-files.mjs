// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { aboutFiles, bundledFonts } from './prepare-release.mjs';

export function validatePackageFiles(files, name, requireFonts = false) {
	const required = [`${name}.exe`, '.mygo-files', 'resources/release.json', 'resources/licenses/LICENSE', 'resources/licenses/LICENSE-AGPL-3.0.txt', 'resources/licenses/LICENSING.md', 'resources/licenses/THIRD_PARTY_NOTICES.md', 'resources/licenses/go-dependencies.json'];
	required.push('resources/about/camp-icon.png');
	const fontPaths = bundledFonts.map(name => `resources/camp/fonts/misans-v1/${name}`);
	if (requireFonts || fontPaths.some(file => files.has(file))) required.push(...fontPaths, 'resources/licenses/MiSans-license.pdf');
	for (const file of required) if (!files.has(file)) throw new Error(`Package missing ${file}`);
	for (const file of files) {
		if (file === `${name}.exe` || file === '.mygo-files' || file === 'resources/release.json' || file.startsWith('resources/licenses/')) continue;
		if (fontPaths.includes(file) || aboutFiles.some(name => file === `resources/about/${name}`)) continue;
		throw new Error(`Unapproved package file: ${file}`);
	}
}
