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
    return Array.from(root.querySelectorAll('[data-outline-row]'));
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

  const statusPicker = {
    open: false,
    rowId: '',
    options: [],
    idx: 0,
    note: '',
    mode: 'list', // 'list' | 'note'
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
          <strong>Status</strong>
          <span class="dim" style="font-size:12px;">Esc to close</span>
        </div>
        <div id="native-status-note-wrap" style="margin-top:10px;display:none;">
          <div class="dim" style="font-size:12px;margin-bottom:6px;">Note required</div>
          <input id="native-status-note" type="text" placeholder="Add a note…" style="width:100%;" />
        </div>
        <div id="native-status-list" style="margin-top:10px;max-height:46vh;overflow:auto;"></div>
        <div id="native-status-hint" class="dim" style="margin-top:10px;font-size:12px;">Up/Down or Ctrl+P/N to move · Enter to pick</div>
      </div>
    `;
    document.body.appendChild(el);
    el.addEventListener('click', (ev) => {
      if (ev.target === el) closeStatusPicker();
    });
    return el;
  };

  const renderStatusPicker = () => {
    const modal = ensureStatusModal();
    const list = modal.querySelector('#native-status-list');
    const noteWrap = modal.querySelector('#native-status-note-wrap');
    const noteInput = modal.querySelector('#native-status-note');
    const hint = modal.querySelector('#native-status-hint');
    if (!list) return;

    const opts = statusPicker.options || [];
    const sel = opts[statusPicker.idx] || null;
    const needsNote = !!(sel && sel.requiresNote);
    const inNoteMode = statusPicker.mode === 'note';
    noteWrap.style.display = (inNoteMode && needsNote) ? 'block' : 'none';
    if (hint) {
      hint.textContent = inNoteMode ? 'Type note · Enter to save · Esc to go back' : 'Up/Down or Ctrl+P/N to move · Enter to pick';
    }
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
    statusPicker.options = opts;
    statusPicker.note = '';
    statusPicker.mode = 'list';
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
    statusPicker.open = false;
    statusPicker.rowId = '';
    statusPicker.options = [];
    statusPicker.idx = 0;
    statusPicker.note = '';
    statusPicker.mode = 'list';
    const modal = document.getElementById('native-status-modal');
    if (modal) modal.style.display = 'none';
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

    const root = nativeOutlineRoot();
    if (!root) return;
    const row = root.querySelector('[data-outline-row][data-id="' + CSS.escape(id) + '"]');
    if (row) nativeRowUpdateStatus(row, sel);
    closeStatusPicker();
    focusNativeRowById(id);

    // Persist async; SSE will converge state.
    outlineApply(root, 'outline:toggle', { id, to: statusID, note }).catch((err) => {
      setOutlineStatus('Error: ' + (err && err.message ? err.message : 'save failed'));
    });
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
    }, 450);
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

  document.addEventListener('keydown', (ev) => {
    if (ev.defaultPrevented) return;
    if (ev.metaKey) return;

    if (statusPicker.open) {
      const k = (ev.key || '').toLowerCase();
      if (k === 'escape') {
        ev.preventDefault();
        if (statusPicker.mode === 'note') {
          statusPicker.mode = 'list';
          statusPicker.note = '';
          renderStatusPicker();
          return;
        }
        closeStatusPicker();
        return;
      }
      if (k === 'enter') {
        ev.preventDefault();
        pickSelectedStatus();
        return;
      }
      if (statusPicker.mode === 'list') {
        if (k === 'arrowdown' || k === 'down' || k === 'j' || (ev.ctrlKey && k === 'n')) {
          ev.preventDefault();
          statusPicker.idx = Math.min((statusPicker.options.length || 1) - 1, statusPicker.idx + 1);
          renderStatusPicker();
          return;
        }
        if (k === 'arrowup' || k === 'up' || k === 'k' || (ev.ctrlKey && k === 'p')) {
          ev.preventDefault();
          statusPicker.idx = Math.max(0, statusPicker.idx - 1);
          renderStatusPicker();
          return;
        }
        // When modal is open, swallow other keys to avoid triggering app navigation.
        return;
      }
      // Note mode: let typing happen in the input, but keep Enter/Esc handled above.
      return;
    }

    if (isTypingTarget(ev.target)) return;

    const key = (ev.key || '').toLowerCase();
    const inOutline = eventTouchesOutlineComponent(ev);
    const nativeRow = nativeRowFromEvent(ev);

    if (key === '?') {
      ev.preventDefault();
      toggleHelp();
      return;
    }

    const now = Date.now();
    if (state.awaiting && now - state.awaitingAt > 1000) {
      clearAwaiting();
    }

    if (!state.awaiting) {
      if (key === 'g') {
        ev.preventDefault();
        state.awaiting = 'g';
        state.awaitingAt = now;
        return;
      }
      if (inOutline) {
        return;
      }
      if (nativeRow) {
        // Native outline-specific shortcuts.
        // Prefer `code` for Alt+J/K on macOS (Option modifies `key` into a symbol).
        if (ev.altKey && ev.code === 'KeyJ') {
          ev.preventDefault();
          nativeReorder(nativeRow, 'next');
          return;
        }
        if (ev.altKey && ev.code === 'KeyK') {
          ev.preventDefault();
          nativeReorder(nativeRow, 'prev');
          return;
        }
        if (ev.altKey && (key === 'arrowdown' || key === 'down')) {
          ev.preventDefault();
          nativeReorder(nativeRow, 'next');
          return;
        }
        if (ev.altKey && (key === 'arrowup' || key === 'up')) {
          ev.preventDefault();
          nativeReorder(nativeRow, 'prev');
          return;
        }
        if (key === 'j') {
          ev.preventDefault();
          nativeRowSibling(nativeRow, +1)?.focus?.();
          return;
        }
        if (key === 'k') {
          ev.preventDefault();
          nativeRowSibling(nativeRow, -1)?.focus?.();
          return;
        }
        if (key === 'arrowdown' || key === 'down' || (ev.ctrlKey && key === 'n')) {
          ev.preventDefault();
          nativeRowSibling(nativeRow, +1)?.focus?.();
          return;
        }
        if (key === 'arrowup' || key === 'up' || (ev.ctrlKey && key === 'p')) {
          ev.preventDefault();
          nativeRowSibling(nativeRow, -1)?.focus?.();
          return;
        }
        if (key === 'enter') {
          ev.preventDefault();
          const href = (nativeRow.dataset.openHref || '').trim();
          if (href) window.location.href = href;
          return;
        }
        if (key === 'e') {
          ev.preventDefault();
          startInlineEdit(nativeRow);
          return;
        }
        if (key === ' ') {
          ev.preventDefault();
          openStatusPicker(nativeRow);
          return;
        }
        // Indent/outdent (match TUI: ctrl+h/l and arrow variants; do not bind Tab/Shift+Tab).
        if (ev.ctrlKey && (key === 'l' || key === 'arrowright')) {
          ev.preventDefault();
          nativeIndent(nativeRow);
          return;
        }
        if (ev.ctrlKey && (key === 'h' || key === 'arrowleft')) {
          ev.preventDefault();
          nativeOutdent(nativeRow);
          return;
        }
        if (ev.altKey && (key === 'arrowright' || key === 'right')) {
          ev.preventDefault();
          nativeIndent(nativeRow);
          return;
        }
        if (ev.altKey && (key === 'arrowleft' || key === 'left')) {
          ev.preventDefault();
          nativeOutdent(nativeRow);
          return;
        }
        // Unhandled key in native outline: fall through.
      }
      if (key === 'j') {
        ev.preventDefault();
        moveFocus(+1);
        return;
      }
      if (key === 'k') {
        ev.preventDefault();
        moveFocus(-1);
        return;
      }
      if (key === 'enter') {
        ev.preventDefault();
        openFocused();
        return;
      }
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
  }, { capture: true });
})();
