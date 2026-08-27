(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const REFRESH_MS = 30000;

  const PLATFORM_LABELS = {
    youtube: "YouTube",
    tiktok: "TikTok",
    instagram: "Instagram",
    x: "X",
    twitter: "X",
    spotify: "Spotify",
    soundcloud: "SoundCloud",
    pinterest: "Pinterest",
    yandexmusic: "Yandex Music",
    unknown: "Прочее",
  };
  const LANG_LABELS = { EN: "EN", RU: "RU", KK: "KK" };
  const BOT_LABELS = {
    saveinator: "saveinator",
    pinterest: "pinterest",
    pinterest_kz: "pinterest_kz",
    soundcloud: "soundcloud",
    spotify: "spotify",
    unknown: "—",
  };

  const fmt = new Intl.NumberFormat("ru-RU");
  const fmtPct = (v) => (Number.isFinite(v) ? v.toFixed(1).replace(".", ",") + " %" : "—");

  const STATUS_LABELS = {
    QUEUED: "в очереди",
    FETCHING: "получение",
    DOWNLOADING: "скачивание",
    TRANSCODING: "конвертация",
    SENDING: "отправка",
    COMPLETED: "отправлено",
    FAILED: "ошибка",
  };

  const state = {
    overview: null,
    services: [],
    prevKpi: {},
    users: [],
    sort: "newest",
    q: "",
    searchTimer: null,
  };

  /* ---------- auth ---------- */

  const BOT_USERNAME = "saveinator_bot"; // бот для Telegram Login Widget

  async function checkAuth() {
    try {
      const res = await fetch("/api/auth/status", { headers: { Accept: "application/json" } });
      const data = await res.json();
      return !!data.authed;
    } catch {
      return false;
    }
  }

  function showLogin() {
    $("app").hidden = true;
    $("login").hidden = false;
  }

  function showApp(userName) {
    $("login").hidden = true;
    $("app").hidden = false;
    const btn = $("logout-btn");
    btn.hidden = false;
    if (userName) {
      btn.textContent = "Выйти · " + userName;
    }
    // Панель только что стала видимой: канвас самописца нужно перемерить.
    resizeCanvas();
  }

  // Вызывается виджетом Telegram при успешном входе.
  window.onTelegramAuth = async function (user) {
    const err = $("login-err");
    err.hidden = true;
    try {
      // Telegram подписывает только те поля, которые реально есть у
      // пользователя (username/last_name/photo_url могут отсутствовать).
      // Пустые поля отправлять нельзя — они ломают HMAC-проверку.
      const params = new URLSearchParams();
      for (const key of ["id", "first_name", "last_name", "username", "photo_url", "auth_date", "hash"]) {
        if (user[key] !== undefined && user[key] !== null && user[key] !== "") {
          params.set(key, user[key]);
        }
      }
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: params,
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        err.textContent =
          res.status === 403
            ? "Этот Telegram-аккаунт не в списке операторов."
            : "Не удалось подтвердить вход: " + (data.error || res.status);
        err.hidden = false;
        return;
      }
      showApp(user.username ? "@" + user.username : user.first_name || "");
      loadAll();
      loadTimeline();
      loadUsers();
    } catch (e) {
      err.textContent = "Сеть недоступна — попробуйте ещё раз.";
      err.hidden = false;
    }
  };

  async function logout() {
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } catch {}
    showLogin();
    location.reload();
  }

  /* ---------- fetch helpers ---------- */

  async function api(path) {
    const res = await fetch(path, { headers: { Accept: "application/json" } });
    if (res.status === 401) {
      // сессия протухла — возвращаемся на экран входа
      showLogin();
      throw new Error("unauthorized");
    }
    if (!res.ok) throw new Error(path + " → " + res.status);
    return res.json();
  }

  /* ---------- header ---------- */

  function tickClock() {
    const el = $("utc-clock");
    if (el) el.textContent = new Date().toISOString().slice(11, 19) + " UTC";
  }

  function setUpdated(iso) {
    const el = $("last-updated");
    if (el) el.textContent = "обновлено " + new Date(iso).toLocaleTimeString("ru-RU") + " UTC";
  }

  /* ---------- services strip ---------- */

  function renderServices(list) {
    const grid = $("services-grid");
    const hint = $("services-hint");
    if (!list || !list.length) {
      grid.innerHTML = "";
      hint.textContent = "";
      return;
    }
    const up = list.filter((s) => s.up).length;
    const all = list.length;
    hint.textContent = up === all ? "все в строю" : up + " из " + all + " в строю";
    grid.innerHTML = list
      .map((s) => {
        const cls = s.up ? "up" : "down";
        const meta = s.up ? s.latency_ms + " ms" : "down";
        return `<div class="svc ${cls}">
          <span class="svc-dot" aria-hidden="true"></span>
          <span class="svc-name" title="${escapeHtml(s.name)}">${escapeHtml(s.name)}</span>
          <span class="svc-meta">${meta}</span>
        </div>`;
      })
      .join("");
  }

  /* ---------- KPI ---------- */

  function setKpi(id, value, deltaHtml, deltaClass, alarm) {
    const el = $(id);
    if (!el) return;
    const prev = state.prevKpi[id];
    el.textContent = value;
    if (deltaHtml !== undefined) {
      const d = el.nextElementSibling;
      if (d) {
        d.innerHTML = deltaHtml;
        d.className = "kpi-delta" + (deltaClass ? " " + deltaClass : "");
      }
    }
    if (alarm) {
      el.closest(".kpi").classList.add("kpi-alarm");
    } else {
      el.closest(".kpi").classList.remove("kpi-alarm");
    }
    if (prev !== undefined && prev !== value) {
      el.classList.remove("flash");
      void el.offsetWidth;
      el.classList.add("flash");
    }
    state.prevKpi[id] = value;
  }

  function renderOverview(o) {
    const growth = o.new_yesterday > 0
      ? ((o.new_today - o.new_yesterday) / o.new_yesterday) * 100
      : (o.new_today > 0 ? 100 : 0);
    const growthCls = growth > 0 ? "up" : growth < 0 ? "down" : "";
    const growthArrow = growth > 0 ? "▲ " : growth < 0 ? "▼ " : "";
    const growthTxt = growthArrow + (growth > 0 ? "+" : "") + growth.toFixed(0).replace(".", ",") + " % к вчера";

    const d30 = o.downloads_30d || 0;
    const success = d30 ? (o.completed_30d / d30) * 100 : 0;
    const failed = o.failed_30d || 0;

    setKpi("kpi-users", fmt.format(o.users), "всего аккаунтов");
    setKpi("kpi-new-today", fmt.format(o.new_today), growthTxt, growthCls);
    setKpi("kpi-downloads-today", fmt.format(o.downloads_today), "за 7 дней: " + fmt.format(o.downloads_7d));
    setKpi("kpi-active-now", fmt.format(o.active_now), "онлайн за 30 минут");
    setKpi("kpi-dau-mau", fmt.format(o.dau) + " / " + fmt.format(o.mau), null);
    setKpi("kpi-stickiness", "стикинес: " + (o.mau ? (o.dau / o.mau * 100).toFixed(1).replace(".", ",") : "0") + " %");
    setKpi("kpi-success", fmtPct(success), (failed > 0 ? "▼ " : "") + "ошибок: " + fmt.format(failed), failed > 0 ? "down" : "up", failed > 0);
  }

  /* ---------- timeline chart (canvas) ---------- */

  const chart = {
    canvas: null,
    ctx: null,
    dpr: 1,
  };

  const CHART_COLORS = {
    grid: "rgba(44, 57, 70, 0.55)",
    axis: "#5d6b78",
    area: "rgba(42, 171, 238, 0.14)",
    total: "#2aabee",
    ok: "#34d399",
    bad: "#f87171",
  };

  function initChart() {
    chart.canvas = $("timeline-chart");
    if (!chart.canvas) return;
    chart.ctx = chart.canvas.getContext("2d");
    chart.dpr = window.devicePixelRatio || 1;
  }

  function resizeCanvas() {
    if (!chart.canvas) return;
    const rect = chart.canvas.getBoundingClientRect();
    if (rect.width < 10) return;
    chart.canvas.width = Math.round(rect.width * chart.dpr);
    chart.canvas.height = Math.round(240 * chart.dpr);
    chart.ctx.setTransform(chart.dpr, 0, 0, chart.dpr, 0, 0);
  }

  function drawChart(points) {
    if (!chart.ctx || !points || !points.length) return;
    const { ctx } = chart;
    const W = chart.canvas.width / chart.dpr;
    const H = 240;
    const pad = { top: 14, right: 12, bottom: 26, left: 38 };

    ctx.clearRect(0, 0, W, H);

    const max = Math.max(1, ...points.map((p) => p.total));
    const iw = W - pad.left - pad.right;
    const ih = H - pad.top - pad.bottom;
    const step = iw / Math.max(1, points.length - 1);
    const x = (i) => pad.left + step * i;
    const y = (v) => pad.top + ih - (v / max) * ih;

    // сетка
    ctx.strokeStyle = CHART_COLORS.grid;
    ctx.lineWidth = 1;
    ctx.fillStyle = CHART_COLORS.axis;
    ctx.font = "10px JetBrains Mono, monospace";
    ctx.textAlign = "right";
    ctx.textBaseline = "middle";
    for (let g = 0; g <= 4; g++) {
      const gy = pad.top + (ih / 4) * g;
      ctx.beginPath();
      ctx.moveTo(pad.left, gy);
      ctx.lineTo(W - pad.right, gy);
      ctx.stroke();
      ctx.fillText(fmt.format(Math.round(max * (1 - g / 4))), pad.left - 6, gy);
    }

    // область под «все»
    const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + ih);
    grad.addColorStop(0, "rgba(42,171,238,0.28)");
    grad.addColorStop(1, "rgba(42,171,238,0)");
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.total)) : ctx.moveTo(x(i), y(p.total))));
    ctx.lineTo(x(points.length - 1), pad.top + ih);
    ctx.lineTo(x(0), pad.top + ih);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // «все»
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.total)) : ctx.moveTo(x(i), y(p.total))));
    ctx.strokeStyle = CHART_COLORS.total;
    ctx.lineWidth = 2;
    ctx.stroke();

    // «успешно»
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.completed)) : ctx.moveTo(x(i), y(p.completed))));
    ctx.strokeStyle = CHART_COLORS.ok;
    ctx.lineWidth = 2;
    ctx.stroke();

    // «ошибки»
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.failed)) : ctx.moveTo(x(i), y(p.failed))));
    ctx.strokeStyle = CHART_COLORS.bad;
    ctx.lineWidth = 2;
    ctx.stroke();

    // подписи дат
    ctx.fillStyle = CHART_COLORS.axis;
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const labelEvery = Math.max(1, Math.ceil(points.length / 8));
    points.forEach((p, i) => {
      if (i % labelEvery !== 0 && i !== points.length - 1) return;
      ctx.fillText(p.day.slice(5), x(i), pad.top + ih + 8);
    });
  }

  /* ---------- platforms ---------- */

  function renderPlatforms(list) {
    const host = $("platform-list");
    if (!host) return;
    if (!list || !list.length) {
      host.innerHTML = `<div class="empty">Пока нет данных за 30 дней.</div>`;
      return;
    }
    const max = Math.max(1, ...list.map((p) => p.downloads));
    host.innerHTML = list
      .map((p) => {
        const total = p.downloads || 0;
        const rate = total ? (p.completed / total) * 100 : 0;
        const poor = rate < 70;
        const width = Math.max(2, Math.round((total / max) * 100));
        return `<div class="plat-row">
          <span class="plat-name">${escapeHtml(platformLabel(p.platform))}</span>
          <span class="plat-bar"><span class="plat-fill${poor ? " poor" : ""}" style="width:${width}%"></span></span>
          <span class="plat-num">${fmt.format(total)}</span>
          <span class="plat-sub">успешность ${fmtPct(rate)} · юзеров ${fmt.format(p.users)}</span>
        </div>`;
      })
      .join("");
  }

  /* ---------- bots ---------- */

  function renderBots(bots) {
    const host = $("bots-grid");
    if (!host) return;
    const list = (bots || []).filter((b) => b.slug && b.slug !== "unknown");
    if (!list.length) {
      host.innerHTML = `<div class="empty">Нет данных по ботам.</div>`;
      return;
    }
    host.innerHTML = list
      .map((b) => {
        const fail = b.failed_30d || 0;
        return `<div class="bot-card">
          <div class="bot-name" title="${escapeHtml(b.slug)}">${escapeHtml(botLabel(b.slug))}</div>
          <div class="bot-stats">
            <div class="bot-stat"><b>${fmt.format(b.users || 0)}</b><span>юзеров</span></div>
            <div class="bot-stat"><b>${fmt.format(b.downloads || 0)}</b><span>скачиваний</span></div>
            <div class="bot-stat"><b>${fmt.format(b.downloads_30d || 0)}</b><span>за 30 дней</span></div>
          </div>
          ${fail ? `<span class="bot-fail">ошибок: ${fmt.format(fail)}</span>` : ""}
        </div>`;
      })
      .join("");
  }

  /* ---------- users table ---------- */

  function renderUsers() {
    const body = $("users-body");
    const empty = $("users-empty");
    if (!body) return;
    let list = state.users;
    const q = state.q.trim().toLowerCase();
    if (q) {
      list = list.filter((u) => {
        const hay = ((u.username || "") + " " + (u.first_name || "")).toLowerCase();
        return hay.includes(q);
      });
    }
    if (!list.length) {
      body.innerHTML = "";
      empty.hidden = false;
      return;
    }
    empty.hidden = true;
    body.innerHTML = list
      .map((u) => {
        const name = u.first_name || "";
        const handle = u.username ? "@" + u.username : "";
        const lang = LANG_LABELS[u.language] || u.language || "—";
        const bot = botLabel(u.bot_id || "unknown");
        const created = new Date(u.created_at).toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit", year: "2-digit" });
        const last = u.last_activity ? new Date(u.last_activity).toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit" }) : "—";
        const failCls = u.failed > 0 ? "fail-num" : "";
        return `<tr class="user-row" tabindex="0" data-id="${u.id}" data-handle="${escapeHtml(handle || name || u.id)}" title="История загрузок">
          <td class="user-id">${u.id}</td>
          <td class="user-handle">${escapeHtml(handle)} ${name ? `<span class="user-name">${escapeHtml(name)}</span>` : ""}</td>
          <td><span class="lang-chip">${lang}</span></td>
          <td><span class="bot-chip">${escapeHtml(bot)}</span></td>
          <td>${created}</td>
          <td class="num">${fmt.format(u.downloads)}</td>
          <td class="num ${failCls}">${fmt.format(u.failed)}</td>
          <td>${last}</td>
        </tr>`;
      })
      .join("");
  }

  /* ---------- user download history drawer ---------- */

  function openDrawer(user) {
    const drawer = $("user-drawer");
    const backdrop = $("drawer-backdrop");
    if (!drawer) return;
    $("drawer-sub").textContent = "ID " + user.id + (user.username ? " · @" + user.username : "");
    $("drawer-body").innerHTML = `<div class="empty">Загрузка…</div>`;
    backdrop.hidden = false;
    drawer.classList.add("open");
    drawer.setAttribute("aria-hidden", "false");
    document.body.style.overflow = "hidden";
    loadUserDownloads(user);
  }

  function closeDrawer() {
    const drawer = $("user-drawer");
    const backdrop = $("drawer-backdrop");
    if (!drawer) return;
    drawer.classList.remove("open");
    drawer.setAttribute("aria-hidden", "true");
    backdrop.hidden = true;
    document.body.style.overflow = "";
  }

  async function loadUserDownloads(user) {
    try {
      const data = await api("/api/users/" + user.id + "/downloads?limit=200");
      renderDownloads(user, data.downloads || []);
    } catch (err) {
      $("drawer-body").innerHTML = `<div class="empty">Не удалось загрузить историю: ${escapeHtml(String(err.message || err))}</div>`;
    }
  }

  function renderDownloads(user, list) {
    const body = $("drawer-body");
    if (!body) return;
    if (!list.length) {
      body.innerHTML = `<div class="empty">У этого пользователя пока нет загрузок.</div>`;
      return;
    }
    const total = list.length;
    const ok = list.filter((d) => d.status === "COMPLETED").length;
    const fail = list.filter((d) => d.status === "FAILED").length;
    const failLine = fail ? ` · <span class="fail-num">ошибок: ${fmt.format(fail)}</span>` : "";
    $("drawer-sub").innerHTML =
      `ID ${user.id}${user.username ? " · @" + escapeHtml(user.username) : ""} · ` +
      `загрузок: ${fmt.format(total)} · <span class="ok-num">успешно: ${fmt.format(ok)}</span>${failLine}`;

    body.innerHTML =
      `<div class="dl-list">` +
      list
        .map((d) => {
          const s = d.status || "UNKNOWN";
          const label = STATUS_LABELS[s] || s.toLowerCase();
          const plat = platformLabel(d.platform);
          const when = new Date(d.created_at).toLocaleString("ru-RU", {
            day: "2-digit", month: "2-digit", year: "2-digit", hour: "2-digit", minute: "2-digit",
          });
          const size = d.file_size_mb > 0 ? fmt.format(Math.round(d.file_size_mb * 10) / 10) + " MB" : "";
          const bot = d.bot_id && d.bot_id !== "unknown" ? botLabel(d.bot_id) : "";
          const url = d.url.startsWith("http") ? d.url : "https://" + d.url;
          return `<div class="dl-item s-${s}">
            <span class="dl-url"><a href="${escapeHtml(url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(d.url)}</a></span>
            <span class="dl-badge s-${s}">${label}</span>
            <span class="dl-meta">
              <span>${escapeHtml(plat)}</span>
              <span>${when}</span>
              ${size ? `<span>${size}</span>` : ""}
              ${bot ? `<span>${escapeHtml(bot)}</span>` : ""}
            </span>
            ${d.error_message ? `<span class="dl-err">${escapeHtml(d.error_message)}</span>` : ""}
          </div>`;
        })
        .join("") +
      `</div>`;
  }

  /* ---------- load ---------- */

  async function loadAll(silent) {
    const btn = $("refresh-btn");
    if (!silent && btn) btn.disabled = true;
    try {
      const [overview, services, platforms] = await Promise.all([
        api("/api/overview"),
        api("/api/services"),
        api("/api/platforms?days=30"),
      ]);
      state.overview = overview;
      state.services = services.services || [];
      renderOverview(overview);
      renderServices(state.services);
      renderPlatforms(platforms.platforms || []);
      renderBots(overview.bots);
      setUpdated(overview.updated_at);
    } catch (err) {
      console.error("dash load failed:", err);
      const grid = $("services-grid");
      if (grid && !state.services.length) {
        grid.innerHTML = `<div class="empty">Не удалось загрузить данные: ${escapeHtml(String(err.message || err))}</div>`;
      }
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  async function loadUsers() {
    try {
      const data = await api("/api/users?sort=" + encodeURIComponent(state.sort) + "&limit=200");
      state.users = data.users || [];
      renderUsers();
    } catch (err) {
      console.error("users load failed:", err);
    }
  }

  async function loadTimeline() {
    try {
      const data = await api("/api/downloads?days=14");
      drawChart(data.points || []);
    } catch (err) {
      console.error("timeline load failed:", err);
    }
  }

  /* ---------- utils ---------- */

  function escapeHtml(s) {
    return String(s ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function platformLabel(p) {
    return PLATFORM_LABELS[p] || p || "—";
  }

  function botLabel(slug) {
    return BOT_LABELS[slug] || slug || "—";
  }

  /* ---------- init ---------- */

  function init() {
    initChart();
    tickClock();
    setInterval(tickClock, 1000);

    checkAuth().then((authed) => {
      if (!authed) {
        showLogin();
        return;
      }
      showApp();
      loadAll();
      loadTimeline();
      loadUsers();
      setInterval(loadAll, REFRESH_MS);
      setInterval(loadTimeline, REFRESH_MS);
    });
    $("refresh-btn").addEventListener("click", () => {
      loadAll();
      loadTimeline();
      loadUsers();
    });

    $("logout-btn").addEventListener("click", logout);

    $("users-sort").addEventListener("change", (e) => {
      state.sort = e.target.value;
      loadUsers();
    });

    $("users-search").addEventListener("input", (e) => {
      state.q = e.target.value;
      clearTimeout(state.searchTimer);
      state.searchTimer = setTimeout(renderUsers, 250);
    });

    $("users-body").addEventListener("click", (e) => {
      const tr = e.target.closest("tr.user-row");
      if (tr) openDrawer(tr.dataset);
    });
    $("users-body").addEventListener("keydown", (e) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      const tr = e.target.closest("tr.user-row");
      if (tr) {
        e.preventDefault();
        openDrawer(tr.dataset);
      }
    });

    $("drawer-close").addEventListener("click", closeDrawer);
    $("drawer-backdrop").addEventListener("click", closeDrawer);
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") closeDrawer();
    });

    window.addEventListener("resize", () => {
      resizeCanvas();
      if (state.overview) loadTimeline();
    });
    resizeCanvas();
  }

  document.addEventListener("DOMContentLoaded", init);
})();
