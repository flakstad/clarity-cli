(() => {
  const state = {
    awaiting: null,
    awaitingAt: 0,
    restoreTimer: null,
  };

  const isTypingTarget = (el) => {
    if (!el) return false;
    const tag = (el.tagName || '').toLowerCase();
    if (tag === 'textarea' || tag === 'input' || tag === 'select') return true;
    if (el.isContentEditable) return true;
    return false;
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
    if (state.restoreTimer) return;
    state.restoreTimer = setTimeout(() => {
      state.restoreTimer = null;
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
    obs.observe(root, { subtree: true, childList: true });
  };
  startObserver();
  scheduleRestoreFocus();

  document.addEventListener('keydown', (ev) => {
    if (ev.defaultPrevented) return;
    if (ev.metaKey || ev.ctrlKey || ev.altKey) return;
    if (isTypingTarget(ev.target)) return;

    const key = (ev.key || '').toLowerCase();
    const inOutline = eventTouchesOutlineComponent(ev);

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
