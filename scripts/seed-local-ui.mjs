// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Deliberately fixed: this fixture must never target a deployed instance.
const base = 'http://localhost:3000';
const desktop = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const artifacts = path.join(desktop, 'artifacts');
const stateFile = path.join(artifacts, 'local-ui-seed-state.json');
const accountFile = path.join(artifacts, 'local-ui-accounts.json');
const reportFile = path.join(artifacts, 'local-ui-report.json');
const prefix = '[本地 UI 验收]';
const mutations = [];
const mutatingEndpoints = new Set([
	'admin/accounts/create', 'signin-flow', 'i/update', 'following/create', 'following/requests/accept', 'following/update',
	'drive/files/create', 'notes/create', 'notes/reactions/create', 'notes/polls/vote', 'notes/drafts/create',
	'notifications/create', 'notifications/test-notification', 'admin/roles/create', 'admin/roles/assign',
	'i/export-notes', 'miauth/gen-token', 'i/claim-achievement', 'chat/rooms/create', 'chat/rooms/invitations/create',
	'chat/rooms/join', 'chat/messages/create-to-user', 'chat/messages/create-to-room',
]);
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
const read = async (file, fallback) => {
	try { return JSON.parse(await fs.readFile(file, 'utf8')); } catch (error) {
		if (error.code === 'ENOENT') return fallback;
		throw error;
	}
};
const write = (file, value) => fs.writeFile(file, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
const state = await read(stateFile, { notes: {}, messages: {}, completed: {} });
const save = () => write(stateFile, state);
const accounts = await read(accountFile, {});
const admin = await read(path.join(artifacts, 'local-account.json'));
assert.equal(admin.username, 'campdev', 'Expected the dedicated local campdev fixture');

async function api(actor, endpoint, data = {}, protocol = 'desktop-v1') {
	const apiPath = protocol === 'web-v1' ? '/api/' : '/api/client/';
	for (let attempt = 0; attempt < 4; attempt++) {
		const headers = { 'X-Camp-Client': protocol, 'X-Camp-Device': 'local-ui-fixtures' };
		if (actor?.session) headers.Authorization = `Bearer ${actor.session}`;
		if (!(data instanceof FormData)) headers['Content-Type'] = 'application/json';
		const response = await fetch(base + apiPath + endpoint, {
			method: 'POST', headers, redirect: 'error', signal: AbortSignal.timeout(30000),
			body: data instanceof FormData ? data : JSON.stringify(data),
		});
		const text = await response.text();
		const result = text ? JSON.parse(text) : null;
		if (response.ok) {
			if (mutatingEndpoints.has(endpoint)) mutations.push({ endpoint, username: actor?.username ?? data.username });
			return result;
		}
		if (response.status === 429 && attempt < 3) { await wait(3000 * (attempt + 1)); continue; }
		const error = new Error(`${endpoint}: HTTP ${response.status} ${result?.error?.code ?? 'request failed'}`);
		error.code = result?.error?.code;
		throw error;
	}
}

async function once(key, action) {
	if (state.completed[key]) return;
	await action();
	state.completed[key] = true;
	await save();
}

admin.user = await api(admin, 'i');
assert.equal(admin.user.isAdmin, true);
let peer = accounts.peer;
if (!peer) {
	peer = { username: 'campqa', password: `Qa!${randomBytes(20).toString('hex')}`, base };
	accounts.peer = peer;
	await write(accountFile, accounts);
}
if (!peer.session) {
	let existing;
	try { existing = await api(admin, 'users/show', { username: peer.username }); } catch (error) {
		if (error.code !== 'NO_SUCH_USER') throw error;
	}
	if (existing) {
		const signin = await api(null, 'signin-flow', { username: peer.username, password: peer.password });
		peer.session = signin.i;
	} else {
		const created = await api(admin, 'admin/accounts/create', { username: peer.username, password: peer.password });
		peer.id = created.id;
		peer.session = created.token;
	}
	await write(accountFile, accounts);
}
peer.user = await api(peer, 'i');
peer.id = peer.user.id;
await once('peer-profile', () => api(peer, 'i/update', {
	name: '验收同伴 · 星野', description: `${prefix} 第二测试账号，用于双向心链、群聊、通知及帖子控件验收。`,
	chatScope: 'mutual', isLocked: true,
}));
let relation = await api(admin, 'users/show', { userId: peer.id });
if (!relation.isFollowing) {
	if (!relation.hasPendingFollowRequestFromYou) await api(admin, 'following/create', { userId: peer.id });
	await api(peer, 'following/requests/accept', { userId: admin.user.id });
}
relation = await api(peer, 'users/show', { userId: admin.user.id });
if (!relation.isFollowing) await api(peer, 'following/create', { userId: admin.user.id });
await once('peer-unlock', () => api(peer, 'i/update', { isLocked: false }));

for (const actor of [admin, peer]) {
	const key = actor === admin ? 'admin' : 'peer';
	accounts[key] = { ...accounts[key], username: actor.username, id: actor.user.id, password: actor.password, session: actor.session, base };
	if (!accounts[key].webSession) {
		const signed = await api(null, 'signin-flow', { username: actor.username, password: actor.password }, 'web-v1');
		accounts[key].webSession = signed.i;
	}
}
accounts.instructions = '仅 localhost:3000。原生客户端使用 username/password 或 session（desktop-v1）；网页使用 username/password 或独立 webSession（web-v1）。不得将桌面 session 注入网页。原 local-account.json 未改动。';
await write(accountFile, accounts);
console.log(`Local accounts ready: @${admin.username}, @${peer.username}; credentials: artifacts/local-ui-accounts.json`);

let files = await api(peer, 'drive/files', { limit: 100 });
let photo = files.find(file => file.name === 'local-ui-fixture.png');
if (!photo) {
	const bytes = await fs.readFile(path.join(desktop, 'scripts/fixtures/192.png'));
	const body = new FormData();
	body.set('file', new Blob([bytes], { type: 'image/png' }), 'local-ui-fixture.png');
	body.set('comment', `${prefix} 图片查看器、缩略图和聊天附件验收`);
	photo = await api(peer, 'drive/files/create', body);
}
await once('peer-avatar', () => api(peer, 'i/update', { avatarId: photo.id }));

const noteCache = new Map();
async function note(actor, key, text, extra = {}) {
	if (state.notes[key]) return state.notes[key];
	if (!noteCache.has(actor.user.id)) noteCache.set(actor.user.id, await api(actor, 'users/notes', { userId: actor.user.id, limit: 100, withReplies: true }));
	const marker = `${prefix} ${key}`;
	let item = noteCache.get(actor.user.id).find(item => item.text?.startsWith(marker));
	if (!item) {
		item = (await api(actor, 'notes/create', { text: `${marker}\n${text}`, localOnly: true, ...extra })).createdNote;
		noteCache.get(actor.user.id).push(item);
		await wait(400);
	}
	state.notes[key] = item.id;
	await save();
	return item.id;
}
const primary = await note(admin, '互动基准帖', '欢迎测试旅人营地！这里可以验证回复、引用、回应和通知。 #本地验收');
const plain = await note(peer, '普通帖子', '今天也一起探索星塔吧。**粗体**、*斜体*、~~删除线~~、`code` 与链接 [本地首页](http://localhost:3000/) 都可以点击查看。 #本地验收');
await note(peer, '长文展开', Array.from({ length: 22 }, (_, i) => `第 ${i + 1} 段：沿着星塔的阶梯前行，记录伙伴们的故事。展开后继续滚动，观察段落布局、帖子点击区域与后退位置。`).join('\n\n'));
await note(peer, 'CW 内容警告', '这是展开之后的正文。\n$[jelly 弹跳文字] 和 $[sparkle 闪耀文字] 用于观察 MFM 加载是否闪烁。', { cw: `${prefix} 剧情讨论，点击展开` });
await note(peer, '图片帖子', '图片与图注测试。点击缩略图进入查看器。', { fileIds: [photo.id] });
const poll = await note(admin, '进行中投票', '下一站想去哪里？投票结果应包含真实票数。', { poll: { choices: ['星塔', '营地', '远行'], multiple: false } });
await once('vote-open', () => api(peer, 'notes/polls/vote', { noteId: poll, choice: 1 }));
const discoveryPoll = await note(peer, '发现页投票', '选一个喜欢的营地活动吧。此帖由同伴发布，用于 campdev 的发现页投票控件。', { poll: { choices: ['一起探索', '围炉聊天', '整理收藏'], multiple: false } });
const ending = await note(peer, '已结束投票', '短期投票，用于结束状态和 pollEnded 通知。', { poll: { choices: ['准备好了', '继续休息'], expiredAfter: 20000, multiple: false } });
await once('vote-ending', () => api(admin, 'notes/polls/vote', { noteId: ending, choice: 0 }));
await note(peer, '提及通知', `@${admin.username} 来查看这条提及通知。`);
await note(peer, '指定可见', `@${admin.username} 这条仅对测试双方可见，用于直接消息通知。`, { visibility: 'specified', visibleUserIds: [admin.user.id] });
await note(peer, '回复通知', '收到，我来验证回复链和返回位置。', { replyId: primary });
await note(peer, '引用通知', '引用同伴的验收帖。', { renoteId: primary });
await once('renote', () => api(peer, 'notes/create', { renoteId: primary, localOnly: true }));
await once('reaction', () => api(peer, 'notes/reactions/create', { noteId: primary, reaction: '❤' }));
await once('peer-reaction', () => api(admin, 'notes/reactions/create', { noteId: plain, reaction: '🎉' }));
await once('follow-notification', () => api(admin, 'following/update', { userId: peer.id, notify: 'normal' }));
await note(peer, '关注者新帖通知', '开启了同伴的新帖通知后，这条内容会出现在通知列表。');
for (const id of Object.values(state.notes)) {
	const item = await api(admin, 'notes/show', { noteId: id });
	if (item.visibility !== 'public' || item.replyId) continue;
	const actor = item.userId === admin.user.id ? peer : admin;
	await once(`featured-reaction-${id}`, async () => {
		const ownView = await api(actor, 'notes/show', { noteId: id });
		if (!ownView.myReaction) await api(actor, 'notes/reactions/create', { noteId: id, reaction: '👍' });
	});
	await once(`featured-renote-${id}`, () => api(actor, 'notes/create', { renoteId: id, localOnly: true }));
}
for (const actor of [admin, peer]) {
	await once(`app-notification-${actor.user.id}`, () => api(actor, 'notifications/create', { header: `${prefix} 自定义通知`, body: '查看通知图标、正文换行与点击状态。' }));
	await once(`test-notification-${actor.user.id}`, () => api(actor, 'notifications/test-notification'));
}

const roles = await api(admin, 'admin/roles/list');
const roleName = `${prefix} 旅人同伴`;
const role = roles.find(role => role.name === roleName) ?? await api(admin, 'admin/roles/create', {
	name: roleName, description: '本地验收使用的普通展示角色，不授予管理权限。', color: '#15a9b5', iconUrl: null,
	target: 'manual', condFormula: {}, isPublic: true, isModerator: false, isAdministrator: false,
	isExplorable: true, asBadge: true, preserveAssignmentOnMoveAccount: false, canEditMembersByModerator: false,
	displayOrder: 0, policies: {},
});
for (const actor of [admin, peer]) {
	await once(`role-${actor.user.id}`, () => api(admin, 'admin/roles/assign', { roleId: role.id, userId: actor.user.id }));
	await once(`export-${actor.user.id}`, () => api(actor, 'i/export-notes'));
	await once(`token-${actor.user.id}`, async () => {
		const existing = await api(actor, 'i/apps');
		if (!existing.some(app => app.name === `${prefix} 无权限通知测试`)) {
			const token = await api(actor, 'miauth/gen-token', { session: null, name: `${prefix} 无权限通知测试`, permission: [] });
			accounts[actor === admin ? 'admin' : 'peer'].notificationToken = token.token;
			await write(accountFile, accounts);
		}
	});
}
await once('peer-achievement', () => api(peer, 'i/claim-achievement', { name: 'notes1' }));
state.roleId = role.id;
state.drafts ??= {};
for (const fail of [false, true]) {
	const key = fail ? '定时发布失败' : '定时发布成功';
	await once(`scheduled-${key}`, async () => {
		const text = `${prefix} ${key}\n用于定时发布通知和草稿恢复验收。`;
		const existing = await api(admin, 'notes/drafts/list', { limit: 100 });
		let draft = existing.find(draft => draft.text === text);
		for (let attempt = 0; !draft && attempt < 40; attempt++) {
			try {
				draft = (await api(admin, 'notes/drafts/create', {
					text, localOnly: true, scheduledAt: Date.now() + 18000, isActuallyScheduled: true,
					...(fail ? { poll: { choices: ['过期选项一', '过期选项二'], expiresAt: Date.now() + 5000 } } : {}),
				})).createdDraft;
			} catch (error) {
				if (error.code !== 'TOO_MANY_SCHEDULED_NOTES' || attempt === 39) throw error;
				await wait(1000);
			}
		}
		state.drafts[key] = draft.id;
	});
}

// A pending request needs a third actor while keeping the primary pair mutual.
let requester = accounts.requester;
if (!requester) {
	requester = { username: 'camprequest', password: `Qa!${randomBytes(20).toString('hex')}`, base };
	accounts.requester = requester;
	await write(accountFile, accounts);
}
if (!requester.session) {
	let existing;
	try { existing = await api(admin, 'users/show', { username: requester.username }); } catch (error) {
		if (error.code !== 'NO_SUCH_USER') throw error;
	}
	if (existing) {
		const signed = await api(null, 'signin-flow', { username: requester.username, password: requester.password });
		requester.id = signed.id;
		requester.session = signed.i;
	} else {
		const created = await api(admin, 'admin/accounts/create', { username: requester.username, password: requester.password });
		requester.id = created.id;
		requester.session = created.token;
	}
	await write(accountFile, accounts);
}
await once('pending-follow', async () => {
	await api(peer, 'i/update', { isLocked: true });
	const relation = await api(requester, 'users/show', { userId: peer.id });
	if (!relation.isFollowing && !relation.hasPendingFollowRequestFromYou) await api(requester, 'following/create', { userId: peer.id });
});
await once('vote-discovery', () => api(requester, 'notes/polls/vote', { noteId: discoveryPoll, choice: 0 }));

async function room(owner, name) {
	const rooms = await api(owner, 'chat/rooms/owned', { limit: 100 });
	return rooms.find(room => room.name === `${prefix} ${name}`) ?? await api(owner, 'chat/rooms/create', { name: `${prefix} ${name}`, description: '仅用于本地原生、网页与手机布局验收。' });
}
const group = await room(admin, '双人群聊');
const joining = await api(peer, 'chat/rooms/joining', { limit: 100 });
if (!joining.some(item => (item.roomId ?? item.room?.id ?? item.id) === group.id)) {
	const inbox = await api(peer, 'chat/rooms/invitations/inbox', { limit: 100 });
	if (!inbox.some(item => (item.roomId ?? item.room?.id) === group.id)) await api(admin, 'chat/rooms/invitations/create', { roomId: group.id, userId: peer.id });
	await api(peer, 'chat/rooms/join', { roomId: group.id });
}
const invitationRoom = await room(peer, '待接受邀请');
await once('pending-room-invitation', () => api(peer, 'chat/rooms/invitations/create', { roomId: invitationRoom.id, userId: admin.user.id }));
state.roomId = group.id;
state.invitationRoomId = invitationRoom.id;
await save();

async function message(actor, other, key, text, roomId, fileId) {
	if (state.messages[key]) return state.messages[key];
	const endpoint = roomId ? 'room' : 'user';
	const list = await api(actor, `chat/messages/${endpoint}-timeline`, { [roomId ? 'roomId' : 'userId']: roomId ?? other.user.id, limit: 100 });
	const content = `${prefix} ${key}\n${text}`;
	let result = list.find(item => item.text === content);
	if (!result) result = await api(actor, `chat/messages/create-to-${endpoint}`, { [roomId ? 'toRoomId' : 'toUserId']: roomId ?? other.user.id, text: content, ...(fileId ? { fileId } : {}) });
	state.messages[key] = result.id;
	await save();
	return result.id;
}
const outgoing = await message(admin, peer, '私聊发出', '你好，这是一条 campdev 发给同伴的真实心链消息。');
const incoming = await message(peer, admin, '私聊收到', '已收到！这里是 campqa 的回复。\n第二行用于验证气泡高度与时间布局。');
await message(peer, admin, '私聊附件', '附上一张本地测试图片。', null, photo.id);
const groupOut = await message(admin, peer, '群聊发出', '欢迎加入双人群聊。', group.id);
const groupIn = await message(peer, admin, '群聊收到', '**同伴已加入**，消息已送达。', group.id);

const checks = [];
for (const [actor, other] of [[admin, peer], [peer, admin]]) {
	const direct = await api(actor, 'chat/messages/user-timeline', { userId: other.user.id, limit: 100 });
	assert(direct.some(item => item.id === outgoing) && direct.some(item => item.id === incoming), `${actor.username}: bidirectional messages missing`);
	const roomMessages = await api(actor, 'chat/messages/room-timeline', { roomId: group.id, limit: 100 });
	assert(roomMessages.some(item => item.id === groupOut) && roomMessages.some(item => item.id === groupIn), `${actor.username}: room messages missing`);
	const history = await api(actor, 'chat/history', { limit: 100 });
	assert(history.some(item => item.fromUserId === other.user.id || item.toUserId === other.user.id), `${actor.username}: private history missing`);
	const roomHistory = await api(actor, 'chat/history', { room: true, limit: 100 });
	assert(roomHistory.some(item => item.toRoomId === group.id), `${actor.username}: room history missing`);
	checks.push({ username: actor.username, directMessages: direct.length, roomMessages: roomMessages.length, directHistory: true, roomHistory: true });
}
const voted = await api(admin, 'notes/show', { noteId: poll });
assert(voted.poll.choices.some(choice => choice.votes > 0), 'Poll has no votes');
const notifications = {};
// Queue jobs are real API operations; wait for their delivery before reporting.
const expectedAsync = ['pollEnded', 'exportCompleted', 'scheduledNotePosted', 'scheduledNotePostFailed'];
for (let attempt = 0; attempt < 35; attempt++) {
	const list = await api(admin, 'i/notifications', { limit: 100, markAsRead: false });
	if (expectedAsync.every(type => list.some(item => item.type === type))) break;
	await wait(1000);
}
for (const actor of [admin, peer]) {
	const list = [];
	let untilId;
	for (;;) {
		const page = await api(actor, 'i/notifications', { limit: 100, markAsRead: false, untilId });
		list.push(...page);
		if (page.length < 100) break;
		untilId = page.at(-1).id;
	}
	notifications[actor.username] = { count: list.length, types: [...new Set(list.map(item => item.type))].sort() };
}
const expectedNotificationTypes = ['note', 'follow', 'mention', 'reply', 'renote', 'quote', 'reaction', 'pollEnded', 'scheduledNotePosted', 'scheduledNotePostFailed', 'receiveFollowRequest', 'followRequestAccepted', 'roleAssigned', 'chatRoomInvitationReceived', 'achievementEarned', 'exportCompleted', 'login', 'createToken', 'app', 'test'];
const observedNotificationTypes = [...new Set(Object.values(notifications).flatMap(result => result.types))].sort();
const missingNotificationTypes = expectedNotificationTypes.filter(type => !observedNotificationTypes.includes(type));
const featured = await api(admin, 'notes/featured', { limit: 100 });
const published = (await api(admin, 'users/notes', { userId: admin.user.id, limit: 100, withReplies: true })).find(item => item.text?.startsWith(`${prefix} 定时发布成功`));
const report = {
	base, prefix, checkedAt: new Date().toISOString(), accounts: [{ username: admin.username, id: admin.user.id }, { username: peer.username, id: peer.id }],
	notes: Object.entries(state.notes).map(([name, id]) => ({ name, id, url: `${base}/notes/${id}` })),
	chat: { checks, roomId: group.id, invitationRoomId: invitationRoom.id, messages: state.messages,
		adminPrivateUrl: `${base}/chat/user/${peer.id}`, peerPrivateUrl: `${base}/chat/user/${admin.user.id}`, groupUrl: `${base}/chat/room/${group.id}` },
	notifications, imageId: photo.id, pollVotes: voted.poll.choices.map(choice => ({ text: choice.text, votes: choice.votes })),
	roleId: role.id, scheduledDrafts: state.drafts, pendingFollowRequest: { username: requester.username, id: requester.id, recipient: peer.username },
	scheduledPublishedNote: published ? { id: published.id, url: `${base}/notes/${published.id}` } : null,
	notificationCoverage: { expected: expectedNotificationTypes.length, observed: observedNotificationTypes, missing: missingNotificationTypes },
	featured: { count: featured.length, fixtureIds: featured.filter(item => item.text?.startsWith(prefix)).map(item => item.id) },
	mutations,
};
await write(reportFile, report);
assert.equal(missingNotificationTypes.length, 0, `Missing notification fixtures: ${missingNotificationTypes.join(', ')}`);
assert(published, 'Scheduled success fixture was not published');
console.log(JSON.stringify(report, null, 2));
