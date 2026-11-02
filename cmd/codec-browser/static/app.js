const $ = id => document.getElementById(id);
let names = [], codecInfo = {}, selected = '', generation = 0, pages = new Map(), chosen = '', shiftedByLink = false;
const search = $('search'), prefix = $('prefix'), font = $('font'), width = $('width'), viewport = $('table-wrap');
const rowHeight = 35, maxScrollHeight = 8000000;
let logicalRow = 0, expectedScroll = null, editingPrefix = false;
let minimapController = null, minimapCells = new Map(), minimapReady = false;
let minimapSizing = false;
const totalRows = () => 16 ** (Number(width.value) * 2 - 1);
const scrollLimit = () => Math.max(0, Math.min(totalRows() * rowHeight, maxScrollHeight) - viewport.clientHeight);
const lastRow = () => Math.max(0, totalRows() - Math.max(1, Math.floor(viewport.clientHeight / rowHeight)));
function setLogicalRow(row) {
 logicalRow = Math.max(0, Math.min(lastRow(), row));
 const target = lastRow() ? logicalRow / lastRow() * scrollLimit() : 0;
 viewport.scrollTop = target; expectedScroll = viewport.scrollTop; renderGrid();
}
function saveHistory(mode = 'push') {
 const url = new URL(location.href);
 url.searchParams.set('codec', selected);
 url.searchParams.set('width', width.value);
 if (prefix.value) url.searchParams.set('prefix', prefix.value); else url.searchParams.delete('prefix');
 if (logicalRow) url.searchParams.set('row', String(logicalRow)); else url.searchParams.delete('row');
 if (chosen) url.searchParams.set('cell', chosen); else url.searchParams.delete('cell');
 if (font.value && font.value !== 'sans-serif') url.searchParams.set('font', font.value); else url.searchParams.delete('font');
 if (shiftedByLink) url.searchParams.set('shift', '1'); else url.searchParams.delete('shift');
 if (mode === 'replace') history.replaceState(null, '', url); else if (url.href !== location.href) history.pushState(null, '', url);
}
const hex = (n, digits) => n.toString(16).toUpperCase().padStart(digits, '0');
function renderList() {
 const filtered = names.filter(n => n.toLowerCase().includes(search.value.toLowerCase()));
 $('count').textContent = `${filtered.length} / ${names.length}`;
 $('codecs').replaceChildren(...filtered.map(n => {
  const button = document.createElement('button'); button.textContent = n;
  button.className = n === selected ? 'codec active' : 'codec';
  button.setAttribute('role', 'option'); button.setAttribute('aria-selected', n === selected);
  button.onclick = () => { selected = n; prefix.value = ''; shiftedByLink = false; updateWidth(); renderList(); load(); }; return button;
 }));
}
function showDetail(m, save = true) {
 chosen = m.bytes;
 const box = $('detail'); box.replaceChildren();
 const glyph = document.createElement('span'); glyph.className = 'detail-glyph'; glyph.textContent = m.text || '—';
 const info = document.createElement('div'), title = document.createElement('b'), description = document.createElement('p');
 title.textContent = m.bytes.match(/../g).join(' ') + '  /  ' + m.status.toUpperCase();
 description.textContent = m.codepoints ? m.codepoints.map((code, i) => `${code} ${m.names[i]}`).join('  ·  ') : 'No character produced for this complete sequence.';
 info.append(title, description); box.append(glyph, info); if (save) saveHistory(); renderGrid();
}
function loadPage(page) {
 if (pages.has(page)) return;
 const current = generation;
 const length = Number(width.value);
 const bytePrefix = length > 1 ? hex(page, (length - 1) * 2) : '';
 const query = new URLSearchParams({codec: selected, prefix: prefix.value + bytePrefix});
 if (length > 1) query.set('atomic', String(length));
 if (pages.size >= 256) pages.delete(pages.keys().next().value);
 pages.set(page, null);
 fetch('/api/mappings?' + query).then(async response => {
  if (!response.ok) throw new Error(await response.text()); return response.json();
 }).then(data => { if (generation !== current) return; pages.set(page, data); const item = chosen && data.find(m => m.bytes === chosen); if (item) showDetail(item, false); else renderGrid(); })
 .catch(err => { if (generation === current) $('error').textContent = err.message; });
}
function drawMinimap() {
 const canvas = $('minimap-canvas'), ctx = canvas.getContext('2d');
 ctx.clearRect(0, 0, canvas.width, canvas.height);
 ctx.fillStyle = '#789c80';
 for (const index of minimapCells.keys()) {
  const x = (index % 16) * 8, y = Math.floor(index / 16) * 8;
  ctx.fillRect(x, y, 7, 7);
 }
}
function loadMinimap() {
 if (minimapController) minimapController.abort();
 minimapCells = new Map(); minimapReady = false; drawMinimap();
 if (Number(width.value) < 2) return;
 const current = generation; minimapController = new AbortController();
 const query = new URLSearchParams({codec: selected, prefix: prefix.value, width: width.value});
 $('minimap').setAttribute('aria-label', 'Table position; mapping overview loading');
 fetch('/api/minimap?' + query, {signal: minimapController.signal}).then(async response => {
  if (!response.ok) throw new Error(await response.text()); return response.json();
 }).then(cells => {
  if (generation !== current) return;
  minimapCells = new Map(cells.map(cell => [cell.index, cell.value])); minimapReady = true; drawMinimap();
  $('minimap').setAttribute('aria-label', 'Table position and mapping coverage');
 }).catch(err => { if (err.name !== 'AbortError' && generation === current) $('minimap').setAttribute('aria-label', 'Table position; mapping overview unavailable'); });
}
function sizeMinimap() {
 if (minimapSizing || $('minimap').hidden) return;
 const cell = $('grid').querySelector('.cell');
 if (!cell || !cell.offsetHeight) return;
 const width = Math.max(16, Math.round($('minimap').clientHeight * 16 / 256 * cell.offsetWidth / cell.offsetHeight));
 if (parseFloat(document.documentElement.style.getPropertyValue('--minimap-width')) === width) return;
 minimapSizing = true;
 document.documentElement.style.setProperty('--minimap-width', `${width}px`);
 requestAnimationFrame(() => { minimapSizing = false; });
}
new ResizeObserver(() => sizeMinimap()).observe(viewport);
function renderGrid() {
 if (!selected) return;
 const grid = $('grid'), length = Number(width.value), two = length > 1, total = totalRows();
 const first = Math.max(0, logicalRow - 3);
 const last = Math.min(total, logicalRow + Math.ceil(viewport.clientHeight / rowHeight) + 3);
 grid.style.height = `${Math.min(total * rowHeight, maxScrollHeight)}px`;
 const fragment = document.createDocumentFragment();
 for (let row = first; row < last; row++) {
  const page = two ? Math.floor(row / 16) : 0; loadPage(page);
  const entries = pages.get(page), line = document.createElement('div'); line.className = 'virtual-row';
  line.style.top = `${viewport.scrollTop + (row - logicalRow) * rowHeight}px`;
  const label = document.createElement('span'); label.className = 'row-label'; label.textContent = hex(row * 16, length * 2); line.append(label);
  for (let col = 0; col < 16; col++) {
   const m = entries && entries[(row % 16) * 16 + col], button = document.createElement('button');
   button.className = 'cell ' + (m && m.status === 'mapped' ? 'mapped' : 'empty');
   if (m) {
    if (m.bytes === chosen) button.classList.add('chosen');
    button.textContent = m.shiftTo ? (m.shiftTo === 'initial' ? 'SI ↗' : 'SO ↗') : m.text && !/[\p{Cc}\p{Cf}\p{Z}]/u.test(m.text) ? m.text : (m.status === 'mapped' ? '·' : '');
    if (!two && m.expandable && m.status !== 'mapped') {
     button.textContent = '+' + hex(col + (row % 16) * 16, 2);
     button.classList.add('prefix-code');
    }
    if (m.shiftTo) button.classList.add('shift');
    button.title = `${m.bytes}: ${m.codepoints ? m.codepoints.join(' ') : m.status}`;
    button.setAttribute('aria-label', button.title + (m.shiftTo ? ', switch state' : ''));
    button.onclick = () => { if (m.shiftTo) { prefix.value = m.shiftTo === 'initial' ? '' : m.shiftTo; shiftedByLink = m.shiftTo !== 'initial'; width.value = m.shiftTo === 'initial' ? '1' : '2'; load(); } else if (!two && m.expandable && m.status !== 'mapped') { prefix.value = m.bytes; shiftedByLink = false; load(); } else showDetail(m); };
   } else { button.disabled = true; button.setAttribute('aria-label', 'Loading'); }
   if (!two && m && m.expandable && m.status === 'mapped' && !m.shiftTo) {
    const cell = document.createElement('div'); cell.className = 'cell-container';
    const next = document.createElement('button'); next.className = 'prefix-next'; next.textContent = '+';
    const byte = hex(col + (row % 16) * 16, 2);
    next.title = `Browse mappings under ${m.bytes} (${byte} appended)`;
    next.setAttribute('aria-label', `Browse mappings under ${m.bytes}`);
    next.onclick = () => { prefix.value = m.bytes; shiftedByLink = false; load(); };
    cell.append(button, next); line.append(cell);
   } else line.append(button);
  }
  fragment.append(line);
 }
 grid.replaceChildren(fragment);
 $('mapped-count').textContent = `${'00'.repeat(length)}–${'FF'.repeat(length)} · scroll or jump to browse`;
 const minimap = $('minimap'), marker = $('minimap-marker');
 minimap.hidden = !two;
 if (two) {
  sizeMinimap();
  const position = lastRow() ? logicalRow / lastRow() : 0;
  const markerHeight = Math.max(12, minimap.clientHeight * Math.min(1, viewport.clientHeight / (total * rowHeight)));
  marker.style.height = `${markerHeight}px`;
  marker.style.top = `${position * Math.max(0, minimap.clientHeight - markerHeight)}px`;
  minimap.setAttribute('aria-valuenow', String(Math.round(position * 100)));
  minimap.setAttribute('aria-valuetext', hex(logicalRow * 16, length * 2));
  minimap.title = `Table position: ${hex(logicalRow * 16, length * 2)}`;
 }
}
function updateWidth() {
 const info = codecInfo[selected.toLowerCase()];
 const allowed = info ? info.ByteLengths : [];
 // One-byte view remains available to follow a longer character's prefix.
 for (let i = 2; i <= 4; i++) width.options[i - 1].disabled = !allowed.includes(i);
 if (width.selectedOptions[0].disabled) width.value = '1';
}
function jumpToCode() {
 const value = $('jump').value.replace(/[ :]/g, '');
 const digits = Number(width.value) * 2;
 if (!new RegExp(`^[0-9a-f]{1,${digits}}$`, 'i').test(value)) {
  $('error').textContent = `Enter up to ${digits} hexadecimal digits.`; return;
 }
 $('error').textContent = ''; setLogicalRow(Math.floor(parseInt(value, 16) / 16)); chosen = ''; saveHistory();
}
function load(mode = 'push') {
 if (!selected) return;
 generation++; pages = new Map(); chosen = ''; logicalRow = 0; viewport.scrollTop = 0; expectedScroll = viewport.scrollTop;
 $('title').textContent = selected; $('subtitle').textContent = 'Decoded from the initial state';
 $('error').textContent = ''; $('back').disabled = !/^(?:[0-9a-f]{2})+$/i.test(prefix.value.replace(/[ :]/g, '')); $('shift-state').hidden = !shiftedByLink || prefix.value.replace(/[ :]/g, '').toUpperCase() !== '0E'; $('detail').innerHTML = '<span>↖</span><div><b>Choose a cell to inspect</b><p>View its byte sequence and Unicode code points.</p></div>';
 renderGrid(); loadMinimap();
 if (mode !== 'none') saveHistory(mode === 'replace' ? 'replace' : 'push');
}
const notoSamples = {
 'Noto Sans': 'A', 'Noto Serif': 'A', 'Noto Sans Mono': 'A', 'Noto Sans Symbols 2': '★',
 'Noto Sans JP': 'あ', 'Noto Serif JP': 'あ', 'Noto Sans KR': '가', 'Noto Serif KR': '가',
 'Noto Sans SC': '汉', 'Noto Serif SC': '汉', 'Noto Sans TC': '漢', 'Noto Serif TC': '漢',
 'Noto Sans HK': '漢', 'Noto Sans Arabic': 'ع', 'Noto Sans Hebrew': 'א',
 'Noto Sans Devanagari': 'क', 'Noto Sans Bengali': 'অ', 'Noto Sans Thai': 'ก',
 'Noto Sans Armenian': 'Ա', 'Noto Sans Georgian': 'ა', 'Noto Sans Ethiopic': 'ሀ'
};
const fontStylesheets = new Map();
let fontRequest = 0;
function loadFontStylesheet(name) {
 if (!fontStylesheets.has(name)) {
  const url = 'https://fonts.googleapis.com/css2?' + new URLSearchParams({family: name, display: 'swap'});
  const promise = new Promise((resolve, reject) => {
   const link = document.createElement('link'); link.rel = 'stylesheet'; link.href = url;
   link.onload = resolve; link.onerror = () => { link.remove(); reject(new Error('Font stylesheet unavailable')); };
   document.head.append(link);
  });
  fontStylesheets.set(name, promise);
  promise.catch(() => fontStylesheets.delete(name));
 }
 return fontStylesheets.get(name);
}
async function updateFont() {
 const request = ++fontRequest, value = font.value || 'sans-serif';
 const isNoto = Object.hasOwn(notoSamples, value);
 const generic = /^(sans-serif|serif|monospace)$/.test(value);
 const family = generic ? value : `"${value}"`;
 document.documentElement.style.setProperty('--glyph-font', `${family}, sans-serif`);
 if (!isNoto) { $('font-status').textContent = generic ? 'Generic family; the browser chooses the font.' : 'Uses the installed font if available; otherwise the browser uses fallback.'; return; }
 $('font-status').textContent = 'Loading from Google Fonts…';
 try {
  await loadFontStylesheet(value);
  const faces = await document.fonts.load(`16px ${family}`, notoSamples[value]);
  if (!faces.length) throw new Error('Font unavailable');
  if (request === fontRequest) $('font-status').textContent = 'Font loaded. Some characters may use fallback.';
 } catch (_) {
  if (request === fontRequest) $('font-status').textContent = 'Could not load font; browser fallback will be used.';
 }
}
function moveMinimap(event) {
 const map = $('minimap');
 const bounds = map.getBoundingClientRect();
 const x = Math.max(0, Math.min(15, Math.floor((event.clientX - bounds.left) / bounds.width * 16)));
 const y = Math.max(0, Math.min(255, Math.floor((event.clientY - bounds.top) / bounds.height * 256)));
 const value = minimapCells.get(y * 16 + x);
 setLogicalRow(value === undefined ? Math.floor(y / 256 * totalRows()) : Math.floor(value / 16));
 saveHistory('replace');
}
$('minimap').addEventListener('pointerdown', event => { event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId); moveMinimap(event); });
$('minimap').addEventListener('pointermove', event => {
 if (event.currentTarget.hasPointerCapture(event.pointerId)) { moveMinimap(event); return; }
 const bounds = event.currentTarget.getBoundingClientRect();
 const x = Math.max(0, Math.min(15, Math.floor((event.clientX - bounds.left) / bounds.width * 16)));
 const y = Math.max(0, Math.min(255, Math.floor((event.clientY - bounds.top) / bounds.height * 256)));
 const cell = minimapCells.get(y * 16 + x);
 event.currentTarget.title = !minimapReady ? 'Mapping overview loading' : cell !== undefined ? 'At least one mapping in this range' : 'No mapping in this range';
});
$('minimap').addEventListener('keydown', event => {
 const page = Math.max(1, Math.floor(viewport.clientHeight / rowHeight));
 const amount = {ArrowUp: -1, ArrowDown: 1, PageUp: -page, PageDown: page}[event.key];
 if (amount !== undefined) { event.preventDefault(); setLogicalRow(logicalRow + amount); saveHistory('replace'); }
 else if (event.key === 'Home' || event.key === 'End') { event.preventDefault(); setLogicalRow(event.key === 'Home' ? 0 : lastRow()); saveHistory('replace'); }
});
$('back').addEventListener('click', () => { const bytes = prefix.value.replace(/[ :]/g, ''); if (!/^(?:[0-9a-f]{2})+$/i.test(bytes)) return; prefix.value = bytes.slice(0, -2); shiftedByLink = false; load(); });
$('shift-in').addEventListener('click', () => { prefix.value = ''; shiftedByLink = false; width.value = '1'; load(); });
search.addEventListener('input', renderList); prefix.addEventListener('input', () => { shiftedByLink = false; load(editingPrefix ? 'replace' : 'push'); editingPrefix = true; }); prefix.addEventListener('blur', () => { editingPrefix = false; }); width.addEventListener('change', () => load()); $('jump-button').addEventListener('click', jumpToCode); $('jump').addEventListener('keydown', e => { if (e.key === 'Enter') jumpToCode(); }); viewport.addEventListener('scroll', () => { if (expectedScroll !== null && Math.abs(viewport.scrollTop - expectedScroll) < 1) { expectedScroll = null; return; } expectedScroll = null; logicalRow = scrollLimit() ? Math.round(viewport.scrollTop / scrollLimit() * lastRow()) : 0; renderGrid(); saveHistory('replace'); }); viewport.addEventListener('wheel', e => { if (totalRows() * rowHeight <= maxScrollHeight) return; e.preventDefault(); setLogicalRow(logicalRow + Math.sign(e.deltaY) * Math.max(1, Math.round(Math.abs(e.deltaY) / rowHeight))); saveHistory('replace'); }, {passive: false}); font.addEventListener('input', () => { updateFont(); if (selected) saveHistory('replace'); }); updateFont();
function restoreURL() {
 editingPrefix = false;
 const params = new URLSearchParams(location.search);
 const requested = params.get('codec');
 selected = names.find(n => n.toLowerCase() === (requested || '').toLowerCase()) || names.find(n => n.toLowerCase() === 'ibm-943_p15a-2003') || names[0];
 prefix.value = params.get('prefix') || '';
 font.value = params.get('font') || 'sans-serif'; if (!font.value) font.value = 'sans-serif'; updateFont();
 shiftedByLink = params.get('shift') === '1';
 width.value = '1'; updateWidth();
 const requestedWidth = params.get('width');
 if (/^[1-4]$/.test(requestedWidth || '') && !width.options[Number(requestedWidth) - 1].disabled) width.value = requestedWidth;
 renderList(); load('none');
 const row = Number(params.get('row'));
 if (Number.isSafeInteger(row) && row > 0) setLogicalRow(row);
 chosen = params.get('cell') || '';
 // The requested page may already be cached when the history event fires.
 for (const data of pages.values()) { const item = data && data.find(m => m.bytes === chosen); if (item) { showDetail(item, false); break; } }
}
window.addEventListener('popstate', () => { if (names.length) restoreURL(); });
Promise.all(['/api/codecs', '/api/codec-info'].map(url => fetch(url).then(r => { if (!r.ok) throw new Error('Could not load registry'); return r.json(); }))).then(([data, info]) => { names = data; codecInfo = info; restoreURL(); saveHistory('replace'); }).catch(e => $('error').textContent = e.message);
