'use strict';
/* The settings page (spec §7): plain JS, no build step. Its API, on 127.0.0.1 only: GET /api/state,
 * PUT /api/readings, POST /api/token, PUT /api/app. Live values refresh every second without touching the
 * fields being edited. */

const MAX_READINGS = 32;

const HWINFO = {
  ok: ['ok', 'HWiNFO is running, and its readings come in.'],
  'not running': ['bad', 'HWiNFO isn\'t running. Start it (Sensors-only is enough), with Settings ▸ Shared Memory Support on.'],
  'shm off': ['bad', 'HWiNFO runs, but its Shared Memory Support is off: turn it on in HWiNFO\'s Settings.'],
  stale: ['warn', 'HWiNFO has stopped updating its readings.'],
};

let state = null;      // the last /api/state
let picked = [];       // the editor's copy of the picked readings
let dirty = false;     // picked differs from the saved list
let filter = '';
let confirming = false; // New token waits for its second click
let message = null;    // { text, error } after a save
let appMessage = null; // the same for the App section
const closedGroups = new Set(); // the sensor groups the owner collapsed
const shown = new Map(); // a reading's key -> the elements showing its live value
const parts = {};      // the sections and controls updated in place

function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === null || v === undefined || v === false) continue;
    if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k === 'text') el.textContent = v;
    else if (k === 'class') el.className = v;
    else if (k === 'value' || k === 'selected' || k === 'checked' || k === 'disabled' || k === 'open') el[k] = v;
    else el.setAttribute(k, v === true ? '' : String(v));
  }
  el.append(...kids.flat(Infinity).filter((k) => k !== null && k !== undefined && k !== false));
  return el;
}

/* replaceChildren without the null and false a condition leaves (a browser would print them). */
function fill(el, ...kids) {
  el.replaceChildren(...kids.flat(Infinity).filter((k) => k !== null && k !== undefined && k !== false));
}

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `The app answered ${res.status}.`);
  return data;
}

function clone(x) { return JSON.parse(JSON.stringify(x)); }

function number(v, decimals) {
  if (v === null || v === undefined || !Number.isFinite(v)) return '—';
  return decimals === undefined ? String(Math.round(v * 100) / 100) : v.toFixed(decimals);
}

/* A scale as typed, a number (a comma works as the point) or a fraction ("1/1024"); NaN for anything else. */
function parseScale(text) {
  const m = /^\s*([0-9]*\.?[0-9]+)\s*(?:\/\s*([0-9]*\.?[0-9]+))?\s*$/.exec(text.replace(/,/g, '.'));
  if (!m) return NaN;
  return m[2] === undefined ? Number(m[1]) : Number(m[1]) / Number(m[2]);
}

/* And shown back: a whole fraction as one ("1/1024"), else the number. */
function showScale(scale) {
  if (scale === undefined || scale === null) return '';
  const inverse = 1 / scale;
  return scale < 1 && Math.abs(inverse - Math.round(inverse)) < 1e-9 ? `1/${Math.round(inverse)}` : String(scale);
}

function itemByKey(key) { return state.items.find((it) => it.key === key); }

/* What a board gets for a picked reading: HWiNFO's value scaled, at its decimals. */
function boardValue(r) {
  const it = itemByKey(r.key);
  if (!it || state.hwinfo.status !== 'ok') return '—';
  return `${number(it.value * (r.scale || 1), r.decimals)} ${r.unit || it.unit}`.trim();
}

/* The first scale the app would refuse: typed, but not a finite number above 0. A scale left out is 1. */
function badScale() {
  return picked.find((r) => r.scale !== undefined && !(Number.isFinite(r.scale) && r.scale > 0));
}

/* What the app saves for a reading's decimals when the page sends none (internal/server/ui.go). */
function defaultDecimals(unit) {
  if (['%', 'MHz', 'MB', 'RPM', 'KB/s', 'MB/s'].includes(unit)) return 0;
  return unit === 'V' ? 3 : 1;
}

function applyScaleCheck() {
  const bad = badScale();
  if (!bad || !parts.save) return;
  parts.save.disabled = true;
  parts.msg.className = 'error';
  parts.msg.textContent = `${bad.id || bad.label || bad.key}: the scale must be a number above 0`;
}

/* An edit only updates Save, Undo and the message in place: redrawing the table would take the focus from the
 * field being typed in, and the Save button from under the click that ends the edit. */
