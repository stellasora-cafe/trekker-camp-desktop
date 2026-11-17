// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

const origin = new URL(seed.origin).origin;
if (window !== window.top || location.origin !== origin) return;

// A page may evaluate its preload again without creating a new document.
const installedKey = Symbol.for('camp.desktop.transport');
if (window[installedKey]) return;
window[installedKey] = true;

if (!localStorage.getItem('lang')) localStorage.setItem('lang', 'zh-CN');

const markDesktop = () => {
	if (!document.documentElement) return false;
	document.documentElement.dataset.campDesktop = 'true';
	return true;
};
if (!markDesktop()) {
	const observer = new MutationObserver(() => {
		if (markDesktop()) observer.disconnect();
	});
	observer.observe(document, { childList: true });
}

const deviceName = encodeURIComponent('旅人营地桌面客户端');
const devicePlatform = seed.platform.replace(/[^\x20-\x7e]/g, '').slice(0, 100);
const isApi = target => target.origin === origin && target.pathname.startsWith('/api/');
let compatibilityCheckPending = false;
const checkCompatibility = () => {
	if (compatibilityCheckPending) return;
	compatibilityCheckPending = true;
	location.href = 'camp-desktop://frontend-check';
	setTimeout(() => { compatibilityCheckPending = false; }, 30000);
};
const accountToken = () => {
	try {
		const account = JSON.parse(localStorage.getItem('account'));
		return typeof account?.token === 'string' ? account.token : null;
	} catch {
		return null;
	}
};

// Explicit credentials are used while validating another account and by
// permission-scoped plugins. An explicit null remains an anonymous request.
const identity = (headers, target, bodyToken) => {
	let token;
	if (headers.has('Authorization')) {
		const authorization = headers.get('Authorization');
		token = authorization.startsWith('Bearer ') ? authorization.slice(7) : null;
	} else if (bodyToken !== undefined) {
		token = bodyToken;
	} else if (target.searchParams.has('i')) {
		token = target.searchParams.get('i');
	} else {
		token = headers.get('X-Camp-Client') === 'third-party-v1' ? null : accountToken();
	}
	if (typeof token !== 'string' || token === '') token = null;
	const protocol = headers.get('X-Camp-Client') === 'third-party-v1' || token && !token.startsWith('cs_') ? 'third-party-v1' : 'desktop-v1';
	return { token, protocol };
};

const authenticated = (target, headers, bodyToken) => {
	const { token, protocol } = identity(headers, target, bodyToken);
	const endpoint = target.pathname.replace(/^\/api\/(?:client\/)?/, '');
	target.pathname = (protocol === 'desktop-v1' ? '/api/client/' : '/api/') + endpoint;
	target.searchParams.delete('i');
	headers.set('X-Camp-Client', protocol);
	headers.set('X-Camp-Device-Name', deviceName);
	headers.set('X-Camp-Device-Platform', devicePlatform);
	headers.set('X-Camp-Desktop-Version', seed.version);
	headers.set('X-Camp-Host-Api', String(seed.hostApi));
	if (!headers.has('Accept')) headers.set('Accept', 'application/json');
	if (token) headers.set('Authorization', 'Bearer ' + token);
	else headers.delete('Authorization');
};

const withoutBodyToken = (body, contentType) => {
	let token;
	if (body instanceof FormData) {
		if (body.has('i')) token = body.get('i');
		const clean = new FormData();
		for (const [key, value] of body) if (key !== 'i') clean.append(key, value);
		body = clean;
	} else if (body instanceof URLSearchParams) {
		body = new URLSearchParams(body);
		if (body.has('i')) token = body.get('i');
		body.delete('i');
	} else if (typeof body === 'string' && /\bapplication\/(?:[^;]+\+)?json\b/i.test(contentType)) {
		try {
			const value = JSON.parse(body);
			if (value && typeof value === 'object' && Object.hasOwn(value, 'i')) {
				token = value.i;
				delete value.i;
				body = JSON.stringify(value);
			}
		} catch { /* The API retains validation of malformed JSON. */ }
	} else if (typeof body === 'string' && contentType.toLowerCase().includes('application/x-www-form-urlencoded')) {
		const clean = withoutBodyToken(new URLSearchParams(body), contentType);
		body = clean.body.toString();
		token = clean.token;
	}
	return { body, token };
};

