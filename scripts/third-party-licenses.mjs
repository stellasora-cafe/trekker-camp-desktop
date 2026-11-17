// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { execFileSync } from 'node:child_process';
import { copyFileSync, mkdirSync, readdirSync, realpathSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const noticeName = /^(?:licen[sc]e|copying|copyright|notice)(?:[._-].*)?$/i;

// Enumerate the dependencies actually linked into the formal entry point.
// The module graph also contains dependencies exclusive to the reference app.
export function copyDependencyLicenses(desktop, licenses, run = execFileSync) {
	const options = { cwd: desktop, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] };
	const listing = run('go', ['list', '-deps', '-f', '{{with .Module}}{{.Path}}|{{.Dir}}{{end}}', '.'], options);
	const modules = new Map();
	for (const line of listing.split(/\r?\n/)) {
		if (!line.trim()) continue;
		const [name, directory] = line.split('|');
		if (!name || !directory || !/^[a-zA-Z0-9._~+/-]+$/.test(name) || name.split('/').some(part => part === '..' || part === '.')) {
			throw new Error('Invalid Go dependency metadata');
		}
		if (realpathSync(directory) === realpathSync(desktop)) continue;
		modules.set(name, directory);
	}
	const goRoot = run('go', ['env', 'GOROOT'], options).trim();
	modules.set('go', goRoot);
	const manifest = [];
	for (const [module, directory] of [...modules].sort(([a], [b]) => a.localeCompare(b))) {
		const files = readdirSync(directory, { withFileTypes: true }).filter(entry => entry.isFile() && noticeName.test(entry.name)).map(entry => entry.name).sort();
		if (!files.length) throw new Error(`Missing license notice for ${module}`);
		const destination = resolve(licenses, 'go', module);
		mkdirSync(destination, { recursive: true });
		for (const file of files) copyFileSync(join(directory, file), join(destination, file));
		manifest.push({ module, files });
	}
	mkdirSync(licenses, { recursive: true });
	writeFileSync(join(licenses, 'go-dependencies.json'), JSON.stringify(manifest, null, 2) + '\n');
	return manifest;
}