function setDirty(on) {
  dirty = on;
  message = null;
  if (parts.save) {
    parts.save.disabled = !dirty;
    parts.undo.disabled = !dirty;
    parts.msg.className = 'muted';
    parts.msg.textContent = '';
    applyScaleCheck();
  }
}

/* A change to the list itself (a reading added, moved or removed) redraws it. */
function restructure() {
  setDirty(true);
  renderPicked();
}

/* ---- sections ---- */

function hwinfoSection() {
  const [, text] = HWINFO[state.hwinfo.status] || ['bad', state.hwinfo.status];
  return h('section', {}, h('h2', { text: 'HWiNFO' }), // the reason first when there is one: it says more
    state.hwinfo.detail ? h('p', { class: 'error', text: state.hwinfo.detail }) : h('p', { text }));
}

function field(r, name, kind, attrs = {}, after = null) {
  const value = r[name] === undefined || r[name] === null ? '' : name === 'scale' ? showScale(r[name]) : r[name];
  return h('input', { type: kind, value: String(value), 'aria-label': name, ...attrs, oninput: (ev) => {
    const raw = ev.target.value.trim();
    if (name === 'scale') {
      r.scale = raw === '' ? 1 : parseScale(raw);
    } else if (kind === 'number') {
      if (raw === '') delete r[name];
      else r[name] = Number(raw);
    } else {
      r[name] = raw;
    }
    setDirty(true);
    if (after) after();
  } });
}

function move(i, by) {
  const j = i + by;
  if (j < 0 || j >= picked.length) return;
  [picked[i], picked[j]] = [picked[j], picked[i]];
  restructure();
}

function pickedRows() {
  shown.forEach((list, key) => shown.set(key, list.filter((el) => !el.picked)));
  return picked.map((r, i) => {
    const it = itemByKey(r.key);
    const live = h('td', { class: 'value', text: boardValue(r) });
    live.picked = true;
    shown.set(r.key, [...(shown.get(r.key) || []), live]);
    // a reading the page sends without decimals gets the unit's default from the app: show that one
    const wanted = () => (r.decimals !== undefined ? r.decimals : defaultDecimals(r.unit || (it ? it.unit : '')));
    const decimals = h('select', { 'aria-label': 'decimals', onchange: (ev) => { r.decimals = Number(ev.target.value); setDirty(true); } },
      [0, 1, 2, 3].map((d) => h('option', { value: String(d), selected: wanted() === d, text: String(d) })));
    const syncDecimals = () => decimals.children.forEach((o) => { o.selected = Number(o.value) === wanted(); });
    return h('tr', {},
      h('td', {}, r.id ? field(r, 'id', 'text', { maxlength: 15 }) : h('span', { class: 'muted', text: '(on save)' })),
      h('td', {}, field(r, 'label', 'text', { maxlength: 23, placeholder: it ? it.label : '' }),
        it ? null : h('div', { class: 'error', text: 'Not in HWiNFO now' })),
      h('td', {}, field(r, 'unit', 'text', { maxlength: 7, placeholder: it ? it.unit : '' }, syncDecimals)),
      h('td', {}, field(r, 'scale', 'text', { placeholder: '1' })),
      h('td', {}, decimals),
      h('td', {}, field(r, 'min', 'number', { step: 'any' })),
      h('td', {}, field(r, 'max', 'number', { step: 'any' })),
      live,
      h('td', {}, h('button', { text: '↑', 'aria-label': 'up', disabled: i === 0, onclick: () => move(i, -1) }),
        h('button', { text: '↓', 'aria-label': 'down', disabled: i === picked.length - 1, onclick: () => move(i, 1) }),
        h('button', { text: '✕', 'aria-label': 'remove', onclick: () => { picked.splice(i, 1); restructure(); renderSensorList(); } })));
  });
}

async function save() {
  if (badScale()) return;
  try {
    state = await api('PUT', '/api/readings', picked);
    picked = clone(state.picked);
    dirty = false;
    message = { text: 'Saved. The boards have the new list.' };
  } catch (e) {
    message = { text: e.message, error: true };
  }
  render();
}

function undo() {
  picked = clone(state.picked);
  dirty = false;
  message = null;
  render();
}

