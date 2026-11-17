// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const preload = readFileSync(0, 'utf8');
const base = 'http://localhost:3000';

class Storage {
	constructor() { this.values = new Map(); }
	getItem(key) { return this.values.get(key) ?? null; }
	setItem(key, value) { this.values.set(key, String(value)); }
	removeItem(key) { this.values.delete(key); }
	clear() { this.values.clear(); }
}

class BrowserEvents extends EventTarget {}
for (const type of ['readystatechange', 'progress', 'load', 'abort']) {
	Object.defineProperty(BrowserEvents.prototype, 'on' + type, {
		get() { return this['_on' + type]; },
		set(value) {
			if (this['_on' + type]) this.removeEventListener(type, this['_on' + type]);
			this['_on' + type] = value;
			if (value) this.addEventListener(type, value);
		},
	});
}

class NativeXHR extends BrowserEvents {
	constructor() {
		super(); this.upload = new BrowserEvents(); this.readyState = 0; this.openCount = 0;
	}
	open(method, url, ...args) {
		this.method = method; this.url = url; this.args = args; this.headers = new Headers();
		this.responseType = ''; this.timeout = 0; this.withCredentials = false; this.mimeType = null;
		this.sent = false; this.readyState = 1; this.openCount++;
		this.dispatchEvent(new Event('readystatechange'));
	}
	setRequestHeader(name, value) {
		if (this.readyState !== 1 || this.sent) throw new DOMException('Invalid state', 'InvalidStateError');
		this.headers.append(name, value);
	}
	overrideMimeType(value) { this.mimeType = value; }
	send(body) {
		if (this.readyState !== 1 || this.sent) throw new DOMException('Invalid state', 'InvalidStateError');
		this.sent = true; this.body = body; this.readyState = 2;
		this.dispatchEvent(new Event('readystatechange'));
		this.upload.dispatchEvent(Object.assign(new Event('progress'), { loaded: 12, total: 24 }));
	}
	abort() { this.readyState = 0; this.upload.dispatchEvent(new Event('abort')); this.dispatchEvent(new Event('abort')); }
}

function realm(options = {}) {
	const requests = [];
	const observers = [];
	const context = vm.createContext({
		URL, URLSearchParams, Headers, Request, Response, FormData, Blob, AbortController,
		setTimeout: () => 0,
		location: new URL(options.origin ?? base + '/'),
		document: { documentElement: options.delayedRoot ? null : { dataset: {} } },
		MutationObserver: class {
			constructor(callback) { this.callback = callback; observers.push(this); }
			observe(target, options) { this.target = target; this.options = options; }
			disconnect() { this.disconnected = true; }
		},
		localStorage: options.localStorage ?? new Storage(),
		sessionStorage: options.sessionStorage ?? new Storage(),
		XMLHttpRequest: NativeXHR,
		WebSocket: class extends EventTarget {
			static OPEN = 1;
			constructor(url, protocols) { super(); this.url = url; this.protocols = protocols; }
		},
		fetch: async (input, init) => {
			const request = new Request(input, init);
			requests.push(request);
			if (request.signal.aborted) throw request.signal.reason;
			return new Response('{}', { status: options.status ?? 200 });
		},
	});
	context.window = context;
	context.top = options.frame ? {} : context;
	vm.runInContext(preload, context);
	return { context, requests, observers, last: () => requests.at(-1) };
}