const fetchOriginal = window.fetch.bind(window);
window.fetch = async (input, init) => {
	const request = new Request(input, init);
	const target = new URL(request.url);
	if (!isApi(target)) return fetchOriginal(request);
	const headers = new Headers(request.headers);
	const contentType = headers.get('Content-Type') || '';
	let body;
	if (request.method !== 'GET' && request.method !== 'HEAD') {
		if (contentType.toLowerCase().includes('multipart/form-data')) {
			body = await request.formData();
			headers.delete('Content-Type');
		} else if (/\bapplication\/(?:[^;]+\+)?json\b/i.test(contentType) || contentType.toLowerCase().includes('application/x-www-form-urlencoded')) {
			body = await request.text();
		} else {
			body = await request.arrayBuffer();
		}
	}
	const clean = withoutBodyToken(body, contentType);
	authenticated(target, headers, clean.token);
	const response = await fetchOriginal(target.href, {
		method: request.method, headers, body: clean.body,
		signal: request.signal, credentials: 'omit', redirect: 'error',
		cache: request.cache, mode: request.mode, referrer: request.referrer,
		referrerPolicy: request.referrerPolicy, integrity: request.integrity,
		keepalive: request.keepalive,
	});
	if (response.status === 426) checkCompatibility();
	return response;
};

// Keep the browser's upload event target, progress, response and abort behavior.
// XHR has no redirect policy option; canonical API endpoints must not redirect.
if (window.XMLHttpRequest) {
	const XHR = window.XMLHttpRequest;
	const requests = new WeakMap();
	window.XMLHttpRequest = class extends XHR {
		constructor() {
			super();
			this.addEventListener('readystatechange', event => {
				if (requests.get(this)?.reopening) event.stopImmediatePropagation();
				if (requests.get(this)?.local && this.readyState === 4 && this.status === 426) checkCompatibility();
			});
		}
		open(method, raw, ...args) {
			const target = new URL(raw, location.href);
			const result = super.open(method, target.href, ...args);
			requests.set(this, { method, target, args, local: isApi(target), headers: new Headers(), sent: false });
			return result;
		}
		setRequestHeader(name, value) {
			const result = super.setRequestHeader(name, value);
			requests.get(this)?.headers.append(name, value);
			return result;
		}
		overrideMimeType(value) {
			const result = super.overrideMimeType(value);
			const state = requests.get(this);
			if (state) state.mimeType = value;
			return result;
		}
		send(body) {
			const state = requests.get(this);
			if (!state?.local || state.sent || this.readyState !== 1) return super.send(body);
			const headers = new Headers(state.headers);
			const clean = withoutBodyToken(body, headers.get('Content-Type') || '');
			if (clean.body instanceof FormData) headers.delete('Content-Type');
			authenticated(state.target, headers, clean.token);
			const responseType = this.responseType;
			const timeout = this.timeout;
			state.reopening = true;
			try {
				super.open(state.method, state.target.href, ...state.args);
			} finally {
				state.reopening = false;
			}
			this.responseType = responseType;
			this.timeout = timeout;
			this.withCredentials = false;
			if (state.mimeType !== undefined) super.overrideMimeType(state.mimeType);
			for (const [name, value] of headers) super.setRequestHeader(name, value);
			state.sent = true;
			return super.send(clean.body);
		}
	};
}

const Socket = window.WebSocket;
const socketOrigin = new URL(origin);
socketOrigin.protocol = socketOrigin.protocol === 'https:' ? 'wss:' : 'ws:';

const recentNotices = new Set();
const notificationBodies = new Map([
	['reply', '回复了你的帖子'],
	['mention', '在帖子中提到了你'],
	['reaction', '回应了你的帖子'],
	['renote', '转发了你的帖子'],
	['quote', '转发了你的帖子'],
	['follow', '关注了你'],
	['receiveFollowRequest', '向你发送了关注请求'],
]);