function renderPicked() {
  const body = picked.length
    ? h('div', { class: 'scroll' }, h('table', {},
      h('tr', {}, ['Id', 'Label', 'Unit', 'Scale', 'Decimals', 'Min', 'Max', 'Value', ''].map((t) => h('th', { text: t }))),
      pickedRows()))
    : h('p', { class: 'muted', text: 'Tick readings below to send them to the boards.' });
  parts.save = h('button', { class: 'primary', text: 'Save', disabled: !dirty, onclick: save });
  parts.undo = h('button', { text: 'Undo changes', disabled: !dirty, onclick: undo });
  parts.msg = h('span', { class: message && message.error ? 'error' : 'muted', text: message ? message.text : '' });
  fill(parts.picked,
    h('h2', { text: `Readings the boards get (${picked.length} of ${MAX_READINGS})` }),
    h('p', { class: 'muted', text: 'The boards name each one pc.<id>. Scale multiplies HWiNFO\'s value: 1/1024 turns MB into GB.' }),
    body,
    h('div', { class: 'row' }, parts.save, parts.undo, parts.msg));
  applyScaleCheck();
}

function toggle(item, on) {
  if (on) picked.push({ key: item.key });
  else picked = picked.filter((r) => r.key !== item.key);
  restructure();
  renderSensorList();
}

/* The sensors' list redraws as the search narrows it; the search box itself stays, with its focus. */
function renderSensorList() {
  const groups = new Map();
  const f = filter.toLowerCase();
  for (const it of state.items) {
    if (f && !`${it.group} ${it.label}`.toLowerCase().includes(f)) continue;
    if (!groups.has(it.group)) groups.set(it.group, []);
    groups.get(it.group).push(it);
  }
  shown.forEach((list, key) => shown.set(key, list.filter((el) => el.picked)));
  const full = picked.length >= MAX_READINGS;
  const list = [...groups].map(([group, its]) => h('details', { open: !closedGroups.has(group), ontoggle: (ev) => {
    if (ev.target.open) closedGroups.delete(group);
    else closedGroups.add(group);
  } },
    h('summary', { text: `${group} (${its.length})` }),
    its.map((it) => {
      const on = picked.some((r) => r.key === it.key);
      const value = h('span', { class: 'value', text: `${number(it.value)} ${it.unit}`.trim() });
      shown.set(it.key, [...(shown.get(it.key) || []), value]);
      return h('label', { class: 'item' },
        h('input', { type: 'checkbox', checked: on, disabled: full && !on, onchange: (ev) => toggle(it, ev.target.checked) }),
        h('span', { text: it.label }), value);
    })));
  fill(parts.sensorList,
    full ? h('p', { class: 'muted', text: `${MAX_READINGS} readings at most.` }) : null,
    list.length ? list : h('p', { class: 'muted', text: state.items.length ? 'Nothing matches.' : 'No readings yet.' }));
}

function renderSensors() {
  parts.sensorList = h('div', {});
  fill(parts.sensors,
    h('h2', { text: 'HWiNFO\'s readings' }),
    h('input', { type: 'text', placeholder: 'Search', value: filter, 'aria-label': 'search', oninput: (ev) => {
      filter = ev.target.value;
      renderSensorList();
    } }),
    parts.sensorList);
  renderSensorList();
}

function boardsSection() {
  const boards = state.boards || [];
  return h('section', {}, h('h2', { text: 'Boards' }),
    boards.length
      ? h('table', {}, h('tr', {}, ['Address', 'Interval', 'Since'].map((t) => h('th', { text: t }))),
        boards.map((b) => h('tr', {}, h('td', { text: b.addr }), h('td', { text: `${b.interval_ms} ms` }),
          h('td', { text: new Date(b.since).toLocaleTimeString() }))))
      : h('p', { class: 'muted', text: 'No board is connected. If one should be, allow reflbo-stats through Windows\' firewall for private networks.' }));
}

async function newToken() {
  if (!confirming) {
    confirming = true;
    render();
    return;
  }
  confirming = false;
  try {
    state = await api('POST', '/api/token', {});
    message = null;
  } catch (e) {
    message = { text: e.message, error: true };
  }
  render();
}

async function copyToken() {
  try {
    await navigator.clipboard.writeText(state.connection.token);
    parts.copied.textContent = 'Copied';
  } catch (e) {
    parts.copied.textContent = 'Select the token and copy it';
  }
}

