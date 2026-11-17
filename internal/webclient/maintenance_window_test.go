// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMaintenancePreloadRemovesActionsWithoutFrontend(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node required for DOM test")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	desktopPackage := filepath.Join(cwd, "../../package.json")
	encodedPackage, _ := json.Marshal(desktopPackage)
	encodedScript, _ := json.Marshal(maintenancePreload("https://camp.example/__camp-maintenance/v1.html#/", "windows", true))
	encodedMainScript, _ := json.Marshal(maintenancePreload("https://camp.example/__camp-maintenance/v1.html#/", "windows", false))
	script := `import {createRequire} from 'node:module';import {pathToFileURL} from 'node:url';import assert from 'node:assert/strict';
const require=createRequire(` + string(encodedPackage) + `);const {Window}=await import(pathToFileURL(require.resolve('happy-dom')).href);
const page=new Window({url:'https://camp.example/__camp-maintenance/v1.html#/'});
page.document.body.innerHTML='<main><h1>维护中</h1><div class="actions"><button>刷新</button><button data-camp-offline-toolbox>工具箱</button></div></main>';
page.eval(` + string(encodedScript) + `);
assert.equal(page.document.querySelector('.actions'),null);assert.equal(page.document.querySelector('[data-camp-offline-toolbox]'),null);
assert.equal(page.document.querySelector('h1').textContent,'维护中');assert.equal(page.document.querySelectorAll('#camp-maintenance-caption button').length,3);
page.document.body.insertAdjacentHTML('beforeend','<div class="actions"><button>刷新</button></div>');
await new Promise(resolve=>setTimeout(resolve,5));assert.equal(page.document.querySelector('.actions'),null);
page.close();
const main=new Window({url:'https://camp.example/__camp-maintenance/v1.html#/'});
main.document.body.innerHTML='<main><div class="actions"><button id="retry">existing retry</button><button data-camp-offline-toolbox disabled>existing toolbox</button></div></main>';
const actions=main.document.querySelector('.actions');let clicks=0;main.document.getElementById('retry').addEventListener('click',()=>clicks++);
main.eval(` + string(encodedMainScript) + `);
assert.equal(main.document.querySelector('.actions'),actions);main.document.getElementById('retry').click();assert.equal(clicks,1);
assert.equal(main.document.querySelector('[data-camp-offline-toolbox]').disabled,true);
assert.equal(main.document.querySelectorAll('#camp-maintenance-caption button').length,3);
assert.ok(!main.document.getElementById('camp-maintenance-style').textContent.includes('.actions'));
main.close();`
	command := exec.Command(node, "--input-type=module", "-e", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("maintenance DOM: %v\n%s", err, output)
	}
}
