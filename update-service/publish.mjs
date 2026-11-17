// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0
import fs from 'node:fs/promises';
import path from 'node:path';
import { createHash, createPublicKey, verify } from 'node:crypto';
import { S3Client, PutObjectCommand } from '@aws-sdk/client-s3';

const [app, channel, manifestPath, publicKeyPath] = process.argv.slice(2);
if (!/^[a-z0-9-]+$/.test(app ?? '') || !/^[a-z0-9-]+$/.test(channel ?? '') || !manifestPath || !publicKeyPath) throw new Error('Usage: node publish.mjs APP CHANNEL MANIFEST PUBLIC_KEY_FILE');
const token = process.env.CLOUDFLARE_API_TOKEN;
if (!token) throw new Error('Set CLOUDFLARE_API_TOKEN with object read/write access to stellasora-app-releases');
const manifest = JSON.parse(await fs.readFile(manifestPath, 'utf8'));
if (!/^[a-zA-Z0-9._-]+$/.test(manifest.version)) throw new Error('Invalid version');
const filename = path.basename(new URL(manifest.url).pathname);
const archive = await fs.readFile(path.join(path.dirname(manifestPath), filename));
const publicBytes = Buffer.from((await fs.readFile(publicKeyPath, 'utf8')).trim(), 'base64');
const publicKey = createPublicKey({ key: Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), publicBytes]), format: 'der', type: 'spki' });
if (archive.length !== manifest.size || !verify(null, createHash('sha256').update(archive).digest(), publicKey, Buffer.from(manifest.signature, 'base64'))) throw new Error('Archive signature or size mismatch');
const account = '3f186037b03a2598dbaaba64a0813364';
const verified = await fetch(`https://api.cloudflare.com/client/v4/accounts/${account}/tokens/verify`, { headers: { Authorization: `Bearer ${token}` } });
const identity = await verified.json();
if (!verified.ok || identity.result?.status !== 'active' || !identity.result?.id) throw new Error('Account API token verification failed');
const s3 = new S3Client({ region: 'auto', endpoint: `https://${account}.r2.cloudflarestorage.com`, credentials: { accessKeyId: identity.result.id, secretAccessKey: createHash('sha256').update(token).digest('hex') } });
async function upload(key, bytes) {
 await s3.send(new PutObjectCommand({ Bucket: 'stellasora-app-releases', Key: key, Body: bytes, ContentType: key.endsWith('.json') ? 'application/json' : 'application/octet-stream' }));
}
const objectKey = `packages/${app}/${manifest.version}/${filename}`;
await upload(objectKey, archive);
manifest.url = `https://update.stellasora.cafe/${objectKey}`;
const feed = path.basename(manifestPath);
if (!/^update-[a-z0-9-]+\.json$/.test(feed)) throw new Error('Invalid platform manifest name');
await upload(`v1/${app}/${channel}/${feed}`, JSON.stringify(manifest));
console.log(`Published https://update.stellasora.cafe/v1/${app}/${channel}/${feed}`);
