// web/app.js against a fake app (spec §7). Node's own test runner, no packages: `node --test test/web/app.test.mjs`.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const APP_JS = fs.readFileSync(fileURLToPath(new URL('../../web/app.js', import.meta.url)), 'utf8');

/* ---- a DOM just big enough for app.js ---- */

class FakeElement {
  constructor(tag) {
    Object.assign(this, { tag, children: [], attrs: {}, listeners: {}, className: '', textContent: '', value: '',
                          hidden: false, disabled: false, checked: false, open: false, parent: null });
  }
  append(...kids) {
    for (const k of kids) {
      if (k instanceof FakeElement) k.parent = this;
      this.children.push(k);
    }
  }
  replaceChildren(...kids) { this.children = []; this.append(...kids); }
  replaceWith(el) {
    const p = this.parent;
    if (!p) return;
    p.children[p.children.indexOf(this)] = el;
    el.parent = p;
  }
  setAttribute(k, v) { this.attrs[k] = v; }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  async fire(type, target = this) {
    await Promise.all((this.listeners[type] || []).map((fn) => fn({ target, preventDefault() {} })));
    await settle();
  }
  click() { return this.fire('click'); }
}

function below(el) {
  return [el, ...el.children.filter((k) => k instanceof FakeElement).flatMap(below)];
}

function text(el) {
  return typeof el === 'string' ? el : el.textContent + el.children.map(text).join('');
}

const settle = () => new Promise((r) => setTimeout(r, 0));

/* ---- the app ---- */

function baseState() {
  return {
    hwinfo: { status: 'ok', detail: '' },
    items: [
      { key: 'k:cpu', group: 'CPU [#0]: Demo', label: 'Total CPU Usage', unit: '%', type: 'usage', value: 12.4 },
      { key: 'k:temp', group: 'CPU [#0]: Demo', label: 'CPU (Tctl/Tdie)', unit: '°C', type: 'temperature', value: 54.26 },
      { key: 'k:ram', group: 'System: Demo', label: 'Physical Memory Used', unit: 'MB', type: 'other', value: 21811 },
    ],
    picked: [
      { key: 'k:cpu', id: 'cpu_load', label: 'CPU load', unit: '%', scale: 1, decimals: 0, min: 0, max: 100 },
      { key: 'k:ram', id: 'ram_used', label: 'RAM', unit: 'GB', scale: 1 / 1024, decimals: 1 },
    ],
    boards: [],
    connection: { addresses: ['192.168.1.20'], port: 47123, listening: 47123, ui_port: 47124, token: 'ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z' },
    app: { version: '1.0.0', name: 'DESKTOP-TEST', autostart: false, config_path: 'C:\\Users\\x\\AppData\\Roaming\\reflbo-stats\\config.json' },
  };
}

function reply(status, body) {
  return { status, ok: status >= 200 && status < 300, json: async () => body };
}

async function open(state = baseState(), handlers = {}) {
  const els = { app: new FakeElement('main'), pill: new FakeElement('span') };
  const calls = [];
  let tick = null;
  const clipboard = [];
  const sandbox = {
    document: { createElement: (tag) => new FakeElement(tag), getElementById: (id) => els[id] },
    fetch: async (path, opts = {}) => {
      const method = opts.method || 'GET';
      const body = opts.body === undefined ? undefined : JSON.parse(opts.body);
      calls.push({ method, path, body, type: (opts.headers || {})['Content-Type'] });
      const handler = handlers[`${method} ${path}`];
      if (handler) return handler(body, state);
      if (method === 'GET' && path === '/api/state') return reply(200, state);
      return reply(404, { error: 'no such API' });
    },
    setInterval: (fn) => { tick = fn; },
    navigator: { clipboard: { writeText: async (t) => { clipboard.push(t); } } },
    console, JSON, Math, Number, String, Map, Promise, Date, Object, Error, RegExp,
  };
  vm.createContext(sandbox);
  vm.runInContext(APP_JS, sandbox);
  await settle();
  await settle();
  return { root: els.app, pill: els.pill, calls, clipboard, tick: async () => { await tick(); await settle(); },
           setState: (s) => { state = s; } };
}

const all = (root, tag) => below(root).filter((e) => e.tag === tag);
const button = (root, name) => {
  const b = all(root, 'button').find((e) => text(e) === name);
  assert.ok(b, `no button "${name}"`);
  return b;
};
const checkbox = (root, label) => {
  const l = all(root, 'label').find((e) => text(e).includes(label));
  assert.ok(l, `no reading "${label}"`);
  return l.children.find((k) => k.tag === 'input');
};