async function run() {
	const accountStorage = new Storage();
	accountStorage.setItem('account', JSON.stringify({ id: 'alpha', token: 'cs_test_alpha' }));
	const first = realm({ localStorage: accountStorage });
	const w = first.context;
	assert.equal(JSON.parse(w.localStorage.getItem('account')).id, 'alpha');
	assert.equal(w.localStorage.getItem('lang'), 'zh-CN');
	assert.equal(w.document.documentElement.dataset.campDesktop, 'true');
	assert.equal(first.observers.length, 0);
	const fetchOnce = w.fetch;
	vm.runInContext(preload, w);
	assert.equal(w.fetch, fetchOnce, 'repeated preload must not wrap the transport twice');

	const json = value => ({ method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Camp-Client': 'web-v1' }, body: JSON.stringify(value) });
	await w.fetch(base + '/api/i', json({}));
	assert.equal(first.last().url, base + '/api/client/i');
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_test_alpha');
	assert.equal(first.last().headers.get('X-Camp-Client'), 'desktop-v1');
	assert.equal(decodeURIComponent(first.last().headers.get('X-Camp-Device-Name')), '旅人营地桌面客户端');
	assert.equal(first.last().headers.get('X-Camp-Device-Platform'), 'windows');
	assert.equal(first.last().headers.get('X-Camp-Desktop-Version'), '0.1.0');
	assert.equal(first.last().headers.get('X-Camp-Host-Api'), '2');
	assert.equal(first.last().redirect, 'error');
	assert.equal(first.last().credentials, 'omit');

	await w.fetch(base + '/api/i?i=cs_query&detail=true', json({ i: 'cs_candidate' }));
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_candidate');
	assert.equal(first.last().url, base + '/api/client/i?detail=true');
	assert.deepEqual(await first.last().json(), {});
	await w.fetch(base + '/api/i?i=cs_query', json({ i: null }));
	assert.equal(first.last().headers.get('Authorization'), null);
	await w.fetch(base + '/api/i?i=cs_query');
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_query');
	await w.fetch(base + '/api/i', { ...json({ i: 'cs_body' }), headers: { 'Content-Type': 'application/json', Authorization: 'Bearer cs_header' } });
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_header');

	await w.fetch(base + '/api/client/notes/create', json({ i: 'scoped_plugin', text: 'test' }));
	assert.equal(first.last().url, base + '/api/notes/create');
	assert.equal(first.last().headers.get('X-Camp-Client'), 'third-party-v1');
	assert.equal(first.last().headers.get('Authorization'), 'Bearer scoped_plugin');
	assert.deepEqual(await first.last().json(), { text: 'test' });
	await w.fetch(base + '/api/i', { headers: { 'X-Camp-Client': 'third-party-v1' } });
	assert.equal(first.last().headers.get('Authorization'), null);
	assert.equal(first.last().url, base + '/api/i');

	await w.fetch('https://outside.invalid/api/i', json({}));
	assert.equal(first.last().url, 'https://outside.invalid/api/i');
	assert.equal(first.last().headers.get('Authorization'), null);
	assert.equal(first.last().headers.get('X-Camp-Client'), 'web-v1');
	assert.equal(first.last().headers.get('X-Camp-Device-Name'), null);
	await w.fetch(base + '/static-assets/image.png');
	assert.equal(first.last().headers.get('Authorization'), null);
	assert.equal(first.last().url, base + '/static-assets/image.png');

	const controller = new AbortController();
	const request = new Request(base + '/api/i', { ...json({}), signal: controller.signal, cache: 'no-cache', mode: 'same-origin', referrerPolicy: 'no-referrer' });
	await w.fetch(request);
	const sent = first.last();
	assert.equal(sent.cache, 'no-cache'); assert.equal(sent.mode, 'same-origin'); assert.equal(sent.referrerPolicy, 'no-referrer');
	controller.abort('test-abort');
	assert.equal(sent.signal.aborted, true); assert.equal(sent.signal.reason, 'test-abort');
	await assert.rejects(w.fetch(new Request(base + '/api/i', { signal: AbortSignal.abort(new Error('aborted request')) })), /aborted request/);

	const multipart = new FormData();
	multipart.set('i', 'cs_upload'); multipart.set('name', 'sample'); multipart.set('file', new Blob(['payload']), 'sample.txt');
	await w.fetch(base + '/api/drive/files/create', { method: 'POST', body: multipart });
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_upload');
	const uploaded = await first.last().formData();
	assert.equal(uploaded.has('i'), false); assert.equal(uploaded.get('file').name, 'sample.txt'); assert.equal(await uploaded.get('file').text(), 'payload');
	assert.equal(multipart.get('i'), 'cs_upload', 'caller-owned form must remain intact');
	await w.fetch(base + '/api/i', { method: 'POST', body: new URLSearchParams({ i: 'cs_form', value: 'kept' }) });
	assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_form');
	assert.equal(await first.last().text(), 'value=kept');
	await w.fetch(base + '/api/i', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{invalid' });
	assert.equal(await first.last().text(), '{invalid');

	const xhr = new w.XMLHttpRequest();
	const uploadTarget = xhr.upload;
	let opened = 0, progress = 0, aborted = 0;
	xhr.onreadystatechange = () => { if (xhr.readyState === 1) opened++; };
	xhr.upload.onprogress = event => { progress = event.loaded; };
	xhr.addEventListener('abort', () => aborted++);
	xhr.open('POST', base + '/api/drive/files/create', true);
	xhr.responseType = 'json'; xhr.timeout = 1234; xhr.withCredentials = true;
	xhr.overrideMimeType('application/json');
	xhr.setRequestHeader('X-Camp-Client', 'web-v1'); xhr.setRequestHeader('X-Trace', 'test');
	xhr.send(multipart);
	assert.equal(xhr.url, base + '/api/client/drive/files/create');
	assert.equal(xhr.headers.get('Authorization'), 'Bearer cs_upload');
	assert.equal(xhr.headers.get('X-Camp-Client'), 'desktop-v1');
	assert.equal(xhr.headers.get('X-Trace'), 'test');
	assert.equal(xhr.headers.get('X-Camp-Device-Platform'), 'windows');
	assert.equal(xhr.headers.get('X-Camp-Desktop-Version'), '0.1.0');
	assert.equal(xhr.headers.get('X-Camp-Host-Api'), '2');
	assert.equal(xhr.headers.has('Content-Type'), false);
	assert.equal(xhr.body.has('i'), false); assert.equal(multipart.has('i'), true);
	assert.equal(xhr.upload, uploadTarget); assert.equal(progress, 12); assert.equal(opened, 1);
	assert.equal(xhr.responseType, 'json'); assert.equal(xhr.timeout, 1234); assert.equal(xhr.mimeType, 'application/json'); assert.equal(xhr.withCredentials, false);
	assert.throws(() => xhr.send(multipart), /Invalid state/);
	xhr.abort(); assert.equal(aborted, 1);
	assert.throws(() => xhr.send(multipart), /Invalid state/);

	for (const token of [null, 'cs_other', 'scoped_upload']) {
		const candidate = new w.XMLHttpRequest(); candidate.open('POST', base + '/api/i');
		candidate.setRequestHeader('Content-Type', 'application/json'); candidate.send(JSON.stringify({ i: token }));
		assert.equal(candidate.headers.get('Authorization'), token ? 'Bearer ' + token : null);
		assert.equal(candidate.headers.get('X-Camp-Client'), token === 'scoped_upload' ? 'third-party-v1' : 'desktop-v1');
		assert.deepEqual(JSON.parse(candidate.body), {});
	}
	const external = new w.XMLHttpRequest(); external.open('POST', 'https://outside.invalid/api/upload'); external.send(new FormData());
	assert.equal(external.headers.get('Authorization'), null); assert.equal(external.openCount, 1);

	let ws = new w.WebSocket('/streaming?client=web-v1', ['protocol']);
	assert.equal(new URL(ws.url).searchParams.get('desktopVersion'), '0.1.0');
	assert.equal(new URL(ws.url).searchParams.get('hostApi'), '2');
	const outdated = realm({ status: 426 });
	await outdated.context.fetch(base + '/api/i');
	assert.equal(outdated.context.location.href, 'camp-desktop://frontend-check');
	const socketOutdated = realm();
	const denied = new socketOutdated.context.WebSocket('/client-streaming');
	denied.dispatchEvent(new Event('error'));
	assert.equal(socketOutdated.context.location.href, 'camp-desktop://frontend-check');
	let target = new URL(ws.url);
	assert.equal(target.pathname, '/client-streaming'); assert.equal(target.searchParams.get('client'), 'desktop-v1'); assert.equal(target.searchParams.get('i'), 'cs_test_alpha');
	assert.deepEqual(ws.protocols, ['protocol']); assert.equal(w.WebSocket.OPEN, 1);
	ws = new w.WebSocket('ws://localhost:3000/streaming?i=cs_other');
	assert.equal(new URL(ws.url).searchParams.get('i'), 'cs_other');
	ws = new w.WebSocket('ws://localhost:3000/streaming?i=');
	assert.equal(new URL(ws.url).searchParams.has('i'), false);
	ws = new w.WebSocket('ws://localhost:3000/streaming?i=scoped_socket&client=web-v1');
	target = new URL(ws.url); assert.equal(target.pathname, '/streaming'); assert.equal(target.searchParams.get('client'), 'third-party-v1'); assert.equal(target.searchParams.get('i'), 'scoped_socket');
	assert.equal(new w.WebSocket('wss://localhost:3000/streaming').url, 'wss://localhost:3000/streaming');
	assert.equal(new w.WebSocket('wss://outside.invalid/streaming').url, 'wss://outside.invalid/streaming');
	assert.equal(w.location.href, base + '/', 'transport must leave the document URL unchanged');

	w.localStorage.setItem('account', JSON.stringify({ id: 'beta', token: 'cs_test_beta' }));
	await w.fetch(base + '/api/i'); assert.equal(first.last().headers.get('Authorization'), 'Bearer cs_test_beta');
	const switched = realm({ localStorage: w.localStorage });
	assert.equal(JSON.parse(switched.context.localStorage.getItem('account')).id, 'beta');
	await switched.context.fetch(base + '/api/i'); assert.equal(switched.last().headers.get('Authorization'), 'Bearer cs_test_beta');
	w.localStorage.clear();
	assert.equal(w.localStorage.values.size, 0);
	const signedOut = realm({ localStorage: w.localStorage });
	assert.equal(signedOut.context.localStorage.getItem('account'), null);
	await signedOut.context.fetch(base + '/api/i'); assert.equal(signedOut.last().headers.get('Authorization'), null);
	assert.equal(new URL(new signedOut.context.WebSocket('/streaming').url).searchParams.has('i'), false);

	const existing = new Storage();
	existing.setItem('account', JSON.stringify({ id: 'existing', token: 'cs_existing' }));
	existing.setItem('lang', 'zh-TW');
	const retained = realm({ localStorage: existing });
	assert.equal(JSON.parse(retained.context.localStorage.getItem('account')).id, 'existing');
	assert.equal(retained.context.localStorage.getItem('lang'), 'zh-TW');
	await retained.context.fetch(base + '/api/i'); assert.equal(retained.last().headers.get('Authorization'), 'Bearer cs_existing');
	const guest = realm();
	assert.equal(guest.context.localStorage.getItem('account'), null);
	await guest.context.fetch(base + '/api/signin-flow', json({ username: 'test-user', password: 'test-password' }));
	assert.equal(guest.last().url, base + '/api/client/signin-flow');
	assert.equal(guest.last().headers.get('Authorization'), null);
	assert.equal(guest.last().headers.get('X-Camp-Client'), 'desktop-v1');
	assert.deepEqual(await guest.last().json(), { username: 'test-user', password: 'test-password' });
	await guest.context.fetch(base + '/api/i', json({ i: 'cs_new_login' }));
	assert.equal(guest.last().headers.get('Authorization'), 'Bearer cs_new_login');
	guest.context.localStorage.setItem('account', JSON.stringify({ id: 'logged-in', token: 'cs_new_login' }));
	const loggedIn = realm({ localStorage: guest.context.localStorage });
	assert.equal(JSON.parse(loggedIn.context.localStorage.getItem('account')).id, 'logged-in');
	await loggedIn.context.fetch(base + '/api/signup', json({ i: null, username: 'new-user', password: 'test-password' }));
	assert.equal(loggedIn.last().url, base + '/api/client/signup');
	assert.equal(loggedIn.last().headers.get('Authorization'), null);
	assert.equal(loggedIn.last().headers.get('X-Camp-Client'), 'desktop-v1');

	const pendingRoot = realm({ delayedRoot: true });
	assert.equal(pendingRoot.observers.length, 1);
	const observer = pendingRoot.observers[0];
	assert.equal(observer.target, pendingRoot.context.document);
	assert.equal(observer.options.childList, true);
	observer.callback();
	assert.equal(observer.disconnected, undefined, 'wait for the document element');
	pendingRoot.context.document.documentElement = { dataset: {} };
	observer.callback();
	assert.equal(pendingRoot.context.document.documentElement.dataset.campDesktop, 'true');
	assert.equal(observer.disconnected, true);

	const other = realm({ origin: 'https://outside.invalid/' });
	assert.equal(other.context.localStorage.getItem('account'), null);
	assert.equal(other.context.document.documentElement.dataset.campDesktop, undefined);
	await other.context.fetch('https://outside.invalid/api/i'); assert.equal(other.last().headers.get('Authorization'), null);
	const iframe = realm({ frame: true });
	assert.equal(iframe.context.localStorage.getItem('account'), null);
	assert.equal(iframe.context.document.documentElement.dataset.campDesktop, undefined);
	await iframe.context.fetch(base + '/api/i'); assert.equal(iframe.last().url, base + '/api/i');
	process.stdout.write('campweb browser transport passed\n');
}

async function testTrayNotifications() {
	const page = realm();
	const w = page.context;
	w.document.documentElement.dataset.campDesktopWindow = 'main';
	w.localStorage.setItem('account', JSON.stringify({ id: 'alpha', token: 'cs_alpha' }));
	w.localStorage.setItem('preferences', JSON.stringify({ preferences: { 'sound.on.chatMessage': [[{}, { volume: 0 }]] } }));
	const socket = new w.WebSocket('/streaming');
	const receive = (connection, type, body, isTrusted = true) => {
		const event = new Event('message');
		Object.defineProperties(event, {
			data: { value: JSON.stringify({ type: 'channel', body: { id: 'existing-main', type, body } }) },
			isTrusted: { value: isTrusted },
		});
		connection.dispatchEvent(event);
	};

	let receivedByPage = 0;
	socket.addEventListener('message', () => { receivedByPage++; });
	const reply = { id: 'reply-1', type: 'reply', user: { name: '旅人' } };
	receive(socket, 'notification', reply);
	assert.equal(receivedByPage, 1);
	assert.equal(page.last().url, base + '/__camp-desktop/tray-notice');
	assert.equal(page.last().method, 'POST');
	assert.equal(page.last().headers.get('Authorization'), null);
	assert.deepEqual(await page.last().json(), { id: 'notification:reply-1', title: '旅人', body: '回复了你的帖子', silent: false });

	const chat = { id: 'chat-1', fromUserId: 'beta', fromUser: { username: 'beta' }, toRoomId: 'room', toRoom: { name: '营地' }, text: 'private text' };
	const reconnected = new w.WebSocket('/streaming');
	receive(reconnected, 'notification', reply);
	receive(reconnected, 'newChatMessage', chat);
	assert.deepEqual(await page.last().json(), { id: 'chat:chat-1', title: '@beta', body: '在「营地」发来一条心链消息', silent: true });

	receive(socket, 'newChatMessage', { ...chat, id: 'self', fromUserId: 'alpha' });
	receive(socket, 'notification', { ...reply, id: 'synthetic' }, false);
	receive(new w.WebSocket('wss://outside.invalid/client-streaming?i=cs_alpha'), 'notification', { ...reply, id: 'external' });
	w.Notification = { permission: 'denied' };
	receive(socket, 'notification', { ...reply, id: 'denied' });
	w.Notification.permission = 'granted';
	w.document.documentElement.dataset.campDesktopWindow = 'tool';
	receive(socket, 'notification', { ...reply, id: 'tool' });
	w.document.documentElement.dataset.campDesktopWindow = 'main';
	w.localStorage.setItem('account', JSON.stringify({ id: 'beta', token: 'cs_beta' }));
	receive(socket, 'notification', { ...reply, id: 'old-account' });
	assert.equal(page.requests.length, 2);
}

(process.argv[2] === 'tray' ? testTrayNotifications() : run()).catch(error => { console.error(error); process.exitCode = 1; });
