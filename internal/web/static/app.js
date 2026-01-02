(() => {
  const state = {
    awaiting: null,
    awaitingAt: 0,
    restoreTimer: null,
  };

  const outlinePendingByEl = new WeakMap();
  const outlineMoveBufferByEl = new WeakMap();

  const isTypingTarget = (el) => {
    if (!el) return false;
    const tag = (el.tagName || '').toLowerCase();
    if (tag === 'textarea' || tag === 'input' || tag === 'select') return true;
    if (el.isContentEditable) return true;
    return false;
  };

  const activeIsTyping = () => isTypingTarget(document.activeElement);

  const escapeHTML = (s) => {
    s = String(s ?? '');
    return s
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#39;');
  };

  const escapeAttr = (s) => escapeHTML(s);

  const uniqueSortedStrings = (xs) => {
    const seen = new Set();
    const out = [];
    for (const x0 of (Array.isArray(xs) ? xs : [])) {
      const x = String(x0 || '').trim();
      if (!x) continue;
      const key = x.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(x);
    }
    out.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()) || a.localeCompare(b));
    return out;
  };

  const eventTouchesOutlineComponent = (ev) => {
    if (!ev || typeof ev.composedPath !== 'function') return false;
    const path = ev.composedPath();
    for (const node of path) {
      if (!node) continue;
      if (node.id === 'outline') return true;
      const tag = (node.tagName || '').toLowerCase();
      if (tag === 'clarity-outline') return true;
    }
    return false;
  };

  const outlineFromEvent = (ev) => {
    if (!ev || typeof ev.composedPath !== 'function') return null;
    const path = ev.composedPath();
    for (const node of path) {
      if (!node) continue;
      const tag = (node.tagName || '').toLowerCase();
      if (node.id === 'outline' || tag === 'clarity-outline') return node;
    }
    return null;
  };

  const outlineFocusKey = (outlineEl) => {
    if (!outlineEl) return 'clarity:outlineFocus';
    const outlineId = (outlineEl.getAttribute('data-outline-id') || '').trim();
    if (!outlineId) return 'clarity:outlineFocus';
    return 'clarity:outlineFocus:' + outlineId;
  };

  const rememberOutlineFocus = (outlineEl, itemId) => {
    if (!outlineEl) return;
    itemId = (itemId || '').trim();
    if (!itemId) return;
    try {
      sessionStorage.setItem(outlineFocusKey(outlineEl), itemId);
    } catch (_) {}
  };

  const restoreOutlineFocus = (outlineEl) => {
    if (!outlineEl) return;
    let itemId = '';
    try {
      itemId = sessionStorage.getItem(outlineFocusKey(outlineEl)) || '';
    } catch (_) {}
    itemId = (itemId || '').trim();
    if (!itemId) return;
    const li = outlineFindLi(outlineEl, itemId);
    if (li && typeof li.focus === 'function') {
      li.focus();
    }
  };

  const nativeOutlineRoot = () => document.getElementById('outline-native');
  const itemPageRoot = () => document.getElementById('item-native');

  const nativeRowFromEvent = (ev) => {
    const t = ev && ev.target;
    if (!t || typeof t.closest !== 'function') return null;
    return t.closest('[data-outline-row]');
  };

  const nativeOutlineRootOrFromRow = (row) => {
    const root = nativeOutlineRoot();
    if (root) return root;
    if (!row) return null;
    const li = nativeLiFromRow(row);
    if (!li) return null;
    return li.closest('#outline-native');
  };

  const nativeRows = () => {
    const root = nativeOutlineRoot();
    if (!root) return [];
    return Array.from(root.querySelectorAll('[data-outline-row]')).filter((el) => {
      if (!el) return false;
      // Filter out rows hidden by collapsed parents.
      try {
        return el.getClientRects().length > 0;
      } catch (_) {
        return true;
      }
    });
  };

  const focusNativeRowById = (id) => {
    id = (id || '').trim();
    if (!id) return;
    const root = nativeOutlineRoot();
    if (!root) return;
    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    row?.focus?.();
  };

  const nativeRowSibling = (row, delta) => {
    const rows = nativeRows();
    if (!rows.length) return null;
    const idx = rows.indexOf(row);
    if (idx < 0) return rows[0];
    const next = Math.max(0, Math.min(rows.length - 1, idx + delta));
    return rows[next] || null;
  };

  const nativeLiFromRow = (row) => {
    if (!row || typeof row.closest !== 'function') return null;
    return row.closest('li[data-node-id]');
  };

  const liSibling = (li, dir) => {
    if (!li) return null;
    let cur = dir === 'prev' ? li.previousElementSibling : li.nextElementSibling;
    while (cur) {
      if ((cur.tagName || '').toUpperCase() === 'LI' && cur.dataset && cur.dataset.nodeId) return cur;
      cur = dir === 'prev' ? cur.previousElementSibling : cur.nextElementSibling;
    }
    return null;
  };

  const parentIdForLi = (li) => {
    if (!li || !li.parentElement) return null;
    const parentLi = li.parentElement.closest('li[data-node-id]');
    if (!parentLi || !parentLi.dataset) return null;
    return (parentLi.dataset.nodeId || '').trim() || null;
  };

  const ensureChildList = (li) => {
    if (!li) return null;
    let ul = li.querySelector(':scope > ul.outline-children');
    if (ul) return ul;
    ul = document.createElement('ul');
    ul.className = 'outline-children';
    li.appendChild(ul);
    return ul;
  };

  const startInlineEdit = (row) => {
    if (!row) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    const titleSpan = row.querySelector('.outline-title');
    if (!titleSpan) return;
    if (titleSpan.querySelector('input')) return;

    const id = (row.dataset.id || '').trim();
    const before = (titleSpan.textContent || '').trim();

    const input = document.createElement('input');
    input.type = 'text';
    input.value = before;
    input.setAttribute('aria-label', 'Edit title');
    titleSpan.textContent = '';
    titleSpan.appendChild(input);
    input.focus();
    input.select();

    const cancel = () => {
      titleSpan.textContent = before;
      row.focus();
    };

    const commit = () => {
      const next = (input.value || '').trim();
      if (!next) {
        setOutlineStatus('Error: title cannot be empty');
        setTimeout(() => setOutlineStatus(''), 1200);
        input.focus();
        return;
      }
      // Optimistic.
      titleSpan.textContent = next;
      row.focus();
      rememberOutlineFocus(nativeOutlineRoot(), id);
      outlineApply(nativeOutlineRoot(), 'outline:edit:save', { id, newText: next }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
    };

    input.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape') {
        ev.preventDefault();
        cancel();
      } else if (ev.key === 'Enter') {
        ev.preventDefault();
        commit();
      }
    });
    input.addEventListener('blur', () => {
      // Keep it simple: commit on blur if changed, otherwise cancel.
      const next = (input.value || '').trim();
      if (next && next !== before) {
        commit();
      } else {
        cancel();
      }
    }, { once: true });
  };

  const parseStatusOptions = (root) => {
    const raw = (root && root.dataset ? root.dataset.statusOptions : '') || '';
    if (!raw.trim()) return [];
    try {
      const xs = JSON.parse(raw);
      return Array.isArray(xs) ? xs : [];
    } catch (_) {
      return [];
    }
  };

  const parseTagOptions = (root) => {
    const raw = (root && root.dataset ? root.dataset.tagOptions : '') || '';
    if (!raw.trim()) return [];
    try {
      const xs = JSON.parse(raw);
      return Array.isArray(xs) ? xs : [];
    } catch (_) {
      return [];
    }
  };

  const parseActorOptions = (root) => {
    const raw = (root && root.dataset ? root.dataset.actorOptions : '') || '';
    if (!raw.trim()) return [];
    try {
      const xs = JSON.parse(raw);
      return Array.isArray(xs) ? xs : [];
    } catch (_) {
      return [];
    }
  };

  const parseOutlineOptions = (root) => {
    const raw = (root && root.dataset ? root.dataset.outlineOptions : '') || '';
    if (!raw.trim()) return [];
    try {
      const xs = JSON.parse(raw);
      return Array.isArray(xs) ? xs : [];
    } catch (_) {
      return [];
    }
  };

  const tagsPicker = {
    open: false,
    rowId: '',
    rootEl: null,
    options: [],
    selected: new Set(),
    idx: 0,
    originalSelected: new Set(),
    restoreFocusId: '',
    saveTimer: 0,
  };

  const ensureTagsModal = () => {
    let el = document.getElementById('native-tags-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-tags-modal';
    el.style.position = 'fixed';
    el.style.left = '0';
    el.style.top = '0';
    el.style.right = '0';
    el.style.bottom = '0';
    el.style.background = 'rgba(0,0,0,.25)';
    el.style.display = 'none';
    el.style.alignItems = 'center';
    el.style.justifyContent = 'center';
    el.style.zIndex = '9998';
    el.innerHTML = `
      <div style="max-width:560px;width:92vw;background:Canvas;color:CanvasText;border:1px solid rgba(127,127,127,.35);border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.25);padding:12px 14px;">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
          <strong>Tags</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-tags-list" tabindex="0" style="margin-top:10px;max-height:40vh;overflow:auto;outline:none;"></div>
        <div style="margin-top:10px;display:flex;gap:10px;align-items:center;">
          <input id="native-tags-new" placeholder="Add tag (without #)" style="flex:1;">
          <button type="button" id="native-tags-add">Add</button>
        </div>
        <div class="dim" style="margin-top:10px;font-size:12px;">Up/Down or Ctrl+P/N to move · Space to toggle (saves) · a to add · Enter to close</div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:12px;align-items:center;">
          <button type="button" id="native-tags-cancel">Cancel</button>
          <button type="button" id="native-tags-done">Done</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeTagsPicker('cancel');
    });
    const addBtn = el.querySelector('#native-tags-add');
    addBtn && addBtn.addEventListener('click', () => addNewTagFromInput());
    const cancelBtn = el.querySelector('#native-tags-cancel');
    cancelBtn && cancelBtn.addEventListener('click', () => closeTagsPicker('cancel'));
    const doneBtn = el.querySelector('#native-tags-done');
    doneBtn && doneBtn.addEventListener('click', () => closeTagsPicker('done'));
    return el;
  };

  const normalizeTag = (t) => String(t || '').trim().replace(/^#+/, '').trim();

  const sortedTagList = (set) => {
    const xs = Array.from(set || []);
    xs.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()) || a.localeCompare(b));
    return xs;
  };

  const renderTagsPicker = () => {
    const modal = ensureTagsModal();
    const list = modal.querySelector('#native-tags-list');
    if (!list) return;
    const opts = tagsPicker.options || [];
    list.innerHTML = '';
    const ul = document.createElement('ul');
    ul.style.listStyle = 'none';
    ul.style.padding = '0';
    ul.style.margin = '0';
    opts.forEach((o, i) => {
      const tag = normalizeTag(o);
      if (!tag) return;
      const li = document.createElement('li');
      li.style.display = 'flex';
      li.style.gap = '10px';
      li.style.alignItems = 'center';
      li.style.padding = '6px 8px';
      li.style.borderRadius = '8px';
      li.style.cursor = 'pointer';
      if (i === tagsPicker.idx) {
        li.style.background = 'color-mix(in oklab, Canvas, CanvasText 10%)';
      }
      const cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.checked = tagsPicker.selected.has(tag.toLowerCase());
      cb.tabIndex = -1;
      const label = document.createElement('span');
      label.textContent = '#' + tag;
      li.appendChild(cb);
      li.appendChild(label);
      li.addEventListener('click', () => {
        tagsPicker.idx = i;
        toggleSelectedTag();
      });
      ul.appendChild(li);
    });
    list.appendChild(ul);
  };

  const restoreNativeFocusAfterModal = (id) => {
    const focusId = (id || '').trim();
    if (!focusId) return;
    setTimeout(() => {
      if (nativeOutlineRoot()) focusNativeRowById(focusId);
    }, 0);
  };

  const tagsPickerSelectedOut = () => {
    const tags = sortedTagList(new Set(Array.from(tagsPicker.selected).map((x) => x.toLowerCase())));
    const casing = new Map();
    for (const t of tagsPicker.options || []) {
      const nt = normalizeTag(t);
      if (!nt) continue;
      casing.set(nt.toLowerCase(), nt);
    }
    return tags.map((t) => casing.get(t) || t);
  };

  const scheduleTagsPickerSave = () => {
    if (!tagsPicker.open) return;
    const root = tagsPicker.rootEl || nativeOutlineRoot();
    const id = (tagsPicker.rowId || '').trim();
    if (!root || !id) return;
    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    const out = tagsPickerSelectedOut();
    if (row) nativeRowUpdateTags(row, out);
    if (tagsPicker.saveTimer) clearTimeout(tagsPicker.saveTimer);
    tagsPicker.saveTimer = setTimeout(() => {
      outlineApply(root, 'outline:set_tags', { id, tags: out }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
    }, 250);
  };

  const openTagsPicker = async (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    let meta;
    try {
      meta = await fetchItemMeta(id);
    } catch (err) {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'load failed'));
      return;
    }

    const all = parseTagOptions(root).map(normalizeTag).filter(Boolean);
    const selected = new Set();
    const cur = (meta && Array.isArray(meta.tags)) ? meta.tags : [];
    cur.map(normalizeTag).filter(Boolean).forEach((t) => selected.add(t.toLowerCase()));

    tagsPicker.open = true;
    tagsPicker.rowId = id;
    tagsPicker.rootEl = root;
    tagsPicker.selected = selected;
    tagsPicker.originalSelected = new Set(Array.from(selected));
    tagsPicker.restoreFocusId = id;
    tagsPicker.options = uniqueSortedStrings(all);
    tagsPicker.idx = 0;
    tagsPicker.saveTimer = 0;

    const modal = ensureTagsModal();
    modal.style.display = 'flex';
    renderTagsPicker();
    const list = modal.querySelector('#native-tags-list');
    list && list.focus();
  };

  const closeTagsPicker = (action) => {
    const modal = document.getElementById('native-tags-modal');
    const act = (action || '').toLowerCase();
    const cancel = act === 'cancel';
    const done = act === 'done';
    if (tagsPicker.saveTimer) clearTimeout(tagsPicker.saveTimer);
    tagsPicker.saveTimer = 0;
    if (cancel && tagsPicker.open) {
      const root = tagsPicker.rootEl || nativeOutlineRoot();
      const id = (tagsPicker.rowId || '').trim();
      if (root && id) {
        const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
        const out = sortedTagList(new Set(Array.from(tagsPicker.originalSelected || []).map((x) => String(x || '').toLowerCase())));
        const casing = new Map();
        for (const t of tagsPicker.options || []) {
          const nt = normalizeTag(t);
          if (!nt) continue;
          casing.set(nt.toLowerCase(), nt);
        }
        const restored = out.map((t) => casing.get(t) || t);
        if (row) nativeRowUpdateTags(row, restored);
        outlineApply(root, 'outline:set_tags', { id, tags: restored }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
        });
      }
    } else if (done && tagsPicker.open) {
      // Ensure the final state is flushed.
      scheduleTagsPickerSave();
    }
    const restoreId = tagsPicker.restoreFocusId;
    tagsPicker.open = false;
    tagsPicker.rowId = '';
    tagsPicker.rootEl = null;
    tagsPicker.options = [];
    tagsPicker.selected = new Set();
    tagsPicker.originalSelected = new Set();
    tagsPicker.idx = 0;
    tagsPicker.restoreFocusId = '';
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
  };

  const toggleSelectedTag = () => {
    if (!tagsPicker.open) return;
    const opt = tagsPicker.options[tagsPicker.idx];
    const tag = normalizeTag(opt);
    if (!tag) return;
    const key = tag.toLowerCase();
    if (tagsPicker.selected.has(key)) tagsPicker.selected.delete(key);
    else tagsPicker.selected.add(key);
    renderTagsPicker();
    scheduleTagsPickerSave();
  };

  const addNewTagFromInput = () => {
    if (!tagsPicker.open) return;
    const modal = document.getElementById('native-tags-modal');
    const input = modal ? modal.querySelector('#native-tags-new') : null;
    const raw = input ? input.value : '';
    const tag = normalizeTag(raw);
    if (!tag) return;
    const key = tag.toLowerCase();
    tagsPicker.selected.add(key);
    const exists = (tagsPicker.options || []).some((t) => normalizeTag(t).toLowerCase() === key);
    if (!exists) tagsPicker.options = uniqueSortedStrings([...tagsPicker.options, tag]);
    if (input) input.value = '';
    renderTagsPicker();
    scheduleTagsPickerSave();
  };

  const moveOutlinePicker = {
    open: false,
    rowId: '',
    outlineId: '',
    rootEl: null,
    options: [],
    idx: 0,
    restoreFocusId: '',
  };

  const ensureMoveOutlineModal = () => {
    let el = document.getElementById('native-move-outline-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-move-outline-modal';
    el.style.position = 'fixed';
    el.style.left = '0';
    el.style.top = '0';
    el.style.right = '0';
    el.style.bottom = '0';
    el.style.background = 'rgba(0,0,0,.25)';
    el.style.display = 'none';
    el.style.alignItems = 'center';
    el.style.justifyContent = 'center';
    el.style.zIndex = '9998';
    el.innerHTML = `
      <div style="max-width:680px;width:92vw;background:Canvas;color:CanvasText;border:1px solid rgba(127,127,127,.35);border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.25);padding:12px 14px;">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
          <strong>Move to outline</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-move-outline-list" style="margin-top:10px;max-height:46vh;overflow:auto;"></div>
        <div class="dim" style="margin-top:10px;font-size:12px;">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:12px;align-items:center;">
          <button type="button" id="native-move-outline-cancel">Cancel</button>
          <button type="button" id="native-move-outline-ok">Move</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeMoveOutlinePicker();
    });
    const cancelBtn = el.querySelector('#native-move-outline-cancel');
    cancelBtn && cancelBtn.addEventListener('click', () => closeMoveOutlinePicker());
    const okBtn = el.querySelector('#native-move-outline-ok');
    okBtn && okBtn.addEventListener('click', () => pickSelectedMoveOutline());
    return el;
  };

  const renderMoveOutlinePicker = () => {
    const modal = ensureMoveOutlineModal();
    const list = modal.querySelector('#native-move-outline-list');
    if (!list) return;
    const opts = moveOutlinePicker.options || [];
    list.innerHTML = '';
    const ul = document.createElement('ul');
    ul.style.listStyle = 'none';
    ul.style.padding = '0';
    ul.style.margin = '0';
    opts.forEach((o, i) => {
      const li = document.createElement('li');
      li.style.padding = '6px 8px';
      li.style.borderRadius = '8px';
      li.style.cursor = 'pointer';
      if (i === moveOutlinePicker.idx) {
        li.style.background = 'color-mix(in oklab, Canvas, CanvasText 10%)';
      }
      const lbl = (o && typeof o.label === 'string') ? o.label : (o && o.id ? o.id : '');
      li.textContent = lbl || '';
      li.addEventListener('click', () => {
        moveOutlinePicker.idx = i;
        pickSelectedMoveOutline();
      });
      ul.appendChild(li);
    });
    list.appendChild(ul);
  };

  const closeMoveOutlinePicker = () => {
    const restoreId = moveOutlinePicker.restoreFocusId;
    moveOutlinePicker.open = false;
    moveOutlinePicker.rowId = '';
    moveOutlinePicker.outlineId = '';
    moveOutlinePicker.rootEl = null;
    moveOutlinePicker.options = [];
    moveOutlinePicker.idx = 0;
    moveOutlinePicker.restoreFocusId = '';
    const modal = document.getElementById('native-move-outline-modal');
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
  };

  const fetchOutlineMeta = async (outlineId) => {
    const id = (outlineId || '').trim();
    if (!id) throw new Error('missing outline id');
    const res = await fetch('/outlines/' + encodeURIComponent(id) + '/meta', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });
    if (!res.ok) throw new Error(await res.text());
    return await res.json();
  };

  const removeRowFromNativeOutline = (root, row) => {
    if (!root || !row) return;
    const next = nativeRowSibling(row, +1) || nativeRowSibling(row, -1) || null;
    const li = nativeLiFromRow(row);
    li && li.remove();
    next && next.focus && next.focus();
  };

  const openMoveOutlineStatusPicker = async (root, itemId, toOutlineId, toLabel) => {
    if (!root) return;
    let meta;
    try {
      meta = await fetchOutlineMeta(toOutlineId);
    } catch (err) {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'load failed'));
      return;
    }
    const raw = (meta && Array.isArray(meta.statusOptions)) ? meta.statusOptions : [];
    const opts = raw.map((o) => ({
      id: (o && o.id) ? String(o.id) : '',
      label: (o && o.label) ? String(o.label) : '',
      isEndState: !!(o && o.isEndState),
      requiresNote: false,
    })).filter((o) => (o.id || '').trim() !== '');
    if (!opts.length) {
      setOutlineStatus('Error: no statuses in target outline');
      setTimeout(() => setOutlineStatus(''), 1500);
      return;
    }
    statusPicker.open = true;
    statusPicker.rowId = itemId;
    statusPicker.rootEl = root;
    statusPicker.options = opts;
    statusPicker.idx = 0;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Move: pick status';
    statusPicker.submit = ({ statusID }) => {
      const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(itemId) + '"]');
      return outlineApply(root, 'outline:move_outline', { id: itemId, toOutlineId, status: statusID, applyStatusToInvalidSubtree: true }).then(() => {
        if (row) removeRowFromNativeOutline(root, row);
        setOutlineStatus('Moved to ' + (toLabel || toOutlineId));
        setTimeout(() => setOutlineStatus(''), 1800);
      });
    };
    const modal = ensureStatusModal();
    modal.style.display = 'flex';
    renderStatusPicker();
  };

  const pickSelectedMoveOutline = () => {
    if (!moveOutlinePicker.open) return;
    const sel = moveOutlinePicker.options[moveOutlinePicker.idx];
    if (!sel) return;
    const toOutlineId = (sel.id || '').trim();
    if (!toOutlineId) return;
    const id = (moveOutlinePicker.rowId || '').trim();
    const root = moveOutlinePicker.rootEl || nativeOutlineRoot();
    const fromOutlineId = (moveOutlinePicker.outlineId || '').trim();
    const label = (sel.label || '').trim();
    closeMoveOutlinePicker();
    if (!root || !id) return;
    if (toOutlineId === fromOutlineId) return;
    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    outlineApply(root, 'outline:move_outline', { id, toOutlineId }).then(() => {
      if (row) removeRowFromNativeOutline(root, row);
      setOutlineStatus('Moved to ' + (label || toOutlineId));
      setTimeout(() => setOutlineStatus(''), 1800);
    }).catch((err) => {
      const msg = (err && err.message) ? String(err.message) : 'move failed';
      if (msg.includes('pick a compatible status')) {
        openMoveOutlineStatusPicker(root, id, toOutlineId, label);
        return;
      }
      setOutlineStatus('Error: ' + msg);
      setTimeout(() => setOutlineStatus(''), 2400);
    });
  };

  const openMoveOutlinePicker = (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    const opts = parseOutlineOptions(root);
    if (!opts.length) {
      setOutlineStatus('Error: no outlines');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    moveOutlinePicker.open = true;
    moveOutlinePicker.rowId = id;
    moveOutlinePicker.outlineId = (root.dataset.outlineId || '').trim();
    moveOutlinePicker.rootEl = root;
    moveOutlinePicker.options = opts;
    moveOutlinePicker.restoreFocusId = id;
    let idx = opts.findIndex((o) => String(o && o.id || '').trim() === moveOutlinePicker.outlineId);
    if (idx < 0) idx = 0;
    moveOutlinePicker.idx = idx;
    const modal = ensureMoveOutlineModal();
    modal.style.display = 'flex';
    renderMoveOutlinePicker();
  };

  const assigneePicker = {
    open: false,
    rowId: '',
    rootEl: null,
    options: [],
    idx: 0,
  };

  const ensureAssigneeModal = () => {
    let el = document.getElementById('native-assignee-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-assignee-modal';
    el.style.position = 'fixed';
    el.style.left = '0';
    el.style.top = '0';
    el.style.right = '0';
    el.style.bottom = '0';
    el.style.background = 'rgba(0,0,0,.25)';
    el.style.display = 'none';
    el.style.alignItems = 'center';
    el.style.justifyContent = 'center';
    el.style.zIndex = '9998';
    el.innerHTML = `
      <div style="max-width:520px;width:92vw;background:Canvas;color:CanvasText;border:1px solid rgba(127,127,127,.35);border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.25);padding:12px 14px;">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
          <strong>Assign</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-assignee-list" style="margin-top:10px;max-height:46vh;overflow:auto;"></div>
        <div class="dim" style="margin-top:10px;font-size:12px;">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:12px;align-items:center;">
          <button type="button" id="native-assignee-cancel">Cancel</button>
          <button type="button" id="native-assignee-ok">Select</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeAssigneePicker();
    });
    const cancelBtn = el.querySelector('#native-assignee-cancel');
    cancelBtn && cancelBtn.addEventListener('click', () => closeAssigneePicker());
    const okBtn = el.querySelector('#native-assignee-ok');
    okBtn && okBtn.addEventListener('click', () => pickSelectedAssignee());
    return el;
  };

  const renderAssigneePicker = () => {
    const modal = ensureAssigneeModal();
    const list = modal.querySelector('#native-assignee-list');
    if (!list) return;

    const opts = assigneePicker.options || [];
    list.innerHTML = '';
    const ul = document.createElement('ul');
    ul.style.listStyle = 'none';
    ul.style.padding = '0';
    ul.style.margin = '0';
    opts.forEach((o, i) => {
      const li = document.createElement('li');
      li.style.padding = '6px 8px';
      li.style.borderRadius = '8px';
      li.style.cursor = 'pointer';
      if (i === assigneePicker.idx) {
        li.style.background = 'color-mix(in oklab, Canvas, CanvasText 10%)';
      }
      li.textContent = (o.label || o.id || '').trim() || '(none)';
      li.addEventListener('click', () => {
        assigneePicker.idx = i;
        renderAssigneePicker();
        pickSelectedAssignee();
      });
      ul.appendChild(li);
    });
    list.appendChild(ul);
  };

  const openAssigneePicker = (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }

    const opts = [{ id: '', label: '(unassigned)' }, ...parseActorOptions(root)];
    if (!opts.length) return;
    assigneePicker.open = true;
    assigneePicker.rowId = id;
    assigneePicker.rootEl = root;
    assigneePicker.options = opts;
    assigneePicker.idx = 0;

    const modal = ensureAssigneeModal();
    modal.style.display = 'flex';
    renderAssigneePicker();
  };

  const closeAssigneePicker = () => {
    const restoreId = assigneePicker.rowId;
    assigneePicker.open = false;
    assigneePicker.rowId = '';
    assigneePicker.rootEl = null;
    assigneePicker.options = [];
    assigneePicker.idx = 0;
    const modal = document.getElementById('native-assignee-modal');
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
  };

  const outlineRowRight = (row) => {
    if (!row) return null;
    return row.querySelector('.outline-right') || row;
  };

  const nativeRowUpdateAssignee = (row, opt) => {
    if (!row) return;
    const root = outlineRowRight(row);
    const lbl = (opt && typeof opt.label === 'string') ? opt.label.trim() : '';
    const wrap = row.querySelector('.outline-assignee');
    if (lbl && lbl !== '(unassigned)') {
      if (wrap) {
        wrap.textContent = '@' + lbl;
      } else {
        const s = document.createElement('span');
        s.className = 'outline-assignee dim';
        s.textContent = '@' + lbl;
        root && root.appendChild(s);
      }
    } else {
      wrap && wrap.remove();
    }
  };

  const pickSelectedAssignee = () => {
    if (!assigneePicker.open) return;
    const sel = assigneePicker.options[assigneePicker.idx];
    if (!sel) return;
    const id = assigneePicker.rowId;
    const root = assigneePicker.rootEl || nativeOutlineRoot();
    if (!root || !id) return;

    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    if (row) nativeRowUpdateAssignee(row, sel);
    const assignedActorId = (sel.id || '').trim();
    if (root && root.id === 'item-native') {
      const selEl = document.getElementById('assignedActorId');
      if (selEl) selEl.value = assignedActorId;
    }
    closeAssigneePicker();
    if (nativeOutlineRoot()) focusNativeRowById(id);

    outlineApply(root, 'outline:set_assign', { id, assignedActorId }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
    });
  };

  const statusPicker = {
    open: false,
    rowId: '',
    rootEl: null,
    options: [],
    idx: 0,
    note: '',
    mode: 'list', // 'list' | 'note'
    title: 'Status',
    submit: null, // optional override: ({statusID, option, note}) => Promise
  };

  const ensureStatusModal = () => {
    let el = document.getElementById('native-status-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-status-modal';
    el.style.position = 'fixed';
    el.style.left = '0';
    el.style.top = '0';
    el.style.right = '0';
    el.style.bottom = '0';
    el.style.background = 'rgba(0,0,0,.25)';
    el.style.display = 'none';
    el.style.alignItems = 'center';
    el.style.justifyContent = 'center';
    el.style.zIndex = '9998';
    el.innerHTML = `
      <div id="native-status-modal-box" style="max-width:520px;width:92vw;background:Canvas;color:CanvasText;border:1px solid rgba(127,127,127,.35);border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.25);padding:12px 14px;">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
          <strong id="native-status-title">Status</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-status-note-wrap" style="margin-top:10px;display:none;">
          <div class="dim" style="font-size:12px;margin-bottom:6px;">Note required</div>
          <input id="native-status-note" type="text" placeholder="Add a note…" style="width:100%;" />
        </div>
        <div id="native-status-list" style="margin-top:10px;max-height:46vh;overflow:auto;"></div>
        <div id="native-status-hint" class="dim" style="margin-top:10px;font-size:12px;">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:12px;align-items:center;">
          <button type="button" id="native-status-cancel">Cancel</button>
          <button type="button" id="native-status-ok">Select</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeStatusPicker();
    });
    const cancelBtn = el.querySelector('#native-status-cancel');
    cancelBtn && cancelBtn.addEventListener('click', () => {
      if (statusPicker.mode === 'note') {
        statusPicker.mode = 'list';
        statusPicker.note = '';
        renderStatusPicker();
        return;
      }
      closeStatusPicker();
    });
    const okBtn = el.querySelector('#native-status-ok');
    okBtn && okBtn.addEventListener('click', () => pickSelectedStatus());
    return el;
  };

  const renderStatusPicker = () => {
    const modal = ensureStatusModal();
    const title = modal.querySelector('#native-status-title');
    const list = modal.querySelector('#native-status-list');
    const noteWrap = modal.querySelector('#native-status-note-wrap');
    const noteInput = modal.querySelector('#native-status-note');
    const hint = modal.querySelector('#native-status-hint');
    const okBtn = modal.querySelector('#native-status-ok');
    const cancelBtn = modal.querySelector('#native-status-cancel');
    if (!list) return;
    if (title) title.textContent = statusPicker.title || 'Status';

    const opts = statusPicker.options || [];
    const sel = opts[statusPicker.idx] || null;
    const needsNote = !!(sel && sel.requiresNote);
    const inNoteMode = statusPicker.mode === 'note';
    noteWrap.style.display = (inNoteMode && needsNote) ? 'block' : 'none';
    if (hint) {
      hint.textContent = inNoteMode ? 'Type note · Enter to save · Esc to go back' : 'Up/Down or Ctrl+P/N to move · Enter to select';
    }
    if (okBtn) okBtn.textContent = inNoteMode ? 'Save' : 'Select';
    if (cancelBtn) cancelBtn.textContent = inNoteMode ? 'Back' : 'Cancel';
    if (inNoteMode && needsNote) {
      noteInput.value = statusPicker.note || '';
      setTimeout(() => noteInput.focus(), 0);
    }

    list.innerHTML = '';
    const ul = document.createElement('ul');
    ul.style.listStyle = 'none';
    ul.style.padding = '0';
    ul.style.margin = '0';
    opts.forEach((o, i) => {
      const li = document.createElement('li');
      li.style.padding = '6px 8px';
      li.style.borderRadius = '8px';
      li.style.cursor = 'pointer';
      if (i === statusPicker.idx) {
        li.style.background = 'color-mix(in oklab, Canvas, CanvasText 10%)';
      }
      const lbl = (o.label || o.id || '').trim() || '(none)';
      li.textContent = lbl;
      li.addEventListener('click', () => {
        statusPicker.idx = i;
        const next = statusPicker.options[statusPicker.idx] || null;
        if (next && next.requiresNote) {
          statusPicker.mode = 'note';
          renderStatusPicker();
          return;
        }
        renderStatusPicker();
        pickSelectedStatus();
      });
      ul.appendChild(li);
    });
    list.appendChild(ul);
  };

  const openStatusPicker = (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }

    const opts = parseStatusOptions(root);
    if (!opts.length) return;

    statusPicker.open = true;
    statusPicker.rowId = id;
    statusPicker.rootEl = root;
    statusPicker.options = opts;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Status';
    statusPicker.submit = null;
    // select current
    const cur = (row.dataset.status || '').trim();
    let idx = opts.findIndex((o) => (o.id || '').trim() === cur);
    if (idx < 0) idx = 0;
    statusPicker.idx = idx;

    const modal = ensureStatusModal();
    modal.style.display = 'flex';
    renderStatusPicker();
  };

  const closeStatusPicker = () => {
    const restoreId = statusPicker.rowId;
    statusPicker.open = false;
    statusPicker.rowId = '';
    statusPicker.rootEl = null;
    statusPicker.options = [];
    statusPicker.idx = 0;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Status';
    statusPicker.submit = null;
    const modal = document.getElementById('native-status-modal');
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
  };

  const nativeRowUpdateStatus = (row, opt) => {
    if (!row || !row.dataset) return;
    const statusID = (opt && typeof opt.id === 'string') ? opt.id : '';
    const label = (opt && typeof opt.label === 'string') ? opt.label : '';
    const isEnd = !!(opt && opt.isEndState);

    row.dataset.status = statusID;
    row.dataset.end = isEnd ? 'true' : 'false';

    const badge = row.querySelector('.outline-status');
    if (!badge) return;
    badge.classList.toggle('outline-status--end', isEnd);
    badge.classList.toggle('outline-status--open', !isEnd);
    const nextText = (label || statusID || '').trim() || '(none)';
    badge.textContent = nextText;
  };

  const pickSelectedStatus = () => {
    if (!statusPicker.open) return;
    const modal = document.getElementById('native-status-modal');
    const noteInput = modal ? modal.querySelector('#native-status-note') : null;
    const sel = statusPicker.options[statusPicker.idx];
    if (!sel) return;
    const id = statusPicker.rowId;
    const statusID = (sel.id || '').trim();
    if (!id) return;
    if (!statusID && statusID !== '') return;
    if (sel.requiresNote && statusPicker.mode !== 'note') {
      statusPicker.mode = 'note';
      renderStatusPicker();
      return;
    }

    let note = '';
    if (sel.requiresNote) {
      note = (noteInput && noteInput.value ? noteInput.value : '').trim();
      if (!note) {
        setOutlineStatus('Error: note required');
        setTimeout(() => setOutlineStatus(''), 1200);
        noteInput && noteInput.focus();
        return;
      }
      statusPicker.note = note;
    }

    if (typeof statusPicker.submit === 'function') {
      const submit = statusPicker.submit;
      const statusID2 = statusID;
      const opt = sel;
      closeStatusPicker();
      submit({ statusID: statusID2, option: opt, note }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
      return;
    }

    const root = statusPicker.rootEl || nativeOutlineRoot();
    if (!root) return;
    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    if (row) nativeRowUpdateStatus(row, sel);
    if (root && root.id === 'item-native') {
      const selEl = document.getElementById('status');
      if (selEl) selEl.value = statusID;
    }
    closeStatusPicker();
    if (nativeOutlineRoot()) focusNativeRowById(id);

    // Persist async; SSE will converge state.
    outlineApply(root, 'outline:toggle', { id, to: statusID, note }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
    });
  };

  const cycleStatus = (row, delta) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    const opts = parseStatusOptions(root);
    if (!opts.length) return;
    const cur = (row.dataset.status || '').trim();
    let idx = opts.findIndex((o) => (o.id || '').trim() === cur);
    if (idx < 0) idx = 0;
    let next = (idx + delta) % opts.length;
    if (next < 0) next += opts.length;
    const sel = opts[next];
    if (!sel) return;
    if (!sel.requiresNote) {
      nativeRowUpdateStatus(row, sel);
      outlineApply(root, 'outline:toggle', { id, to: (sel.id || '').trim(), note: '' }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
      return;
    }
    // Requires a note: open picker preselected; require Enter to confirm.
    statusPicker.open = true;
    statusPicker.rowId = id;
    statusPicker.rootEl = root;
    statusPicker.options = opts;
    statusPicker.idx = next;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Status';
    statusPicker.submit = null;
    const modal = ensureStatusModal();
    modal.style.display = 'flex';
    renderStatusPicker();
  };

  const prompt = {
    open: false,
    kind: '',
    rowId: '',
    outlineId: '',
    submit: null,
  };

  const ensurePromptModal = () => {
    let el = document.getElementById('native-prompt-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-prompt-modal';
    el.style.position = 'fixed';
    el.style.left = '0';
    el.style.top = '0';
    el.style.right = '0';
    el.style.bottom = '0';
    el.style.background = 'rgba(0,0,0,.25)';
    el.style.display = 'none';
    el.style.alignItems = 'center';
    el.style.justifyContent = 'center';
    el.style.zIndex = '9998';
    el.innerHTML = `
      <div id="native-prompt-box" style="max-width:640px;width:92vw;background:Canvas;color:CanvasText;border:1px solid rgba(127,127,127,.35);border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.25);padding:12px 14px;">
        <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
          <strong id="native-prompt-title"></strong>
          <span class="dim" id="native-prompt-hint" style="font-size:12px;">Esc to close · Ctrl+Enter to save</span>
        </div>
        <div id="native-prompt-body" style="margin-top:10px;"></div>
        <div style="display:flex;justify-content:flex-end;gap:10px;margin-top:12px;align-items:center;">
          <button type="button" id="native-prompt-cancel">Cancel</button>
          <button type="button" id="native-prompt-save">Save</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closePrompt();
    });
    const cancel = el.querySelector('#native-prompt-cancel');
    cancel && cancel.addEventListener('click', () => closePrompt());
    const save = el.querySelector('#native-prompt-save');
    save && save.addEventListener('click', () => submitPrompt());
    return el;
  };

  const openPrompt = ({ title, hint, bodyHTML, onSubmit, focusSelector }) => {
    const modal = ensurePromptModal();
    modal.querySelector('#native-prompt-title').textContent = title || '';
    modal.querySelector('#native-prompt-hint').textContent = hint || 'Esc to close · Ctrl+Enter to save';
    modal.querySelector('#native-prompt-body').innerHTML = bodyHTML || '';
    prompt.open = true;
    prompt.submit = onSubmit || null;
    modal.style.display = 'flex';
    const focus = focusSelector ? modal.querySelector(focusSelector) : null;
    focus && focus.focus();
  };

  const closePrompt = () => {
    prompt.open = false;
    prompt.kind = '';
    prompt.rowId = '';
    prompt.outlineId = '';
    prompt.submit = null;
    const modal = document.getElementById('native-prompt-modal');
    if (modal) modal.style.display = 'none';
  };

  const submitPrompt = () => {
    if (!prompt.open) return;
    if (typeof prompt.submit === 'function') prompt.submit();
  };

  const fetchItemMeta = async (itemId) => {
    const id = (itemId || '').trim();
    if (!id) throw new Error('missing id');
    const res = await fetch('/items/' + encodeURIComponent(id) + '/meta', {
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    });
    if (!res.ok) throw new Error(await res.text());
    return await res.json();
  };

  const rowSetFlag = (row, flagClass, on) => {
    if (!row) return;
    const root = outlineRowRight(row);
    const existing = row.querySelector('.' + flagClass);
    if (on) {
      if (existing) return;
      const s = document.createElement('span');
      s.className = 'outline-flag ' + flagClass;
      s.textContent = flagClass.includes('priority') ? 'P' : 'H';
      root && root.appendChild(s);
    } else {
      existing && existing.remove();
    }
  };

  const nativeRowUpdatePriority = (row, on) => {
    if (!row || !row.dataset) return;
    row.dataset.priority = on ? 'true' : 'false';
    rowSetFlag(row, 'outline-flag--priority', on);
  };

  const nativeRowUpdateOnHold = (row, on) => {
    if (!row || !row.dataset) return;
    row.dataset.onHold = on ? 'true' : 'false';
    rowSetFlag(row, 'outline-flag--hold', on);
  };

  const nativeRowUpdateTags = (row, tags) => {
    if (!row) return;
    const root = outlineRowRight(row);
    const raw = Array.isArray(tags) ? tags : [];
    const seen = new Set();
    const next = [];
    for (const t0 of raw) {
      const t = String(t0 || '').trim().replace(/^#+/, '');
      if (!t) continue;
      const key = t.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      next.push(t);
    }
    next.sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()) || a.localeCompare(b));
    let el = row.querySelector('.outline-tags');
    if (!next.length) {
      el && el.remove();
      return;
    }
    if (!el) {
      el = document.createElement('span');
      el.className = 'outline-tags dim';
      root && root.appendChild(el);
    }
    el.textContent = next.map((t) => '#' + t).join(' ');
  };

  const nativeRowUpdateDateTime = (row, kind, dt) => {
    if (!row || !row.dataset) return;
    const root = outlineRowRight(row);
    const keyDate = kind === 'due' ? 'dueDate' : 'schDate';
    const keyTime = kind === 'due' ? 'dueTime' : 'schTime';
    row.dataset[keyDate] = dt && dt.date ? dt.date : '';
    row.dataset[keyTime] = dt && dt.time ? dt.time : '';
    const cls = kind === 'due' ? 'outline-meta--due' : 'outline-meta--sch';
    let el = row.querySelector('.' + cls);
    if (!dt || !dt.date) {
      el && el.remove();
      return;
    }
    const label = (kind === 'due' ? 'Due ' : 'Sch ') + dt.date + (dt.time ? ' ' + dt.time : '');
    if (!el) {
      el = document.createElement('span');
      el.className = 'outline-meta dim ' + cls;
      root && root.appendChild(el);
    }
    el.textContent = label;
  };

  const parseTagsText = (txt) => {
    const raw = (txt || '').trim();
    if (!raw) return [];
    const parts = raw.split(/[\s,]+/g).map((x) => x.trim()).filter(Boolean);
    return parts.map((x) => x.replace(/^#+/, '').trim()).filter(Boolean);
  };

  const openStatusPickerForItemPage = (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    const opts = parseStatusOptions(root);
    if (!opts.length) return;
    let cur = '';
    const sel = document.getElementById('status');
    if (sel && sel.value != null) cur = String(sel.value || '').trim();
    let idx = opts.findIndex((o) => (o.id || '').trim() === cur);
    if (idx < 0) idx = 0;
    statusPicker.open = true;
    statusPicker.rowId = id;
    statusPicker.rootEl = root;
    statusPicker.options = opts;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Status';
    statusPicker.submit = null;
    statusPicker.idx = idx;
    const modal = ensureStatusModal();
    modal.style.display = 'flex';
    renderStatusPicker();
  };

  const openAssigneePickerForItemPage = (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    const opts = [{ id: '', label: '(unassigned)' }, ...parseActorOptions(root)];
    if (!opts.length) return;
    let cur = '';
    const sel = document.getElementById('assignedActorId');
    if (sel && sel.value != null) cur = String(sel.value || '').trim();
    let idx = opts.findIndex((o) => (o.id || '').trim() === cur);
    if (idx < 0) idx = 0;
    assigneePicker.open = true;
    assigneePicker.rowId = id;
    assigneePicker.rootEl = root;
    assigneePicker.options = opts;
    assigneePicker.idx = idx;
    const modal = ensureAssigneeModal();
    modal.style.display = 'flex';
    renderAssigneePicker();
  };

  const openItemTitlePrompt = (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    let cur = '';
    const titleInput = document.getElementById('title');
    if (titleInput && titleInput.value != null) cur = String(titleInput.value || '');
    if (!cur) {
      const h1 = document.querySelector('#clarity-main h1');
      if (h1) cur = h1.textContent || '';
    }
    openPrompt({
      title: 'Edit title',
      hint: 'Esc to close · Enter to save',
      bodyHTML: `<input id="native-prompt-input" placeholder="Title" style="width:100%;" value="${escapeAttr(cur)}">`,
      focusSelector: '#native-prompt-input',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const input = modal ? modal.querySelector('#native-prompt-input') : null;
        const newText = input ? (input.value || '').trim() : '';
        if (!newText) return;
        if (titleInput) titleInput.value = newText;
        const h1 = document.querySelector('#clarity-main h1');
        if (h1) h1.textContent = newText;
        closePrompt();
        outlineApply(root, 'outline:edit:save', { id, newText }).catch(() => {});
      },
    });
  };

  const openItemDescriptionPrompt = async (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    let meta;
    try {
      meta = await fetchItemMeta(id);
    } catch (_) {
      return;
    }
    const initial = (meta && typeof meta.description === 'string') ? meta.description : '';
    openPrompt({
      title: 'Edit description',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `<textarea id="native-prompt-textarea" rows="10" style="width:100%;font-family:var(--mono);">${escapeHTML(initial)}</textarea>`,
      focusSelector: '#native-prompt-textarea',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const ta = modal ? modal.querySelector('#native-prompt-textarea') : null;
        const description = ta ? (ta.value || '') : '';
        const desc = document.getElementById('description');
        if (desc) desc.value = description;
        closePrompt();
        outlineApply(root, 'outline:set_description', { id, description }).catch(() => {});
      },
    });
  };

  const openItemTagsPrompt = async (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    let meta;
    try {
      meta = await fetchItemMeta(id);
    } catch (_) {
      return;
    }
    const tags = (meta && Array.isArray(meta.tags)) ? meta.tags : [];
    const initial = tags.map((t) => '#' + t).join(' ');
    openPrompt({
      title: 'Edit tags',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `<input id="native-prompt-tags" placeholder="#tag1 #tag2" style="width:100%;" value="${escapeAttr(initial)}">`,
      focusSelector: '#native-prompt-tags',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const input = modal ? modal.querySelector('#native-prompt-tags') : null;
        const txt = input ? (input.value || '') : '';
        const tags = parseTagsText(txt);
        const t = document.getElementById('tags');
        if (t) t.value = tags.map((x) => '#' + x).join(' ');
        closePrompt();
        outlineApply(root, 'outline:set_tags', { id, tags }).catch(() => {});
      },
    });
  };

  const openItemDatePrompt = (root, kind) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
    const dateName = kind === 'due' ? 'dueDate' : 'schDate';
    const timeName = kind === 'due' ? 'dueTime' : 'schTime';
    const dateEl = document.querySelector('input[type="date"][name="' + dateName + '"]');
    const timeEl = document.querySelector('input[type="time"][name="' + timeName + '"]');
    const curDate = dateEl ? (dateEl.value || '').trim() : '';
    const curTime = timeEl ? (timeEl.value || '').trim() : '';
    openPrompt({
      title: kind === 'due' ? 'Set due' : 'Set schedule',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;">
          <label class="dim" style="font-size:12px;">Date <input id="native-prompt-date" type="date" value="${escapeAttr(curDate)}"></label>
          <label class="dim" style="font-size:12px;">Time <input id="native-prompt-time" type="time" value="${escapeAttr(curTime)}"></label>
          <button type="button" id="native-prompt-clear">Clear</button>
        </div>
      `,
      focusSelector: '#native-prompt-date',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const di = modal ? modal.querySelector('#native-prompt-date') : null;
        const ti = modal ? modal.querySelector('#native-prompt-time') : null;
        const date = di ? (di.value || '').trim() : '';
        const time = ti ? (ti.value || '').trim() : '';
        if (dateEl) dateEl.value = date;
        if (timeEl) timeEl.value = time;
        closePrompt();
        const typ = kind === 'due' ? 'outline:set_due' : 'outline:set_schedule';
        outlineApply(root, typ, { id, date, time }).catch(() => {});
      },
    });
    const modal = document.getElementById('native-prompt-modal');
    const clear = modal ? modal.querySelector('#native-prompt-clear') : null;
    clear && clear.addEventListener('click', () => {
      const di = modal.querySelector('#native-prompt-date');
      const ti = modal.querySelector('#native-prompt-time');
      if (di) di.value = '';
      if (ti) ti.value = '';
      submitPrompt();
    }, { once: true });
  };

  const openItemTextPostPrompt = (root, kind) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    openPrompt({
      title: kind === 'comment' ? 'Add comment' : 'Log work',
      hint: 'Esc to close · Ctrl+Enter to post',
      bodyHTML: `<textarea id="native-prompt-textarea" rows="8" style="width:100%;font-family:var(--mono);" placeholder="${kind === 'comment' ? 'Write a comment…' : 'Log work…'}"></textarea>`,
      focusSelector: '#native-prompt-textarea',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const ta = modal ? modal.querySelector('#native-prompt-textarea') : null;
        const body = ta ? (ta.value || '').trim() : '';
        if (!body) return;
        closePrompt();
        const path = kind === 'comment' ? '/items/' + encodeURIComponent(id) + '/comments' : '/items/' + encodeURIComponent(id) + '/worklog';
        fetch(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ body }).toString(),
        }).catch(() => {});
      },
    });
  };

  const openNewItemPrompt = (mode, row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root) return;
    const outlineId = (root.dataset.outlineId || '').trim();
    if (!outlineId) return;
    const refId = row ? (row.dataset.id || '').trim() : '';
    openPrompt({
      title: mode === 'child' ? 'New child' : 'New sibling',
      hint: 'Esc to close · Enter to create',
      bodyHTML: `<input id="native-prompt-input" placeholder="Title" style="width:100%;">`,
      focusSelector: '#native-prompt-input',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const input = modal ? modal.querySelector('#native-prompt-input') : null;
        const title = input ? (input.value || '').trim() : '';
        if (!title) return;
        closePrompt();
        const typ = mode === 'child' ? 'outline:new_child' : 'outline:new_sibling';
        const detail = mode === 'child' ? { title, parentId: refId } : { title, afterId: refId };
        outlineApply(root, typ, detail).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
        });
      },
    });
  };

  const openEditDescriptionPrompt = async (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    let meta;
    try {
      meta = await fetchItemMeta(id);
    } catch (err) {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'load failed'));
      return;
    }
    const initial = (meta && typeof meta.description === 'string') ? meta.description : '';
    openPrompt({
      title: 'Edit description',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `<textarea id="native-prompt-textarea" rows="10" style="width:100%;font-family:var(--mono);">${escapeHTML(initial)}</textarea>`,
      focusSelector: '#native-prompt-textarea',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const ta = modal ? modal.querySelector('#native-prompt-textarea') : null;
        const description = ta ? (ta.value || '') : '';
        closePrompt();
        outlineApply(root, 'outline:set_description', { id, description }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
        });
      },
    });
  };

  const openTagsPrompt = async (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    let meta;
    try {
      meta = await fetchItemMeta(id);
    } catch (err) {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'load failed'));
      return;
    }
    const tags = (meta && Array.isArray(meta.tags)) ? meta.tags : [];
    const initial = tags.map((t) => '#' + t).join(' ');
    openPrompt({
      title: 'Edit tags',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `<input id="native-prompt-tags" placeholder="#tag1 #tag2" style="width:100%;" value="${escapeAttr(initial)}">`,
      focusSelector: '#native-prompt-tags',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const input = modal ? modal.querySelector('#native-prompt-tags') : null;
        const txt = input ? (input.value || '') : '';
        const tags = parseTagsText(txt);
        nativeRowUpdateTags(row, tags);
        closePrompt();
        outlineApply(root, 'outline:set_tags', { id, tags }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
        });
      },
    });
  };

  const openDatePrompt = (row, kind) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    const dateKey = kind === 'due' ? 'dueDate' : 'schDate';
    const timeKey = kind === 'due' ? 'dueTime' : 'schTime';
    const curDate = (row.dataset[dateKey] || '').trim();
    const curTime = (row.dataset[timeKey] || '').trim();
    openPrompt({
      title: kind === 'due' ? 'Set due' : 'Set schedule',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div style="display:flex;gap:10px;align-items:center;flex-wrap:wrap;">
          <label class="dim" style="font-size:12px;">Date <input id="native-prompt-date" type="date" value="${escapeAttr(curDate)}"></label>
          <label class="dim" style="font-size:12px;">Time <input id="native-prompt-time" type="time" value="${escapeAttr(curTime)}"></label>
          <button type="button" id="native-prompt-clear">Clear</button>
        </div>
      `,
      focusSelector: '#native-prompt-date',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const di = modal ? modal.querySelector('#native-prompt-date') : null;
        const ti = modal ? modal.querySelector('#native-prompt-time') : null;
        const date = di ? (di.value || '').trim() : '';
        const time = ti ? (ti.value || '').trim() : '';
        nativeRowUpdateDateTime(row, kind, date ? { date, time: time || null } : null);
        closePrompt();
        const typ = kind === 'due' ? 'outline:set_due' : 'outline:set_schedule';
        outlineApply(root, typ, { id, date, time }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
        });
      },
    });
    const modal = document.getElementById('native-prompt-modal');
    const clear = modal ? modal.querySelector('#native-prompt-clear') : null;
    clear && clear.addEventListener('click', () => {
      const di = modal.querySelector('#native-prompt-date');
      const ti = modal.querySelector('#native-prompt-time');
      if (di) di.value = '';
      if (ti) ti.value = '';
      submitPrompt();
    }, { once: true });
  };

  const openTextPostPrompt = (row, kind) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    openPrompt({
      title: kind === 'comment' ? 'Add comment' : 'Log work',
      hint: 'Esc to close · Ctrl+Enter to post',
      bodyHTML: `<textarea id="native-prompt-textarea" rows="8" style="width:100%;font-family:var(--mono);" placeholder="${kind === 'comment' ? 'Write a comment…' : 'Log work…'}"></textarea>`,
      focusSelector: '#native-prompt-textarea',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const ta = modal ? modal.querySelector('#native-prompt-textarea') : null;
        const body = ta ? (ta.value || '').trim() : '';
        if (!body) return;
        closePrompt();
        const path = kind === 'comment' ? '/items/' + encodeURIComponent(id) + '/comments' : '/items/' + encodeURIComponent(id) + '/worklog';
        fetch(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body: new URLSearchParams({ body }).toString(),
        }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'post failed'));
        });
      },
    });
  };

  const openArchivePrompt = (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    if (!root || !row) return;
    const id = (row.dataset.id || '').trim();
    if (!id) return;
    if ((row.dataset.canEdit || '') !== 'true') {
      setOutlineStatus('Error: owner-only');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    openPrompt({
      title: 'Archive item',
      hint: 'Esc to cancel · Enter to archive',
      bodyHTML: `<div>Archive <code>${escapeHTML(id)}</code>?</div>`,
      focusSelector: '#native-prompt-save',
      onSubmit: () => {
        closePrompt();
        outlineApply(root, 'outline:archive', { id }).then(() => {
          const li = nativeLiFromRow(row);
          li && li.remove();
        }).catch((err) => {
          setOutlineStatus('Error: ' + (err && err.message ? err.message : 'archive failed'));
        });
      },
    });
  };

  const outlineCollapseKey = (outlineId) => 'clarity:outline:' + outlineId + ':collapsed';

  const loadCollapsedSet = (root) => {
    const outlineId = (root && root.dataset ? root.dataset.outlineId : '') || '';
    if (!outlineId) return new Set();
    try {
      const raw = localStorage.getItem(outlineCollapseKey(outlineId));
      const xs = raw ? JSON.parse(raw) : [];
      return new Set(Array.isArray(xs) ? xs : []);
    } catch (_) {
      return new Set();
    }
  };

  const saveCollapsedSet = (root, set) => {
    const outlineId = (root && root.dataset ? root.dataset.outlineId : '') || '';
    if (!outlineId) return;
    try {
      localStorage.setItem(outlineCollapseKey(outlineId), JSON.stringify(Array.from(set)));
    } catch (_) {}
  };

  const applyCollapsed = (root, set) => {
    if (!root) return;
    root.querySelectorAll('li[data-node-id]').forEach((li) => {
      const id = (li.dataset.nodeId || '').trim();
      const ul = li.querySelector(':scope > ul.outline-children');
      if (!ul) return;
      const collapsed = set.has(id);
      ul.style.display = collapsed ? 'none' : '';
      const row = li.querySelector(':scope > [data-outline-row]');
      const caret = row ? row.querySelector('.outline-caret') : null;
      if (caret) {
        caret.textContent = collapsed ? '▸' : '▾';
        caret.classList.remove('outline-caret--none');
      }
    });
    // Mark carets for nodes without children.
    root.querySelectorAll('[data-outline-row]').forEach((row) => {
      const li = nativeLiFromRow(row);
      if (!li) return;
      const ul = li.querySelector(':scope > ul.outline-children');
      const caret = row.querySelector('.outline-caret');
      if (!caret) return;
      if (!ul) {
        caret.textContent = '';
        caret.classList.add('outline-caret--none');
      }
    });
  };

  const toggleCollapseRow = (row) => {
    const root = nativeOutlineRootOrFromRow(row);
    const li = nativeLiFromRow(row);
    if (!root || !li || !li.dataset) return;
    const id = (li.dataset.nodeId || '').trim();
    if (!id) return;
    const set = loadCollapsedSet(root);
    if (set.has(id)) set.delete(id);
    else set.add(id);
    saveCollapsedSet(root, set);
    applyCollapsed(root, set);
  };

  const toggleCollapseAll = (root) => {
    if (!root) return;
    const ids = [];
    root.querySelectorAll('li[data-node-id]').forEach((li) => {
      const id = (li.dataset.nodeId || '').trim();
      const ul = li.querySelector(':scope > ul.outline-children');
      if (id && ul) ids.push(id);
    });
    const set = loadCollapsedSet(root);
    const anyExpanded = ids.some((id) => !set.has(id));
    const next = new Set();
    if (anyExpanded) {
      ids.forEach((id) => next.add(id));
    }
    saveCollapsedSet(root, next);
    applyCollapsed(root, next);
  };

  const nativeReorder = (row, dir) => {
    const root = nativeOutlineRoot();
    const li = nativeLiFromRow(row);
    if (!root || !li || !li.dataset) return;
    if ((row.dataset.canEdit || '') !== 'true') return;

    const sib = liSibling(li, dir);
    if (!sib) return;
    const parentId = parentIdForLi(li);
    const id = (li.dataset.nodeId || '').trim();
    const sibId = (sib.dataset.nodeId || '').trim();
    if (!id || !sibId) return;

    // Optimistic DOM move.
    if (dir === 'prev') {
      li.parentElement.insertBefore(li, sib);
      queueOutlineMove(root, { id, parentId, beforeId: sibId });
    } else {
      li.parentElement.insertBefore(li, sib.nextSibling);
      queueOutlineMove(root, { id, parentId, afterId: sibId });
    }
    focusNativeRowById(id);
  };

  const nativeIndent = (row) => {
    const root = nativeOutlineRoot();
    const li = nativeLiFromRow(row);
    if (!root || !li || !li.dataset) return;
    if ((row.dataset.canEdit || '') !== 'true') return;
    const prev = liSibling(li, 'prev');
    if (!prev) return;

    const id = (li.dataset.nodeId || '').trim();
    const parentId = (prev.dataset.nodeId || '').trim();
    if (!id || !parentId) return;

    const ul = ensureChildList(prev);
    // Determine afterId (append at end).
    const lastChild = ul.lastElementChild && ul.lastElementChild.dataset ? (ul.lastElementChild.dataset.nodeId || '').trim() : '';
    ul.appendChild(li);

    const detail = { id, parentId };
    if (lastChild) detail.afterId = lastChild;
    queueOutlineMove(root, detail);
    focusNativeRowById(id);
  };

  const nativeOutdent = (row) => {
    const root = nativeOutlineRoot();
    const li = nativeLiFromRow(row);
    if (!root || !li || !li.dataset) return;
    if ((row.dataset.canEdit || '') !== 'true') return;
    const parentLi = li.parentElement ? li.parentElement.closest('li[data-node-id]') : null;
    if (!parentLi || !parentLi.dataset) return;

    const id = (li.dataset.nodeId || '').trim();
    const afterId = (parentLi.dataset.nodeId || '').trim(); // insert after parent
    if (!id || !afterId) return;

    const grandParentUl = parentLi.parentElement;
    if (!grandParentUl) return;
    grandParentUl.insertBefore(li, parentLi.nextSibling);

    const parentId = parentIdForLi(parentLi);
    const detail = { id, afterId };
    if (parentId) detail.parentId = parentId;
    queueOutlineMove(root, detail);
    focusNativeRowById(id);
  };

  const setOutlineStatus = (msg) => {
    const el = document.getElementById('outline-status');
    if (!el) return;
    el.textContent = msg || '';
  };

  const copyTextToClipboard = async (text) => {
    const txt = String(text == null ? '' : text);
    if (!txt) throw new Error('empty');
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(txt);
      return;
    }
    // Fallback for non-secure contexts.
    const ta = document.createElement('textarea');
    ta.value = txt;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    ta.style.top = '0';
    ta.setAttribute('readonly', 'readonly');
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand && document.execCommand('copy');
    ta.remove();
    if (!ok) throw new Error('copy failed');
  };

  const drainOutlineMoveOps = (outlineEl) => {
    if (!outlineEl) return [];
    const buf = outlineMoveBufferByEl.get(outlineEl);
    if (!buf || !buf.ops || buf.ops.length === 0) return [];
    const ops = buf.ops.slice();
    buf.ops = [];
    if (buf.timer) {
      clearTimeout(buf.timer);
      buf.timer = null;
    }
    outlineMoveBufferByEl.set(outlineEl, buf);
    return ops;
  };

  const flushOutlineMoves = (outlineEl) => {
    const ops = drainOutlineMoveOps(outlineEl);
    if (ops.length === 0) return Promise.resolve();
    return outlineApplyOps(outlineEl, ops);
  };

  const queueOutlineMove = (outlineEl, detail) => {
    if (!outlineEl) return;
    let buf = outlineMoveBufferByEl.get(outlineEl);
    if (!buf) buf = { ops: [], timer: null };
    buf.ops.push({ type: 'outline:move', detail });
    if (buf.timer) clearTimeout(buf.timer);
    buf.timer = setTimeout(() => {
      flushOutlineMoves(outlineEl).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
    }, 1500);
    outlineMoveBufferByEl.set(outlineEl, buf);
  };

  const outlineApplyPayload = async (outlineEl, payload) => {
    if (!outlineEl) return;
    const outlineId = (outlineEl.getAttribute('data-outline-id') || '').trim();
    if (!outlineId) return;

    // Coalesce any pending outline moves into the next non-move mutation so we get a
    // single server-side apply (and therefore one git commit) once the user stops moving.
    if (payload && !Array.isArray(payload.ops)) {
      const moveOps = drainOutlineMoveOps(outlineEl);
      if (moveOps.length > 0) {
        payload = { ops: [...moveOps, payload] };
      }
    }

    let pending = outlinePendingByEl.get(outlineEl);
    if (!pending) pending = Promise.resolve();

    pending = pending.then(async () => {
      const res = await fetch('/outlines/' + encodeURIComponent(outlineId) + '/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (!res.ok) {
        const txt = await res.text();
        throw new Error(txt || ('HTTP ' + res.status));
      }
      // Do not update the component directly here. When using Datastar, the outline page is
      // kept in sync by signals pushed over SSE, which then drive `data-attr:*` bindings.
      // This avoids fighting Datastar and keeps component state stable.
    });

    outlinePendingByEl.set(outlineEl, pending);
    return pending;
  };

  const outlineApply = async (outlineEl, type, detail) => {
    return outlineApplyPayload(outlineEl, { type, detail });
  };

  const outlineApplyOps = async (outlineEl, ops) => {
    ops = Array.isArray(ops) ? ops : [];
    if (ops.length === 0) return;
    return outlineApplyPayload(outlineEl, { ops });
  };

  const outlineFindLi = (outlineEl, id) => {
    if (!outlineEl || !outlineEl.shadowRoot) return null;
    try {
      return outlineEl.shadowRoot.querySelector('li[data-id="' + id.replaceAll('"', '\\"') + '"]');
    } catch (_) {
      return null;
    }
  };

  const outlineSiblingId = (node, dir) => {
    let cur = dir === 'prev' ? node.previousElementSibling : node.nextElementSibling;
    while (cur) {
      if ((cur.tagName || '').toUpperCase() === 'LI') return cur.dataset.id || null;
      cur = dir === 'prev' ? cur.previousElementSibling : cur.nextElementSibling;
    }
    return null;
  };

  const clearAwaiting = () => {
    state.awaiting = null;
    state.awaitingAt = 0;
  };

  const ensureHelp = () => {
    let el = document.getElementById('clarity-kb-help');
    if (el) return el;

    el = document.createElement('div');
    el.id = 'clarity-kb-help';
    el.style.position = 'fixed';
    el.style.top = '16px';
    el.style.right = '16px';
    el.style.maxWidth = '420px';
    el.style.padding = '12px 14px';
    el.style.border = '1px solid rgba(127,127,127,.35)';
    el.style.borderRadius = '10px';
    el.style.background = 'Canvas';
    el.style.color = 'CanvasText';
    el.style.boxShadow = '0 6px 18px rgba(0,0,0,.15)';
    el.style.zIndex = '9999';
    el.innerHTML = `
      <div style="display:flex;justify-content:space-between;gap:12px;align-items:baseline;">
        <strong>Shortcuts</strong>
        <a href="#" id="clarity-kb-close" class="dim">close</a>
      </div>
      <div style="margin-top:8px;line-height:1.6;">
        <div><code>g</code> <code>h</code> — Home</div>
        <div><code>g</code> <code>p</code> — Projects</div>
        <div><code>g</code> <code>a</code> — Agenda</div>
        <div><code>g</code> <code>s</code> — Sync</div>
        <div><code>j</code>/<code>k</code> — Move focus in lists</div>
        <div><code>Enter</code> — Open focused item</div>
        <div><code>?</code> — Toggle this help</div>
      </div>
    `;
    document.body.appendChild(el);
    el.querySelector('#clarity-kb-close')?.addEventListener('click', (ev) => {
      ev.preventDefault();
      el.style.display = 'none';
    });
    return el;
  };

  const toggleHelp = () => {
    const el = ensureHelp();
    el.style.display = (el.style.display === 'none' ? 'block' : 'none');
  };

  const focusables = () => {
    const root = document.getElementById('clarity-main');
    if (!root) return [];
    return Array.from(root.querySelectorAll('[data-kb-item]')).filter((el) => {
      if (!el) return false;
      if (el.hasAttribute('disabled')) return false;
      if (el.getAttribute('aria-disabled') === 'true') return false;
      return true;
    });
  };

  const moveFocus = (delta) => {
    const xs = focusables();
    if (xs.length === 0) return;
    const active = document.activeElement;
    let idx = xs.indexOf(active);
    if (idx < 0) idx = (delta > 0 ? -1 : 0);
    const next = xs[Math.max(0, Math.min(xs.length - 1, idx + delta))];
    next?.focus?.();
  };

  const openFocused = () => {
    const el = document.activeElement;
    if (!el) return;
    if (typeof el.click === 'function') el.click();
  };

  const rememberFocus = () => {
    const el = document.activeElement;
    if (!el) return;
    const id = el.getAttribute && el.getAttribute('data-focus-id');
    if (!id) return;
    try {
      sessionStorage.setItem('clarity:lastFocus', id);
    } catch (_) {}
  };

  const restoreFocus = () => {
    let id = '';
    try {
      id = sessionStorage.getItem('clarity:lastFocus') || '';
    } catch (_) {}
    id = (id || '').trim();
    if (!id) return;
    const root = document.getElementById('clarity-main') || document;
    const el = root.querySelector?.('[data-focus-id="' + CSS.escape(id) + '"]');
    if (el && typeof el.focus === 'function') {
      el.focus();
    }
  };

  const scheduleRestoreFocus = () => {
    if (activeIsTyping()) return;
    if (state.restoreTimer) return;
    state.restoreTimer = setTimeout(() => {
      state.restoreTimer = null;
      if (activeIsTyping()) return;
      restoreFocus();
      const native = nativeOutlineRoot();
      if (native) {
        applyCollapsed(native, loadCollapsedSet(native));
      }
    }, 50);
  };

  document.addEventListener('focusin', () => {
    rememberFocus();
  }, { capture: true });

  const obs = new MutationObserver(() => {
    scheduleRestoreFocus();
  });
  const startObserver = () => {
    const root = document.getElementById('clarity-main');
    if (!root) return;
    try { obs.disconnect(); } catch (_) {}
    obs.observe(root, { subtree: true, childList: true });
  };
  startObserver();
  // Datastar morph can replace `#clarity-main`; ensure our observer reattaches.
  const bodyObs = new MutationObserver(() => startObserver());
  try {
    bodyObs.observe(document.documentElement || document.body, { subtree: true, childList: true });
  } catch (_) {}
  scheduleRestoreFocus();

  // Toggle collapse by clicking the caret.
  document.addEventListener('click', (ev) => {
    const t = ev && ev.target;
    if (!t || typeof t.closest !== 'function') return;
    const caret = t.closest('.outline-caret');
    if (!caret) return;
    const row = caret.closest('[data-outline-row]');
    if (!row) return;
    ev.preventDefault();
    toggleCollapseRow(row);
  }, { capture: true });

  // Outline component events (delegated so it survives Datastar morphs).
  document.addEventListener('focusin', (ev) => {
    const outlineEl = outlineFromEvent(ev);
    if (!outlineEl) return;
    const path = typeof ev.composedPath === 'function' ? ev.composedPath() : [];
    for (const node of path) {
      if (!node) continue;
      if ((node.tagName || '').toUpperCase() === 'LI' && node.dataset && node.dataset.id) {
        rememberOutlineFocus(outlineEl, node.dataset.id);
        break;
      }
    }
  }, { capture: true });

  // When outline signals update, attempt to restore the focused item inside the component.
  document.addEventListener('datastar-signal-patch', (ev) => {
    const d = ev && ev.detail ? ev.detail : null;
    if (!d) return;
    if (!Object.prototype.hasOwnProperty.call(d, 'outlineItems')) return;
    const outlineEl = document.getElementById('outline');
    if (!outlineEl) return;
    // Allow the component to re-render from updated attributes first.
    setTimeout(() => restoreOutlineFocus(outlineEl), 50);
  }, { capture: true });

  document.addEventListener('outline:open', (ev) => {
    const id = ev.detail && ev.detail.id;
    if (!id) return;
    window.location.href = '/items/' + encodeURIComponent(id);
  }, { capture: true });

  document.addEventListener('outline:edit:save', (ev) => {
    const outlineEl = outlineFromEvent(ev);
    const id = ev.detail && ev.detail.id;
    // The component may emit `text` or `newText` depending on version.
    let text = '';
    if (ev.detail) {
      text = ev.detail.text ?? ev.detail.newText ?? ev.detail.title ?? '';
    }
    if (!outlineEl || !id) return;
    const newText = (text || '').trim();
    if (!newText) {
      setOutlineStatus('Error: title cannot be empty');
      setTimeout(() => setOutlineStatus(''), 1200);
      return;
    }
    outlineApply(outlineEl, 'outline:edit:save', { id, newText }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
    });
  }, { capture: true });

  document.addEventListener('outline:toggle', (ev) => {
    const outlineEl = outlineFromEvent(ev);
    const id = ev.detail && ev.detail.id;
    const status = ev.detail && ev.detail.status;
    if (!outlineEl || !id) return;
    outlineApply(outlineEl, 'outline:toggle', { id, to: status || '' }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
    });
  }, { capture: true });

  const handleOutlineReorderLike = (ev) => {
    const outlineEl = outlineFromEvent(ev);
    const id = ev.detail && ev.detail.id;
    if (!outlineEl || !id) return;
    const li = outlineFindLi(outlineEl, id);
    if (!li) return;

    const parentLi = li.parentElement ? li.parentElement.closest('li') : null;
    const parentId = parentLi ? (parentLi.dataset.id || null) : null;
    const prevId = outlineSiblingId(li, 'prev');
    const nextId = outlineSiblingId(li, 'next');
    const detail = {
      id,
      parentId,
      afterId: prevId,
      beforeId: (!prevId && nextId) ? nextId : null
    };

    queueOutlineMove(outlineEl, detail);
    li.focus();
  };

  document.addEventListener('outline:move', handleOutlineReorderLike, { capture: true });
  document.addEventListener('outline:indent', handleOutlineReorderLike, { capture: true });
  document.addEventListener('outline:outdent', handleOutlineReorderLike, { capture: true });

  const handleStatusPickerKeydown = (ev) => {
    if (!statusPicker.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (k === 'escape') {
      ev.preventDefault();
      if (statusPicker.mode === 'note') {
        statusPicker.mode = 'list';
        statusPicker.note = '';
        renderStatusPicker();
        return true;
      }
      closeStatusPicker();
      return true;
    }
    if (ev.ctrlKey && k === 'enter') {
      ev.preventDefault();
      pickSelectedStatus();
      return true;
    }
    if (k === 'enter') {
      ev.preventDefault();
      pickSelectedStatus();
      return true;
    }
    if (statusPicker.mode === 'list') {
      if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
        ev.preventDefault();
        statusPicker.idx = Math.min((statusPicker.options.length || 1) - 1, statusPicker.idx + 1);
        renderStatusPicker();
        return true;
      }
      if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
        ev.preventDefault();
        statusPicker.idx = Math.max(0, statusPicker.idx - 1);
        renderStatusPicker();
        return true;
      }
      // When modal is open, swallow other keys to avoid triggering app navigation.
      return true;
    }
    // Note mode: let typing happen in the input, but keep Enter/Esc handled above.
    return true;
  };

  const handleAssigneePickerKeydown = (ev) => {
    if (!assigneePicker.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (k === 'escape') {
      ev.preventDefault();
      closeAssigneePicker();
      return true;
    }
    if (ev.ctrlKey && k === 'enter') {
      ev.preventDefault();
      pickSelectedAssignee();
      return true;
    }
    if (k === 'enter') {
      ev.preventDefault();
      pickSelectedAssignee();
      return true;
    }
    if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
      ev.preventDefault();
      assigneePicker.idx = Math.min((assigneePicker.options.length || 1) - 1, assigneePicker.idx + 1);
      renderAssigneePicker();
      return true;
    }
    if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
      ev.preventDefault();
      assigneePicker.idx = Math.max(0, assigneePicker.idx - 1);
      renderAssigneePicker();
      return true;
    }
    return true;
  };

  const handleTagsPickerKeydown = (ev) => {
    if (!tagsPicker.open) return false;
    const k = (ev.key || '').toLowerCase();
    const modal = document.getElementById('native-tags-modal');
    const input = modal ? modal.querySelector('#native-tags-new') : null;
    const inInput = input && ev.target === input;

    if (!inInput && input && !ev.ctrlKey && !ev.metaKey && !ev.altKey && k === 'a') {
      // Quick "add tag" focus (keyboard-first).
      ev.preventDefault();
      input.focus();
      input.select && input.select();
      return true;
    }

    if (k === 'escape') {
      ev.preventDefault();
      closeTagsPicker('cancel');
      return true;
    }
    if (ev.ctrlKey && k === 'enter') {
      ev.preventDefault();
      closeTagsPicker('done');
      return true;
    }
    if (k === 'enter') {
      ev.preventDefault();
      if (inInput) {
        addNewTagFromInput();
      } else {
        closeTagsPicker('done');
      }
      return true;
    }
    if (k === ' ') {
      ev.preventDefault();
      toggleSelectedTag();
      return true;
    }
    if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
      ev.preventDefault();
      tagsPicker.idx = Math.min((tagsPicker.options.length || 1) - 1, tagsPicker.idx + 1);
      renderTagsPicker();
      return true;
    }
    if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
      ev.preventDefault();
      tagsPicker.idx = Math.max(0, tagsPicker.idx - 1);
      renderTagsPicker();
      return true;
    }

    // When modal is open, swallow other keys to avoid triggering app navigation.
    return true;
  };

  const handleMoveOutlinePickerKeydown = (ev) => {
    if (!moveOutlinePicker.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (k === 'escape') {
      ev.preventDefault();
      closeMoveOutlinePicker();
      return true;
    }
    if (ev.ctrlKey && k === 'enter') {
      ev.preventDefault();
      pickSelectedMoveOutline();
      return true;
    }
    if (k === 'enter') {
      ev.preventDefault();
      pickSelectedMoveOutline();
      return true;
    }
    if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
      ev.preventDefault();
      moveOutlinePicker.idx = Math.min((moveOutlinePicker.options.length || 1) - 1, moveOutlinePicker.idx + 1);
      renderMoveOutlinePicker();
      return true;
    }
    if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
      ev.preventDefault();
      moveOutlinePicker.idx = Math.max(0, moveOutlinePicker.idx - 1);
      renderMoveOutlinePicker();
      return true;
    }
    // Keep modal isolated, but allow normal browser behavior (e.g. Tab) by not preventing default.
    return true;
  };

  const handlePromptKeydown = (ev) => {
    if (!prompt.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (k === 'escape') {
      ev.preventDefault();
      closePrompt();
      return true;
    }
    if (ev.ctrlKey && k === 'enter') {
      ev.preventDefault();
      submitPrompt();
      return true;
    }
    if (k === 'enter') {
      const tag = (ev.target && ev.target.tagName ? ev.target.tagName.toLowerCase() : '');
      if (tag && tag !== 'textarea') {
        ev.preventDefault();
        submitPrompt();
        return true;
      }
    }
    // When modal is open, swallow other keys to avoid triggering app navigation.
    return true;
  };

  const handleNativeOutlineKeydown = (ev, key, nativeRow) => {
    if (!nativeRow) return false;
    // Native outline-specific shortcuts.
    // Prefer `code` for Alt+J/K on macOS (Option modifies `key` into a symbol).
    if (ev.altKey && ev.code === 'KeyJ') {
      ev.preventDefault();
      nativeReorder(nativeRow, 'next');
      return true;
    }
    if (ev.altKey && ev.code === 'KeyK') {
      ev.preventDefault();
      nativeReorder(nativeRow, 'prev');
      return true;
    }
    if (ev.altKey && (key === 'arrowdown' || key === 'down')) {
      ev.preventDefault();
      nativeReorder(nativeRow, 'next');
      return true;
    }
    if (ev.altKey && (key === 'arrowup' || key === 'up')) {
      ev.preventDefault();
      nativeReorder(nativeRow, 'prev');
      return true;
    }
    if (key === 'j') {
      ev.preventDefault();
      nativeRowSibling(nativeRow, +1)?.focus?.();
      return true;
    }
    if (key === 'k') {
      ev.preventDefault();
      nativeRowSibling(nativeRow, -1)?.focus?.();
      return true;
    }
    if (key === 'arrowdown' || key === 'down' || (ev.ctrlKey && key === 'n')) {
      ev.preventDefault();
      nativeRowSibling(nativeRow, +1)?.focus?.();
      return true;
    }
    if (key === 'arrowup' || key === 'up' || (ev.ctrlKey && key === 'p')) {
      ev.preventDefault();
      nativeRowSibling(nativeRow, -1)?.focus?.();
      return true;
    }
    if (key === 'enter') {
      ev.preventDefault();
      const href = (nativeRow.dataset.openHref || '').trim();
      if (href) window.location.href = href;
      return true;
    }
    if (key === 'e') {
      ev.preventDefault();
      startInlineEdit(nativeRow);
      return true;
    }
    if (key === ' ') {
      ev.preventDefault();
      openStatusPicker(nativeRow);
      return true;
    }
    if (ev.shiftKey && (key === 'arrowright' || key === 'right')) {
      ev.preventDefault();
      cycleStatus(nativeRow, +1);
      return true;
    }
    if (ev.shiftKey && (key === 'arrowleft' || key === 'left')) {
      ev.preventDefault();
      cycleStatus(nativeRow, -1);
      return true;
    }
    if (key === 'n' && !ev.shiftKey) {
      ev.preventDefault();
      openNewItemPrompt('sibling', nativeRow);
      return true;
    }
    if (key === 'n' && ev.shiftKey) {
      ev.preventDefault();
      openNewItemPrompt('child', nativeRow);
      return true;
    }
    if (key === 'p') {
      ev.preventDefault();
      if ((nativeRow.dataset.canEdit || '') !== 'true') {
        setOutlineStatus('Error: owner-only');
        setTimeout(() => setOutlineStatus(''), 1200);
        return true;
      }
      const on = (nativeRow.dataset.priority || '') !== 'true';
      nativeRowUpdatePriority(nativeRow, on);
      const root = nativeOutlineRootOrFromRow(nativeRow);
      outlineApply(root, 'outline:toggle_priority', { id: nativeRow.dataset.id }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
      return true;
    }
    if (key === 'o') {
      ev.preventDefault();
      if ((nativeRow.dataset.canEdit || '') !== 'true') {
        setOutlineStatus('Error: owner-only');
        setTimeout(() => setOutlineStatus(''), 1200);
        return true;
      }
      const on = (nativeRow.dataset.onHold || '') !== 'true';
      nativeRowUpdateOnHold(nativeRow, on);
      const root = nativeOutlineRootOrFromRow(nativeRow);
      outlineApply(root, 'outline:toggle_on_hold', { id: nativeRow.dataset.id }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
      });
      return true;
    }
    if (key === 't') {
      ev.preventDefault();
      openTagsPicker(nativeRow);
      return true;
    }
    if (key === 'm') {
      ev.preventDefault();
      openMoveOutlinePicker(nativeRow);
      return true;
    }
    if (key === 'd' && !ev.shiftKey) {
      ev.preventDefault();
      openDatePrompt(nativeRow, 'due');
      return true;
    }
    if (key === 's' && !ev.shiftKey) {
      ev.preventDefault();
      openDatePrompt(nativeRow, 'schedule');
      return true;
    }
    if (key === 'c' && ev.shiftKey) {
      ev.preventDefault();
      openTextPostPrompt(nativeRow, 'comment');
      return true;
    }
    if (key === 'w') {
      ev.preventDefault();
      openTextPostPrompt(nativeRow, 'worklog');
      return true;
    }
    if (key === 'd' && ev.shiftKey) {
      ev.preventDefault();
      openEditDescriptionPrompt(nativeRow);
      return true;
    }
    if (key === 'r') {
      ev.preventDefault();
      openArchivePrompt(nativeRow);
      return true;
    }
    if (key === 'z' && !ev.shiftKey) {
      ev.preventDefault();
      toggleCollapseRow(nativeRow);
      return true;
    }
    if (key === 'z' && ev.shiftKey) {
      ev.preventDefault();
      toggleCollapseAll(nativeOutlineRootOrFromRow(nativeRow));
      return true;
    }
    if (key === 'a') {
      ev.preventDefault();
      openAssigneePicker(nativeRow);
      return true;
    }
    // Indent/outdent (match TUI: ctrl+h/l and arrow variants; do not bind Tab/Shift+Tab).
    if (ev.ctrlKey && (key === 'l' || key === 'arrowright')) {
      ev.preventDefault();
      nativeIndent(nativeRow);
      return true;
    }
    if (ev.ctrlKey && (key === 'h' || key === 'arrowleft')) {
      ev.preventDefault();
      nativeOutdent(nativeRow);
      return true;
    }
    if (ev.altKey && (key === 'arrowright' || key === 'right')) {
      ev.preventDefault();
      nativeIndent(nativeRow);
      return true;
    }
    if (ev.altKey && (key === 'arrowleft' || key === 'left')) {
      ev.preventDefault();
      nativeOutdent(nativeRow);
      return true;
    }
    if (key === 'y' && !ev.shiftKey) {
      ev.preventDefault();
      const id = (nativeRow.dataset.id || '').trim();
      if (!id) return true;
      copyTextToClipboard(id).then(() => {
        setOutlineStatus('Copied item id');
        setTimeout(() => setOutlineStatus(''), 1200);
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed'));
      });
      return true;
    }
    if (key === 'y' && ev.shiftKey) {
      ev.preventDefault();
      const id = (nativeRow.dataset.id || '').trim();
      if (!id) return true;
      copyTextToClipboard('clarity items show ' + id).then(() => {
        setOutlineStatus('Copied command');
        setTimeout(() => setOutlineStatus(''), 1200);
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed'));
      });
      return true;
    }
    return false;
  };

  const handleGlobalListKeydown = (ev, key) => {
    if (key === 'j') {
      ev.preventDefault();
      moveFocus(+1);
      return true;
    }
    if (key === 'k') {
      ev.preventDefault();
      moveFocus(-1);
      return true;
    }
    if (key === 'enter') {
      ev.preventDefault();
      openFocused();
      return true;
    }
    return false;
  };

  const handleItemPageKeydown = (ev, key) => {
    const root = itemPageRoot();
    if (!root) return false;

    const itemId = (root.dataset.itemId || '').trim();
    if (!itemId) return false;

    if (key === 'y' && !ev.shiftKey) {
      ev.preventDefault();
      copyTextToClipboard(itemId).then(() => {
        setOutlineStatus('Copied item id');
        setTimeout(() => setOutlineStatus(''), 1200);
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed'));
      });
      return true;
    }
    if (key === 'y' && ev.shiftKey) {
      ev.preventDefault();
      copyTextToClipboard('clarity items show ' + itemId).then(() => {
        setOutlineStatus('Copied command');
        setTimeout(() => setOutlineStatus(''), 1200);
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed'));
      });
      return true;
    }

    if (key === 'e') {
      ev.preventDefault();
      openItemTitlePrompt(root);
      return true;
    }
    if (key === 'd' && ev.shiftKey) {
      ev.preventDefault();
      openItemDescriptionPrompt(root);
      return true;
    }
    if (key === ' ' || key === 'spacebar') {
      ev.preventDefault();
      openStatusPickerForItemPage(root);
      return true;
    }
    if (key === 'a') {
      ev.preventDefault();
      openAssigneePickerForItemPage(root);
      return true;
    }
    if (key === 't') {
      ev.preventDefault();
      openItemTagsPrompt(root);
      return true;
    }
    if (key === 'p') {
      ev.preventDefault();
      if ((root.dataset.canEdit || '') !== 'true') return true;
      const cb = document.querySelector('input[type="checkbox"][name="priority"]');
      if (cb) cb.checked = !cb.checked;
      outlineApply(root, 'outline:toggle_priority', { id: itemId }).catch(() => {});
      return true;
    }
    if (key === 'o') {
      ev.preventDefault();
      if ((root.dataset.canEdit || '') !== 'true') return true;
      const cb = document.querySelector('input[type="checkbox"][name="onHold"]');
      if (cb) cb.checked = !cb.checked;
      outlineApply(root, 'outline:toggle_on_hold', { id: itemId }).catch(() => {});
      return true;
    }
    if (key === 'd' && !ev.shiftKey) {
      ev.preventDefault();
      openItemDatePrompt(root, 'due');
      return true;
    }
    if (key === 's' && !ev.shiftKey) {
      ev.preventDefault();
      openItemDatePrompt(root, 'schedule');
      return true;
    }
    if (key === 'c' && ev.shiftKey) {
      ev.preventDefault();
      openItemTextPostPrompt(root, 'comment');
      return true;
    }
    if (key === 'w') {
      ev.preventDefault();
      openItemTextPostPrompt(root, 'worklog');
      return true;
    }
    return false;
  };

  const handleKeydown = (ev) => {
    if (ev.defaultPrevented) return;
    if (ev.metaKey) return;
    if (handleTagsPickerKeydown(ev)) return;
    if (handleMoveOutlinePickerKeydown(ev)) return;
    if (handlePromptKeydown(ev)) return;
    if (handleAssigneePickerKeydown(ev)) return;
    if (handleStatusPickerKeydown(ev)) return;
    if (isTypingTarget(ev.target)) return;

    const key = (ev.key || '').toLowerCase();
    const now = Date.now();
    if (state.awaiting && now-state.awaitingAt > 1000) clearAwaiting();

    if (key === '?') {
      ev.preventDefault();
      toggleHelp();
      return;
    }

    if (state.awaiting === 'g') {
      ev.preventDefault();
      clearAwaiting();
      switch (key) {
        case 'h': window.location.href = '/'; return;
        case 'p': window.location.href = '/projects'; return;
        case 'a': window.location.href = '/agenda'; return;
        case 's': window.location.href = '/sync'; return;
        default: return;
      }
    }

    if (key === 'g') {
      ev.preventDefault();
      state.awaiting = 'g';
      state.awaitingAt = now;
      return;
    }

    const inOutline = eventTouchesOutlineComponent(ev);
    if (inOutline) return;

    if (handleItemPageKeydown(ev, key)) return;

    const nativeRow = nativeRowFromEvent(ev);
    if (handleNativeOutlineKeydown(ev, key, nativeRow)) return;

    handleGlobalListKeydown(ev, key);
  };

  document.addEventListener('keydown', handleKeydown, { capture: true });
})();