test('the page shows HWiNFO, the picked readings as the boards get them, and the sensors', async () => {
  const p = await open();
  const page = text(p.root);
  assert.match(page, /HWiNFO is running, and its readings come in\./);
  assert.equal(p.pill.textContent, 'HWiNFO ok');
  assert.match(page, /Readings the boards get \(2 of 32\)/);
  assert.match(page, /12 %/); // cpu_load at 0 decimals
  assert.match(page, /21\.3 GB/); // 21811 MB × 1/1024 at 1 decimal
  assert.match(page, /CPU \[#0\]: Demo \(2\)/);
  assert.match(page, /System: Demo \(1\)/);
  assert.equal(checkbox(p.root, 'Total CPU Usage').checked, true);
  assert.equal(checkbox(p.root, 'CPU (Tctl/Tdie)').checked, false);
  const scale = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'scale' && e.value === '1/1024');
  assert.ok(scale, 'the RAM scale shows as 1/1024');
  assert.equal(button(p.root, 'Save').disabled, true);
});

test('ticking a reading and saving sends it for the app to fill in', async () => {
  const saved = [];
  const p = await open(baseState(), {
    'PUT /api/readings': (body, state) => {
      saved.push(body);
      const next = { ...state, picked: [...body.slice(0, 2), { key: 'k:temp', id: 'cpu_tctl_tdie', label: 'CPU (Tctl/Tdie)', unit: '°C', scale: 1, decimals: 1 }] };
      return reply(200, next);
    },
  });
  const box = checkbox(p.root, 'CPU (Tctl/Tdie)');
  box.checked = true;
  await box.fire('change');
  assert.match(text(p.root), /\(on save\)/);
  assert.equal(button(p.root, 'Save').disabled, false);
  await button(p.root, 'Save').click();
  assert.deepEqual(saved[0][2], { key: 'k:temp' });
  assert.equal(p.calls.at(-1).type, 'application/json');
  assert.match(text(p.root), /Saved\. The boards have the new list\./);
  assert.match(text(p.root), /Readings the boards get \(3 of 32\)/);
  assert.equal(button(p.root, 'Save').disabled, true);
});

test('a refusal shows the app\'s reason and keeps the edits', async () => {
  const p = await open(baseState(), {
    'PUT /api/readings': () => reply(400, { error: 'id "Bad": 1-15 characters of a-z, 0-9 and _' }),
  });
  const id = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'id' && e.value === 'cpu_load');
  id.value = 'Bad';
  await id.fire('input');
  await button(p.root, 'Save').click();
  assert.match(text(p.root), /id "Bad": 1-15 characters of a-z, 0-9 and _/);
  assert.equal(button(p.root, 'Save').disabled, false);
  const still = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'id' && e.value === 'Bad');
  assert.ok(still, 'the edit stays');
});

test('an empty min goes as none, a fraction as its value, and Undo brings the saved list back', async () => {
  const sent = [];
  const p = await open(baseState(), { 'PUT /api/readings': (body, state) => { sent.push(body); return reply(200, state); } });
  const min = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'min' && e.value === '0');
  min.value = '';
  await min.fire('input');
  const scale = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'scale' && e.value === '1');
  scale.value = '1/1000';
  await scale.fire('input');
  await button(p.root, 'Save').click();
  assert.equal('min' in sent[0][0], false);
  assert.equal(sent[0][0].scale, 0.001);
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  label.value = 'Changed';
  await label.fire('input');
  await button(p.root, 'Undo changes').click();
  assert.ok(all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load'));
});

test('a new token needs a second click, and Cancel keeps the old one', async () => {
  let posts = 0;
  const p = await open(baseState(), {
    'POST /api/token': (body, state) => { posts++; return reply(200, { ...state, connection: { ...state.connection, token: 'ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-Z' } }); },
  });
  await button(p.root, 'New token').click();
  assert.equal(posts, 0);
  await button(p.root, 'Cancel').click();
  assert.match(text(p.root), /New token/);
  await button(p.root, 'New token').click();
  await button(p.root, 'Confirm: every board needs the new token').click();
  assert.equal(posts, 1);
  assert.match(text(p.root), /ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-Z/);
});

test('Copy puts the token on the clipboard', async () => {
  const p = await open();
  await button(p.root, 'Copy').click();
  assert.deepEqual(p.clipboard, ['ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z']);
  assert.match(text(p.root), /Copied/);
});

