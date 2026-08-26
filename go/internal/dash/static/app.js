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

  const state = {
    overview: null,
    services: [],
    prevKpi: {},
    users: [],
    sort: "newest",
    q: "",
    searchTimer: null,
  };

  /* ---------- fetch helpers ---------- */

  async function api(path) {
    const res = await fetch(path, { headers: { Accept: "application/json" } });
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
    hint.textContent = up + " из " + list.length + " в строю";
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

  function setKpi(id, value, deltaHtml, deltaClass) {
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
    const growthTxt = (growth > 0 ? "+" : "") + growth.toFixed(0).replace(".", ",") + " % к вчера";

    const d30 = o.downloads_30d || 0;
    const success = d30 ? (o.completed_30d / d30) * 100 : 0;

    setKpi("kpi-users", fmt.format(o.users), "всего аккаунтов");
    setKpi("kpi-new-today", fmt.format(o.new_today), growthTxt, growthCls);
    setKpi("kpi-downloads-today", fmt.format(o.downloads_today), "за 7 дней: " + fmt.format(o.downloads_7d));
    setKpi("kpi-active-now", fmt.format(o.active_now), "онлайн за 30 минут");
    setKpi("kpi-dau-mau", fmt.format(o.dau) + " / " + fmt.format(o.mau), null);
    setKpi("kpi-stickiness", "стикинес: " + (o.mau ? (o.dau / o.mau * 100).toFixed(1).replace(".", ",") : "0") + " %");
    setKpi("kpi-success", fmtPct(success), "ошибок: " + fmt.format(o.failed_30d), o.failed_30d > 0 ? "down" : "up");
  }

  /* ---------- timeline chart (canvas) ---------- */

  const chart = {
    canvas: null,
    ctx: null,
    dpr: 1,
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
    chart.canvas.height = 260 * chart.dpr;
    chart.ctx.setTransform(chart.dpr, 0, 0, chart.dpr, 0, 0);
  }

  function drawChart(points) {
    if (!chart.ctx || !points || !points.length) return;
    const { ctx } = chart;
    const W = chart.canvas.width / chart.dpr;
    const H = 260;
    const pad = { top: 14, right: 10, bottom: 24, left: 34 };

    ctx.clearRect(0, 0, W, H);

    const max = Math.max(1, ...points.map((p) => p.total));
    const iw = W - pad.left - pad.right;
    const ih = H - pad.top - pad.bottom;
    const step = iw / Math.max(1, points.length - 1);
    const x = (i) => pad.left + step * i;
    const y = (v) => pad.top + ih - (v / max) * ih;

    // grid lines
    ctx.strokeStyle = "rgba(30,42,55,0.8)";
    ctx.lineWidth = 1;
    ctx.fillStyle = "#8FA0B0";
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

    // area under "total"
    const grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + ih);
    grad.addColorStop(0, "rgba(42,171,238,0.35)");
    grad.addColorStop(1, "rgba(42,171,238,0)");
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.total)) : ctx.moveTo(x(i), y(p.total))));
    ctx.lineTo(x(points.length - 1), pad.top + ih);
    ctx.lineTo(x(0), pad.top + ih);
    ctx.closePath();
    ctx.fillStyle = grad;
    ctx.fill();

    // total line
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.total)) : ctx.moveTo(x(i), y(p.total))));
    ctx.strokeStyle = "#2AABEE";
    ctx.lineWidth = 2;
    ctx.stroke();

    // completed line
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.completed)) : ctx.moveTo(x(i), y(p.completed))));
    ctx.strokeStyle = "#34D399";
    ctx.lineWidth = 2;
    ctx.stroke();

    // failed line
    ctx.beginPath();
    points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p.failed)) : ctx.moveTo(x(i), y(p.failed))));
    ctx.strokeStyle = "#F87171";
    ctx.lineWidth = 2;
    ctx.stroke();

    // x labels
    ctx.fillStyle = "#8FA0B0";
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const labelEvery = Math.max(1, Math.ceil(points.length / 8));
    points.forEach((p, i) => {
      if (i % labelEvery !== 0 && i !== points.length - 1) return;
      ctx.fillText(p.day.slice(5), x(i), pad.top + ih + 6);
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
        return `<tr>
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
    setInterval(loadAll, REFRESH_MS);
    setInterval(loadTimeline, REFRESH_MS);

    $("refresh-btn").addEventListener("click", () => {
      loadAll();
      loadTimeline();
      loadUsers();
    });

    $("users-sort").addEventListener("change", (e) => {
      state.sort = e.target.value;
      loadUsers();
    });

    $("users-search").addEventListener("input", (e) => {
      state.q = e.target.value;
      clearTimeout(state.searchTimer);
      state.searchTimer = setTimeout(renderUsers, 250);
    });

    window.addEventListener("resize", () => {
      resizeCanvas();
      if (state.overview) loadTimeline();
    });
    resizeCanvas();

    loadAll();
    loadTimeline();
    loadUsers();
  }

  document.addEventListener("DOMContentLoaded", init);
})();
