// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0
'use strict';
const control = (action) => { location.href = `camp-desktop://window-control?action=${action}`; };
document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => control(button.dataset.action)));
document.querySelector('#drag').addEventListener('mousedown', (event) => { if (event.button === 0 && event.detail === 1) control('drag'); });
document.querySelector('#drag').addEventListener('dblclick', () => control('maximize'));
document.querySelectorAll('img').forEach((image) => { image.addEventListener('error', () => image.remove()); if (image.complete && image.naturalWidth === 0) image.remove(); });
const updateCaption = () => {
 const maximized = document.documentElement.dataset.campWindowMaximized === 'true';
 const maximize = document.querySelector('[data-action="maximize"]');
 const label = maximized ? '还原' : '最大化';
 maximize.setAttribute('aria-label', label);
 maximize.title = label;
};
window.addEventListener('camp-window-state-change', updateCaption);
updateCaption();