test('Start with Windows and the port go to the app', async () => {
  const puts = [];
  const p = await open(baseState(), {
    'PUT /api/app': (body, state) => {
      puts.push(body);
      const next = clone(state);
      if (body.autostart !== undefined) next.app.autostart = body.autostart;
      if (body.port !== undefined) next.connection.port = body.port;
      return reply(200, next);
    },
  });
  const auto = checkbox(p.root, 'Start with Windows');
  auto.checked = true;
  await auto.fire('change');
  assert.deepEqual(puts[0], { autostart: true });
  const port = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'port');
  port.value = '48000';
  await button(p.root, 'Apply').click();
  assert.deepEqual(puts[1], { port: 48000 });
  assert.doesNotMatch(text(p.root), /after a restart/);
});

function clone(x) { return JSON.parse(JSON.stringify(x)); }

test('the live refresh moves the values and keeps an edit in progress', async () => {
  const state = baseState();
  const p = await open(state);
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  label.value = 'Mine';
  await label.fire('input');
  const next = clone(state);
  next.items[0].value = 87.6;
  next.boards = [{ addr: '192.168.1.66:51000', interval_ms: 1000, since: '2026-10-19T12:00:00Z' }];
  p.setState(next);
  await p.tick();
  assert.match(text(p.root), /88 %/);
  assert.match(text(p.root), /87\.6 %/);
  assert.match(text(p.root), /192\.168\.1\.66:51000/);
  assert.ok(all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'Mine'), 'the edit stays');
  assert.equal(button(p.root, 'Save').disabled, false);
});

test('HWiNFO\'s other states say what to do', async () => {
  for (const [status, detail, words] of [['not running', '', /Start it \(Sensors-only is enough\)/],
    ['not running', 'HWiNFO\'s shared memory version 9 is unknown to this app', /version 9 is unknown to this app/],
    ['shm off', '', /Shared Memory Support is off/], ['stale', '', /stopped updating/]]) {
    const s = baseState();
    s.hwinfo = { status, detail };
    const p = await open(s);
    assert.match(text(p.root), words);
    assert.equal(p.pill.textContent, `HWiNFO ${status}`);
    assert.match(text(p.root), /—/); // the boards get no value
    if (detail) assert.doesNotMatch(text(p.root), /Start it/); // the reason says more than the advice
  }
});

test('32 picked readings leave the other boxes off', async () => {
  const s = baseState();
  s.picked = Array.from({ length: 32 }, (_, i) => ({ key: `x:${i}`, id: `r${i}`, label: `R${i}`, unit: '', scale: 1, decimals: 0 }));
  s.picked[0].key = 'k:cpu';
  const p = await open(s);
  assert.equal(checkbox(p.root, 'CPU (Tctl/Tdie)').disabled, true);
  assert.equal(checkbox(p.root, 'Total CPU Usage').disabled, false);
  assert.match(text(p.root), /32 readings at most\./);
});

test('the search narrows the sensors', async () => {
  const p = await open();
  const search = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'search');
  search.value = 'memory';
  await search.fire('input');
  assert.match(text(p.root), /Physical Memory Used/);
  assert.doesNotMatch(text(p.root), /Total CPU Usage/);
});

test('an edit keeps the field it is typed in and enables the same Save button', async () => {
  const sent = [];
  const p = await open(baseState(), { 'PUT /api/readings': (body, state) => { sent.push(body); return reply(200, state); } });
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  const save = button(p.root, 'Save');
  label.value = 'CPU busy';
  await label.fire('input');
  assert.ok(below(p.root).includes(label), 'the field being typed in is still the same one');
  assert.ok(below(p.root).includes(save), 'the Save button is still the same one');
  assert.equal(save.disabled, false);
  await save.click(); // the first click after the edit saves
  assert.equal(sent.length, 1);
  assert.equal(sent[0][0].label, 'CPU busy');
});

test('the search keeps its box, and a collapsed group stays collapsed', async () => {
  const p = await open();
  const system = all(p.root, 'details').find((d) => text(d).startsWith('System: Demo'));
  system.open = false;
  await system.fire('toggle');
  const search = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'search');
  search.value = 'cpu';
  await search.fire('input');
  assert.ok(below(p.root).includes(search), 'the search box is still the same one');
  search.value = '';
  await search.fire('input');
  const again = all(p.root, 'details').find((d) => text(d).startsWith('System: Demo'));
  assert.equal(again.open, false);
  assert.equal(all(p.root, 'details').find((d) => text(d).startsWith('CPU [#0]')).open, true);
});

test('a LAN port the app can\'t open says so', async () => {
  const s = baseState();
  s.connection = { ...s.connection, listening: 0, error: 'listen tcp :47123: bind: address already in use' };
  const p = await open(s);
  assert.match(text(p.root), /The boards can't reach this PC: listen tcp :47123: bind: address already in use\. Choose another LAN port under App, and Apply\./);
  assert.match(text(p.root), /port —/);
});
