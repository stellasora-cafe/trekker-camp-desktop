// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import worker from './_worker.js';

const packagePath = '/packages/test/1/test.tar.gz';
const manifestPath = '/v1/test/stable/update-windows-amd64.json';
const fixtures = new Map([[packagePath.slice(1), 'data'], [manifestPath.slice(1), '{"version":"1.0.0"}']]);

function request(path, options) {
 const head = async key => {
  const body = fixtures.get(key);
  return body == null ? null : { size: body.length, httpEtag: '"test"', writeHttpMetadata() {} };
 };
 const get = async (key, options) => {
  const object = await head(key);
  if (!object) return null;
  const body = fixtures.get(key);
  const range = options?.range;
  return { ...object, body: new Response(range ? body.slice(range.offset, range.offset + range.length) : body).body };
 };
 return worker.fetch(new Request(`https://update.stellasora.cafe${path}`, options), { RELEASES: { get, head } });
}

test('package downloads support streaming, HEAD, and byte ranges', async () => {
 for (const method of ['GET', 'HEAD']) {
  const response = await request(packagePath, { method });
  assert.equal(response.status, 200);
  assert.equal(response.headers.get('Content-Range'), null);
  assert.equal(response.headers.get('Content-Length'), '4');
  assert.match(response.headers.get('Cache-Control'), /immutable/);
  assert.equal(await response.text(), method === 'HEAD' ? '' : 'data');
 }
 const ranged = await request(packagePath, { headers: { Range: 'bytes=-2' } });
 assert.equal(ranged.status, 206);
 assert.equal(ranged.headers.get('Content-Range'), 'bytes 2-3/4');
 assert.equal(ranged.headers.get('Content-Length'), '2');
 assert.equal(await ranged.text(), 'ta');

 const unsatisfiable = await request(packagePath, { headers: { Range: 'bytes=4-' } });
 assert.equal(unsatisfiable.status, 416);
 assert.equal(unsatisfiable.headers.get('Content-Range'), 'bytes */4');
 const changed = await request(packagePath, { headers: { Range: 'bytes=2-', 'If-Range': '"old"' } });
 assert.equal(changed.status, 200);
 assert.equal(await changed.text(), 'data');
});

test('discovery and release manifests stay fresh and the feed is read-only', async () => {
 assert.equal((await (await request('/')).json()).runtime, 'worker');
 const manifest = await request(manifestPath, { headers: { Range: 'bytes=0-5' } });
 assert.equal(manifest.status, 200);
 assert.equal(manifest.headers.get('Cache-Control'), 'no-store');
 assert.deepEqual(await manifest.json(), { version: '1.0.0' });

 const missing = await request('/v1/another-app/stable/update-windows-amd64.json');
 assert.equal(missing.status, 404);
 assert.equal(missing.headers.get('Cache-Control'), 'no-store');
 assert.deepEqual(await missing.json(), { error: 'release_not_found' });
 assert.equal((await request(manifestPath, { method: 'PUT' })).status, 405);
});
