(() => {
  const state = {
    awaiting: null,
    awaitingAt: 0,
    restoreTimer: null,
  };

  const themeKey = 'clarity:theme';
  const themeDefault = 'default';

  const themeNormalize = (id) => {
    id = (id || '').trim();
    if (!id) return themeDefault;
    if (id === 'default' || id === 'alternative') return id;
    return themeDefault;
  };

  const themeCurrent = () => {
    const id = (document.documentElement?.dataset?.theme || '').trim();
    if (id) return themeNormalize(id);
    let stored = '';
    try {
      stored = localStorage.getItem(themeKey) || '';
    } catch (_) {}
    return themeNormalize(stored);
  };

  const themeSyncUI = () => {
    const sel = document.getElementById('theme-select');
    if (!sel) return;
    const cur = themeCurrent();
    if (sel.value !== cur) sel.value = cur;
  };

  const themeApply = (id) => {
    id = themeNormalize(id);
    if (document.documentElement) document.documentElement.dataset.theme = id;
    try {
      localStorage.setItem(themeKey, id);
    } catch (_) {}
    themeSyncUI();
  };

  const initTheme = () => {
    themeApply(themeCurrent());

    document.addEventListener(
      'change',
      (ev) => {
        const t = ev && ev.target;
        if (!t || t.id !== 'theme-select') return;
        themeApply(t.value);
      },
      { capture: true }
    );

    try {
      const obs = new MutationObserver(() => themeSyncUI());
      obs.observe(document.documentElement, { subtree: true, childList: true });
    } catch (_) {}
  };

  const outlinePendingByEl = new WeakMap();
  const outlineMoveBufferByEl = new WeakMap();

  const actionPalette = {
    open: false,
    options: [],
    idx: 0,
    restoreEl: null,
    mode: 'context', // context|nav|agenda|capture|sync|outline
    stack: [], // stack of modes; top = current
  };

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

  const workspaceFlag = () => {
    const el = document.getElementById('workspace-name');
    const w = String(el?.dataset?.workspace || '').trim();
    if (!w) return '';
    return ' --workspace "' + w.replaceAll('"', '\\"') + '"';
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

  const initOutlineViewMode = () => {
    const attach = (rootEl) => {
      if (!rootEl) return;
      const outlineId = (rootEl.dataset.outlineId || '').trim();
      const mode = outlineViewNormalize(outlineViewGetStored(outlineId));
      outlineViewApply(rootEl, mode);

      rootEl.addEventListener(
        'focusin',
        (ev) => {
          const v = outlineViewNormalize(rootEl.dataset.viewMode || 'list');
          if (v !== 'list+preview') return;
          const t = ev && ev.target;
          const row = t && typeof t.closest === 'function' ? t.closest('[data-outline-row]') : null;
          const id = row && row.dataset ? String(row.dataset.id || '').trim() : '';
          if (!id) return;
          refreshOutlinePreview(rootEl, id);
        },
        true
      );

      // Initial preview render (if needed).
      if (outlineViewNormalize(rootEl.dataset.viewMode || '') === 'list+preview') {
        let id = '';
        try { id = sessionStorage.getItem(outlineFocusKey(rootEl)) || ''; } catch (_) {}
        id = String(id || '').trim();
        if (id) refreshOutlinePreview(rootEl, id);
      }

      // Initial columns render (if needed) — render once per outline DOM root.
      if (outlineViewNormalize(rootEl.dataset.viewMode || '') === 'columns') {
        const pane = document.getElementById('outline-columns-pane');
        if (pane && pane.childElementCount === 0) renderOutlineColumns(rootEl);
      }
    };

    let current = nativeOutlineRoot();
    if (!current) return;
    attach(current);

    // Re-attach only when the outline root element itself is replaced (e.g. via SSE morph).
    const mo = new MutationObserver(() => {
      const fresh = nativeOutlineRoot();
      if (!fresh) return;
      if (fresh === current) return;
      current = fresh;
      attach(fresh);
    });
    try {
      mo.observe(document.body, { childList: true, subtree: true });
    } catch (_) {}
  };

  const nativeOutlineRoot = () => document.getElementById('outline-native');
  const itemPageRoot = () => document.getElementById('item-native');
  const agendaRoot = () => document.getElementById('agenda-native');

  const nativeRowFromEvent = (ev) => {
    const t = ev && ev.target;
    if (!t || typeof t.closest !== 'function') return null;
    return t.closest('[data-outline-row]');
  };

  const agendaRowFromEvent = (ev) => {
    const t = ev && ev.target;
    if (!t || typeof t.closest !== 'function') return null;
    return t.closest('[data-agenda-row]');
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

  const focusOutlineById = (id) => {
    id = String(id || '').trim();
    if (!id) return;
    const root = nativeOutlineRoot();
    if (!root) return;
    const mode = outlineViewNormalize(root.dataset.viewMode || '');
    if (mode === 'columns') {
      const pane = document.getElementById('outline-columns-pane');
      const el = pane ? pane.querySelector('[data-focus-id="' + CSS.escape(id) + '"]') : null;
      if (el && typeof el.focus === 'function') {
        el.focus();
        return;
      }
    }
    focusNativeRowById(id);
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
    ul.className = 'outline-children outline-list';
    ul.setAttribute('data-preserve-attr', 'style');
    li.appendChild(ul);
    return ul;
  };

  const randomTempID = () => {
    try {
      if (crypto && typeof crypto.randomUUID === 'function') return 'tmp-' + crypto.randomUUID();
    } catch (_) {}
    return 'tmp-' + Math.random().toString(16).slice(2) + '-' + Date.now().toString(16);
  };

  const insertOptimisticNativeItem = ({ root, mode, refRow, tempId, title }) => {
    if (!root) return null;
    const statusOpts = parseStatusOptions(root);
    const first = statusOpts && statusOpts.length ? statusOpts[0] : { id: 'todo', label: 'TODO', isEndState: false };

    const li = document.createElement('li');
    li.id = 'outline-node-' + tempId;
    li.dataset.nodeId = tempId;

    const row = document.createElement('div');
    row.className = 'outline-row';
    row.tabIndex = 0;
    row.id = 'outline-row-' + tempId;
    row.dataset.outlineRow = '';
    row.dataset.kbItem = '';
    row.dataset.focusId = tempId;
    row.dataset.id = tempId;
    row.dataset.status = (first.id || '').trim();
    row.dataset.end = first.isEndState ? 'true' : 'false';
    row.dataset.canEdit = 'true';
    row.dataset.priority = 'false';
    row.dataset.onHold = 'false';
    row.dataset.dueDate = '';
    row.dataset.dueTime = '';
    row.dataset.schDate = '';
    row.dataset.schTime = '';
    row.dataset.openHref = '';
    // Keep IDs out of the visible UI; they remain available via copy actions.

    const caret = document.createElement('span');
    caret.className = 'outline-caret outline-chevron outline-caret--none';
    caret.setAttribute('aria-hidden', 'true');
    caret.textContent = '';

    const badge = document.createElement('span');
    badge.className = 'outline-status outline-label ' + (first.isEndState ? 'outline-status--end' : 'outline-status--open');
    badge.textContent = (first.label || first.id || '(none)').trim();

    const text = document.createElement('span');
    text.className = 'outline-title outline-text';
    text.textContent = title;

    const right = document.createElement('span');
    right.className = 'outline-right';

    row.appendChild(caret);
    row.appendChild(badge);
    row.appendChild(text);
    row.appendChild(right);
    li.appendChild(row);

    if (mode === 'child') {
      const refLi = refRow ? nativeLiFromRow(refRow) : null;
      if (!refLi) return null;
      const ul = ensureChildList(refLi);
      if (!ul) return null;

      // If the parent is collapsed, expand so the new child is visible immediately.
      const set = loadCollapsedSet(root);
      const parentId = (refLi.dataset.nodeId || '').trim();
      if (parentId && set.has(parentId)) {
        set.delete(parentId);
        saveCollapsedSet(root, set);
        applyCollapsed(root, set);
      }

      ul.style.display = '';
      ul.appendChild(li);

      // Ensure the parent shows children affordances immediately.
      updateOutlineProgressForLi(refLi);
      const parentRow = refLi.querySelector(':scope > [data-outline-row]');
      const caret2 = parentRow ? parentRow.querySelector('.outline-caret') : null;
      if (caret2) {
        caret2.classList.remove('outline-caret--none');
        caret2.textContent = '▾';
      }
    } else {
      const refLi = refRow ? nativeLiFromRow(refRow) : null;
      if (!refLi || !refLi.parentElement) return null;
      refLi.parentElement.insertBefore(li, refLi.nextSibling);
    }

    row.focus();
    rememberOutlineFocus(root, tempId);
    return row;
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
    input.className = 'outline-edit-input';
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
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box native-modal-box--narrow">
        <div class="native-modal-head">
          <strong>Tags</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-tags-list" tabindex="0" class="native-modal-list" style="max-height:40vh;outline:none;"></div>
        <div class="native-modal-field-row" style="margin-top:10px;">
          <input id="native-tags-new" placeholder="Add tag (without #)" style="flex:1;">
          <button type="button" id="native-tags-add">Add</button>
        </div>
        <div class="dim native-modal-hint">Up/Down or Ctrl+P/N to move · Space to toggle (saves) · a to add · Enter to close</div>
        <div class="native-modal-actions">
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
      const root = document.getElementById('clarity-main') || document;
      const direct = root.querySelector?.('[data-focus-id="' + CSS.escape(focusId) + '"]');
      if (direct && typeof direct.focus === 'function') {
        direct.focus();
        return;
      }
      const native = nativeOutlineRoot();
      if (native) {
        focusNativeRowById(focusId);
        return;
      }
      const ar = agendaRoot();
      if (ar) {
        const row = document.querySelector('[data-agenda-row][data-id="' + CSS.escape(focusId) + '"]');
        row?.focus?.();
        return;
      }
      const ir = itemPageRoot();
      if (ir && String(ir.dataset.itemId || '').trim() === focusId) {
        const row = document.querySelector('[data-item-page]')?.querySelector?.('[data-outline-row]');
        row?.focus?.();
      }
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

  const outlineViewKey = (outlineId) => {
    const id = (outlineId || '').trim();
    if (!id) return 'clarity:outlineViewMode';
    return 'clarity:outlineViewMode:' + id;
  };

  const outlineViewNormalize = (mode) => {
    mode = String(mode || '').trim();
    if (mode === 'list' || mode === 'list+preview' || mode === 'columns') return mode;
    if (mode === 'preview' || mode === 'split' || mode === 'list-preview') return 'list+preview';
    return 'list';
  };

  const outlineViewGetStored = (outlineId) => {
    let v = '';
    try {
      v = sessionStorage.getItem(outlineViewKey(outlineId)) || '';
    } catch (_) {}
    return outlineViewNormalize(v);
  };

  const outlineViewSetStored = (outlineId, mode) => {
    const v = outlineViewNormalize(mode);
    try {
      sessionStorage.setItem(outlineViewKey(outlineId), v);
    } catch (_) {}
    return v;
  };

  const outlineViewApply = (root, mode) => {
    if (!root) return;
    const outlineId = (root.dataset.outlineId || '').trim();
    const v = outlineViewNormalize(mode);
    root.dataset.viewMode = v;

    const listPane = document.getElementById('outline-list-pane');
    const previewPane = document.getElementById('outline-preview-pane');
    const columnsPane = document.getElementById('outline-columns-pane');
    if (listPane) listPane.style.display = (v === 'columns') ? 'none' : 'block';
    if (previewPane) previewPane.style.display = (v === 'list+preview') ? 'block' : 'none';
    if (columnsPane) columnsPane.style.display = (v === 'columns') ? 'block' : 'none';

    if (v === 'list+preview') {
      const id = (() => {
        try { return sessionStorage.getItem(outlineFocusKey(root)) || ''; } catch (_) { return ''; }
      })();
      const itemId = String(id || '').trim();
      if (itemId) refreshOutlinePreview(root, itemId);
    }
    if (v === 'columns') {
      renderOutlineColumns(root);
    }

    if (outlineId) outlineViewSetStored(outlineId, v);
    setOutlineStatus('View: ' + v);
    setTimeout(() => setOutlineStatus(''), 1200);
  };

  const cycleOutlineViewMode = (root) => {
    if (!root) return false;
    const outlineId = (root.dataset.outlineId || '').trim();
    const cur = outlineViewNormalize(root.dataset.viewMode || outlineViewGetStored(outlineId));
    let next = 'list';
    if (cur === 'list') next = 'list+preview';
    else if (cur === 'list+preview') next = 'columns';
    else next = 'list';
    outlineViewApply(root, next);
    return true;
  };

  let previewTimer = null;
  let previewFor = '';
  const refreshOutlinePreview = (root, itemId) => {
    if (!root) return;
    const pane = document.getElementById('outline-preview-pane');
    if (!pane) return;
    const id = (itemId || '').trim();
    if (!id) return;
    if (previewTimer) clearTimeout(previewTimer);
    previewTimer = setTimeout(async () => {
      if (previewFor === id && pane.dataset.previewLoaded === 'true') return;
      previewFor = id;
      pane.dataset.previewLoaded = 'false';
      try {
        const res = await fetch('/items/' + encodeURIComponent(id) + '/preview', {
          method: 'GET',
          headers: { 'Accept': 'text/html' },
        });
        if (!res.ok) throw new Error(await res.text());
        pane.innerHTML = await res.text();
        pane.dataset.previewLoaded = 'true';
      } catch (err) {
        pane.innerHTML = '<div class="dim">Preview error</div>';
      }
    }, 120);
  };

  const renderOutlineColumns = (root) => {
    const pane = document.getElementById('outline-columns-pane');
    if (!root || !pane) return;
    const statusOpts = parseStatusOptions(root);
    const order = [];
    const labelByID = new Map();
    for (const o of statusOpts) {
      const id = String(o && o.id || '').trim();
      if (!id) continue;
      order.push(id);
      labelByID.set(id, String(o.label || o.id || id));
    }
    const rows = Array.from(root.querySelectorAll('[data-outline-row]'));
    const groups = new Map();
    for (const id of order) groups.set(id, []);
    for (const row of rows) {
      if (!row || !row.dataset) continue;
      const itemId = String(row.dataset.id || '').trim();
      if (!itemId) continue;
      const li = nativeLiFromRow(row);
      const parentID = parentIdForLi(li);
      if (parentID) continue; // columns: top-level only (v1)
      const statusID = String(row.dataset.status || '').trim();
      if (!groups.has(statusID)) groups.set(statusID, []);
      groups.get(statusID).push({ id: itemId, title: row.querySelector('.outline-title')?.textContent || '' });
    }

    const wrap = document.createElement('div');
    wrap.className = 'outline-columns';
    const ordered = order.length ? order : Array.from(groups.keys());
    for (const statusID of ordered) {
      const col = document.createElement('div');
      col.className = 'outline-column';
      const head = document.createElement('div');
      head.className = 'outline-column-header';
      head.textContent = labelByID.get(statusID) || statusID || '(none)';
      head.tabIndex = 0;
      head.dataset.kbItem = '';
      head.dataset.focusId = 'col-' + statusID;
      head.dataset.colStatus = statusID;
      col.appendChild(head);
      const list = document.createElement('div');
      list.className = 'outline-column-list';
      const items = groups.get(statusID) || [];
      for (const it of items) {
        const card = document.createElement('div');
        card.className = 'outline-card';
        card.tabIndex = 0;
        card.dataset.kbItem = '';
        card.dataset.focusId = it.id;
        card.dataset.itemId = it.id;
        card.dataset.colStatus = statusID;
        card.dataset.openHref = '/items/' + it.id;
        card.innerHTML = `<span class="outline-title outline-text">${escapeHTML(it.title || '(untitled)')}</span>`;
        card.addEventListener('click', () => { window.location.href = '/items/' + encodeURIComponent(it.id); });
        list.appendChild(card);
      }
      col.appendChild(list);
      wrap.appendChild(col);
    }
    pane.innerHTML = '';
    pane.appendChild(wrap);
  };

  const ensureActionModal = () => {
    let el = document.getElementById('native-action-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-action-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box">
        <div class="native-modal-head">
          <strong id="native-action-title">Actions</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-action-list" class="native-modal-list"></div>
        <div class="dim native-modal-hint">Up/Down or Ctrl+P/N to move · Enter to run</div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeActionPalette();
    });
    return el;
  };

  const renderActionPalette = () => {
    const modal = ensureActionModal();
    const titleEl = modal.querySelector('#native-action-title');
    const list = modal.querySelector('#native-action-list');
    if (!list) return;
    if (titleEl) {
      const mode = actionPalette.mode || 'context';
      titleEl.textContent =
        mode === 'nav' ? 'Go to' :
        mode === 'agenda' ? 'Agenda Commands' :
        mode === 'capture' ? 'Capture' :
        mode === 'sync' ? 'Sync' :
        mode === 'outline' ? 'Outline…' :
        'Actions';
    }
    const opts = actionPalette.options || [];
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
      if (i === actionPalette.idx) {
        li.style.background = 'color-mix(in oklab, Canvas, CanvasText 10%)';
      }
      const keyLbl = (o && o.key) ? String(o.key) : '';
      const label = (o && o.label) ? String(o.label) : '';
      li.innerHTML = `<code style="display:inline-block;min-width:2.2em;">${escapeHTML(keyLbl)}</code> ${escapeHTML(label)}`;
      li.addEventListener('click', () => {
        actionPalette.idx = i;
        runSelectedAction({ trigger: 'click' });
      });
      ul.appendChild(li);
    });
    list.appendChild(ul);
  };

  const actionPanelReset = () => {
    actionPalette.open = false;
    actionPalette.options = [];
    actionPalette.idx = 0;
    actionPalette.mode = 'context';
    actionPalette.stack = [];
  };

  const closeActionPalette = () => {
    actionPanelReset();
    const restore = actionPalette.restoreEl;
    actionPalette.restoreEl = null;
    const modal = document.getElementById('native-action-modal');
    if (modal) modal.style.display = 'none';
    setTimeout(() => {
      try {
        if (restore && typeof restore.focus === 'function') restore.focus();
      } catch (_) {}
    }, 0);
  };

  const currentProjectId = () => {
    const fromOutline = (nativeOutlineRoot()?.dataset?.projectId || '').trim();
    if (fromOutline) return fromOutline;
    const fromItem = (itemPageRoot()?.dataset?.projectId || '').trim();
    if (fromItem) return fromItem;
    const fromMain = (document.getElementById('clarity-main')?.dataset?.projectId || '').trim();
    return fromMain;
  };

  const currentOutlineId = () => {
    const fromOutline = (nativeOutlineRoot()?.dataset?.outlineId || '').trim();
    if (fromOutline) return fromOutline;
    const fromItem = (itemPageRoot()?.dataset?.outlineId || '').trim();
    return fromItem;
  };

  const currentItemId = () => {
    const fromItem = (itemPageRoot()?.dataset?.itemId || '').trim();
    return fromItem;
  };

  const currentView = () => {
    const v = (document.getElementById('clarity-main')?.dataset?.view || '').trim();
    return v || '';
  };

  const focusedKbEl = () => {
    const el = document.activeElement;
    if (!el) return null;
    try {
      if (el.getAttribute && el.getAttribute('data-kb-item') !== null) return el;
      if (el.closest) return el.closest('[data-kb-item]');
    } catch (_) {}
    return null;
  };

  const focusedProject = () => {
    const el = focusedKbEl();
    const pid = (el?.dataset?.projectId || '').trim();
    const name = (el?.dataset?.projectName || '').trim();
    return { id: pid, name };
  };

  const focusedOutline = () => {
    const el = focusedKbEl();
    const oid = (el?.dataset?.outlineId || '').trim();
    const name = (el?.dataset?.outlineName || '').trim();
    return { id: oid, name };
  };

  const openJumpToItemPrompt = () => {
    openPrompt({
      title: 'Jump to item by id…',
      hint: 'Esc to close · Enter to go',
      bodyHTML: `
        <div>
          <label class="dim" for="jump-item-id">Item id</label>
          <input id="jump-item-id" type="text" placeholder="item-…" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const id = String(document.getElementById('jump-item-id')?.value || '').trim();
        if (!id) return;
        closePrompt();
        window.location.href = '/items/' + encodeURIComponent(id);
      },
      focusSelector: '#jump-item-id',
    });
  };

  const actionsForContext = () => {
    const opts = [];
    // Root panel: global entrypoints (TUI parity).
    opts.push({ key: 'g', label: 'Go to…', kind: 'nav', next: 'nav' });
    opts.push({ key: 'W', label: 'Workspaces…', kind: 'exec', run: () => { window.location.href = '/workspaces'; } });
    opts.push({ key: 's', label: 'Sync…', kind: 'nav', next: 'sync' });
    opts.push({ key: 'a', label: 'Agenda Commands…', kind: 'nav', next: 'agenda' });
    opts.push({ key: 'c', label: 'Capture…', kind: 'exec', run: () => openCaptureModal() });

    // Context actions (best-effort parity; grows over time).
    const view = currentView();
    if (view === 'projects') {
      opts.push({ key: 'n', label: 'New project', kind: 'exec', run: () => openNewProjectPrompt() });
      opts.push({ key: 'e', label: 'Rename project', kind: 'exec', run: () => openRenameProjectPrompt() });
      opts.push({ key: 'r', label: 'Archive project', kind: 'exec', run: () => archiveFocusedProject() });
    }
    if (view === 'outlines') {
      opts.push({ key: 'n', label: 'New outline', kind: 'exec', run: () => openNewOutlinePrompt() });
      opts.push({ key: 'e', label: 'Rename outline', kind: 'exec', run: () => openRenameOutlinePrompt() });
      opts.push({ key: 'D', label: 'Edit outline description', kind: 'exec', run: () => openEditOutlineDescriptionPrompt() });
      opts.push({ key: 'O', label: 'Outline…', kind: 'nav', next: 'outline' });
      opts.push({ key: 'S', label: 'Edit outline statuses…', kind: 'exec', run: () => openOutlineStatusesEditor({ preferCurrentOutline: false }) });
      opts.push({ key: 'r', label: 'Archive outline', kind: 'exec', run: () => archiveFocusedOutline() });
    }
    if (view === 'workspaces') {
      opts.push({ key: 'n', label: 'New workspace', kind: 'exec', run: () => openNewWorkspacePrompt() });
      opts.push({ key: 'r', label: 'Rename workspace', kind: 'exec', run: () => openRenameWorkspacePrompt() });
    }

    const outlineRoot = nativeOutlineRoot();
    if (outlineRoot) {
      const focusedRow = () => {
        const a = document.activeElement;
        const row = a && typeof a.closest === 'function' ? a.closest('[data-outline-row]') : null;
        if (row) return row;
        const rows = nativeRows();
        return rows.length ? rows[0] : null;
      };
      const withRow = (fn) => () => {
        const row = focusedRow();
        if (!row) return;
        fn(row);
      };

      // Mirror TUI outline-view action panel entries (even if there are direct keys).
      opts.push({ key: 'enter', label: 'Open item', kind: 'exec', run: withRow((row) => { const href = (row.dataset.openHref || '').trim(); if (href) window.location.href = href; }) });
      opts.push({ key: 'v', label: 'Cycle view mode', kind: 'exec', run: () => cycleOutlineViewMode(outlineRoot) });
      opts.push({ key: 'O', label: 'Outline…', kind: 'nav', next: 'outline' });
      opts.push({ key: 'S', label: 'Edit outline statuses…', kind: 'exec', run: () => openOutlineStatusesEditor({ preferCurrentOutline: true }) });

      // Item mutations (TUI parity; discoverable from action panel).
      opts.push({ key: 'e', label: 'Edit title', kind: 'exec', run: withRow((row) => startInlineEdit(row)) });
      opts.push({ key: 'n', label: 'New sibling', kind: 'exec', run: withRow((row) => openNewItemPrompt('sibling', row)) });
      opts.push({ key: 'N', label: 'New child', kind: 'exec', run: withRow((row) => openNewItemPrompt('child', row)) });
      opts.push({ key: ' ', label: 'Change status', kind: 'exec', run: withRow((row) => openStatusPicker(row)) });
      opts.push({ key: 'm', label: 'Move to outline…', kind: 'exec', run: withRow((row) => openMoveOutlinePicker(row)) });

      opts.push({ key: 'z', label: 'Toggle collapse', kind: 'exec', run: withRow((row) => toggleCollapseRow(row)) });
      opts.push({ key: 'Z', label: 'Collapse/expand all', kind: 'exec', run: () => toggleCollapseAll(outlineRoot) });

      opts.push({ key: 'y', label: 'Copy item ref (includes --workspace)', kind: 'exec', run: withRow((row) => {
        const id = String(row.dataset.id || '').trim();
        if (!id) return;
        copyTextToClipboard(id + workspaceFlag()).then(() => {
          setOutlineStatus('Copied item ref');
          setTimeout(() => setOutlineStatus(''), 1200);
        }).catch((err) => setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed')));
      }) });
      opts.push({ key: 'Y', label: 'Copy CLI show command (includes --workspace)', kind: 'exec', run: withRow((row) => {
        const id = String(row.dataset.id || '').trim();
        if (!id) return;
        copyTextToClipboard('clarity items show ' + id + workspaceFlag()).then(() => {
          setOutlineStatus('Copied command');
          setTimeout(() => setOutlineStatus(''), 1200);
        }).catch((err) => setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed')));
      }) });

      opts.push({ key: 'C', label: 'Add comment', kind: 'exec', run: withRow((row) => openTextPostPrompt(row, 'comment')) });
      opts.push({ key: 'w', label: 'Add worklog', kind: 'exec', run: withRow((row) => openTextPostPrompt(row, 'worklog')) });
      opts.push({ key: 'p', label: 'Toggle priority', kind: 'exec', run: withRow((row) => {
        // Reuse the same codepath as the direct keybinding (optimistic + async persist).
        const e = new KeyboardEvent('keydown', { key: 'p' });
        // If the direct handler changes behavior, keep action panel consistent by reusing it.
        handleNativeOutlineKeydown(e, 'p', row);
      }) });
      opts.push({ key: 'o', label: 'Toggle on hold', kind: 'exec', run: withRow((row) => {
        const e = new KeyboardEvent('keydown', { key: 'o' });
        handleNativeOutlineKeydown(e, 'o', row);
      }) });
      // In action panel, use `u` like the TUI (avoid shadowing global `a: agenda`).
      opts.push({ key: 'u', label: 'Assign…', kind: 'exec', run: withRow((row) => openAssigneePicker(row)) });
      opts.push({ key: 't', label: 'Tags…', kind: 'exec', run: withRow((row) => openTagsPicker(row)) });
      opts.push({ key: 'd', label: 'Set due', kind: 'exec', run: withRow((row) => openDatePrompt(row, 'due')) });
      opts.push({ key: 's', label: 'Set schedule', kind: 'exec', run: withRow((row) => openDatePrompt(row, 'schedule')) });
      opts.push({ key: 'D', label: 'Edit description', kind: 'exec', run: withRow((row) => openEditDescriptionPrompt(row)) });
      opts.push({ key: 'r', label: 'Archive item', kind: 'exec', run: withRow((row) => openArchivePrompt(row)) });
    }

    const itemRoot = itemPageRoot();
    if (itemRoot) {
      const itemId = String(itemRoot.dataset.itemId || '').trim();
      if (itemId) {
        opts.push({ key: 'e', label: 'Edit title', kind: 'exec', run: () => openItemTitlePrompt(itemRoot) });
        opts.push({ key: 'D', label: 'Edit description', kind: 'exec', run: () => openItemDescriptionPrompt(itemRoot) });
        opts.push({ key: 'p', label: 'Toggle priority', kind: 'exec', run: () => handleItemPageKeydown(new KeyboardEvent('keydown', { key: 'p' }), 'p') });
        opts.push({ key: 'o', label: 'Toggle on hold', kind: 'exec', run: () => handleItemPageKeydown(new KeyboardEvent('keydown', { key: 'o' }), 'o') });
        opts.push({ key: 'u', label: 'Assign…', kind: 'exec', run: () => openAssigneePickerForItemPage(itemRoot) });
        opts.push({ key: 't', label: 'Tags…', kind: 'exec', run: () => openItemTagsPrompt(itemRoot) });
        opts.push({ key: 'd', label: 'Set due', kind: 'exec', run: () => openItemDatePrompt(itemRoot, 'due') });
        opts.push({ key: 's', label: 'Set schedule', kind: 'exec', run: () => openItemDatePrompt(itemRoot, 'schedule') });
        opts.push({ key: ' ', label: 'Change status', kind: 'exec', run: () => openStatusPickerForItemPage(itemRoot) });
        opts.push({ key: 'C', label: 'Add comment', kind: 'exec', run: () => openItemTextPostPrompt(itemRoot, 'comment') });
        opts.push({ key: 'w', label: 'Add worklog', kind: 'exec', run: () => openItemTextPostPrompt(itemRoot, 'worklog') });
        opts.push({ key: 'y', label: 'Copy item ref (includes --workspace)', kind: 'exec', run: () => {
          copyTextToClipboard(itemId + workspaceFlag()).then(() => {
            setOutlineStatus('Copied item ref');
            setTimeout(() => setOutlineStatus(''), 1200);
          }).catch((err) => setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed')));
        } });
        opts.push({ key: 'Y', label: 'Copy CLI show command (includes --workspace)', kind: 'exec', run: () => {
          copyTextToClipboard('clarity items show ' + itemId + workspaceFlag()).then(() => {
            setOutlineStatus('Copied command');
            setTimeout(() => setOutlineStatus(''), 1200);
          }).catch((err) => setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed')));
        } });
        opts.push({ key: 'm', label: 'Move to outline…', kind: 'exec', run: () => openMoveOutlinePickerForItemPage(itemRoot) });
        opts.push({ key: 'r', label: 'Archive item', kind: 'exec', run: () => handleItemPageKeydown(new KeyboardEvent('keydown', { key: 'r' }), 'r') });
      }
    }
    return opts;
  };

  const actionsForNav = () => {
    const opts = [];
    opts.push({ key: 'p', label: 'Projects', kind: 'exec', run: () => { window.location.href = '/projects'; } });
    opts.push({ key: 's', label: 'Sync…', kind: 'nav', next: 'sync' });
    opts.push({ key: 'W', label: 'Workspaces…', kind: 'exec', run: () => { window.location.href = '/workspaces'; } });
    opts.push({ key: 'j', label: 'Jump to item by id…', kind: 'exec', run: () => openJumpToItemPrompt() });
    opts.push({ key: 'A', label: 'Archived', kind: 'exec', run: () => { window.location.href = '/archived'; } });

    const pid = currentProjectId();
    if (pid) {
      opts.push({ key: 'o', label: 'Outlines (current project)', kind: 'exec', run: () => { window.location.href = '/projects/' + encodeURIComponent(pid); } });
    }

    const oid = currentOutlineId();
    if (oid) {
      opts.push({ key: 'l', label: 'Outline (current)', kind: 'exec', run: () => { window.location.href = '/outlines/' + encodeURIComponent(oid); } });
    }

    const iid = currentItemId();
    if (iid) {
      opts.push({ key: 'i', label: 'Item (open)', kind: 'exec', run: () => { window.location.href = '/items/' + encodeURIComponent(iid); } });
    }

    // Recent items (loaded from server, like TUI's RecentItemIDs).
    if (Array.isArray(navOptions.recent) && navOptions.recent.length) {
      for (let i = 0; i < navOptions.recent.length && i < 5; i++) {
        const it = navOptions.recent[i] || {};
        const id = String(it.id || '').trim();
        if (!id) continue;
        const title = String(it.title || '').trim() || '(untitled)';
        const key = String(i + 1);
        opts.push({ key, label: 'Recent: ' + title, kind: 'exec', run: () => { window.location.href = '/items/' + encodeURIComponent(id); } });
      }
    }
    return opts;
  };

  const navOptions = { recent: [] };

  const loadNavOptions = async () => {
    const res = await fetch('/nav/options', { method: 'GET', headers: { 'Accept': 'application/json' } });
    if (!res.ok) throw new Error(await res.text());
    return await res.json();
  };

  const refreshNavOptionsIfOpen = () => {
    if (!actionPalette.open || actionPalette.mode !== 'nav') return;
    const prevKey = String((actionPalette.options || [])[actionPalette.idx]?.key || '');
    setActionPanelMode('nav');
    if (prevKey) {
      const idx = (actionPalette.options || []).findIndex((o) => String(o?.key || '') === prevKey);
      if (idx >= 0) actionPalette.idx = idx;
      renderActionPalette();
    }
  };

  const actionsForAgenda = () => ([
    { key: 't', label: 'List all TODO entries', kind: 'exec', run: () => { window.location.href = '/agenda'; } },
  ]);

  const actionsForSync = () => ([
    { key: 's', label: 'Refresh status', kind: 'exec', run: () => { window.location.href = '/sync'; } },
    { key: 'g', label: 'Setup Git…', kind: 'exec', run: () => openSyncSetupGitPrompt() },
    { key: 'p', label: 'Pull --rebase', kind: 'exec', run: () => submitPost('/sync/pull', {}) },
    { key: 'P', label: 'Commit + pull + push', kind: 'exec', run: () => submitPost('/sync/push', {}) },
    { key: 'r', label: 'Resolve conflicts (help)', kind: 'exec', run: () => openSyncResolveHelp() },
  ]);

  const actionsForOutline = () => {
    const view = currentView();
    const oid = (view === 'outlines' ? focusedOutline().id : currentOutlineId());
    if (!oid) return [];
    return [
      { key: 'e', label: 'Rename outline', kind: 'exec', run: () => openRenameOutlinePrompt({ preferCurrentOutline: view !== 'outlines' }) },
      { key: 'D', label: 'Edit outline description', kind: 'exec', run: () => openEditOutlineDescriptionPrompt({ preferCurrentOutline: view !== 'outlines' }) },
    ];
  };

  const submitPost = (path, fields) => {
    const form = document.createElement('form');
    form.method = 'post';
    form.action = path;
    const fs = fields && typeof fields === 'object' ? fields : {};
    for (const [k, v] of Object.entries(fs)) {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = k;
      input.value = String(v ?? '');
      form.appendChild(input);
    }
    document.body.appendChild(form);
    form.submit();
  };

  const openNotImplemented = (label) => {
    openPrompt({
      title: label || 'Not implemented',
      hint: 'Esc to close',
      bodyHTML: `<div class="dim">Not implemented in the web UI yet. Use the TUI for now.</div>`,
      onSubmit: () => {},
    });
  };

  const openNewProjectPrompt = () => {
    openPrompt({
      title: 'New project',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="new-project-name">Project name</label>
          <input id="new-project-name" type="text" placeholder="Name" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('new-project-name')?.value || '').trim();
        if (!name) return;
        closePrompt();
        submitPost('/projects', { name });
      },
      focusSelector: '#new-project-name',
    });
  };

  const focusedWorkspace = () => {
    const a = document.activeElement;
    const name = a && a.dataset ? String(a.dataset.focusId || '').trim() : '';
    return { name };
  };

  const openNewWorkspacePrompt = () => {
    openPrompt({
      title: 'New workspace',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="new-workspace-name">Workspace name</label>
          <input id="new-workspace-name" type="text" placeholder="Name" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('new-workspace-name')?.value || '').trim();
        if (!name) return;
        closePrompt();
        submitPost('/workspaces/new', { name });
      },
      focusSelector: '#new-workspace-name',
    });
  };

  const openRenameWorkspacePrompt = () => {
    const fw = focusedWorkspace();
    if (!fw.name) return;
    openPrompt({
      title: 'Rename workspace',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="rename-workspace-name">Workspace name</label>
          <input id="rename-workspace-name" type="text" value="${escapeAttr(fw.name)}" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const to = String(document.getElementById('rename-workspace-name')?.value || '').trim();
        if (!to) return;
        closePrompt();
        submitPost('/workspaces/rename', { from: fw.name, to });
      },
      focusSelector: '#rename-workspace-name',
    });
  };

  const openRenameProjectPrompt = () => {
    const fp = focusedProject();
    if (!fp.id) return;
    openPrompt({
      title: 'Rename project',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="rename-project-name">Project name</label>
          <input id="rename-project-name" type="text" value="${escapeHTML(fp.name)}" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('rename-project-name')?.value || '').trim();
        if (!name) return;
        closePrompt();
        submitPost('/projects/' + encodeURIComponent(fp.id) + '/rename', { name });
      },
      focusSelector: '#rename-project-name',
    });
  };

  const archiveFocusedProject = () => {
    const fp = focusedProject();
    if (!fp.id) return;
    submitPost('/projects/' + encodeURIComponent(fp.id) + '/archive', {});
  };

  const openNewOutlinePrompt = () => {
    const pid = currentProjectId();
    if (!pid) return;
    openPrompt({
      title: 'New outline',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="new-outline-name">Outline name (optional)</label>
          <input id="new-outline-name" type="text" placeholder="Name" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('new-outline-name')?.value || '').trim();
        closePrompt();
        submitPost('/projects/' + encodeURIComponent(pid) + '/outlines', { name });
      },
      focusSelector: '#new-outline-name',
    });
  };

  const openRenameOutlinePrompt = (opts) => {
    const preferCurrent = !!(opts && opts.preferCurrentOutline);
    const o = preferCurrent ? { id: currentOutlineId(), name: String(nativeOutlineRoot()?.dataset?.outlineName || '').trim() } : focusedOutline();
    if (!o.id) return;
    openPrompt({
      title: 'Rename outline',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="rename-outline-name">Outline name (optional)</label>
          <input id="rename-outline-name" type="text" value="${escapeHTML(o.name)}" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('rename-outline-name')?.value || '').trim();
        closePrompt();
        submitPost('/outlines/' + encodeURIComponent(o.id) + '/rename', { name });
      },
      focusSelector: '#rename-outline-name',
    });
  };

  const openEditOutlineDescriptionPrompt = (opts) => {
    const preferCurrent = !!(opts && opts.preferCurrentOutline);
    const oid = preferCurrent ? currentOutlineId() : focusedOutline().id;
    if (!oid) return;
    let curDesc = '';
    if (preferCurrent) {
      curDesc = String(nativeOutlineRoot()?.dataset?.outlineDescription || '').trim();
    } else {
      const el = focusedKbEl();
      curDesc = String(el?.dataset?.outlineDescription || '').trim();
    }
    openPrompt({
      title: 'Edit outline description',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="outline-desc">Markdown outline description…</label>
          <textarea id="outline-desc" rows="10" style="width:100%;">${escapeHTML(curDesc)}</textarea>
        </div>
      `,
      onSubmit: () => {
        const description = String(document.getElementById('outline-desc')?.value || '').trim();
        closePrompt();
        submitPost('/outlines/' + encodeURIComponent(oid) + '/description', { description });
      },
      focusSelector: '#outline-desc',
    });
  };

  const archiveFocusedOutline = () => {
    const fo = focusedOutline();
    if (!fo.id) return;
    submitPost('/outlines/' + encodeURIComponent(fo.id) + '/archive', {});
  };

  const outlineStatuses = {
    open: false,
    outlineId: '',
    options: [],
    idx: 0,
    restoreEl: null,
  };

  const ensureOutlineStatusesModal = () => {
    let el = document.getElementById('native-outline-statuses-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-outline-statuses-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box">
        <div class="native-modal-head">
          <strong>Outline statuses</strong>
          <span class="dim" style="font-size:12px;">a:add r:rename e:end n:note d:delete ctrl+j/k:move esc:close</span>
        </div>
        <div id="native-outline-statuses-msg" class="dim" style="margin-top:8px;"></div>
        <div id="native-outline-statuses-list" class="native-modal-list" style="max-height:55vh;"></div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeOutlineStatusesEditor();
    });
    return el;
  };

  const renderOutlineStatusesEditor = () => {
    const modal = ensureOutlineStatusesModal();
    const list = modal.querySelector('#native-outline-statuses-list');
    const msg = modal.querySelector('#native-outline-statuses-msg');
    if (msg) msg.textContent = '';
    const opts = Array.isArray(outlineStatuses.options) ? outlineStatuses.options : [];
    if (!list) return;
    if (opts.length === 0) {
      list.innerHTML = `<div class="dim">(none)</div>`;
      return;
    }
    const idx = Math.max(0, Math.min(opts.length - 1, outlineStatuses.idx || 0));
    outlineStatuses.idx = idx;
    list.innerHTML = opts.map((o, i) => {
      const label = String(o?.label || o?.id || '').trim();
      const id = String(o?.id || '').trim();
      const end = !!o?.isEndState;
      const note = !!o?.requiresNote;
      const active = i === idx;
      const flags = (end ? ' end' : '') + (note ? ' note' : '');
      return `
        <div style="display:flex;gap:10px;align-items:baseline;padding:6px 8px;border-radius:8px;${active ? 'background:rgba(127,127,127,.18);' : ''}">
          <span style="flex:0 0 auto;"><code>${escapeHTML(id)}</code></span>
          <span style="flex:1;min-width:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${escapeHTML(label)}</span>
          <span class="dim" style="flex:0 0 auto;">${escapeHTML(flags.trim())}</span>
        </div>
      `;
    }).join('');
  };

  const closeOutlineStatusesEditor = () => {
    outlineStatuses.open = false;
    outlineStatuses.outlineId = '';
    outlineStatuses.options = [];
    outlineStatuses.idx = 0;
    const restore = outlineStatuses.restoreEl;
    outlineStatuses.restoreEl = null;
    const modal = document.getElementById('native-outline-statuses-modal');
    if (modal) modal.style.display = 'none';
    setTimeout(() => {
      try { restore?.focus?.(); } catch (_) {}
    }, 0);
  };

  const loadOutlineMeta = async (outlineId) => {
    const res = await fetch('/outlines/' + encodeURIComponent(outlineId) + '/meta', { method: 'GET', headers: { 'Accept': 'application/json' } });
    if (!res.ok) throw new Error(await res.text());
    return await res.json();
  };

  const postJSON = async (path, payload) => {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify(payload || {}),
    });
    if (!res.ok) throw new Error(await res.text());
    return await res.json().catch(() => ({}));
  };

  const openOutlineStatusesEditor = (opts) => {
    if (outlineStatuses.open) return;
    const preferCurrent = !!(opts && opts.preferCurrentOutline);
    const outlineId = preferCurrent ? currentOutlineId() : focusedOutline().id;
    if (!outlineId) return;
    outlineStatuses.open = true;
    outlineStatuses.outlineId = outlineId;
    outlineStatuses.restoreEl = document.activeElement;
    outlineStatuses.options = [];
    outlineStatuses.idx = 0;
    const modal = ensureOutlineStatusesModal();
    modal.style.display = 'flex';
    renderOutlineStatusesEditor();

    loadOutlineMeta(outlineId).then((m) => {
      outlineStatuses.options = Array.isArray(m?.statusOptions) ? m.statusOptions : [];
      outlineStatuses.idx = 0;
      renderOutlineStatusesEditor();
    }).catch((err) => {
      const msg = modal.querySelector('#native-outline-statuses-msg');
      if (msg) msg.textContent = 'Error: ' + (err && err.message ? err.message : 'load failed');
    });
  };

  const outlineStatusesSelected = () => {
    const opts = Array.isArray(outlineStatuses.options) ? outlineStatuses.options : [];
    if (opts.length === 0) return null;
    const idx = Math.max(0, Math.min(opts.length - 1, outlineStatuses.idx || 0));
    return opts[idx] || null;
  };

  const outlineStatusesRefresh = () => {
    const outlineId = String(outlineStatuses.outlineId || '').trim();
    if (!outlineId) return Promise.resolve();
    return loadOutlineMeta(outlineId).then((m) => {
      outlineStatuses.options = Array.isArray(m?.statusOptions) ? m.statusOptions : [];
      outlineStatuses.idx = Math.max(0, Math.min((outlineStatuses.options.length || 1) - 1, outlineStatuses.idx || 0));
      renderOutlineStatusesEditor();
    });
  };

  const outlineStatusesSetMsg = (text) => {
    const modal = document.getElementById('native-outline-statuses-modal');
    const msg = modal ? modal.querySelector('#native-outline-statuses-msg') : null;
    if (msg) msg.textContent = String(text || '');
  };

  const outlineStatusesAdd = () => {
    openPrompt({
      title: 'Add status',
      hint: 'Esc to close · Ctrl+Enter to add',
      bodyHTML: `
        <div>
          <label class="dim" for="add-status-label">Status label</label>
          <input id="add-status-label" type="text" placeholder="Label" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const label = String(document.getElementById('add-status-label')?.value || '').trim();
        if (!label) return;
        closePrompt();
        const outlineId = String(outlineStatuses.outlineId || '').trim();
        postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/add', { label }).then(() => outlineStatusesRefresh()).catch((err) => {
          outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'add failed'));
        });
      },
      focusSelector: '#add-status-label',
    });
  };

  const outlineStatusesRename = () => {
    const cur = outlineStatusesSelected();
    if (!cur) return;
    const curLabel = String(cur.label || '').trim();
    openPrompt({
      title: 'Rename status',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div>
          <label class="dim" for="rename-status-label">Status label</label>
          <input id="rename-status-label" type="text" value="${escapeHTML(curLabel)}" style="width:100%;" />
        </div>
      `,
      onSubmit: () => {
        const label = String(document.getElementById('rename-status-label')?.value || '').trim();
        if (!label) return;
        closePrompt();
        const outlineId = String(outlineStatuses.outlineId || '').trim();
        postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/update', { id: cur.id, label }).then(() => outlineStatusesRefresh()).catch((err) => {
          outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'rename failed'));
        });
      },
      focusSelector: '#rename-status-label',
    });
  };

  const outlineStatusesToggleEnd = () => {
    const cur = outlineStatusesSelected();
    if (!cur) return;
    const outlineId = String(outlineStatuses.outlineId || '').trim();
    postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/update', { id: cur.id, isEndState: !cur.isEndState }).then(() => outlineStatusesRefresh()).catch((err) => {
      outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'update failed'));
    });
  };

  const outlineStatusesToggleNote = () => {
    const cur = outlineStatusesSelected();
    if (!cur) return;
    const outlineId = String(outlineStatuses.outlineId || '').trim();
    postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/update', { id: cur.id, requiresNote: !cur.requiresNote }).then(() => outlineStatusesRefresh()).catch((err) => {
      outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'update failed'));
    });
  };

  const outlineStatusesDelete = () => {
    const cur = outlineStatusesSelected();
    if (!cur) return;
    const outlineId = String(outlineStatuses.outlineId || '').trim();
    postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/remove', { id: cur.id }).then(() => outlineStatusesRefresh()).catch((err) => {
      outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'delete failed'));
    });
  };

  const outlineStatusesMove = (delta) => {
    const opts = Array.isArray(outlineStatuses.options) ? outlineStatuses.options : [];
    const idx = outlineStatuses.idx || 0;
    const j = idx + delta;
    if (idx < 0 || j < 0 || idx >= opts.length || j >= opts.length) return;
    const swapped = opts.slice();
    const tmp = swapped[idx];
    swapped[idx] = swapped[j];
    swapped[j] = tmp;
    outlineStatuses.options = swapped;
    outlineStatuses.idx = j;
    renderOutlineStatusesEditor();

    const labels = swapped.map((x) => String(x?.label || '').trim()).filter((x) => x);
    const outlineId = String(outlineStatuses.outlineId || '').trim();
    postJSON('/outlines/' + encodeURIComponent(outlineId) + '/statuses/reorder', { labels }).catch((err) => {
      outlineStatusesSetMsg('Error: ' + (err && err.message ? err.message : 'reorder failed'));
    });
  };

  const openSyncSetupGitPrompt = () => {
    openPrompt({
      title: 'Setup Git…',
      hint: 'Esc to close · Ctrl+Enter to save',
      bodyHTML: `
        <div style="display:flex;gap:10px;align-items:flex-end;flex-wrap:wrap;">
          <div style="flex:0 0 140px;">
            <label class="dim" for="sync-remote-name">Remote name</label>
            <input id="sync-remote-name" type="text" value="origin" />
          </div>
          <div style="flex:1;min-width:280px;">
            <label class="dim" for="sync-remote-url">Remote url</label>
            <input id="sync-remote-url" type="text" placeholder="git@github.com:org/repo.git" style="width:100%;" />
          </div>
        </div>
        <div class="dim" style="margin-top:10px;">Tip: you can also use the Sync screen for full setup.</div>
      `,
      onSubmit: () => {
        const name = String(document.getElementById('sync-remote-name')?.value || '').trim() || 'origin';
        const url = String(document.getElementById('sync-remote-url')?.value || '').trim();
        if (!url) return;
        closePrompt();
        submitPost('/sync/remote', { remoteName: name, remoteUrl: url });
      },
      focusSelector: '#sync-remote-url',
    });
  };

  const openSyncResolveHelp = () => {
    openPrompt({
      title: 'Resolve conflicts (help)',
      hint: 'Esc to close',
      bodyHTML: `
        <div class="dim">
          If the repo is unmerged or a rebase is in progress:
          <ul>
            <li>Run <code>git status</code> and resolve conflicts</li>
            <li>Then <code>git add</code> + <code>git rebase --continue</code> (or merge continue)</li>
            <li>Finally, return to <code>/sync</code> and run <em>Commit + pull + push</em></li>
          </ul>
          You can also run <code>clarity doctor --fail</code> to detect multi-head entities.
        </div>
      `,
      onSubmit: () => {},
    });
  };

  const captureState = {
    open: false,
    restoreEl: null,
    phase: 'select', // select|draft|templates|templates-add|templates-delete
    outlines: [],
    templates: [],
    statusByOutline: {},
    tree: null,
    prefix: [],
    list: [],
    idx: 0,
    templatesIdx: 0,
    templatesDeleteKeyPath: '',
    selectedTemplate: null,
    draftOutlineId: '',
    draftStatusId: '',
    draftTitle: '',
    draftDesc: '',
    tmplName: '',
    tmplKeyPath: '',
    tmplOutlineId: '',
    err: '',
  };

  const ensureCaptureModal = () => {
    let el = document.getElementById('native-capture-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-capture-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box">
        <div class="native-modal-head">
          <strong>Capture</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-capture-hint" class="dim" style="margin-top:8px;font-size:12px;"></div>
        <div id="native-capture-body" class="native-modal-body"></div>
        <div id="native-capture-err" class="dim" style="margin-top:10px;display:none;"></div>
        <div class="native-modal-actions">
          <button type="button" id="native-capture-cancel">Cancel</button>
          <button type="button" id="native-capture-ok">OK</button>
        </div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeCaptureModal();
    });
    el.querySelector('#native-capture-cancel')?.addEventListener('click', () => closeCaptureModal());
    el.querySelector('#native-capture-ok')?.addEventListener('click', () => submitCapture());
    return el;
  };

  const closeCaptureModal = () => {
    captureState.open = false;
    const restore = captureState.restoreEl;
    captureState.restoreEl = null;
    const modal = document.getElementById('native-capture-modal');
    if (modal) modal.style.display = 'none';
    setTimeout(() => {
      try { restore?.focus?.(); } catch (_) {}
    }, 0);
  };

  const setCaptureError = (msg) => {
    const el = document.getElementById('native-capture-err');
    if (!el) return;
    const m = String(msg || '').trim();
    captureState.err = m;
    if (!m) {
      el.style.display = 'none';
      el.textContent = '';
      return;
    }
    el.style.display = 'block';
    el.textContent = m;
  };

  const loadCaptureOptions = async () => {
    const res = await fetch('/capture/options', { method: 'GET', headers: { 'Accept': 'application/json' } });
    if (!res.ok) throw new Error(await res.text());
    return await res.json();
  };

  const captureOutlineLabelByID = () => {
    const m = new Map();
    for (const o of (captureState.outlines || [])) {
      const id = String(o?.id || '').trim();
      const label = String(o?.label || '').trim();
      if (id) m.set(id, label || id);
    }
    return m;
  };

  const buildCaptureTree = (templates) => {
    const root = { children: {}, template: null };
    for (const t0 of (Array.isArray(templates) ? templates : [])) {
      const keys = Array.isArray(t0?.keys) ? t0.keys.map((k) => String(k || '').trim()).filter(Boolean) : [];
      if (!keys.length) continue;
      const name = String(t0?.name || '').trim();
      const outlineId = String(t0?.outlineId || '').trim();
      if (!name || !outlineId) continue;
      let cur = root;
      for (const k of keys) {
        if (!cur.children) cur.children = {};
        if (!cur.children[k]) cur.children[k] = { children: {}, template: null };
        cur = cur.children[k];
      }
      cur.template = { name, keys, keyPath: String(t0?.keyPath || keys.join('')), outlineId };
    }
    return root;
  };

  const captureNodeAtPrefix = () => {
    let cur = captureState.tree;
    for (const k of (captureState.prefix || [])) {
      if (!cur || !cur.children || !cur.children[k]) return null;
      cur = cur.children[k];
    }
    return cur;
  };

  const captureRefreshList = () => {
    const node = captureNodeAtPrefix();
    const opts = [];
    // Always provide a manual capture option (works even with zero templates).
    opts.push({ kind: 'manual', key: '', label: 'Manual… (choose outline)' });
    if (node && node.template) {
      opts.push({ kind: 'select', key: 'Enter', label: 'Use template: ' + node.template.name, template: node.template });
    }
    const children = node && node.children ? node.children : {};
    const keys = Object.keys(children || {}).sort((a, b) => a.localeCompare(b));
    for (const k of keys) {
      const child = children[k];
      const leafName = child && child.template && (!child.children || Object.keys(child.children).length === 0) ? child.template.name : '';
      const label = leafName ? (k + '  ' + leafName) : (k + '  …');
      opts.push({ kind: 'prefix', key: k, label, nextKey: k });
    }
    captureState.list = opts;
    captureState.idx = Math.max(0, Math.min((captureState.idx || 0), Math.max(0, opts.length - 1)));
  };

  const captureStartDraft = (tmpl) => {
    captureState.selectedTemplate = tmpl;
    captureState.phase = 'draft';
    let outID = String(tmpl?.outlineId || '').trim();
    if (!outID) outID = String((captureState.outlines || [])[0]?.id || '').trim();
    captureState.draftOutlineId = outID;
    const sts = captureState.statusByOutline && captureState.draftOutlineId ? (captureState.statusByOutline[captureState.draftOutlineId] || []) : [];
    captureState.draftStatusId = String(sts[0]?.id || '').trim();
    captureState.draftTitle = '';
    captureState.draftDesc = '';
    renderCapture();
  };

  const captureSelectCurrent = () => {
    const cur = (captureState.list || [])[captureState.idx] || null;
    if (!cur) return;
    if (cur.kind === 'manual') {
      captureStartDraft(null);
      return;
    }
    if (cur.kind === 'select') {
      captureStartDraft(cur.template);
      return;
    }
    if (cur.kind === 'prefix' && cur.nextKey) {
      captureState.prefix = [...(captureState.prefix || []), String(cur.nextKey)];
      captureState.idx = 0;
      renderCapture();
      const node = captureNodeAtPrefix();
      if (node && node.template && (!node.children || Object.keys(node.children).length === 0)) {
        captureStartDraft(node.template);
      }
    }
  };

  const captureOpenTemplates = () => {
    captureState.phase = 'templates';
    captureState.templatesIdx = 0;
    captureState.templatesDeleteKeyPath = '';
    renderCapture();
  };

  const captureStartAddTemplate = () => {
    captureState.phase = 'templates-add';
    captureState.tmplName = '';
    captureState.tmplKeyPath = '';
    captureState.tmplOutlineId = String((captureState.outlines || [])[0]?.id || '').trim();
    renderCapture();
  };

  const captureConfirmDeleteTemplate = (keyPath) => {
    captureState.phase = 'templates-delete';
    captureState.templatesDeleteKeyPath = String(keyPath || '').trim();
    renderCapture();
  };

  const submitCaptureTemplateUpsert = async () => {
    const modal = ensureCaptureModal();
    const body = modal.querySelector('#native-capture-body');
    const name = String(body?.querySelector('#native-capture-tmpl-name')?.value || captureState.tmplName || '').trim();
    const keyPath = String(body?.querySelector('#native-capture-tmpl-keys')?.value || captureState.tmplKeyPath || '').trim();
    const outlineId = String(body?.querySelector('#native-capture-tmpl-outline')?.value || captureState.tmplOutlineId || '').trim();
    if (!name || !keyPath || !outlineId) {
      setCaptureError('Error: missing name, keys, or outline');
      return;
    }
    const res = await fetch('/capture/templates', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
      body: JSON.stringify({ name, keyPath, outlineId }),
    });
    if (!res.ok) throw new Error(await res.text());
    const data = await loadCaptureOptions();
    captureState.outlines = Array.isArray(data?.outlines) ? data.outlines : [];
    captureState.templates = Array.isArray(data?.templates) ? data.templates : [];
    captureState.statusByOutline = (data && typeof data.statusOptionsByOutline === 'object' && data.statusOptionsByOutline) ? data.statusOptionsByOutline : {};
    captureState.tree = buildCaptureTree(captureState.templates);
    setCaptureError('');
    captureState.phase = 'templates';
    renderCapture();
  };

  const submitCaptureTemplateDelete = async () => {
    const keyPath = String(captureState.templatesDeleteKeyPath || '').trim();
    if (!keyPath) {
      captureState.phase = 'templates';
      renderCapture();
      return;
    }
    const res = await fetch('/capture/templates/' + encodeURIComponent(keyPath), {
      method: 'DELETE',
      headers: { 'Accept': 'application/json' },
    });
    if (!res.ok) throw new Error(await res.text());
    const data = await loadCaptureOptions();
    captureState.outlines = Array.isArray(data?.outlines) ? data.outlines : [];
    captureState.templates = Array.isArray(data?.templates) ? data.templates : [];
    captureState.statusByOutline = (data && typeof data.statusOptionsByOutline === 'object' && data.statusOptionsByOutline) ? data.statusOptionsByOutline : {};
    captureState.tree = buildCaptureTree(captureState.templates);
    captureState.phase = 'templates';
    captureState.templatesDeleteKeyPath = '';
    setCaptureError('');
    renderCapture();
  };

  const renderCapture = () => {
    const modal = ensureCaptureModal();
    const hint = modal.querySelector('#native-capture-hint');
    const body = modal.querySelector('#native-capture-body');
    const okBtn = modal.querySelector('#native-capture-ok');
    if (!hint || !body || !okBtn) return;

    const prefix = (captureState.prefix || []).join('');
    if (captureState.phase === 'templates') {
      hint.textContent = 'Capture templates (current workspace) · a add · d delete · Enter back · Ctrl+T templates';
      okBtn.textContent = 'Back';
      const rows = Array.isArray(captureState.templates) ? captureState.templates : [];
      const outlineLabels = captureOutlineLabelByID();
      const idx = Math.max(0, Math.min(rows.length - 1, captureState.templatesIdx || 0));
      captureState.templatesIdx = idx;
      body.innerHTML = `
        <div class="dim" style="margin-bottom:8px;">Templates</div>
        <ul class="kb-list" id="native-capture-templates-list" style="list-style:none;padding:0;margin:0;"></ul>
        <div class="dim native-modal-hint">Tip: configure more templates per-workspace via <code>Ctrl+T</code> in the TUI, or here.</div>
      `;
      const ul = body.querySelector('#native-capture-templates-list');
      if (ul) {
        if (!rows.length) {
          const li = document.createElement('li');
          li.className = 'dim';
          li.textContent = '(none) — press a to add';
          ul.appendChild(li);
        } else {
          rows.forEach((t, i) => {
            const keyPath = String(t?.keyPath || '').trim();
            const name = String(t?.name || '').trim();
            const outID = String(t?.outlineId || '').trim();
            const dst = outlineLabels.get(outID) || outID;
            const li = document.createElement('li');
            li.className = 'list-row';
            li.tabIndex = -1;
            li.style.cursor = 'pointer';
            if (i === idx) li.style.background = 'color-mix(in oklab, var(--bg), var(--fg) 8%)';
            li.innerHTML = `<span style="min-width:80px;display:inline-block;"><code>${escapeHTML(keyPath)}</code></span><span>${escapeHTML(name || '(unnamed)')}</span><span class="dim" style="margin-left:10px;">→ ${escapeHTML(dst || '-')}</span>`;
            li.addEventListener('click', () => { captureState.templatesIdx = i; renderCapture(); });
            ul.appendChild(li);
          });
        }
      }
      return;
    }
    if (captureState.phase === 'templates-delete') {
      hint.textContent = 'Delete template · Enter confirm · Esc back';
      okBtn.textContent = 'Delete';
      const keyPath = String(captureState.templatesDeleteKeyPath || '').trim();
      body.innerHTML = `<div>Delete capture template <code>${escapeHTML(keyPath)}</code>?</div>`;
      return;
    }
    if (captureState.phase === 'templates-add') {
      hint.textContent = 'New template · Ctrl+S save · Esc back';
      okBtn.textContent = 'Save';
      body.innerHTML = `
        <div class="dim">New capture template (current workspace)</div>
        <div style="margin-top:12px;">
          <label class="dim" for="native-capture-tmpl-name">Name</label>
          <input id="native-capture-tmpl-name" type="text" placeholder="e.g. Inbox task" />
        </div>
        <div style="margin-top:10px;">
          <label class="dim" for="native-capture-tmpl-keys">Keys</label>
          <input id="native-capture-tmpl-keys" type="text" placeholder="e.g. tt" />
        </div>
        <div style="margin-top:10px;">
          <label class="dim" for="native-capture-tmpl-outline">Outline</label>
          <select id="native-capture-tmpl-outline"></select>
        </div>
      `;
      const nameEl = body.querySelector('#native-capture-tmpl-name');
      const keysEl = body.querySelector('#native-capture-tmpl-keys');
      const outEl = body.querySelector('#native-capture-tmpl-outline');
      if (nameEl) {
        nameEl.value = captureState.tmplName || '';
        nameEl.addEventListener('input', () => { captureState.tmplName = String(nameEl.value || ''); });
      }
      if (keysEl) {
        keysEl.value = captureState.tmplKeyPath || '';
        keysEl.addEventListener('input', () => { captureState.tmplKeyPath = String(keysEl.value || ''); });
      }
      if (outEl) {
        outEl.innerHTML = '';
        for (const o of (captureState.outlines || [])) {
          const id = String(o?.id || '').trim();
          const label = String(o?.label || '').trim() || id;
          if (!id) continue;
          const opt = document.createElement('option');
          opt.value = id;
          opt.textContent = label;
          if (id === captureState.tmplOutlineId) opt.selected = true;
          outEl.appendChild(opt);
        }
        outEl.addEventListener('change', () => { captureState.tmplOutlineId = String(outEl.value || '').trim(); });
      }
      setTimeout(() => nameEl?.focus?.(), 0);
      return;
    }

    if (captureState.phase === 'select') {
      hint.textContent = prefix ? ('Keys: ' + prefix + ' · type next key · Backspace up · Enter select · Ctrl+T templates') : 'Type a key to start a capture template sequence. Enter for manual capture · Ctrl+T templates';
      okBtn.textContent = 'Select';
      captureRefreshList();
      const rows = captureState.list || [];
      body.innerHTML = `
        <div class="dim" style="margin-bottom:8px;">Templates (current workspace)</div>
        <ul class="kb-list" id="native-capture-list" style="list-style:none;padding:0;margin:0;"></ul>
      `;
      const ul = body.querySelector('#native-capture-list');
      if (ul) {
        rows.forEach((o, i) => {
          const li = document.createElement('li');
          li.className = 'list-row';
          li.tabIndex = -1;
          li.style.cursor = 'pointer';
          if (i === captureState.idx) li.style.background = 'color-mix(in oklab, var(--bg), var(--fg) 8%)';
          li.innerHTML = `<span style="min-width:64px;display:inline-block;">${o.key ? ('<code>' + escapeHTML(o.key) + '</code>') : ''}</span><span>${escapeHTML(o.label)}</span>`;
          li.addEventListener('click', () => {
            captureState.idx = i;
            renderCapture();
            captureSelectCurrent();
          });
          ul.appendChild(li);
        });
      }
      return;
    }

    hint.textContent = 'e title · D description · m outline · Space status · Enter capture · Ctrl+Enter capture (textarea)';
    okBtn.textContent = 'Capture';
    const outlineLabels = captureOutlineLabelByID();
    const outlineLabel = outlineLabels.get(String(captureState.draftOutlineId || '')) || captureState.draftOutlineId || '-';
    const statusOpts = captureState.statusByOutline && captureState.draftOutlineId ? (captureState.statusByOutline[captureState.draftOutlineId] || []) : [];
    const statusLabel = (statusOpts.find((o) => String(o?.id || '') === String(captureState.draftStatusId || ''))?.label) || captureState.draftStatusId || '-';
    body.innerHTML = `
      <div class="dim">Template: <strong>${escapeHTML(String(captureState.selectedTemplate?.name || 'Manual'))}</strong></div>
      <div class="dim" style="margin-top:6px;">Destination: <strong>${escapeHTML(outlineLabel)}</strong></div>
      <div class="dim" style="margin-top:6px;">Status: <strong>${escapeHTML(statusLabel)}</strong></div>
      <div style="margin-top:12px;">
        <label class="dim" for="native-capture-draft-title">Title</label>
        <input id="native-capture-draft-title" type="text" />
      </div>
      <div style="margin-top:10px;">
        <label class="dim" for="native-capture-draft-desc">Description (markdown)</label>
        <textarea id="native-capture-draft-desc" rows="6" style="font-family:var(--mono);"></textarea>
      </div>
      <div class="native-modal-field-row" style="margin-top:10px;">
        <label class="dim">Outline
          <select id="native-capture-draft-outline"></select>
        </label>
        <label class="dim">Status
          <select id="native-capture-draft-status"></select>
        </label>
      </div>
    `;
    const titleEl = body.querySelector('#native-capture-draft-title');
    const descEl = body.querySelector('#native-capture-draft-desc');
    const outlineEl = body.querySelector('#native-capture-draft-outline');
    const statusEl = body.querySelector('#native-capture-draft-status');
    if (titleEl) titleEl.value = captureState.draftTitle || '';
    if (descEl) descEl.value = captureState.draftDesc || '';
    if (outlineEl) {
      outlineEl.innerHTML = '';
      for (const o of (captureState.outlines || [])) {
        const id = String(o?.id || '').trim();
        const label = String(o?.label || '').trim() || id;
        if (!id) continue;
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = label;
        if (id === captureState.draftOutlineId) opt.selected = true;
        outlineEl.appendChild(opt);
      }
      outlineEl.addEventListener('change', () => {
        captureState.draftOutlineId = String(outlineEl.value || '').trim();
        const sts = captureState.statusByOutline[captureState.draftOutlineId] || [];
        captureState.draftStatusId = String(sts[0]?.id || '').trim();
        renderCapture();
      }, { once: true });
    }
    if (statusEl) {
      statusEl.innerHTML = '';
      for (const s of (statusOpts || [])) {
        const id = String(s?.id || '').trim();
        const label = String(s?.label || '').trim() || id;
        if (!id) continue;
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = label;
        if (id === captureState.draftStatusId) opt.selected = true;
        statusEl.appendChild(opt);
      }
      statusEl.addEventListener('change', () => {
        captureState.draftStatusId = String(statusEl.value || '').trim();
      }, { once: true });
    }
    titleEl && titleEl.addEventListener('input', () => { captureState.draftTitle = String(titleEl.value || ''); });
    descEl && descEl.addEventListener('input', () => { captureState.draftDesc = String(descEl.value || ''); });
    setTimeout(() => titleEl && titleEl.focus(), 0);
  };

  const openCaptureModal = () => {
    if (captureState.open) return;
    captureState.open = true;
    captureState.restoreEl = document.activeElement;
    const modal = ensureCaptureModal();
    captureState.phase = 'select';
    captureState.prefix = [];
    captureState.idx = 0;
    captureState.templatesIdx = 0;
    captureState.templatesDeleteKeyPath = '';
    captureState.selectedTemplate = null;
    captureState.draftOutlineId = '';
    captureState.draftStatusId = '';
    captureState.draftTitle = '';
    captureState.draftDesc = '';
    captureState.tmplName = '';
    captureState.tmplKeyPath = '';
    captureState.tmplOutlineId = '';
    captureState.outlines = [];
    captureState.templates = [];
    captureState.statusByOutline = {};
    captureState.tree = null;
    captureState.list = [];
    setCaptureError('');
    modal.style.display = 'flex';
    renderCapture();

    loadCaptureOptions().then((data) => {
      captureState.outlines = Array.isArray(data?.outlines) ? data.outlines : [];
      captureState.templates = Array.isArray(data?.templates) ? data.templates : [];
      captureState.statusByOutline = (data && typeof data.statusOptionsByOutline === 'object' && data.statusOptionsByOutline) ? data.statusOptionsByOutline : {};
      captureState.tree = buildCaptureTree(captureState.templates);
      renderCapture();
    }).catch((err) => {
      setCaptureError('Error: ' + (err && err.message ? err.message : 'failed to load capture data'));
    });
  };

  const appendCapturedItemIfVisible = (outlineId, itemId, title) => {
    const root = nativeOutlineRoot();
    if (!root) return false;
    const cur = (root.dataset.outlineId || '').trim();
    if (!cur || cur !== String(outlineId || '').trim()) return false;

    const rows = nativeRows();
    const refRow = rows.length ? rows[rows.length - 1] : null;
    const li = document.createElement('li');
    li.id = 'outline-node-' + itemId;
    li.dataset.nodeId = itemId;

    const row = document.createElement('div');
    row.className = 'outline-row';
    row.tabIndex = 0;
    row.id = 'outline-row-' + itemId;
    row.dataset.outlineRow = '';
    row.dataset.kbItem = '';
    row.dataset.focusId = itemId;
    row.dataset.id = itemId;
    const statusOpts = parseStatusOptions(root);
    const first = statusOpts && statusOpts.length ? statusOpts[0] : { id: 'todo', label: 'TODO', isEndState: false };
    row.dataset.status = (first.id || '').trim();
    row.dataset.end = first.isEndState ? 'true' : 'false';
    row.dataset.canEdit = 'true';
    row.dataset.priority = 'false';
    row.dataset.onHold = 'false';
    row.dataset.dueDate = '';
    row.dataset.dueTime = '';
    row.dataset.schDate = '';
    row.dataset.schTime = '';
    row.dataset.openHref = '/items/' + itemId;
    // Keep IDs out of the visible UI; they remain available via copy actions.

    row.innerHTML = `
      <span class="outline-caret outline-chevron" aria-hidden="true"></span>
      <span class="outline-status outline-label">${escapeHTML(first.label || first.id || '')}</span>
      <span class="outline-title outline-text">${escapeHTML(title || '')}</span>
      <span class="outline-right"></span>
    `;
    li.appendChild(row);
    const ul = root.querySelector('ul.outline-list');
    if (!ul) return false;
    ul.appendChild(li);
    rememberOutlineFocus(root, itemId);
    row.focus?.();
    if (root.dataset.viewMode === 'list+preview') refreshOutlinePreview(root, itemId);
    if (root.dataset.viewMode === 'columns') renderOutlineColumns(root);
    return true;
  };

  const submitCapture = async () => {
    if (!captureState.open) return;
    if (captureState.phase === 'templates') {
      captureState.phase = 'select';
      renderCapture();
      return;
    }
    if (captureState.phase === 'templates-add') {
      try {
        await submitCaptureTemplateUpsert();
      } catch (err) {
        setCaptureError('Error: ' + (err && err.message ? err.message : 'save failed'));
      }
      return;
    }
    if (captureState.phase === 'templates-delete') {
      try {
        await submitCaptureTemplateDelete();
      } catch (err) {
        setCaptureError('Error: ' + (err && err.message ? err.message : 'delete failed'));
      }
      return;
    }
    if (captureState.phase === 'select') {
      captureSelectCurrent();
      return;
    }
    const modal = ensureCaptureModal();
    const body = modal.querySelector('#native-capture-body');
    const title = String(body?.querySelector('#native-capture-draft-title')?.value || '').trim();
    const outlineId = String(body?.querySelector('#native-capture-draft-outline')?.value || captureState.draftOutlineId || '').trim();
    const statusId = String(body?.querySelector('#native-capture-draft-status')?.value || captureState.draftStatusId || '').trim();
    const description = String(body?.querySelector('#native-capture-draft-desc')?.value || captureState.draftDesc || '');
    if (!title || !outlineId) {
      setCaptureError('Title and destination are required (press e/m)');
      return;
    }
    setCaptureError('');
    try {
      const res = await fetch('/capture', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
        body: JSON.stringify({ outlineId, statusId, title, description }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      const id = String(data?.id || '').trim();
      const oid = String(data?.outlineId || outlineId).trim();
      closeCaptureModal();
      if (id) {
        if (!appendCapturedItemIfVisible(oid, id, title)) {
          window.location.href = '/outlines/' + encodeURIComponent(oid);
        }
      }
    } catch (err) {
      setCaptureError('Error: ' + (err && err.message ? err.message : 'capture failed'));
    }
  };

  const setActionPanelMode = (mode) => {
    mode = String(mode || '').trim() || 'context';
    actionPalette.mode = mode;
    switch (mode) {
      case 'nav':
        actionPalette.options = actionsForNav();
        break;
      case 'agenda':
        actionPalette.options = actionsForAgenda();
        break;
      case 'sync':
        actionPalette.options = actionsForSync();
        break;
      case 'outline':
        actionPalette.options = actionsForOutline();
        break;
      case 'capture':
        actionPalette.options = [];
        break;
      default:
        actionPalette.options = actionsForContext();
        break;
    }
    actionPalette.idx = 0;
    renderActionPalette();
  };

  const pushActionPanel = (mode) => {
    if (!actionPalette.open) return;
    const cur = (actionPalette.stack || []).length ? actionPalette.stack[actionPalette.stack.length - 1] : '';
    if (cur === mode) return;
    actionPalette.stack.push(mode);
    setActionPanelMode(mode);
  };

  const popActionPanel = () => {
    if (!actionPalette.open) return;
    if ((actionPalette.stack || []).length <= 1) {
      closeActionPalette();
      return;
    }
    actionPalette.stack.pop();
    const top = actionPalette.stack[actionPalette.stack.length - 1] || 'context';
    setActionPanelMode(top);
  };

  const runSelectedAction = ({ trigger } = {}) => {
    if (!actionPalette.open) return;
    const sel = (actionPalette.options || [])[actionPalette.idx];
    if (!sel) return;
    const kind = String(sel.kind || '').trim();
    if (kind === 'nav') {
      const next = String(sel.next || '').trim();
      if (next) pushActionPanel(next);
      return;
    }
    if (typeof sel.run === 'function') {
      closeActionPalette();
      try { sel.run(); } catch (_) {}
      return;
    }
    // If it's not runnable, keep the panel open (no-op).
    if (trigger === 'enter') return;
  };

  const openActionPalette = () => {
    if (actionPalette.open) return;
    actionPalette.open = true;
    actionPalette.stack = ['context'];
    setActionPanelMode('context');
    actionPalette.restoreEl = document.activeElement;
    actionPalette.idx = 0;
    const modal = ensureActionModal();
    modal.style.display = 'flex';
    renderActionPalette();
  };

  const openNavPanel = () => {
    if (!actionPalette.open) {
      actionPalette.open = true;
      actionPalette.restoreEl = document.activeElement;
      actionPalette.stack = ['nav'];
      setActionPanelMode('nav');
      const modal = ensureActionModal();
      modal.style.display = 'flex';
      renderActionPalette();
      loadNavOptions().then((data) => {
        navOptions.recent = Array.isArray(data?.recent) ? data.recent : [];
        refreshNavOptionsIfOpen();
      }).catch(() => {});
      return;
    }
    pushActionPanel('nav');
    loadNavOptions().then((data) => {
      navOptions.recent = Array.isArray(data?.recent) ? data.recent : [];
      refreshNavOptionsIfOpen();
    }).catch(() => {});
  };

  const openAgendaPanel = () => {
    if (!actionPalette.open) {
      actionPalette.open = true;
      actionPalette.restoreEl = document.activeElement;
      actionPalette.stack = ['agenda'];
      setActionPanelMode('agenda');
      const modal = ensureActionModal();
      modal.style.display = 'flex';
      renderActionPalette();
      return;
    }
    pushActionPanel('agenda');
  };

  const openSyncPanel = () => {
    if (!actionPalette.open) openActionPalette();
    pushActionPanel('sync');
  };

  const openOutlinePanel = () => {
    if (!actionPalette.open) openActionPalette();
    pushActionPanel('outline');
  };

  const ensureMoveOutlineModal = () => {
    let el = document.getElementById('native-move-outline-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-move-outline-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box">
        <div class="native-modal-head">
          <strong>Move to outline</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-move-outline-list" class="native-modal-list"></div>
        <div class="dim native-modal-hint">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div class="native-modal-actions">
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
        if (root && root.id === 'item-native') {
          window.location.href = '/items/' + encodeURIComponent(itemId);
        }
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
      if (root && root.id === 'item-native') {
        window.location.href = '/items/' + encodeURIComponent(id);
      }
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
    restoreFocusId: '',
  };

  const ensureAssigneeModal = () => {
    let el = document.getElementById('native-assignee-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-assignee-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div class="native-modal-box native-modal-box--narrow">
        <div class="native-modal-head">
          <strong>Assign</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-assignee-list" class="native-modal-list"></div>
        <div class="dim native-modal-hint">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div class="native-modal-actions">
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
    const restoreId = (assigneePicker.restoreFocusId || assigneePicker.rowId || '').trim();
    assigneePicker.open = false;
    assigneePicker.rowId = '';
    assigneePicker.rootEl = null;
    assigneePicker.options = [];
    assigneePicker.idx = 0;
    assigneePicker.restoreFocusId = '';
    const modal = document.getElementById('native-assignee-modal');
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
  };

  const outlineRowRight = (row) => {
    if (!row) return null;
    return row.querySelector('.outline-right') || row;
  };

  const findOutlineRowById = (root, id) => {
    id = String(id || '').trim();
    if (!id) return null;
    let row = null;
    try {
      row = root && root.querySelector ? root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]') : null;
    } catch (_) {
      row = null;
    }
    if (row) return row;
    const main = document.getElementById('clarity-main') || document;
    try {
      return main.querySelector ? main.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]') : null;
    } catch (_) {
      return null;
    }
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

    const row = findOutlineRowById(root, id);
    if (row) nativeRowUpdateAssignee(row, sel);
    const assignedActorId = (sel.id || '').trim();
    if (root && root.id === 'item-native') {
      const selEl = document.getElementById('assignedActorId');
      if (selEl) selEl.value = assignedActorId;
    }
    closeAssigneePicker();
    focusOutlineById(id);

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
    restoreFocusId: '',
  };

  const ensureStatusModal = () => {
    let el = document.getElementById('native-status-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-status-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div id="native-status-modal-box" class="native-modal-box native-modal-box--narrow">
        <div class="native-modal-head">
          <strong id="native-status-title">Status</strong>
          <span class="dim" style="font-size:12px;">Esc to cancel</span>
        </div>
        <div id="native-status-note-wrap" class="native-modal-body" style="display:none;">
          <div class="dim" style="font-size:12px;margin-bottom:6px;">Note required</div>
          <input id="native-status-note" type="text" placeholder="Add a note…" style="width:100%;" />
        </div>
        <div id="native-status-list" class="native-modal-list"></div>
        <div id="native-status-hint" class="dim native-modal-hint">Up/Down or Ctrl+P/N to move · Enter to select</div>
        <div class="native-modal-actions">
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
    const restoreId = (statusPicker.restoreFocusId || statusPicker.rowId || '').trim();
    statusPicker.open = false;
    statusPicker.rowId = '';
    statusPicker.rootEl = null;
    statusPicker.options = [];
    statusPicker.idx = 0;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    statusPicker.title = 'Status';
    statusPicker.submit = null;
    statusPicker.restoreFocusId = '';
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

    // Parent progress cookies depend on direct children end-state.
    const li = nativeLiFromRow(row);
    const parentLi = li ? li.parentElement?.closest('li[data-node-id]') : null;
    if (parentLi) updateOutlineProgressForLi(parentLi);
  };

  const updateOutlineProgressForLi = (li) => {
    if (!li || !li.querySelector) return;
    const row = li.querySelector(':scope > [data-outline-row]');
    if (!row) return;
    const ul = li.querySelector(':scope > ul.outline-children');
    const existing = row.querySelector('.outline-progress');
    if (!ul) {
      existing && existing.remove();
      return;
    }
    const kids = Array.from(ul.querySelectorAll(':scope > li[data-node-id] > [data-outline-row]'));
    const total = kids.length;
    if (total <= 0) {
      existing && existing.remove();
      return;
    }
    let done = 0;
    for (const k of kids) {
      if ((k.dataset && String(k.dataset.end || '') === 'true')) done++;
    }
    let el = existing;
    if (!el) {
      el = document.createElement('span');
      el.className = 'outline-progress dim';
      el.setAttribute('data-ignore-morph', '');
      const caret = row.querySelector('.outline-caret');
      if (caret && caret.nextSibling) {
        caret.parentNode.insertBefore(el, caret.nextSibling);
      } else {
        row.insertBefore(el, row.firstChild);
      }
    }
    el.textContent = `[${done}/${total}]`;
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
    const row = findOutlineRowById(root, id);
    if (row) nativeRowUpdateStatus(row, sel);
    if (root && root.id === 'item-native') {
      const selEl = document.getElementById('status');
      if (selEl) selEl.value = statusID;
    }
    closeStatusPicker();
    if (outlineViewNormalize(root.dataset?.viewMode || '') === 'columns') renderOutlineColumns(root);
    focusOutlineById(id);

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
    restoreFocusId: '',
  };

  const ensurePromptModal = () => {
    let el = document.getElementById('native-prompt-modal');
    if (el) return el;
    el = document.createElement('div');
    el.id = 'native-prompt-modal';
    el.className = 'native-modal-backdrop';
    el.innerHTML = `
      <div id="native-prompt-box" class="native-modal-box">
        <div class="native-modal-head">
          <strong id="native-prompt-title"></strong>
          <span class="dim" id="native-prompt-hint" style="font-size:12px;">Esc to close · Ctrl+Enter to save</span>
        </div>
        <div id="native-prompt-body" class="native-modal-body"></div>
        <div class="native-modal-actions">
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

  const openPrompt = ({ title, hint, bodyHTML, onSubmit, focusSelector, restoreFocusId }) => {
    const inferRestoreFocusId = () => {
      const explicit = (restoreFocusId || '').trim();
      if (explicit) return explicit;
      const a = document.activeElement;
      if (a && typeof a.closest === 'function') {
        const fid = a.getAttribute ? String(a.getAttribute('data-focus-id') || '').trim() : '';
        if (fid) return fid;
        const row = a.closest('[data-outline-row]');
        if (row && row.dataset) {
          const id = String(row.dataset.id || '').trim();
          if (id) return id;
        }
        const ar = a.closest('[data-agenda-row]');
        if (ar && ar.dataset) {
          const id = String(ar.dataset.id || '').trim();
          if (id) return id;
        }
      }
      const ir = itemPageRoot();
      if (ir) {
        const id = String(ir.dataset.itemId || '').trim();
        if (id) return id;
      }
      return '';
    };

    const modal = ensurePromptModal();
    modal.querySelector('#native-prompt-title').textContent = title || '';
    modal.querySelector('#native-prompt-hint').textContent = hint || 'Esc to close · Ctrl+Enter to save';
    modal.querySelector('#native-prompt-body').innerHTML = bodyHTML || '';
    prompt.open = true;
    prompt.submit = onSubmit || null;
    prompt.restoreFocusId = inferRestoreFocusId();
    modal.style.display = 'flex';
    const focus = focusSelector ? modal.querySelector(focusSelector) : null;
    focus && focus.focus();
  };

  const closePrompt = () => {
    const restoreId = (prompt.restoreFocusId || '').trim();
    prompt.open = false;
    prompt.kind = '';
    prompt.rowId = '';
    prompt.outlineId = '';
    prompt.submit = null;
    prompt.restoreFocusId = '';
    const modal = document.getElementById('native-prompt-modal');
    if (modal) modal.style.display = 'none';
    restoreNativeFocusAfterModal(restoreId);
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
    statusPicker.restoreFocusId = String(document.activeElement?.getAttribute?.('data-focus-id') || '').trim();
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
    assigneePicker.restoreFocusId = String(document.activeElement?.getAttribute?.('data-focus-id') || '').trim();
    const modal = ensureAssigneeModal();
    modal.style.display = 'flex';
    renderAssigneePicker();
  };

  const openMoveOutlinePickerForItemPage = (root) => {
    if (!root) return;
    const id = (root.dataset.itemId || '').trim();
    if (!id) return;
    if ((root.dataset.canEdit || '') !== 'true') return;
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
        const row = findOutlineRowById(root, id);
        if (row) {
          row.dataset.title = newText;
          const t = row.querySelector('.outline-title');
          if (t) t.textContent = newText;
        }
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
        const row = findOutlineRowById(root, id);
        if (row) nativeRowUpdateTags(row, tags);
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
        <div class="native-modal-field-row">
          <label class="dim">Date <input id="native-prompt-date" type="date" value="${escapeAttr(curDate)}"></label>
          <label class="dim">Time <input id="native-prompt-time" type="time" value="${escapeAttr(curTime)}"></label>
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
        const row = findOutlineRowById(root, id);
        if (row) nativeRowUpdateDateTime(row, kind === 'due' ? 'due' : 'schedule', date ? { date, time: time || null } : null);
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
      restoreFocusId: refId,
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const input = modal ? modal.querySelector('#native-prompt-input') : null;
        const title = input ? (input.value || '').trim() : '';
        if (!title) return;
        closePrompt();
        const tempId = randomTempID();
        const optimisticRow = insertOptimisticNativeItem({ root, mode, refRow: row, tempId, title });
        if (root.dataset.viewMode === 'columns') renderOutlineColumns(root);
        const typ = mode === 'child' ? 'outline:new_child' : 'outline:new_sibling';
        const detail = mode === 'child' ? { title, parentId: refId, tempId } : { title, afterId: refId, tempId };
        outlineApply(root, typ, detail).then((resp) => {
          const created = resp && Array.isArray(resp.created) ? resp.created : [];
          const match = created.find((c) => c && String(c.tempId || '').trim() === tempId) || null;
          const realId = match && match.id ? String(match.id).trim() : '';
          if (!realId) return;
          const root2 = nativeOutlineRoot() || root;
          const row2 = root2 ? root2.querySelector('[data-outline-row][data-id="' + CSS.escape(tempId) + '"]') : null;
          const li = row2 ? nativeLiFromRow(row2) : null;
          if (li) {
            li.dataset.nodeId = realId;
            li.id = 'outline-node-' + realId;
          }
          if (row2) {
            row2.dataset.id = realId;
            row2.dataset.focusId = realId;
            row2.dataset.openHref = '/items/' + realId;
            row2.id = 'outline-row-' + realId;
            // Keep IDs out of the visible UI; they remain available via copy actions.
            try { sessionStorage.setItem('clarity:lastFocus', realId); } catch (_) {}
            row2.focus();
          } else if (optimisticRow) {
            // Fallback: keep focus stable.
            try { sessionStorage.setItem('clarity:lastFocus', realId); } catch (_) {}
          }
        }).catch((err) => {
          const msg = err && err.message ? err.message : 'save failed';
          setOutlineStatus('Error: ' + msg);
          setTimeout(() => setOutlineStatus(''), 2000);
          // Remove optimistic placeholder if it still exists.
          const root2 = nativeOutlineRoot() || root;
          const row2 = root2 ? root2.querySelector('[data-outline-row][data-id="' + CSS.escape(tempId) + '"]') : null;
          const li = row2 ? nativeLiFromRow(row2) : null;
          li && li.remove();
          refId && focusNativeRowById(refId);
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
      restoreFocusId: id,
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
      restoreFocusId: id,
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
        <div class="native-modal-field-row">
          <label class="dim">Date <input id="native-prompt-date" type="date" value="${escapeAttr(curDate)}"></label>
          <label class="dim">Time <input id="native-prompt-time" type="time" value="${escapeAttr(curTime)}"></label>
          <button type="button" id="native-prompt-clear">Clear</button>
        </div>
      `,
      focusSelector: '#native-prompt-date',
      restoreFocusId: id,
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
      restoreFocusId: id,
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
      restoreFocusId: id,
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
  const outlineCollapseCookieName = (outlineId) => 'clarity_outline_collapsed_' + outlineId;

  const setCookie = (name, value) => {
    name = String(name || '').trim();
    if (!name) return;
    value = String(value ?? '');
    const maxAge = 60 * 60 * 24 * 365; // 1 year
    document.cookie = `${name}=${value}; path=/; max-age=${maxAge}; samesite=lax`;
  };

  const syncCollapsedCookie = (root, set) => {
    const outlineId = (root && root.dataset ? root.dataset.outlineId : '') || '';
    if (!outlineId) return;
    const ids = Array.from(set || []).map((x) => String(x || '').trim()).filter(Boolean);
    // Best-effort bound to keep cookie size reasonable.
    const out = [];
    let bytes = 0;
    for (const id of ids) {
      const next = out.length ? (',' + id) : id;
      bytes += next.length;
      if (bytes > 3500) break;
      out.push(id);
    }
    setCookie(outlineCollapseCookieName(outlineId), out.join(','));
  };

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
    syncCollapsedCookie(root, set);
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

  const agendaCollapseKey = (actorId) => 'clarity:agenda:' + (actorId || 'anon') + ':collapsed';

  const loadAgendaCollapsedSet = (root) => {
    const actorId = (root && root.dataset ? String(root.dataset.actorId || '') : '').trim();
    try {
      const raw = localStorage.getItem(agendaCollapseKey(actorId));
      const xs = raw ? JSON.parse(raw) : [];
      return new Set(Array.isArray(xs) ? xs : []);
    } catch (_) {
      return new Set();
    }
  };

  const saveAgendaCollapsedSet = (root, set) => {
    const actorId = (root && root.dataset ? String(root.dataset.actorId || '') : '').trim();
    try {
      localStorage.setItem(agendaCollapseKey(actorId), JSON.stringify(Array.from(set)));
    } catch (_) {}
  };

  const agendaApplyVisibility = () => {
    document.querySelectorAll('[data-agenda-row]').forEach((row) => {
      const li = row.closest('li');
      if (!li) return;
      const filterHidden = li.dataset.filterHidden === '1';
      const collapsedHidden = li.dataset.collapsedHidden === '1';
      li.style.display = (filterHidden || collapsedHidden) ? 'none' : '';
    });
  };

  const applyAgendaCollapsed = (root, set) => {
    if (!root) return;
    const rows = Array.from(document.querySelectorAll('[data-agenda-row]'));
    const stack = [];
    for (const row of rows) {
      if (!row || !row.dataset) continue;
      const li = row.closest('li');
      if (!li) continue;
      const depth = parseInt(String(row.dataset.depth || '0'), 10) || 0;
      while (stack.length && depth <= stack[stack.length - 1]) stack.pop();
      li.dataset.collapsedHidden = stack.length ? '1' : '0';

      const hasChildren = String(row.dataset.hasChildren || '') === 'true';
      const caret = row.querySelector('.outline-caret');
      const id = String(row.dataset.id || '').trim();
      if (hasChildren) {
        const collapsed = id && set.has(id);
        if (caret) caret.textContent = collapsed ? '▸' : '▾';
        if (collapsed) stack.push(depth);
      } else {
        if (caret) caret.textContent = '';
      }
    }
    agendaApplyVisibility();
  };

  const ensureAgendaDefaultCollapse = (root) => {
    if (!root) return;
    const actorId = (root.dataset ? String(root.dataset.actorId || '') : '').trim();
    const key = agendaCollapseKey(actorId);
    try {
      if (localStorage.getItem(key) != null) return;
    } catch (_) {
      return;
    }
    const set = new Set();
    document.querySelectorAll('[data-agenda-row][data-has-children="true"]').forEach((row) => {
      const id = String(row.dataset.id || '').trim();
      if (id) set.add(id);
    });
    saveAgendaCollapsedSet(root, set);
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

    const oldParentLi = li.parentElement ? li.parentElement.closest('li[data-node-id]') : null;

    const ul = ensureChildList(prev);
    // Determine afterId (append at end).
    const lastChild = ul.lastElementChild && ul.lastElementChild.dataset ? (ul.lastElementChild.dataset.nodeId || '').trim() : '';
    ul.appendChild(li);

    const detail = { id, parentId };
    if (lastChild) detail.afterId = lastChild;
    queueOutlineMove(root, detail);
    focusNativeRowById(id);

    if (oldParentLi) updateOutlineProgressForLi(oldParentLi);
    updateOutlineProgressForLi(prev);
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

    updateOutlineProgressForLi(parentLi);
    const gpLi = grandParentUl.closest ? grandParentUl.closest('li[data-node-id]') : null;
    if (gpLi) updateOutlineProgressForLi(gpLi);
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
      const ct = (res.headers.get('Content-Type') || '').toLowerCase();
      if (ct.includes('application/json')) {
        try {
          return await res.json();
        } catch (_) {
          return null;
        }
      }
      return null;
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
        <div><code>x</code>/<code>?</code> — Actions</div>
        <div><code>g</code> — Go to…</div>
        <div><code>a</code>/<code>A</code> — Agenda commands</div>
        <div><code>c</code> — Capture</div>
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
    const scope = document.querySelector('[data-kb-scope-active="true"]');
    const root = scope || document.getElementById('clarity-main');
    if (!root) return [];
    return Array.from(root.querySelectorAll('[data-kb-item]')).filter((el) => {
      if (!el) return false;
      if (el.hasAttribute('disabled')) return false;
      if (el.getAttribute('aria-disabled') === 'true') return false;
      try {
        return el.getClientRects().length > 0;
      } catch (_) {
        return true;
      }
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
    if (el.tagName && el.tagName.toLowerCase() === 'a' && el.getAttribute('href')) {
      window.location.href = el.getAttribute('href');
      return;
    }
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
      // Keep item-side panels open across Datastar morphs (state is client-side).
      if (itemPageRoot()) {
        try { applyItemSideState(); } catch (_) {}
      }
      restoreFocus();
      const native = nativeOutlineRoot();
      if (native) {
        const set = loadCollapsedSet(native);
        applyCollapsed(native, set);
        syncCollapsedCookie(native, set);
      }
      const ar = agendaRoot();
      if (ar) {
        ensureAgendaDefaultCollapse(ar);
        applyAgendaCollapsed(ar, loadAgendaCollapsedSet(ar));
      }
    }, 0);
  };

  document.addEventListener('focusin', () => {
    rememberFocus();
  }, { capture: true });

  // Keep the outline "focused item" in sync for list+preview mode.
  document.addEventListener('focusin', (ev) => {
    const row = nativeRowFromEvent(ev);
    if (!row || !row.dataset) return;
    const root = nativeOutlineRootOrFromRow(row);
    if (!root) return;
    const id = String(row.dataset.id || '').trim();
    if (!id) return;
    rememberOutlineFocus(root, id);
    if (outlineViewNormalize(root.dataset.viewMode || '') === 'list+preview') {
      refreshOutlinePreview(root, id);
    }
  }, { capture: true });

  document.addEventListener('focusin', (ev) => {
    const root = nativeOutlineRoot();
    if (!root) return;
    if (outlineViewNormalize(root.dataset.viewMode || '') !== 'columns') return;
    const pane = document.getElementById('outline-columns-pane');
    if (!pane) return;
    const t = ev && ev.target;
    const card = t && typeof t.closest === 'function' ? t.closest('.outline-card') : null;
    if (!card || !pane.contains(card)) return;
    const id = String(card.dataset.itemId || card.dataset.focusId || '').trim();
    if (!id) return;
    rememberOutlineFocus(root, id);
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

  // Item side panels: click a related row to open the panel.
  document.addEventListener('click', (ev) => {
    const root = itemPageRoot();
    if (!root) return;
    const t = ev && ev.target;
    if (!t || typeof t.closest !== 'function') return;
    const el = t.closest('[data-item-side-open]');
    if (!el || !el.dataset) return;
    const kind = String(el.dataset.itemSideOpen || '').trim();
    if (!kind) return;
    ev.preventDefault();
    itemSideOpen(kind, String(el.dataset.focusId || '').trim());
  }, { capture: true });

  // Item side panel forms: submit via fetch so the side panel stays open (SSE will refresh the view).
  document.addEventListener('submit', (ev) => {
    const root = itemPageRoot();
    if (!root) return;
    const form = ev && ev.target;
    if (!form || !(form instanceof HTMLFormElement)) return;
    const action = String(form.getAttribute('action') || '').trim();
    if (!action) return;
    if (!action.startsWith('/items/')) return;
    const inSide = !!form.closest('#item-side-pane');
    if (!inSide) return;
    // Only intercept comment/worklog posts.
    if (!action.endsWith('/comments') && !action.endsWith('/worklog')) return;
    ev.preventDefault();
    const fd = new FormData(form);
    const bodyTxt = String(fd.get('body') || '').trim();
    const optimisticInsert = () => {
      if (!bodyTxt) return;
      const pane = itemSidePane();
      const kind = action.endsWith('/worklog') ? 'worklog' : 'comments';
      const panel = pane ? pane.querySelector('[data-item-side-panel="' + CSS.escape(kind) + '"]') : null;
      const ul = panel ? panel.querySelector('ul.kb-list') : null;
      if (!ul) return;
      const now = new Date().toISOString().replace('.000Z', 'Z');
      const li = document.createElement('li');
      li.innerHTML = `
        <div class="list-row dim" tabindex="-1">
          <span>${escapeHTML(now)}</span>
          <span>—</span>
          <span>(pending)</span>
        </div>
        <div class="md comment-body" style="margin: 6px 0 10px;">${escapeHTML(bodyTxt)}</div>
      `;
      ul.insertBefore(li, ul.firstChild);
    };
    fetch(action, {
      method: String(form.getAttribute('method') || 'POST').toUpperCase(),
      body: new URLSearchParams(fd).toString(),
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    }).then(async (res) => {
      if (!res.ok) throw new Error(await res.text());
      optimisticInsert();
      // Clear textarea on success (SSE will bring the new entry).
      const ta = form.querySelector('textarea[name="body"]');
      if (ta) {
        ta.value = '';
        ta.focus?.();
      }
    }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'post failed'));
      setTimeout(() => setOutlineStatus(''), 1800);
    });
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
    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closeStatusPicker();
      return true;
    }
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
    if (ev.ctrlKey && k === 's') {
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

  const handleOutlineStatusesKeydown = (ev) => {
    if (!outlineStatuses.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closeOutlineStatusesEditor();
      return true;
    }
    if (k === 'escape') {
      ev.preventDefault();
      closeOutlineStatusesEditor();
      return true;
    }
    if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
      ev.preventDefault();
      outlineStatuses.idx = Math.min((outlineStatuses.options.length || 1) - 1, (outlineStatuses.idx || 0) + 1);
      renderOutlineStatusesEditor();
      return true;
    }
    if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
      ev.preventDefault();
      outlineStatuses.idx = Math.max(0, (outlineStatuses.idx || 0) - 1);
      renderOutlineStatusesEditor();
      return true;
    }
    if (!ev.ctrlKey && !ev.altKey && !ev.metaKey) {
      if (k === 'a') {
        ev.preventDefault();
        outlineStatusesAdd();
        return true;
      }
      if (k === 'r') {
        ev.preventDefault();
        outlineStatusesRename();
        return true;
      }
      if (k === 'e') {
        ev.preventDefault();
        outlineStatusesToggleEnd();
        return true;
      }
      if (k === 'n') {
        ev.preventDefault();
        outlineStatusesToggleNote();
        return true;
      }
      if (k === 'd') {
        ev.preventDefault();
        outlineStatusesDelete();
        return true;
      }
    }
    if (ev.ctrlKey && (k === 'j' || k === 'down')) {
      ev.preventDefault();
      outlineStatusesMove(+1);
      return true;
    }
    if (ev.ctrlKey && (k === 'k' || k === 'up')) {
      ev.preventDefault();
      outlineStatusesMove(-1);
      return true;
    }
    return true;
  };

  const handleAssigneePickerKeydown = (ev) => {
    if (!assigneePicker.open) return false;
    const k = (ev.key || '').toLowerCase();
    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closeAssigneePicker();
      return true;
    }
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
    if (ev.ctrlKey && k === 's') {
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

    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closeTagsPicker('cancel');
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
    if (ev.ctrlKey && k === 's') {
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
    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closeMoveOutlinePicker();
      return true;
    }
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
    if (ev.ctrlKey && k === 's') {
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
    if (ev.ctrlKey && k === 'g') {
      ev.preventDefault();
      closePrompt();
      return true;
    }
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
    if (ev.ctrlKey && k === 's') {
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

  const handleOutlineColumnsKeydown = (ev, rawKey, key) => {
    const root = nativeOutlineRoot();
    if (!root) return false;
    if (outlineViewNormalize(root.dataset.viewMode || '') !== 'columns') return false;
    const pane = document.getElementById('outline-columns-pane');
    if (!pane || pane.style.display === 'none') return false;
    const a = document.activeElement;
    if (!a || !pane.contains(a)) return false;
    if (isTypingTarget(ev.target)) return false;

    const card = a.closest ? a.closest('.outline-card') : null;
    const header = !card ? ((a.classList && a.classList.contains('outline-column-header')) ? a : (a.closest ? a.closest('.outline-column-header') : null)) : null;
    const col = (card || header) && (card || header).closest ? (card || header).closest('.outline-column') : null;
    if (!col) return false;
    const wrap = col.parentElement;
    const cols = wrap ? Array.from(wrap.querySelectorAll(':scope > .outline-column')) : [];
    const colIdx = cols.indexOf(col);
    const cards = Array.from(col.querySelectorAll('.outline-column-list > .outline-card'));
    const cardIdx = card ? cards.indexOf(card) : -1;

    const focusInCol = (colEl, idx, fallbackToHeader) => {
      const cs = Array.from(colEl.querySelectorAll('.outline-column-list > .outline-card'));
      if (cs.length > 0) {
        const i = Math.max(0, Math.min(cs.length - 1, idx));
        cs[i]?.focus?.();
        return;
      }
      if (fallbackToHeader) colEl.querySelector('.outline-column-header')?.focus?.();
    };

    const moveCol = (delta) => {
      if (!cols.length || colIdx < 0) return;
      const next = Math.max(0, Math.min(cols.length - 1, colIdx + delta));
      if (next === colIdx) return;
      focusInCol(cols[next], cardIdx >= 0 ? cardIdx : 0, true);
    };

    // Status cycling (TUI parity) takes precedence over column navigation.
    if (card && ev.shiftKey && (key === 'arrowright' || key === 'right')) {
      ev.preventDefault();
      const itemId = String(card.dataset.itemId || card.dataset.focusId || '').trim();
      const row = findOutlineRowById(root, itemId);
      if (row) cycleStatus(row, +1);
      renderOutlineColumns(root);
      focusOutlineById(itemId);
      return true;
    }
    if (card && ev.shiftKey && (key === 'arrowleft' || key === 'left')) {
      ev.preventDefault();
      const itemId = String(card.dataset.itemId || card.dataset.focusId || '').trim();
      const row = findOutlineRowById(root, itemId);
      if (row) cycleStatus(row, -1);
      renderOutlineColumns(root);
      focusOutlineById(itemId);
      return true;
    }

    // Column navigation (left/right).
    if (!ev.shiftKey && !ev.altKey && (key === 'arrowright' || key === 'right' || key === 'l' || (ev.ctrlKey && key === 'f'))) {
      ev.preventDefault();
      moveCol(+1);
      return true;
    }
    if (!ev.shiftKey && !ev.altKey && (key === 'arrowleft' || key === 'left' || key === 'h' || (ev.ctrlKey && key === 'b'))) {
      ev.preventDefault();
      moveCol(-1);
      return true;
    }

    // Within-column navigation (up/down).
    if (key === 'j' || key === 'arrowdown' || key === 'down' || (ev.ctrlKey && key === 'n')) {
      ev.preventDefault();
      if (!cards.length) return true;
      const next = card ? Math.min(cards.length - 1, cardIdx + 1) : 0;
      cards[next]?.focus?.();
      return true;
    }
    if (key === 'k' || key === 'arrowup' || key === 'up' || (ev.ctrlKey && key === 'p')) {
      ev.preventDefault();
      if (!cards.length) return true;
      if (!card) {
        col.querySelector('.outline-column-header')?.focus?.();
        return true;
      }
      const prev = Math.max(0, cardIdx - 1);
      cards[prev]?.focus?.();
      return true;
    }

    if (key === 'enter') {
      if (card) {
        ev.preventDefault();
        const href = String(card.dataset.openHref || '').trim();
        if (href) window.location.href = href;
        return true;
      }
      return false;
    }

    if (card && key === 'e') {
      ev.preventDefault();
      const itemId = String(card.dataset.itemId || card.dataset.focusId || '').trim();
      const row = findOutlineRowById(root, itemId);
      const before = String(row?.querySelector?.('.outline-title')?.textContent || card.textContent || '').trim();
      openPrompt({
        title: 'Edit title',
        hint: 'Esc to cancel · Ctrl+S save',
        bodyHTML: `<input id="native-prompt-input" placeholder="Title" style="width:100%;" value="${escapeAttr(before)}">`,
        focusSelector: '#native-prompt-input',
        restoreFocusId: itemId,
        onSubmit: () => {
          const modal = document.getElementById('native-prompt-modal');
          const input = modal ? modal.querySelector('#native-prompt-input') : null;
          const next = input ? String(input.value || '').trim() : '';
          if (!next) return;
          if (row) {
            row.dataset.title = next;
            const t = row.querySelector('.outline-title');
            if (t) t.textContent = next;
          }
          const t2 = card.querySelector('.outline-title');
          if (t2) t2.textContent = next;
          closePrompt();
          outlineApply(root, 'outline:edit:save', { id: itemId, newText: next }).catch((err) => {
            setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
          });
        },
      });
      return true;
    }

    // Delegate remaining item operations to the native outline handler using the hidden row as source of truth.
    if (card) {
      const itemId = String(card.dataset.itemId || card.dataset.focusId || '').trim();
      const row = findOutlineRowById(root, itemId);
      if (row) {
        const handled = handleNativeOutlineKeydown(ev, key, row);
        if (handled) {
          // Keep the columns view in sync and restore focus to the card.
          setTimeout(() => {
            renderOutlineColumns(root);
            focusOutlineById(itemId);
          }, 0);
          return true;
        }
      }
    }
    return false;
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
    if (ev.altKey && ev.code === 'KeyL') {
      ev.preventDefault();
      nativeIndent(nativeRow);
      return true;
    }
    if (ev.altKey && ev.code === 'KeyH') {
      ev.preventDefault();
      nativeOutdent(nativeRow);
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
    // Hierarchy navigation (match TUI): Right/L/Ctrl+F => into first child; Left/H/Ctrl+B => parent.
    if (!ev.shiftKey && !ev.altKey && (key === 'arrowright' || key === 'right' || key === 'l' || (ev.ctrlKey && key === 'f'))) {
      ev.preventDefault();
      const li = nativeLiFromRow(nativeRow);
      if (!li) return true;
      const ul = li.querySelector(':scope > ul.outline-children');
      if (!ul) return true;
      if (ul.style.display === 'none') {
        const root = nativeOutlineRootOrFromRow(nativeRow);
        if (root) {
          const set = loadCollapsedSet(root);
          const id = (li.dataset && li.dataset.nodeId ? String(li.dataset.nodeId) : '').trim();
          if (id) {
            set.delete(id);
            saveCollapsedSet(root, set);
            applyCollapsed(root, set);
          }
        } else {
          ul.style.display = '';
        }
      }
      const first = ul.querySelector(':scope > li[data-node-id] > [data-outline-row]');
      first?.focus?.();
      return true;
    }
    if (!ev.shiftKey && !ev.altKey && (key === 'arrowleft' || key === 'left' || key === 'h' || (ev.ctrlKey && key === 'b'))) {
      ev.preventDefault();
      const li = nativeLiFromRow(nativeRow);
      const parentLi = li ? li.parentElement?.closest('li[data-node-id]') : null;
      const row = parentLi ? parentLi.querySelector(':scope > [data-outline-row]') : null;
      row?.focus?.();
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
    if (key === '/' && window.location && window.location.pathname === '/agenda') {
      ev.preventDefault();
      const f = document.getElementById('agenda-filter');
      f?.focus?.();
      return true;
    }
    if (key === 'j' || key === 'arrowdown' || key === 'down' || (ev.ctrlKey && key === 'n')) {
      ev.preventDefault();
      moveFocus(+1);
      return true;
    }
    if (key === 'k' || key === 'arrowup' || key === 'up' || (ev.ctrlKey && key === 'p')) {
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

  const handleProjectsAndOutlinesListKeydown = (ev, key) => {
    const view = currentView();
    if (view === 'projects') {
      if (key === 'n' && !ev.shiftKey) {
        ev.preventDefault();
        openNewProjectPrompt();
        return true;
      }
      if (key === 'e') {
        ev.preventDefault();
        openRenameProjectPrompt();
        return true;
      }
      if (key === 'r') {
        ev.preventDefault();
        archiveFocusedProject();
        return true;
      }
      return false;
    }
    if (view === 'outlines') {
      if (key === 'n' && !ev.shiftKey) {
        ev.preventDefault();
        openNewOutlinePrompt();
        return true;
      }
      if (key === 'e') {
        ev.preventDefault();
        openRenameOutlinePrompt();
        return true;
      }
      if (key === 'd' && ev.shiftKey) {
        ev.preventDefault();
        openEditOutlineDescriptionPrompt();
        return true;
      }
      if (key === 'r') {
        ev.preventDefault();
        archiveFocusedOutline();
        return true;
      }
      return false;
    }
    if (view === 'workspaces') {
      if (key === 'n' && !ev.shiftKey) {
        ev.preventDefault();
        openNewWorkspacePrompt();
        return true;
      }
      if (key === 'r') {
        ev.preventDefault();
        openRenameWorkspacePrompt();
        return true;
      }
      return false;
    }
    return false;
  };

  const initAgendaFilter = () => {
    const input = document.getElementById('agenda-filter');
    if (!input) return;
    const key = 'clarity:agendaFilter';
    try {
      const prev = sessionStorage.getItem(key) || '';
      if (prev) input.value = prev;
    } catch (_) {}

    const apply = () => {
      const q = String(input.value || '').trim().toLowerCase();
      try { sessionStorage.setItem(key, q); } catch (_) {}
      const rows = Array.from(document.querySelectorAll('[data-agenda-row]'));
      for (const a of rows) {
        const title = String(a.dataset.title || '').toLowerCase();
        const status = String(a.dataset.statusLabel || a.dataset.status || '').toLowerCase();
        const id = String(a.dataset.focusId || '').toLowerCase();
        const ok = !q || title.includes(q) || status.includes(q) || id.includes(q);
        const li = a.closest('li');
        if (li) li.dataset.filterHidden = ok ? '0' : '1';
      }
      agendaApplyVisibility();
    };

    input.addEventListener('input', apply);
    apply();
  };

  const handleAgendaKeydown = (ev, key, row) => {
    const root = agendaRoot();
    if (!root || !row || !row.dataset) return false;
    const id = String(row.dataset.id || '').trim();
    const depth = parseInt(String(row.dataset.depth || '0'), 10) || 0;
    const rows = Array.from(document.querySelectorAll('[data-agenda-row]'));
    const idx = rows.indexOf(row);

    const loadSet = () => {
      ensureAgendaDefaultCollapse(root);
      return loadAgendaCollapsedSet(root);
    };

    if (key === 'enter') {
      ev.preventDefault();
      const href = String(row.dataset.openHref || '').trim();
      if (href) window.location.href = href;
      return true;
    }
    if (key === 'e') {
      ev.preventDefault();
      if ((row.dataset.canEdit || '') !== 'true') {
        setOutlineStatus('Error: owner-only');
        setTimeout(() => setOutlineStatus(''), 1200);
        return true;
      }
      openPrompt({
        title: 'Edit title',
        hint: 'Esc to cancel · Ctrl+Enter to save',
        bodyHTML: `
          <div>
            <label class="dim" for="agenda-edit-title">Title</label>
            <input id="agenda-edit-title" type="text" style="width:100%;" value="${escapeHTML(String(row.dataset.title || ''))}" />
          </div>
        `,
        focusSelector: '#agenda-edit-title',
        restoreFocusId: id,
        onSubmit: () => {
          const next = String(document.getElementById('agenda-edit-title')?.value || '').trim();
          if (!next) return;
          closePrompt();
          row.dataset.title = next;
          const t = row.querySelector('.outline-title');
          if (t) t.textContent = next;
          outlineApply(row, 'outline:edit:save', { id, newText: next }).catch((err) => {
            setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
          });
        },
      });
      return true;
    }
    if (key === ' ' || key === 'spacebar') {
      ev.preventDefault();
      if ((row.dataset.canEdit || '') !== 'true') {
        setOutlineStatus('Error: owner-only');
        setTimeout(() => setOutlineStatus(''), 1200);
        return true;
      }
      const outlineId = String(row.dataset.outlineId || '').trim();
      if (!outlineId) return true;
      fetchOutlineMeta(outlineId).then((meta) => {
        const raw = (meta && Array.isArray(meta.statusOptions)) ? meta.statusOptions : [];
        const opts = raw.map((o) => ({
          id: (o && o.id) ? String(o.id) : '',
          label: (o && o.label) ? String(o.label) : '',
          isEndState: !!(o && o.isEndState),
          requiresNote: !!(o && o.requiresNote),
        })).filter((o) => (o.id || '').trim() !== '' || o.id === '');
        if (!opts.length) return;
        statusPicker.open = true;
        statusPicker.rowId = id;
        statusPicker.rootEl = row;
        statusPicker.options = opts;
        statusPicker.note = '';
        statusPicker.mode = 'list';
        statusPicker.title = 'Status';
        statusPicker.submit = ({ statusID, option, note }) => {
          if (option) nativeRowUpdateStatus(row, option);
          // Keep filter-friendly label in sync.
          row.dataset.statusLabel = (option && option.label) ? String(option.label) : String(statusID || '');
          return outlineApply(row, 'outline:toggle', { id, to: statusID, note }).then(() => {
            // If it moved to an end-state, let SSE remove it; keep it visible until then.
          });
        };
        const cur = String(row.dataset.status || '').trim();
        let sidx = opts.findIndex((o) => String(o.id || '').trim() === cur);
        if (sidx < 0) sidx = 0;
        statusPicker.idx = sidx;
        const modal = ensureStatusModal();
        modal.style.display = 'flex';
        renderStatusPicker();
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'load failed'));
      });
      return true;
    }
    if (key === 'r') {
      ev.preventDefault();
      if ((row.dataset.canEdit || '') !== 'true') {
        setOutlineStatus('Error: owner-only');
        setTimeout(() => setOutlineStatus(''), 1200);
        return true;
      }
      openPrompt({
        title: 'Archive item',
        hint: 'Esc to cancel · Enter to archive',
        bodyHTML: `<div>Archive <code>${escapeHTML(id)}</code>?</div>`,
        focusSelector: '#native-prompt-save',
        restoreFocusId: id,
        onSubmit: () => {
          closePrompt();
          outlineApply(row, 'outline:archive', { id }).then(() => {
            const li = row.closest('li');
            li && li.remove();
            agendaApplyVisibility();
          }).catch((err) => {
            setOutlineStatus('Error: ' + (err && err.message ? err.message : 'archive failed'));
          });
        },
      });
      return true;
    }
    if (key === 'z' && !ev.shiftKey) {
      ev.preventDefault();
      const hasChildren = String(row.dataset.hasChildren || '') === 'true';
      if (!hasChildren || !id) return true;
      const set = loadSet();
      if (set.has(id)) set.delete(id);
      else set.add(id);
      saveAgendaCollapsedSet(root, set);
      applyAgendaCollapsed(root, set);
      row.focus?.();
      return true;
    }
    if (key === 'z' && ev.shiftKey) {
      ev.preventDefault();
      const set = loadSet();
      const ids = Array.from(document.querySelectorAll('[data-agenda-row][data-has-children="true"]')).map((el) => String(el.dataset.id || '').trim()).filter(Boolean);
      const anyExpanded = ids.some((x) => !set.has(x));
      const next = new Set();
      if (anyExpanded) ids.forEach((x) => next.add(x));
      saveAgendaCollapsedSet(root, next);
      applyAgendaCollapsed(root, next);
      row.focus?.();
      return true;
    }
    if (!ev.altKey && (key === 'arrowright' || key === 'right' || key === 'l' || (ev.ctrlKey && key === 'f'))) {
      ev.preventDefault();
      const hasChildren = String(row.dataset.hasChildren || '') === 'true';
      if (!hasChildren || !id) return true;
      const set = loadSet();
      const collapsed = set.has(id);
      if (collapsed) {
        set.delete(id);
        saveAgendaCollapsedSet(root, set);
        applyAgendaCollapsed(root, set);
        row.focus?.();
        return true;
      }
      // Move to first child if next row is deeper.
      if (idx >= 0 && idx+1 < rows.length) {
        const nextDepth = parseInt(String(rows[idx+1].dataset.depth || '0'), 10) || 0;
        if (nextDepth > depth) rows[idx+1].focus?.();
      }
      return true;
    }
    if (!ev.altKey && (key === 'arrowleft' || key === 'left' || key === 'h' || (ev.ctrlKey && key === 'b'))) {
      ev.preventDefault();
      if (idx <= 0 || depth <= 0) return true;
      const want = depth - 1;
      for (let i = idx - 1; i >= 0; i--) {
        const d = parseInt(String(rows[i].dataset.depth || '0'), 10) || 0;
        if (d === want) {
          rows[i].focus?.();
          break;
        }
      }
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
      copyTextToClipboard(itemId + workspaceFlag()).then(() => {
        setOutlineStatus('Copied item ref');
        setTimeout(() => setOutlineStatus(''), 1200);
      }).catch((err) => {
        setOutlineStatus('Error: ' + (err && err.message ? err.message : 'copy failed'));
      });
      return true;
    }
    if (key === 'y' && ev.shiftKey) {
      ev.preventDefault();
      copyTextToClipboard('clarity items show ' + itemId + workspaceFlag()).then(() => {
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
    if (key === 'm') {
      ev.preventDefault();
      openMoveOutlinePickerForItemPage(root);
      return true;
    }
    if (key === 'r') {
      ev.preventDefault();
      if ((root.dataset.canEdit || '') !== 'true') return true;
      const outlineId = String(root.dataset.outlineId || '').trim();
      openPrompt({
        title: 'Archive item',
        hint: 'Esc to cancel · Enter to archive',
        bodyHTML: `<div>Archive <code>${escapeHTML(itemId)}</code>?</div>`,
        focusSelector: '#native-prompt-save',
        restoreFocusId: itemId,
        onSubmit: () => {
          closePrompt();
          outlineApply(root, 'outline:archive', { id: itemId }).then(() => {
            if (outlineId) window.location.href = '/outlines/' + encodeURIComponent(outlineId) + '?ok=archived';
            else window.location.href = '/projects';
          }).catch((err) => {
            setOutlineStatus('Error: ' + (err && err.message ? err.message : 'archive failed'));
            setTimeout(() => setOutlineStatus(''), 2400);
          });
        },
      });
      return true;
    }
    return false;
  };

  const itemSide = {
    open: false,
    kind: '',
    restoreFocusId: '',
  };

  const itemSidePane = () => document.getElementById('item-side-pane');

  const applyItemSideState = () => {
    const pane = itemSidePane();
    const main = document.getElementById('clarity-main');
    if (!pane || !main) return;
    if (!itemSide.open) {
      delete main.dataset.itemSideOpen;
      pane.style.display = 'none';
      pane.removeAttribute('data-kb-scope-active');
      return;
    }
    const kind = String(itemSide.kind || 'comments').trim() || 'comments';
    main.dataset.itemSideOpen = 'true';
    pane.style.display = 'block';
    pane.dataset.kbScopeActive = 'true';

    const title = document.getElementById('item-side-title');
    if (title) title.textContent = kind === 'worklog' ? 'Worklog' : (kind === 'history' ? 'History' : 'Comments');

    pane.querySelectorAll('[data-item-side-panel]').forEach((el) => {
      el.style.display = (String(el.dataset.itemSidePanel || '') === kind) ? 'block' : 'none';
    });
  };

  const itemSideOpen = (kind, restoreFocusId) => {
    const pane = itemSidePane();
    const main = document.getElementById('clarity-main');
    if (!pane || !main) return;
    kind = String(kind || '').trim();
    if (!kind) kind = 'comments';

    itemSide.open = true;
    itemSide.kind = kind;
    itemSide.restoreFocusId = String(restoreFocusId || '').trim();
    applyItemSideState();

    // Focus the first row inside the panel (or the textarea).
    const first = pane.querySelector('[data-item-side-panel="' + CSS.escape(kind) + '"] [data-kb-item]') ||
      pane.querySelector('[data-item-side-panel="' + CSS.escape(kind) + '"] textarea');
    first && first.focus && first.focus();
  };

  const itemSideClose = () => {
    const restoreId = itemSide.restoreFocusId;
    itemSide.open = false;
    itemSide.kind = '';
    itemSide.restoreFocusId = '';
    applyItemSideState();
    if (restoreId) {
      const el = document.querySelector('[data-focus-id="' + CSS.escape(restoreId) + '"]');
      el && el.focus && el.focus();
    }
  };

  const postItemComment = (itemId, body, replyTo) => {
    const id = String(itemId || '').trim();
    body = String(body || '').trim();
    replyTo = String(replyTo || '').trim();
    if (!id || !body) return Promise.resolve();
    const params = new URLSearchParams({ body });
    if (replyTo) params.set('replyTo', replyTo);
    return fetch('/items/' + encodeURIComponent(id) + '/comments', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: params.toString(),
    });
  };

  const commentQuote = (meta) => {
    const author = String(meta?.author || '').trim();
    const ts = String(meta?.ts || '').trim();
    const body = String(meta?.body || '').trim();
    const head = (author || ts) ? (author + (ts ? (' · ' + ts) : '')) : '';
    const lines = body.split('\n').map((l) => '> ' + l);
    return (head ? ('> ' + head + '\n') : '') + lines.join('\n') + '\n\n';
  };

  const openReplyPromptForFocusedComment = () => {
    const root = itemPageRoot();
    if (!root) return;
    const itemId = String(root.dataset.itemId || '').trim();
    if (!itemId) return;
    const a = document.activeElement;
    const row = a && typeof a.closest === 'function' ? a.closest('[data-comment-id]') : null;
    const commentId = String(row?.dataset?.commentId || '').trim();
    if (!commentId) return;
    const li = row.closest('li');
    const bodyEl = li ? li.querySelector('.comment-body') : null;
    const body = bodyEl ? String(bodyEl.innerText || bodyEl.textContent || '') : '';
    const author = String(row?.dataset?.commentAuthor || '').trim();
    const ts = String(row?.dataset?.commentTs || '').trim();
    const initial = commentQuote({ author, ts, body });
    openPrompt({
      title: 'Reply',
      hint: 'Esc to close · Ctrl+Enter to post',
      bodyHTML: `<textarea id="native-prompt-textarea" rows="8" style="width:100%;font-family:var(--mono);">${escapeHTML(initial)}</textarea>`,
      focusSelector: '#native-prompt-textarea',
      onSubmit: () => {
        const modal = document.getElementById('native-prompt-modal');
        const ta = modal ? modal.querySelector('#native-prompt-textarea') : null;
        const txt = ta ? String(ta.value || '').trim() : '';
        if (!txt) return;
        closePrompt();
        postItemComment(itemId, txt, commentId).catch(() => {});
      },
    });
  };

  const handleItemSideKeydown = (ev, rawKey, key) => {
    if (!itemPageRoot()) return false;
    if (!itemSide.open) return false;
    // Always allow closing the side pane, even while typing in inputs.
    if (key === 'escape' || (ev.ctrlKey && key === 'g')) {
      ev.preventDefault();
      itemSideClose();
      return true;
    }
    if (isTypingTarget(ev.target)) return false;
    if (rawKey === 'R' && itemSide.kind === 'comments') {
      ev.preventDefault();
      openReplyPromptForFocusedComment();
      return true;
    }
    if (key === 'enter' && itemSide.kind === 'comments') {
      const a = document.activeElement;
      const isComment = a && typeof a.closest === 'function' ? a.closest('[data-comment-id]') : null;
      if (isComment) {
        ev.preventDefault();
        openReplyPromptForFocusedComment();
        return true;
      }
    }
    return false;
  };

  // Ctrl+Enter in the side pane textarea submits the nearest form (TUI parity).
  document.addEventListener('keydown', (ev) => {
    const root = itemPageRoot();
    if (!root) return;
    if (!itemSide.open) return;
    if (!(ev.ctrlKey && (ev.key === 'Enter' || ev.key === 'enter'))) return;
    const t = ev && ev.target;
    if (!t || !isTypingTarget(t)) return;
    const form = t.closest ? t.closest('form') : null;
    if (!form) return;
    if (!form.closest('#item-side-pane')) return;
    ev.preventDefault();
    if (typeof form.requestSubmit === 'function') form.requestSubmit();
    else if (typeof form.submit === 'function') form.submit();
  }, { capture: true });

  const handleActionPaletteKeydown = (ev) => {
    if (!actionPalette.open) return false;
    const key = (ev.key || '').toLowerCase();
    const raw = String(ev.key || '');
    if (key === 'escape') {
      ev.preventDefault();
      popActionPanel();
      return true;
    }
    if (key === 'backspace') {
      ev.preventDefault();
      popActionPanel();
      return true;
    }
    if (ev.ctrlKey && key === 'g') {
      ev.preventDefault();
      closeActionPalette();
      return true;
    }
    if (!ev.ctrlKey && !ev.altKey && (raw === 'g' || raw === 'G')) {
      ev.preventDefault();
      pushActionPanel('nav');
      return true;
    }
    if (key && key.length === 1 && !ev.ctrlKey && !ev.altKey) {
      const opts = actionPalette.options || [];
      // Prefer exact match (case-sensitive) so `C` doesn't accidentally select `c`.
      let idx = opts.findIndex((o) => String(o?.key || '') === raw);
      if (idx < 0) {
        idx = opts.findIndex((o) => {
          const k = String(o?.key || '');
          if (!k) return false;
          return k.toLowerCase() === key;
        });
      }
      if (idx >= 0) {
        ev.preventDefault();
        actionPalette.idx = idx;
        runSelectedAction({ trigger: 'key' });
        return true;
      }
    }
    if (key === 'enter') {
      ev.preventDefault();
      runSelectedAction({ trigger: 'enter' });
      return true;
    }
    if (key === 'arrowdown' || key === 'down' || key === 'j' || (ev.ctrlKey && key === 'n')) {
      ev.preventDefault();
      const n = actionPalette.options.length || 0;
      if (n > 0) actionPalette.idx = (actionPalette.idx + 1) % n;
      renderActionPalette();
      return true;
    }
    if (key === 'arrowup' || key === 'up' || key === 'k' || (ev.ctrlKey && key === 'p')) {
      ev.preventDefault();
      const n = actionPalette.options.length || 0;
      if (n > 0) actionPalette.idx = (actionPalette.idx - 1 + n) % n;
      renderActionPalette();
      return true;
    }
    return true;
  };

  const handleCaptureKeydown = (ev) => {
    if (!captureState.open) return false;
    const rawKey = String(ev.key || '');
    const key = rawKey.toLowerCase();
    if (ev.ctrlKey && key === 'g') {
      ev.preventDefault();
      closeCaptureModal();
      return true;
    }
    if (ev.ctrlKey && key === 't') {
      ev.preventDefault();
      captureOpenTemplates();
      return true;
    }
    if (key === 'escape') {
      ev.preventDefault();
      if (captureState.phase === 'templates-add' || captureState.phase === 'templates-delete') {
        captureState.phase = 'templates';
        renderCapture();
        return true;
      }
      if (captureState.phase === 'templates') {
        captureState.phase = 'select';
        renderCapture();
        return true;
      }
      closeCaptureModal();
      return true;
    }

    if (captureState.phase === 'templates') {
      const rows = Array.isArray(captureState.templates) ? captureState.templates : [];
      if (key === 'enter') {
        ev.preventDefault();
        captureState.phase = 'select';
        renderCapture();
        return true;
      }
      if (key === 'a' && !ev.ctrlKey && !ev.altKey) {
        ev.preventDefault();
        captureStartAddTemplate();
        return true;
      }
      if (key === 'd' && !ev.ctrlKey && !ev.altKey) {
        ev.preventDefault();
        const cur = rows[captureState.templatesIdx] || null;
        const kp = String(cur?.keyPath || '').trim();
        if (kp) captureConfirmDeleteTemplate(kp);
        return true;
      }
      if (key === 'arrowdown' || key === 'down' || key === 'j' || (ev.ctrlKey && key === 'n')) {
        ev.preventDefault();
        if (rows.length) captureState.templatesIdx = Math.min(rows.length - 1, (captureState.templatesIdx || 0) + 1);
        renderCapture();
        return true;
      }
      if (key === 'arrowup' || key === 'up' || key === 'k' || (ev.ctrlKey && key === 'p')) {
        ev.preventDefault();
        if (rows.length) captureState.templatesIdx = Math.max(0, (captureState.templatesIdx || 0) - 1);
        renderCapture();
        return true;
      }
      return true;
    }

    if (captureState.phase === 'templates-delete') {
      if (key === 'enter' || (ev.ctrlKey && key === 's')) {
        ev.preventDefault();
        submitCapture();
        return true;
      }
      return true;
    }

    if (captureState.phase === 'templates-add') {
      const body = document.getElementById('native-capture-body');
      if (ev.ctrlKey && key === 's') {
        ev.preventDefault();
        submitCapture();
        return true;
      }
      if (key === 'enter') {
        const t = ev && ev.target;
        const tag = (t && t.tagName ? String(t.tagName).toLowerCase() : '');
        if (tag !== 'textarea') {
          ev.preventDefault();
          submitCapture();
          return true;
        }
      }
      if (key === 'e') {
        ev.preventDefault();
        body?.querySelector('#native-capture-tmpl-name')?.focus?.();
        return true;
      }
      if (key === 'k') {
        ev.preventDefault();
        body?.querySelector('#native-capture-tmpl-keys')?.focus?.();
        return true;
      }
      if (key === 'm') {
        ev.preventDefault();
        body?.querySelector('#native-capture-tmpl-outline')?.focus?.();
        return true;
      }
      return true;
    }

    if (captureState.phase === 'select') {
      if (key === 'backspace') {
        ev.preventDefault();
        if (captureState.prefix.length) {
          captureState.prefix = captureState.prefix.slice(0, captureState.prefix.length - 1);
          captureState.idx = 0;
          renderCapture();
        }
        return true;
      }
      if (key === 'enter') {
        ev.preventDefault();
        submitCapture();
        return true;
      }
      if (key === 'arrowdown' || key === 'down' || key === 'j' || (ev.ctrlKey && key === 'n')) {
        ev.preventDefault();
        const n = (captureState.list || []).length;
        if (n > 0) captureState.idx = (captureState.idx + 1) % n;
        renderCapture();
        return true;
      }
      if (key === 'arrowup' || key === 'up' || key === 'k' || (ev.ctrlKey && key === 'p')) {
        ev.preventDefault();
        const n = (captureState.list || []).length;
        if (n > 0) captureState.idx = (captureState.idx - 1 + n) % n;
        renderCapture();
        return true;
      }
      if (!ev.ctrlKey && !ev.altKey && rawKey && rawKey.length === 1 && rawKey !== ' ') {
        ev.preventDefault();
        const node = captureNodeAtPrefix();
        const child = node && node.children ? node.children[rawKey] : null;
        if (!child) {
          setCaptureError('No template for key: ' + rawKey);
          return true;
        }
        setCaptureError('');
        captureState.prefix = [...(captureState.prefix || []), rawKey];
        captureState.idx = 0;
        renderCapture();
        const next = captureNodeAtPrefix();
        if (next && next.template && (!next.children || Object.keys(next.children).length === 0)) {
          captureStartDraft(next.template);
        }
        return true;
      }
      return true;
    }

    // Draft phase.
    const body = document.getElementById('native-capture-body');
    if (key === 'e') {
      ev.preventDefault();
      body?.querySelector('#native-capture-draft-title')?.focus?.();
      return true;
    }
    if (rawKey === 'D') {
      ev.preventDefault();
      body?.querySelector('#native-capture-draft-desc')?.focus?.();
      return true;
    }
    if (key === 'm') {
      ev.preventDefault();
      body?.querySelector('#native-capture-draft-outline')?.focus?.();
      return true;
    }
    if (key === ' ') {
      ev.preventDefault();
      body?.querySelector('#native-capture-draft-status')?.focus?.();
      return true;
    }
    if (key === 'enter') {
      const t = ev && ev.target;
      const tag = (t && t.tagName ? String(t.tagName).toLowerCase() : '');
      if (tag === 'textarea' && !ev.ctrlKey) return true;
      if (tag === 'textarea' && ev.ctrlKey) {
        ev.preventDefault();
        submitCapture();
        return true;
      }
      ev.preventDefault();
      submitCapture();
      return true;
    }
    return true;
  };

  const handleKeydown = (ev) => {
    if (ev.defaultPrevented) return;
    if (ev.metaKey) return;
    if (handleCaptureKeydown(ev)) return;
    if (handleTagsPickerKeydown(ev)) return;
    if (handleMoveOutlinePickerKeydown(ev)) return;
    if (handleActionPaletteKeydown(ev)) return;
    if (handlePromptKeydown(ev)) return;
    if (handleAssigneePickerKeydown(ev)) return;
    if (handleStatusPickerKeydown(ev)) return;
    if (handleOutlineStatusesKeydown(ev)) return;

    const rawKey = String(ev.key || '');
    const key = rawKey.toLowerCase();

    if (handleItemSideKeydown(ev, rawKey, key)) return;
    if (isTypingTarget(ev.target)) return;
    if (key === '?' || key === 'x') {
      ev.preventDefault();
      openActionPalette();
      return;
    }

    if (key === 'g') {
      ev.preventDefault();
      openNavPanel();
      return;
    }

    // Agenda (TUI parity): 'a' is global except inside outline and item; 'A' is always available.
    if ((rawKey === 'A') || (key === 'a' && ev.shiftKey)) {
      ev.preventDefault();
      openAgendaPanel();
      return;
    }
    if (key === 'a') {
      const inItem = !!itemPageRoot();
      const nativeRow = nativeRowFromEvent(ev);
      const inNativeOutline = !!nativeRow;
      const inOutlineComponent = eventTouchesOutlineComponent(ev);
      if (!inItem && !inNativeOutline && !inOutlineComponent) {
        ev.preventDefault();
        openAgendaPanel();
        return;
      }
      // Otherwise, 'a' is context-specific (assign) and handled below.
    }

    // Capture is lowercase `c` (TUI parity). Uppercase `C` is "Add comment".
    if (rawKey === 'C') {
      const itemRoot = itemPageRoot();
      const nativeRow = nativeRowFromEvent(ev);
      if (itemRoot) {
        ev.preventDefault();
        openItemTextPostPrompt(itemRoot, 'comment');
        return;
      }
      if (nativeRow) {
        ev.preventDefault();
        openTextPostPrompt(nativeRow, 'comment');
        return;
      }
    }

    if (rawKey === 'c' && !ev.shiftKey) {
      ev.preventDefault();
      openCaptureModal();
      return;
    }

    if (key === 'v') {
      const root = nativeOutlineRoot();
      if (!root) return;
      ev.preventDefault();
      cycleOutlineViewMode(root);
      return;
    }

    const inOutline = eventTouchesOutlineComponent(ev);
    if (inOutline) return;

    if (handleItemPageKeydown(ev, key)) return;

    // Item details (Enter activates focused field).
    if (key === 'enter' && itemPageRoot()) {
      const a = document.activeElement;
      const field = a && a.dataset ? String(a.dataset.itemField || '').trim() : '';
      if (field) {
        ev.preventDefault();
        const root = itemPageRoot();
        if (!root) return;
        switch (field) {
          case 'title':
            openItemTitlePrompt(root);
            return;
          case 'status':
            openStatusPickerForItemPage(root);
            return;
          case 'assign':
            openAssigneePickerForItemPage(root);
            return;
          case 'tags':
            openItemTagsPrompt(root);
            return;
          case 'due':
            openItemDatePrompt(root, 'due');
            return;
          case 'schedule':
            openItemDatePrompt(root, 'schedule');
            return;
          case 'priority':
            handleItemPageKeydown(new KeyboardEvent('keydown', { key: 'p' }), 'p');
            return;
          case 'onHold':
            handleItemPageKeydown(new KeyboardEvent('keydown', { key: 'o' }), 'o');
            return;
          case 'description':
            openItemDescriptionPrompt(root);
            return;
        }
      }
    }

    // Item side panels (comments/worklog/history): Enter on a "Related" row opens the side pane.
    if (key === 'enter' && itemPageRoot()) {
      const a = document.activeElement;
      const openKind = a && a.dataset ? String(a.dataset.itemSideOpen || '').trim() : '';
      if (openKind) {
        ev.preventDefault();
        itemSideOpen(openKind, String(a.dataset.focusId || '').trim());
        return;
      }
    }

    const nativeRow = nativeRowFromEvent(ev);
    if (handleOutlineColumnsKeydown(ev, rawKey, key)) return;
    if (handleNativeOutlineKeydown(ev, key, nativeRow)) return;

    const agendaRow = agendaRowFromEvent(ev);
    if (handleAgendaKeydown(ev, key, agendaRow)) return;

    if (handleProjectsAndOutlinesListKeydown(ev, key)) return;
    handleGlobalListKeydown(ev, key);
  };

  initTheme();
  initOutlineViewMode();
  initAgendaFilter();
  const ar = agendaRoot();
  if (ar) {
    ensureAgendaDefaultCollapse(ar);
    applyAgendaCollapsed(ar, loadAgendaCollapsedSet(ar));
  }
  document.addEventListener('keydown', handleKeydown, { capture: true });
})();
