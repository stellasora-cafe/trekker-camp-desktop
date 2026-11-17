// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { copyFileSync, existsSync, mkdirSync, readFileSync, realpathSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { execFileSync } from 'node:child_process';
import { copyDependencyLicenses } from './third-party-licenses.mjs';

export const bundledFonts = ['MiSans-Regular.ttf', ...['Regular', 'Medium', 'Semibold', 'Bold'].map(weight => `MiSans-${weight}.woff2`)];
export const aboutFiles = ['camp-icon.png', 'organization.png'];

export function resolveAssets(desktop, env = process.env) {
	const explicit = env.CAMP_DESKTOP_ASSETS_DIR || env.CAMP_ASSETS_DIR;
	if (explicit) return resolve(desktop, explicit);
	const local = join(desktop, 'private-assets/public');
	if (bundledFonts.every(name => existsSync(join(local, 'fonts/misans-v1', name)))) return local;
}

export function assetProvenance(assets, expected, run = execFileSync) {
	if (!assets) return { assetsRevision: null, assetsDirty: false };
	const options = { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] };
	const assetsRevision = run('git', ['-C', assets, 'rev-parse', 'HEAD'], options).trim();
	const assetsDirty = run('git', ['-C', assets, 'status', '--porcelain', '--untracked-files=all'], options).trim() !== '';
	if (expected && (assetsRevision !== expected || assetsDirty)) throw new Error('Release fonts must match the pinned commit with a clean checkout');
	return { assetsRevision, assetsDirty };
}

function copyRegularFile(source, target, allowedRoot) {
	if (!realpathSync(source).startsWith(realpathSync(allowedRoot) + sep) || !statSync(source).isFile()) throw new Error('Resource leaves its approved directory');
	mkdirSync(dirname(target), { recursive: true });
	copyFileSync(source, target);
}

// Exact font/about files only; the frontend and other site assets stay online.
export function prepareResources(desktop, assets, requireFonts = false) {
	if (requireFonts && !assets) throw new Error('Official releases require the MiSans font directory');
	const staging = resolve(desktop, '.release-resources');
	if (dirname(staging) !== resolve(desktop)) throw new Error('Invalid staging directory');
	rmSync(staging, { recursive: true, force: true });
	const resources = join(staging, 'resources');
	const licenses = join(resources, 'licenses');
	mkdirSync(licenses, { recursive: true });
	for (const name of ['LICENSE', 'LICENSING.md', 'THIRD_PARTY_NOTICES.md']) {
		copyFileSync(join(desktop, name), join(licenses, name));
	}
	copyFileSync(join(desktop, 'LICENSE-AGPL-3.0.txt'), join(licenses, 'LICENSE-AGPL-3.0.txt'));
	copyFileSync(join(desktop, 'third_party/mygo/LICENSE'), join(licenses, 'MyGo-MIT.txt'));
	const brand = join(desktop, 'assets/app-icon-512-v1.png');
	copyRegularFile(brand, join(resources, 'about/camp-icon.png'), join(desktop, 'assets'));
	const organization = join(desktop, '.release-source/about/organization.png');
	if (existsSync(organization)) copyRegularFile(organization, join(resources, 'about/organization.png'), join(desktop, '.release-source/about'));
	if (assets) {
		const source = realpathSync(assets);
		for (const name of bundledFonts) copyRegularFile(join(source, 'fonts/misans-v1', name), join(resources, 'camp/fonts/misans-v1', name), source);
		copyRegularFile(join(source, '../originals/fonts/misans-v1/MiSans-license.pdf'), join(licenses, 'MiSans-license.pdf'), dirname(source));
	}
	return resources;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
	const desktop = resolve(dirname(fileURLToPath(import.meta.url)), '..');
	const assets = resolveAssets(desktop);
	const provenance = assetProvenance(assets, process.env.CAMP_DESKTOP_ASSETS_REVISION);
	const resources = prepareResources(desktop, assets, Boolean(process.env.CAMP_DESKTOP_ASSETS_REVISION));
	copyDependencyLicenses(desktop, join(resources, 'licenses'));
	writeFileSync(join(resources, 'release.json'), JSON.stringify({
		version: JSON.parse(readFileSync(join(desktop, 'mygo.json'), 'utf8')).version,
		sourceTag: process.env.GITHUB_REF_NAME ?? null,
		sourceURL: process.env.GITHUB_REF_NAME ? `https://github.com/stellasora-cafe/trekker-camp-desktop/tree/${process.env.GITHUB_REF_NAME}` : null,
		...provenance,
	}, null, 2) + '\n');
	console.log(`Prepared legal notices and about artwork${assets ? ' with MiSans fonts' : ' with system font fallback'}; no frontend archive.`);
}