function connectionSection() {
  const c = state.connection;
  parts.copied = h('span', { class: 'muted' });
  return h('section', {}, h('h2', { text: 'Connection' }),
    c.error ? h('p', { class: 'error', text: `The boards can't reach this PC: ${c.error}. Choose another LAN port under App, and Apply.` }) : null,
    h('p', { text: 'Enter these on reflbo\'s PC page:' }),
    h('p', {}, 'Address: ', (c.addresses || []).length ? c.addresses.map((a, i) => [i ? ' or ' : '', h('code', { text: a })]) : h('span', { class: 'muted', text: 'no network' }),
      ', port ', h('code', { text: c.listening ? String(c.listening) : '—' })),
    h('p', { class: 'row' }, 'Token: ', h('code', { text: c.token }),
      h('button', { text: 'Copy', onclick: copyToken }), parts.copied,
      h('button', { text: confirming ? 'Confirm: every board needs the new token' : 'New token', onclick: newToken }),
      confirming ? h('button', { text: 'Cancel', onclick: () => { confirming = false; render(); } }) : null));
}

async function putApp(body) {
  try {
    state = await api('PUT', '/api/app', body);
    appMessage = null;
  } catch (e) {
    appMessage = { text: e.message, error: true };
  }
  render();
}

function appSection() {
  const a = state.app;
  const c = state.connection;
  const port = h('input', { type: 'number', value: String(c.port), min: 1024, max: 65535, 'aria-label': 'port' });
  return h('section', {}, h('h2', { text: 'App' }),
    h('label', { class: 'row' }, h('input', { type: 'checkbox', checked: a.autostart, onchange: (ev) => putApp({ autostart: ev.target.checked }) }),
      'Start with Windows'),
    h('p', { class: 'row' }, 'LAN port ', port, h('button', { text: 'Apply', onclick: () => putApp({ port: Number(port.value) }) }),
      appMessage ? h('span', { class: 'error', text: appMessage.text }) : null),
    h('p', { class: 'muted', text: `Version ${a.version} on ${a.name}. Settings in ${a.config_path}.` }));
}

function renderPill() {
  const pill = document.getElementById('pill');
  const [cls] = HWINFO[state.hwinfo.status] || ['bad'];
  pill.className = `pill ${cls}`;
  pill.textContent = `HWiNFO ${state.hwinfo.status}`;
}

function render() {
  renderPill();
  shown.clear();
  parts.hwinfo = hwinfoSection();
  parts.picked = h('section', {});
  parts.sensors = h('section', {});
  parts.boards = boardsSection();
  renderPicked();
  renderSensors();
  fill(document.getElementById('app'), parts.hwinfo, parts.picked, parts.sensors, parts.boards,
    connectionSection(), appSection());
}

/* The live refresh: the values, HWiNFO's state and the boards; the editor and the search stay as they are. */
async function refresh() {
  let next;
  try {
    next = await api('GET', '/api/state');
  } catch (e) {
    document.getElementById('pill').textContent = 'The app isn\'t answering';
    return;
  }
  const savedChanged = JSON.stringify(next.picked) !== JSON.stringify(state.picked);
  const sensorsChanged = next.hwinfo.status !== state.hwinfo.status
    || JSON.stringify(next.items.map((it) => it.key)) !== JSON.stringify(state.items.map((it) => it.key));
  state = next;
  if (savedChanged && !dirty) {
    picked = clone(state.picked);
    renderPicked();
  } else if (sensorsChanged && !dirty) {
    renderPicked(); // the rows find their sensors, or lose them
  }
  if (sensorsChanged) renderSensorList(); // keeps the search and the collapsed groups
  renderPill();
  const hw = hwinfoSection();
  parts.hwinfo.replaceWith(hw);
  parts.hwinfo = hw;
  const boards = boardsSection();
  parts.boards.replaceWith(boards);
  parts.boards = boards;
  for (const it of state.items) {
    for (const el of shown.get(it.key) || []) {
      el.textContent = el.picked ? boardValue(picked.find((r) => r.key === it.key) || { key: it.key })
        : `${number(it.value)} ${it.unit}`.trim();
    }
  }
}

async function start() {
  try {
    state = await api('GET', '/api/state');
  } catch (e) {
    document.getElementById('app').replaceChildren(h('p', { class: 'error', text: `The app isn't answering: ${e.message}` }));
    return;
  }
  picked = clone(state.picked);
  render();
  setInterval(refresh, 1000);
}

start();
