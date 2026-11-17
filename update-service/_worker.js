// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0
export default {
 async fetch(request, env) {
  const { pathname } = new URL(request.url);
  const headers = new Headers({
   'Access-Control-Allow-Origin': '*',
   'Cache-Control': 'no-store',
   'X-Content-Type-Options': 'nosniff',
  });
  const head = request.method === 'HEAD';
  const json = (body, status = 200) => {
   headers.set('Content-Type', 'application/json');
   return new Response(head ? null : JSON.stringify(body), { status, headers });
  };
  if (request.method !== 'GET' && !head) {
   headers.set('Allow', 'GET, HEAD');
   return new Response(null, { status: 405, headers });
  }
  if (pathname === '/') return json({
   service: 'Stella Sora Café Updates',
   api: 1,
   runtime: 'worker',
   manifest: '/v1/{app}/{channel}/update-{platform}-{arch}.json',
   packages: '/packages/{app}/{version}/{file}',
  });

  const manifest = /^\/v1\/[a-z0-9-]+\/[a-z0-9-]+\/update-[a-z0-9-]+\.json$/.test(pathname);
  const artifact = /^\/packages\/[a-z0-9-]+\/[a-zA-Z0-9._-]+\/[a-zA-Z0-9._-]+$/.test(pathname);
  if (!manifest && !artifact) return json({ error: 'not_found' }, 404);

  const key = pathname.slice(1);
  const rangeHeader = request.headers.get('Range');
  const rangeMatch = artifact && !head && rangeHeader?.match(/^bytes=(\d*)-(\d*)$/i);
  let range;
  if (rangeMatch && (rangeMatch[1] || rangeMatch[2])) {
   const metadata = await env.RELEASES.head(key);
   if (!metadata) return json({ error: 'release_not_found' }, 404);
   const ifRange = request.headers.get('If-Range');
   if (!ifRange || ifRange === metadata.httpEtag) {
    const size = metadata.size;
    const start = rangeMatch[1] ? Number(rangeMatch[1]) : Math.max(0, size - Number(rangeMatch[2]));
    const end = rangeMatch[1] && rangeMatch[2] ? Math.min(size - 1, Number(rangeMatch[2])) : size - 1;
    if (start > end) {
     headers.set('Content-Range', `bytes */${size}`);
     return new Response(null, { status: 416, headers });
    }
    range = { offset: start, length: end - start + 1 };
   }
  }

  const object = head ? await env.RELEASES.head(key) : await env.RELEASES.get(key, range ? { range } : undefined);
  if (!object) return json({ error: 'release_not_found' }, 404);
  object.writeHttpMetadata(headers);
  headers.set('ETag', object.httpEtag);
  headers.set('Cache-Control', manifest ? 'no-store' : 'public, max-age=31536000, immutable');
  headers.set('Content-Type', manifest ? 'application/json' : 'application/octet-stream');
  headers.set('Content-Length', String(range?.length ?? object.size));
  if (artifact) headers.set('Accept-Ranges', 'bytes');
  if (range) headers.set('Content-Range', `bytes ${range.offset}-${range.offset + range.length - 1}/${object.size}`);
  return new Response(head ? null : object.body, { status: range ? 206 : 200, headers });
 },
};
