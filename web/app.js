(function () {
  'use strict';

  // ───── theme ─────
  const root = document.documentElement;
  const saved = localStorage.getItem('fs-theme');
  if (saved === 'dark' || saved === 'light') {
    root.setAttribute('data-theme', saved);
  }

  const themeBtn = document.getElementById('theme-btn');
  if (themeBtn) {
    themeBtn.addEventListener('click', () => {
      const cur = root.getAttribute('data-theme');
      let next;
      if (cur === 'dark') next = 'light';
      else if (cur === 'light') next = 'dark';
      else next = matchMedia('(prefers-color-scheme: dark)').matches ? 'light' : 'dark';
      root.setAttribute('data-theme', next);
      localStorage.setItem('fs-theme', next);
    });
  }

  // ───── search ─────
  const search = document.getElementById('search');
  const rows = Array.from(document.querySelectorAll('.row[data-name]'));
  const empty = document.querySelector('.empty');
  const countEl = document.getElementById('count');

  function applyFilter(q) {
    q = (q || '').trim().toLowerCase();
    let visible = 0;
    rows.forEach(r => {
      const match = !q || r.dataset.name.includes(q);
      r.style.display = match ? '' : 'none';
      if (match) visible++;
    });
    if (countEl) {
      const total = rows.length;
      countEl.textContent = q
        ? `${visible} / ${total} 项匹配`
        : `共 ${total} 项`;
    }
  }

  if (search) {
    search.addEventListener('input', e => applyFilter(e.target.value));
    document.addEventListener('keydown', e => {
      if (e.key === '/' && document.activeElement !== search) {
        e.preventDefault();
        search.focus();
        search.select();
      } else if (e.key === 'Escape' && document.activeElement === search) {
        search.value = '';
        applyFilter('');
        search.blur();
      }
    });
  }

  // ───── sort ─────
  const list = document.getElementById('list');
  const head = list && list.querySelector('.list-head');
  let sortKey = 'name';
  let sortDir = 'asc';

  function sortRows() {
    const visible = rows.filter(r => r.style.display !== 'none');
    visible.sort((a, b) => {
      let va, vb;
      if (sortKey === 'name') {
        va = a.dataset.name.toLowerCase();
        vb = b.dataset.name.toLowerCase();
      } else if (sortKey === 'size') {
        va = parseInt(a.dataset.size, 10) || 0;
        vb = parseInt(b.dataset.size, 10) || 0;
      } else {
        va = parseInt(a.dataset.time, 10) || 0;
        vb = parseInt(b.dataset.time, 10) || 0;
      }
      if (va < vb) return sortDir === 'asc' ? -1 : 1;
      if (va > vb) return sortDir === 'asc' ?  1 : -1;
      return 0;
    });
    // directories first when sorting by name (preserve original order otherwise)
    if (sortKey === 'name') {
      visible.sort((a, b) => {
        const ad = a.querySelector('.name').getAttribute('href').endsWith('/') ? 0 : 1;
        const bd = b.querySelector('.name').getAttribute('href').endsWith('/') ? 0 : 1;
        if (ad !== bd) return ad - bd;
        return 0;
      });
    }
    // Re-append in new order (after list-head, before empty)
    visible.forEach(r => list.appendChild(r));
    if (empty) list.appendChild(empty);
  }

  function updateArrows() {
    head && head.querySelectorAll('.arrow').forEach(a => a.className = 'arrow');
    const btn = head && head.querySelector(`[data-sort="${sortKey}"] .arrow`);
    if (btn) btn.className = 'arrow ' + sortDir;
  }

  if (head) {
    head.addEventListener('click', e => {
      const btn = e.target.closest('button[data-sort]');
      if (!btn) return;
      const k = btn.dataset.sort;
      if (sortKey === k) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
      else { sortKey = k; sortDir = 'asc'; }
      updateArrows();
      sortRows();
    });
    updateArrows();
  }

  // ───── upload ─────
  const cfg = window.FS_CFG || {};
  const modal = document.getElementById('upload-modal');
  const openBtn = document.getElementById('upload-btn');
  const closeBtn = document.getElementById('upload-close');
  const dropZone = document.getElementById('drop-zone');
  const fileInput = document.getElementById('file-input');
  const uploadList = document.getElementById('upload-list');

  function showToast(msg, kind) {
    const t = document.getElementById('toast');
    if (!t) return;
    t.textContent = msg;
    t.className = 'toast ' + (kind || '');
    t.hidden = false;
    clearTimeout(t._timer);
    t._timer = setTimeout(() => { t.hidden = true; }, 2200);
  }

  if (openBtn && modal) {
    openBtn.addEventListener('click', () => { modal.hidden = false; });
  }
  if (closeBtn && modal) {
    closeBtn.addEventListener('click', () => { modal.hidden = true; uploadList.innerHTML = ''; });
  }
  if (modal) {
    modal.addEventListener('click', e => {
      if (e.target === modal) { modal.hidden = true; uploadList.innerHTML = ''; }
    });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && !modal.hidden) { modal.hidden = true; uploadList.innerHTML = ''; }
    });
  }

  function uploadFiles(files) {
    if (!files || !files.length) return;
    const fd = new FormData();
    Array.from(files).forEach(f => fd.append('files', f, f.name));
    const xhr = new XMLHttpRequest();
    xhr.open('POST', '/upload?path=' + encodeURIComponent(cfg.path || '/'), true);

    // Build list items
    const items = Array.from(files).map(f => {
      const li = document.createElement('li');
      li.innerHTML = `
        <span class="name" title="${escapeHtml(f.name)}">${escapeHtml(f.name)}</span>
        <div class="progress"><div></div></div>
        <span class="status">0%</span>
      `;
      uploadList.appendChild(li);
      return li;
    });

    xhr.upload.addEventListener('progress', e => {
      if (!e.lengthComputable) return;
      const p = (e.loaded / e.total * 100).toFixed(0);
      items.forEach(li => {
        li.querySelector('.progress > div').style.width = p + '%';
        li.querySelector('.status').textContent = p + '%';
      });
    });

    xhr.addEventListener('load', () => {
      try {
        const resp = JSON.parse(xhr.responseText);
        items.forEach((li, i) => {
          const r = (resp.results || [])[i] || { ok: true };
          if (r.ok) {
            li.querySelector('.progress > div').style.background = 'var(--success)';
            li.querySelector('.status').textContent = '✓ 完成';
          } else {
            li.querySelector('.progress > div').style.background = 'var(--danger)';
            li.querySelector('.status').classList.add('err');
            li.querySelector('.status').textContent = '✗ ' + (r.err || 'fail');
          }
        });
        const okCount = (resp.results || []).filter(r => r.ok).length;
        showToast(`已上传 ${okCount} 个文件`, okCount === files.length ? 'ok' : 'error');
        if (okCount > 0) setTimeout(() => location.reload(), 800);
      } catch (e) {
        items.forEach(li => {
          li.querySelector('.progress > div').style.background = 'var(--danger)';
          li.querySelector('.status').classList.add('err');
          li.querySelector('.status').textContent = '✗ 错误';
        });
        showToast('上传失败: ' + e.message, 'error');
      }
    });

    xhr.addEventListener('error', () => {
      items.forEach(li => {
        li.querySelector('.status').classList.add('err');
        li.querySelector('.status').textContent = '✗ 网络错误';
      });
      showToast('网络错误', 'error');
    });

    xhr.send(fd);
  }

  if (fileInput) {
    fileInput.addEventListener('change', e => uploadFiles(e.target.files));
  }
  if (dropZone) {
    ['dragenter', 'dragover'].forEach(ev =>
      dropZone.addEventListener(ev, e => { e.preventDefault(); dropZone.classList.add('drag'); }));
    ['dragleave', 'drop'].forEach(ev =>
      dropZone.addEventListener(ev, e => { e.preventDefault(); dropZone.classList.remove('drag'); }));
    dropZone.addEventListener('drop', e => uploadFiles(e.dataTransfer.files));
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  // ───── keyboard nav ─────
  // press 'r' to reload
  document.addEventListener('keydown', e => {
    if (e.key === 'r' && document.activeElement === document.body &&
        !e.ctrlKey && !e.metaKey) {
      location.reload();
    }
  });
})();