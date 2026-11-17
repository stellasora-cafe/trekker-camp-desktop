// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs';
import { resolve, basename, join } from 'node:path';
import { createHash, createPublicKey, verify } from 'node:crypto';
import * as tar from 'tar';
import { validatePackageFiles } from './package-files.mjs';

const directory = resolve(process.argv[2]);
const config = JSON.parse(readFileSync(new URL('../mygo.json', import.meta.url), 'utf8'));
const manifest = JSON.parse(readFileSync(join(directory, 'update-windows-amd64.json'), 'utf8'));
if (manifest.version !== config.version) throw new Error('Package version mismatch');
const filename = join(directory, basename(new URL(manifest.url).pathname));
const archive = readFileSync(filename);
const key = createPublicKey({ key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), Buffer.from(config.updates.publicKey, 'base64')]), format: 'der', type: 'spki' });
if (archive.length !== manifest.size || !verify(null, createHash('sha256').update(archive).digest(), key, Buffer.from(manifest.signature, 'base64'))) throw new Error('Invalid package signature or size');
const files = new Set();
await tar.t({ file: filename, onReadEntry: entry => {
	if (entry.type === 'Directory') return;
	if (entry.type !== 'File') throw new Error(`Unexpected archive entry: ${entry.path}`);
	files.add(entry.path);
} });
validatePackageFiles(files, config.name, process.argv.includes('--require-fonts'));
console.log(`Verified signed application package: ${manifest.version}, host, legal notices and approved font/about files (${files.size} files).`);