function relayTrayNotice(event, token) {
	if (!event.isTrusted || document.documentElement?.dataset.campDesktopWindow !== 'main' ||
		window.Notification?.permission === 'denied' || typeof event.data !== 'string' ||
		(!event.data.includes('"notification"') && !event.data.includes('"newChatMessage"'))) return;

	let frame, account, preferences;
	try {
		frame = JSON.parse(event.data);
		account = JSON.parse(localStorage.getItem('account'));
	} catch { return; }
	const type = frame?.body?.type;
	const data = frame?.body?.body;
	if (frame?.type !== 'channel' || (type !== 'notification' && type !== 'newChatMessage') ||
		typeof data?.id !== 'string' || !data.id || !account?.id || account.token !== token) return;

	const chat = type === 'newChatMessage';
	if (chat && data.fromUserId === account.id) return;
	const id = (chat ? 'chat:' : 'notification:') + data.id;
	if (recentNotices.has(id)) return;
	if (recentNotices.size >= 128) recentNotices.delete(recentNotices.values().next().value);
	recentNotices.add(id);

	try {
		preferences = JSON.parse(localStorage.getItem('preferences'))?.preferences;
	} catch { /* Use default notification sounds. */ }
	const pref = key => preferences?.[key]?.find?.(row => row?.[0]?.account == null && row?.[0]?.server == null && row?.[0]?.device == null)?.[1];
	const sound = pref(chat ? 'sound.on.chatMessage' : 'sound.on.notification');
	const silent = pref('sound.notUseSound') === true || pref('sound.useSoundOnlyWhenActive') === true ||
		pref('sound.masterVolume') === 0 || sound?.volume === 0 || (sound != null && sound.type == null);
	const person = chat ? data.fromUser : data.user;
	const name = typeof person?.name === 'string' ? person.name.trim() : '';
	const title = name || (typeof person?.username === 'string' && person.username ? '@' + person.username : '旅人营地');
	const body = chat
		? (data.toRoomId && typeof data.toRoom?.name === 'string' && data.toRoom.name
			? '在「' + data.toRoom.name.slice(0, 100) + '」发来一条心链消息'
			: '发来一条心链消息')
		: notificationBodies.get(data.type) ?? '你有一条新通知';
	void fetchOriginal(origin + '/__camp-desktop/tray-notice', {
		method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Camp-Tray-Notice': '1' },
		body: JSON.stringify({ id: id.slice(0, 128), title: title.slice(0, 128), body, silent }),
		credentials: 'omit', cache: 'no-store', redirect: 'error',
	}).catch(() => {});
}

window.WebSocket = class extends Socket {
	constructor(raw, protocols) {
		const target = new URL(raw, location.href);
		if (target.protocol === 'https:') target.protocol = 'wss:';
		else if (target.protocol === 'http:') target.protocol = 'ws:';
		if (target.origin === socketOrigin.origin && ['/streaming', '/client-streaming'].includes(target.pathname)) {
			const headers = new Headers();
			if (target.searchParams.has('client')) headers.set('X-Camp-Client', target.searchParams.get('client'));
			const { token, protocol } = identity(headers, target);
			target.pathname = protocol === 'desktop-v1' ? '/client-streaming' : '/streaming';
			target.searchParams.set('client', protocol);
			if (protocol === 'desktop-v1') {
				target.searchParams.set('desktopVersion', seed.version);
				target.searchParams.set('hostApi', String(seed.hostApi));
			}
			if (token) target.searchParams.set('i', token);
			else target.searchParams.delete('i');
		}
		super(target.href, protocols);
		if (target.origin === socketOrigin.origin && target.pathname === '/client-streaming') {
			this.addEventListener('error', checkCompatibility);
			const token = target.searchParams.get('i');
			if (seed.platform === 'windows' && token?.startsWith('cs_')) {
				this.addEventListener('message', event => relayTrayNotice(event, token));
			}
		}
	}
};
