// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { Window } from 'happy-dom';

const source = readFileSync(new URL('../internal/webclient/package_update.go', import.meta.url), 'utf8');
const script = source.match(/const packageRestartSafeScript = `([\s\S]*?)`/)[1];

test('package restart permits empty inline composer and protects drafts, uploads and dialogs', () => {
	const window = new Window();
	const document = window.document;
	document.body.innerHTML = '<main id="misskey_app"><form><textarea></textarea><input type="file" hidden></form></main>';
	const safe = () => window.eval(script);
	assert.equal(safe(), true);
	document.querySelector('textarea').value = 'unsent draft';
	assert.equal(safe(), false);
	document.querySelector('textarea').value = '';
	document.querySelector('form').dataset.campPendingEdit = 'true';
	assert.equal(safe(), false);
	delete document.querySelector('form').dataset.campPendingEdit;
	const dialog = document.createElement('dialog');
	dialog.setAttribute('open', '');
	document.body.append(dialog);
	assert.equal(safe(), false);
	dialog.remove();
	const transfer = new window.DataTransfer();
	transfer.items.add(new window.File(['pending'], 'upload.txt'));
	document.querySelector('input').files = transfer.files;
	assert.equal(safe(), false);
	document.getElementById('misskey_app').remove();
	assert.equal(safe(), false);
	window.close();
});

test('maintenance permits closing cached pages but preserves edits and other dialogs', () => {
	const window = new Window();
	const document = window.document;
	document.documentElement.dataset.campMaintenance = 'true';
	document.body.innerHTML = '<main id="misskey_app"><textarea></textarea><input><select><option selected>A</option><option>B</option></select></main><dialog open><div tabindex="-1"><section aria-live="polite">maintenance</section></div></dialog>';
	// happy-dom does not expose HTMLOptionElement.defaultSelected.
	for (const option of document.querySelectorAll('option')) Object.defineProperty(option, 'defaultSelected', { value: option.hasAttribute('selected') });
	const suffix = source.match(/const maintenanceCloseSafeScript = `\(` \+ packageRestartSafeScript \+ `([\s\S]*?)`/)[1];
	const safe = () => window.eval('(' + script + suffix);
	assert.equal(safe(), true);
	document.querySelector('textarea').value = 'unsaved';
	assert.equal(safe(), false);
	document.querySelector('textarea').value = '';
	document.querySelector('input').value = 'changed';
	assert.equal(safe(), false);
	document.querySelector('input').value = '';
	document.querySelector('select').selectedIndex = 1;
	assert.equal(safe(), false);
	document.querySelector('select').selectedIndex = 0;
	document.body.insertAdjacentHTML('beforeend', '<dialog open>other work</dialog>');
	assert.equal(safe(), false);
	document.body.lastElementChild.remove();
	delete document.documentElement.dataset.campMaintenance;
	assert.equal(safe(), false);
	document.querySelector('dialog').setAttribute('data-camp-maintenance-view', '');
	assert.equal(safe(), true);
	window.close();
});
