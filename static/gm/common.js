/* GM 壳：顶栏 + 侧栏八模块 + 内容区；权限按钮由 data-perm 控制 */

async function ensureAuth() {
  const r = await fetch('/api/me');
  const j = await r.json();
  if (!j.ok) {
    location.href = '/login.html';
    return null;
  }
  window.__GM_ME = j;
  const el = document.getElementById('who');
  if (el) {
    const roleLabel = { super: '超管', gm: '正式GM', trainee: '见习GM' }[j.role] || j.role || '';
    el.textContent = roleLabel ? `${j.user}（${roleLabel}）` : j.user;
  }
  applyPermUI();
  return j;
}

function can(code) {
  const me = window.__GM_ME;
  if (!me) return false;
  if (me.role === 'super') return true;
  const perms = me.perms || [];
  if (!perms.length && me.role === 'gm') return true; // 过渡期：未下发 perms 时正式 GM 全开写（只读页除外）
  return perms.includes(code);
}

function applyPermUI() {
  document.querySelectorAll('[data-perm]').forEach((el) => {
    const code = el.getAttribute('data-perm');
    el.classList.toggle('hidden', !can(code));
  });
  document.querySelectorAll('[data-perm-any]').forEach((el) => {
    const codes = el.getAttribute('data-perm-any').split(',').map((s) => s.trim());
    el.classList.toggle('hidden', !codes.some((c) => can(c)));
  });
}

/** 打开/关闭弹层（避开 hidden 与其它样式冲突） */
function showModal(id) {
  const el = typeof id === 'string' ? document.getElementById(id) : id;
  if (!el) return;
  el.classList.remove('hidden');
  el.style.display = 'flex';
}
function hideModal(id) {
  const el = typeof id === 'string' ? document.getElementById(id) : id;
  if (!el) return;
  el.classList.add('hidden');
  el.style.display = 'none';
}

function renderShell(active) {
  const nav = [
    { href: '/dashboard.html', key: 'dashboard', label: '数据仪表盘' },
    { href: '/accounts.html', key: 'accounts', label: '玩家账号管理' },
    { href: '/pets.html', key: 'pets', label: '精灵管控' },
    { href: '/items.html', key: 'items', label: '道具货币发放' },
    { href: '/online.html', key: 'online', label: '在线玩家管理' },
    { href: '/mail.html', key: 'mail', label: '邮件系统' },
    { href: '/gm-accounts.html', key: 'gm', label: 'GM权限设置', perm: 'gm.manage' },
    { href: '/audit.html', key: 'audit', label: '操作日志' },
    { href: '/', key: 'grant', label: '快捷发放（旧）' },
  ];
  const navEl = document.getElementById('gm-nav');
  if (!navEl) return;
  navEl.innerHTML = nav
    .filter((n) => !n.perm || can(n.perm))
    .map(
      (n) =>
        `<a class="${n.key === active ? 'active' : ''}" href="${n.href}">${n.label}</a>`
    )
    .join('');
}

/** 高危红色确认：返回 true 才继续 */
function dangerConfirm(message) {
  return window.confirm(`【高危操作】\n\n${message}\n\n确定继续？`);
}

async function postJSON(url, body) {
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  return r.json();
}

async function logout() {
  await fetch('/api/logout', { method: 'POST' });
  location.href = '/login.html';
}

/** 搜玩家：空关键字返回最近登录若干 */
async function searchUsersAPI(q) {
  const r = await fetch('/api/users?q=' + encodeURIComponent(q || ''));
  const j = await r.json();
  return { ok: !!j.ok, list: j.users || j.list || [], error: j.error };
}

function userBriefFields(u) {
  return {
    userId: u.userId || u.user_id || u.UserID || u.id || 0,
    nickname: u.nickname || u.Nickname || '',
    email: u.email || u.Email || '',
    coins: u.coins != null ? u.coins : (u.Coins || 0),
    gold: u.gold != null ? u.gold : (u.Gold || 0),
    mapId: u.mapId != null ? u.mapId : (u.map_id || u.MapID || 0),
  };
}

/** 客户端分页：返回 { page, pages, total, slice } */
function paginateList(list, page, pageSize) {
  const arr = list || [];
  const size = Math.max(1, pageSize || 15);
  const total = arr.length;
  const pages = Math.max(1, Math.ceil(total / size) || 1);
  let p = Math.max(0, parseInt(page, 10) || 0);
  if (p >= pages) p = pages - 1;
  const start = p * size;
  return { page: p, pages, total, size, slice: arr.slice(start, start + size) };
}

/** 渲染分页条；onGo(deltaOrPage) 传 ±1 或绝对页码 */
function renderPager(el, page, pages, total, onGo, opts) {
  if (!el) return;
  const o = opts || {};
  if (!total) {
    el.innerHTML = '';
    return;
  }
  el.innerHTML = `
    <span class="pager-info">第 ${page + 1} / ${pages} 页 · 共 ${total} ${o.unit || '条'}</span>
    <button type="button" class="ghost" ${page <= 0 ? 'disabled' : ''} data-pager="-1">上一页</button>
    <button type="button" class="ghost" ${page >= pages - 1 ? 'disabled' : ''} data-pager="1">下一页</button>
  `;
  el.querySelectorAll('[data-pager]').forEach((btn) => {
    btn.onclick = () => {
      if (btn.disabled) return;
      onGo && onGo(parseInt(btn.getAttribute('data-pager'), 10));
    };
  });
}

/** 渲染可点选玩家列表；支持分页 state={page,pageSize} */
function renderUserList(el, users, selectedUid, onPick, pagerEl, state) {
  if (!el) return;
  const list = (users || []).map(userBriefFields);
  const st = state || { page: 0, pageSize: 12 };
  const pg = paginateList(list, st.page, st.pageSize || 12);
  st.page = pg.page;
  if (!list.length) {
    el.innerHTML = '<div class="hint" style="padding:12px">无结果，换关键词试试</div>';
    if (pagerEl) pagerEl.innerHTML = '';
    return;
  }
  el.innerHTML = pg.slice.map((u) => {
    const active = Number(selectedUid) === Number(u.userId) ? ' active' : '';
    const nick = u.nickname || '未命名';
    return `<div class="user-item${active}" data-uid="${u.userId}">
      <strong>${escapeHtml(nick)}</strong>
      <small>UID ${u.userId} · ${escapeHtml(u.email || '—')} · 豆${u.coins} / 金${u.gold}</small>
    </div>`;
  }).join('');
  el.querySelectorAll('.user-item').forEach((node) => {
    node.onclick = () => onPick && onPick(Number(node.getAttribute('data-uid')));
  });
  if (pagerEl) {
    renderPager(pagerEl, pg.page, pg.pages, pg.total, (delta) => {
      st.page = Math.max(0, Math.min(pg.pages - 1, st.page + delta));
      renderUserList(el, users, selectedUid, onPick, pagerEl, st);
    }, { unit: '人' });
  }
}

function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/** 账号条 HTML：可点进详情 / 精灵管控 */
function accountBarHTML(u, opts) {
  const o = opts || {};
  if (!u || !u.userId) return '';
  const nick = escapeHtml(u.nickname || '未命名');
  const email = escapeHtml(u.email || '—');
  const online = o.online === true ? '在线' : (o.online === false ? '离线' : '');
  const onlineCls = o.online === true ? 'color:var(--ok)' : 'color:var(--muted)';
  const extra = o.extra || '';
  return `<div class="account-bar" id="${o.id || 'accountBar'}">
    <div><a href="/user.html?uid=${u.userId}"><strong>${nick}</strong></a>
      <span class="meta">UID ${u.userId}</span></div>
    <div class="meta">${email}</div>
    <div class="meta">豆 ${u.coins != null ? u.coins : '—'} · 金 ${u.gold != null ? u.gold : '—'}</div>
    ${online ? `<div style="${onlineCls}">${online}${o.liveMapId != null ? ' · 地图' + o.liveMapId : ''}</div>` : ''}
    ${extra}
    <div style="margin-left:auto;display:flex;gap:8px;flex-wrap:wrap">
      <a class="ghost" href="/user.html?uid=${u.userId}" style="padding:5px 10px;border:1px solid var(--line);border-radius:8px">账号详情</a>
      <a class="ghost" href="/pets.html?uid=${u.userId}" style="padding:5px 10px;border:1px solid var(--line);border-radius:8px">精灵清单</a>
      <a class="ghost" href="/accounts.html" style="padding:5px 10px;border:1px solid var(--line);border-radius:8px">账号管理</a>
    </div>
  </div>`;
}
