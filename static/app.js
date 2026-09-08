/* 自动追番管理界面：订阅、条目、下载状态。全部请求走本地 API，无外部依赖。 */
"use strict";

const $ = (s) => document.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
/* Lucide 图标统一走本地 sprite（static/icons.svg），动态渲染的内容也用这个函数产出 */
const icon = (name, cls = "") => `<svg class="ic${cls ? " " + cls : ""}" aria-hidden="true"><use href="/static/icons.svg#i-${name}"></use></svg>`;

const state = {
  view: "animes",
  statusFilter: "全部",
  search: "",
  feeds: [],
  items: [],
  animes: [],
  animeDetail: null,
  editingFeedId: null,
  editingFeedEnabled: true,
};

const MODE_LABEL = { direct: "本地直链", aria2: "Aria2", qbittorrent: "qBittorrent", transmission: "Transmission" };

/* ---- 状态归类 ---- */
const CHIP_DEFS = [
  { key: "全部", match: () => true },
  { key: "待下载", match: (s) => s === "待下载" || s === "等待下载" },
  { key: "下载中", match: (s) => s === "下载中" || s.startsWith("已发送到") },
  { key: "已保存", match: (s) => s === "已保存" },
  { key: "下载失败", match: (s) => s === "下载失败" },
  { key: "等待下载器", match: (s) => s === "等待下载器" },
];
let globalCounts = {};

function badgeInfo(status) {
  if (status === "已保存") return { cls: "saved", text: "已保存", ico: "circle-check" };
  if (status === "下载中" || status.startsWith("已发送到")) return { cls: "downloading", text: status === "下载中" ? "下载中" : status, ico: "loader-circle", spin: true };
  if (status === "下载失败") return { cls: "failed", text: "下载失败", ico: "circle-alert" };
  if (status === "等待下载器") return { cls: "waiting", text: "等待下载器", ico: "hourglass" };
  if (status === "等待下载" || status === "待下载") return { cls: "pending", text: "待下载", ico: "clock" };
  return { cls: "pending", text: status, ico: "circle-dashed" };
}

function badgeHTML(badge, suffix = "") {
  return `<span class="badge ${badge.cls}">${icon(badge.ico, "ic-sm" + (badge.spin ? " spin" : ""))}<span>${esc(badge.text)}${suffix}</span></span>`;
}

function formatTime(raw) {
  if (!raw) return "暂无时间";
  const t = new Date(raw);
  if (isNaN(t.getTime())) return raw;
  const pad = (n) => String(n).padStart(2, "0");
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())} ${pad(t.getHours())}:${pad(t.getMinutes())}`;
}

/* ---- Toast ---- */
function toast(message, isError = false) {
  const el = document.createElement("div");
  el.className = "toast" + (isError ? " error" : "");
  el.innerHTML = icon(isError ? "triangle-alert" : "circle-check", "ic-md");
  const text = document.createElement("span");
  text.textContent = message;
  el.appendChild(text);
  $("#toasts").appendChild(el);
  setTimeout(() => {
    el.classList.add("leaving");
    setTimeout(() => el.remove(), 240);
  }, 4200);
}

/* 会话失效时只跳转一次，避免 5 秒轮询反复触发 */
let redirectingToLogin = false;

async function api(url, options) {
  const res = await fetch(url, options);
  if (res.status === 401 && !redirectingToLogin) {
    redirectingToLogin = true;
    location.href = "/login?next=" + encodeURIComponent(location.pathname);
    throw new Error("登录已过期，正在跳转到登录页");
  }
  if (!res.ok) {
    let detail = await res.text();
    try { detail = (await res.clone().json()).detail || detail; } catch {}
    throw new Error(detail || `请求失败 HTTP ${res.status}`);
  }
  const type = res.headers.get("Content-Type") || "";
  return type.includes("json") ? res.json() : res.text();
}

/* ---- 确认弹窗 ---- */
let confirmResolve = null;
function confirmDialog(title, text) {
  $("#confirm-title").textContent = title;
  $("#confirm-text").textContent = text;
  $("#confirm-dialog").classList.add("open");
  return new Promise((resolve) => { confirmResolve = resolve; });
}
$("#confirm-ok").onclick = () => { $("#confirm-dialog").classList.remove("open"); confirmResolve?.(true); };
$("#confirm-cancel").onclick = () => { $("#confirm-dialog").classList.remove("open"); confirmResolve?.(false); };

/* 各弹窗右上角关闭按钮；确认弹窗关闭视为取消 */
document.querySelectorAll(".dialog-x").forEach((btn) => {
  btn.addEventListener("click", () => {
    const overlay = btn.closest(".overlay");
    overlay.classList.remove("open");
    if (overlay.id === "confirm-dialog") confirmResolve?.(false);
  });
});

/* ---- 主题切换 ---- */
const THEME_KEY = "theme";
const themeMedia = window.matchMedia ? window.matchMedia("(prefers-color-scheme: light)") : null;

function storedTheme() {
  try { return localStorage.getItem(THEME_KEY); } catch { return null; }
}
function isLightTheme() { return document.documentElement.dataset.theme === "light"; }
function syncThemeButton() {
  const btn = $("#btn-theme");
  const light = isLightTheme();
  btn.innerHTML = icon(light ? "moon" : "sun");
  const label = light ? "切换到暗色主题" : "切换到亮色主题";
  btn.title = label;
  btn.setAttribute("aria-label", label);
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.content = light ? "#f2f4f7" : "#0a0d12";
}
$("#btn-theme").onclick = () => {
  const next = isLightTheme() ? "dark" : "light";
  document.documentElement.dataset.theme = next;
  try { localStorage.setItem(THEME_KEY, next); } catch {}
  syncThemeButton();
};
/* 未手动选择过主题时，跟随系统亮暗变化 */
if (themeMedia?.addEventListener) {
  themeMedia.addEventListener("change", (e) => {
    const stored = storedTheme();
    if (stored !== "light" && stored !== "dark") {
      document.documentElement.dataset.theme = e.matches ? "light" : "dark";
      syncThemeButton();
    }
  });
}
syncThemeButton();

/* ---- 退出登录 ---- */
$("#btn-logout").onclick = async () => {
  try { await api("/api/logout", { method: "POST" }); } catch {}
  location.href = "/login";
};

/* ---- 视图切换 ---- */
document.querySelectorAll(".nav-btn").forEach((btn) => {
  btn.onclick = () => {
    state.view = btn.dataset.view;
    document.querySelectorAll(".nav-btn").forEach((b) => b.classList.toggle("active", b === btn));
    document.querySelectorAll(".view").forEach((v) => v.classList.toggle("active", v.id === "view-" + state.view));
    if (state.view === "settings") loadSettingsForm();
    if (state.view === "discover") initDiscoverView();
  };
});

/* ---- 条目渲染 ---- */
function renderChips() {
  $("#status-chips").innerHTML = CHIP_DEFS.map((def) => {
    const n = def.key === "全部"
      ? Object.values(globalCounts).reduce((a, b) => a + b, 0)
      : Object.entries(globalCounts).filter(([status]) => def.match(status)).reduce((a, [, b]) => a + b, 0);
    return `<button class="chip${state.statusFilter === def.key ? " active" : ""}" data-chip="${esc(def.key)}">${esc(def.key)}<span class="n">${n}</span></button>`;
  }).join("");
  document.querySelectorAll("#status-chips .chip").forEach((chip) => {
    chip.onclick = () => { state.statusFilter = chip.dataset.chip; renderChips(); renderItems(); };
  });
}

function renderItems() {
  const def = CHIP_DEFS.find((d) => d.key === state.statusFilter) || CHIP_DEFS[0];
  const list = state.items.filter((x) => def.match(x.status || "待下载"));
  const box = $("#items");
  if (!list.length) {
    box.innerHTML = state.items.length
      ? `<div class="empty"><div class="e-ico">${icon("filter-x", "ic-lg")}</div><p>当前筛选下没有条目</p><small>换个状态筛选，或清除搜索关键词</small></div>`
      : `<div class="empty"><div class="e-ico">${icon("tv", "ic-lg")}</div><p>暂无条目</p><small>添加 RSS 订阅并同步后，新番会出现在这里</small></div>`;
    return;
  }
  box.innerHTML = list.map((x) => {
    const badge = badgeInfo(x.status || "待下载");
    const downloading = badge.cls === "downloading";
    const progress = Math.max(0, Math.min(100, x.progress || 0));
    const retryable = ["待下载", "下载失败", "等待下载器"].includes(x.status);
    const cover = x.cover
      ? `<img class="cover" src="${esc(x.cover)}" alt="" loading="lazy">`
      : `<div class="cover ph" aria-hidden="true">${icon("tv")}</div>`;
    return `<article class="item-row">
      <div class="item-main">
        ${cover}
        <div class="info">
          <h3>${esc(x.anime_title || x.title)}${x.episode ? `<span class="ep">第 ${esc(x.episode)} 集</span>` : ""}</h3>
          <div class="meta-line">
            <span>${esc(x.fansub_group || "未知字幕组")}</span>
            <span>${esc(x.quality || "未识别清晰度")}</span>
            <span>${esc(x.feed_name || "未知订阅")}</span>
            <span>${esc(formatTime(x.published))}</span>
          </div>
          <div class="original">${esc(x.title)}</div>
          ${badge.cls === "saved" && x.file_path ? `<div class="original" title="归档位置">${esc(x.file_path)}</div>` : ""}
          ${x.task_error ? `<div class="error-line">${esc(x.task_error)}</div>` : ""}
          ${downloading ? `<div class="progress"><div class="bar" style="width:${progress}%"></div></div>` : ""}
        </div>
      </div>
      <div class="item-side">
        ${badgeHTML(badge, downloading && progress > 0 ? " " + progress + "%" : "")}
        <div class="item-actions">
          ${retryable ? `<button class="small" data-download="${x.id}">${icon(x.status === "待下载" ? "download" : "rotate-cw", "ic-sm")}${x.status === "待下载" ? "下载" : "重试"}</button>` : ""}
          ${x.anime_title ? `<button class="small" data-storage="${x.id}">${icon("folder-open", "ic-sm")}位置</button>` : ""}
          <button class="small" data-correct="${x.id}">${icon("pen-line", "ic-sm")}纠错</button>
          <button class="small danger" data-remove="${x.id}">${icon("trash-2", "ic-sm")}移除</button>
        </div>
      </div>
    </article>`;
  }).join("");
  box.querySelectorAll("img.cover").forEach((img) => {
    img.onerror = () => {
      const ph = document.createElement("div");
      ph.className = "cover ph";
      ph.innerHTML = icon("tv");
      ph.setAttribute("aria-hidden", "true");
      img.replaceWith(ph);
    };
  });
  box.querySelectorAll("[data-download]").forEach((btn) => {
    btn.onclick = async () => {
      btn.disabled = true;
      try { await api(`/api/items/${btn.dataset.download}/download`, { method: "POST" }); toast("已加入下载队列"); }
      catch (e) { toast(e.message, true); }
      await load();
    };
  });
  box.querySelectorAll("[data-storage]").forEach((btn) => {
    btn.onclick = () => {
      const it = state.items.find((x) => x.id === Number(btn.dataset.storage));
      if (it) openStorageDialog(it.anime_title || it.title);
    };
  });
  box.querySelectorAll("[data-correct]").forEach((btn) => {
    btn.onclick = () => {
      const it = state.items.find((x) => x.id === Number(btn.dataset.correct));
      if (it) openCorrectDialog(it);
    };
  });
  box.querySelectorAll("[data-remove]").forEach((btn) => {
    btn.onclick = async () => {
      if (!(await confirmDialog("移除记录", "只删除这条记录，不会删除 NAS 上已保存的文件。"))) return;
      try { await api(`/api/items/${btn.dataset.remove}`, { method: "DELETE" }); toast("已移除记录"); }
      catch (e) { toast(e.message, true); }
      await load();
    };
  });
}

/* ---- 订阅渲染 ---- */
function renderFeeds() {
  const box = $("#feeds");
  if (!state.feeds.length) {
    box.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="e-ico">${icon("satellite-dish", "ic-lg")}</div><p>还没有订阅</p><small>点击右上角「添加订阅」，粘贴一个 RSS 地址开始</small></div>`;
    return;
  }
  box.innerHTML = state.feeds.map((f) => {
    const stateLine = f.enabled === false
      ? `<div class="state ok">已暂停，不参与定时同步</div>`
      : f.last_error
        ? `<div class="state err">同步失败：${esc(f.last_error)}</div>`
        : `<div class="state ok">最近同步：${esc(formatTime(f.last_sync))}</div>`;
    return `<div class="feed-card">
      <div class="head">
        <div class="name">${esc(f.name)}</div>
        ${f.enabled === false ? `<span class="paused-tag">已暂停</span>` : ""}
      </div>
      <div class="url" title="${esc(f.url)}">${esc(f.url)}</div>
      <div class="meta-line">
        <span>${f.auto_download ? "自动下载开" : "自动下载关"}</span>
        ${f.include_words ? `<span>包含：${esc(f.include_words)}</span>` : ""}
        ${f.exclude_words ? `<span>排除：${esc(f.exclude_words)}</span>` : ""}
      </div>
      ${stateLine}
      <div class="actions">
        <button class="small" data-sync="${f.id}">${icon("refresh-cw", "ic-sm")}立即同步</button>
        <button class="small" data-edit="${f.id}">${icon("pencil", "ic-sm")}编辑</button>
        <button class="small" data-toggle="${f.id}">${icon(f.enabled === false ? "play" : "pause", "ic-sm")}${f.enabled === false ? "启用" : "暂停"}</button>
        <button class="small danger" data-del="${f.id}">${icon("trash-2", "ic-sm")}删除</button>
      </div>
    </div>`;
  }).join("");
  box.querySelectorAll("[data-sync]").forEach((btn) => {
    btn.onclick = async () => {
      btn.disabled = true;
      try {
        const r = await api("/api/sync/" + btn.dataset.sync, { method: "POST" });
        toast(`同步完成，新增 ${r.added} 条`);
      } catch (e) { toast(e.message, true); }
      await load();
    };
  });
  box.querySelectorAll("[data-edit]").forEach((btn) => { btn.onclick = () => openFeedDialog(state.feeds.find((f) => f.id === Number(btn.dataset.edit))); });
  box.querySelectorAll("[data-toggle]").forEach((btn) => {
    btn.onclick = async () => {
      const feed = state.feeds.find((f) => f.id === Number(btn.dataset.toggle));
      try {
        await api("/api/feeds/" + feed.id, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: feed.name, include_words: feed.include_words, exclude_words: feed.exclude_words, auto_download: feed.auto_download, enabled: feed.enabled === false }) });
        toast(feed.enabled === false ? "订阅已启用" : "订阅已暂停");
      } catch (e) { toast(e.message, true); }
      await load();
    };
  });
  box.querySelectorAll("[data-del]").forEach((btn) => {
    btn.onclick = async () => {
      const feed = state.feeds.find((f) => f.id === Number(btn.dataset.del));
      if (!(await confirmDialog("删除订阅", `删除订阅“${feed.name}”及其全部条目记录？NAS 上已保存的文件不会被删除。`))) return;
      try { await api("/api/feeds/" + feed.id, { method: "DELETE" }); toast("订阅已删除"); }
      catch (e) { toast(e.message, true); }
      await load();
    };
  });
}

/* ---- 订阅弹窗 ---- */
// feed 可传完整订阅（编辑），也可传只有预填字段的伪对象（找番结果一键订阅）。
function openFeedDialog(feed) {
  const editing = !!(feed && feed.id);
  state.editingFeedId = editing ? feed.id : null;
  state.editingFeedEnabled = feed ? !!feed.enabled : true;
  $("#feed-dialog-title").textContent = editing ? "编辑订阅" : "添加订阅";
  $("#f-name").value = feed ? feed.name : "";
  $("#f-url").value = feed ? feed.url : "";
  $("#f-include").value = feed ? feed.include_words || "" : "";
  $("#f-exclude").value = feed ? feed.exclude_words || "" : "";
  $("#f-auto").checked = feed ? !!feed.auto_download : true;
  $("#f-url").disabled = editing;
  $("#f-preview").hidden = true;
  $("#f-preview").textContent = "";
  $("#f-msg").textContent = "";
  $("#feed-dialog").classList.add("open");
  $("#f-name").focus();
}
function closeFeedDialog() { $("#feed-dialog").classList.remove("open"); }
$("#btn-add-feed").onclick = () => openFeedDialog(null);
$("#f-cancel").onclick = closeFeedDialog;
$("#feed-dialog").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeFeedDialog(); });
$("#confirm-dialog").addEventListener("click", (e) => {
  if (e.target !== e.currentTarget) return;
  $("#confirm-dialog").classList.remove("open");
  confirmResolve?.(false);
});
document.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  closeFeedDialog();
  $("#confirm-dialog").classList.remove("open");
  confirmResolve?.(false);
  closeStorageDialog();
  closeAnimeDetail();
  $("#anime-edit-dialog").classList.remove("open");
  $("#bind-dialog").classList.remove("open");
  $("#correct-dialog").classList.remove("open");
});

/* ---- 存储位置弹窗 ---- */
let storageAnimeTitle = "";
async function openStorageDialog(title) {
  storageAnimeTitle = title;
  $("#s-anime").textContent = title;
  $("#s-msg").textContent = "";
  $("#s-dir").value = "";
  $("#s-season").value = 1;
  $("#s-default").textContent = "";
  $("#storage-dialog").classList.add("open");
  try {
    const s = await api("/api/anime/" + encodeURIComponent(title) + "/storage");
    $("#s-dir").value = s.storage_dir || "";
    $("#s-season").value = s.season || 1;
    $("#s-default").textContent = "默认位置：" + s.default_path;
  } catch (e) {
    $("#s-msg").textContent = "读取失败：" + e.message;
  }
}
function closeStorageDialog() { $("#storage-dialog").classList.remove("open"); }
$("#s-cancel").onclick = closeStorageDialog;
$("#storage-dialog").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeStorageDialog(); });
$("#s-save").onclick = async () => {
  const msg = $("#s-msg");
  msg.textContent = "";
  const season = Number($("#s-season").value);
  const saveBtn = $("#s-save");
  saveBtn.disabled = true;
  try {
    await api("/api/anime/" + encodeURIComponent(storageAnimeTitle) + "/storage", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ storage_dir: $("#s-dir").value.trim(), season: Number.isInteger(season) ? season : 1 }),
    });
    toast("存储位置已保存，之后归档的集数使用新位置");
    closeStorageDialog();
  } catch (e) { msg.textContent = e.message; }
  saveBtn.disabled = false;
};

/* ---- 番剧库 ---- */
async function renderAnimes() {
  const box = $("#animes");
  if (!state.animes.length) {
    box.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="e-ico">${icon("clapperboard", "ic-lg")}</div><p>番剧库还是空的</p><small>添加订阅并同步后，识别出的番剧会出现在这里</small></div>`;
    return;
  }
  box.innerHTML = state.animes.map((a) => {
    const cover = a.cover
      ? `<img class="anime-card-cover" src="${esc(a.cover)}" alt="" loading="lazy">`
      : `<div class="anime-card-cover ph" aria-hidden="true">${icon("film")}</div>`;
    const tags = [];
    if (a.bangumi_id) tags.push(`<span class="tag group">已绑定</span>`);
    else if (a.include_rules) tags.push(`<span class="tag">多来源规则</span>`);
    if (a.season > 1) tags.push(`<span class="tag">默认第 ${a.season} 季</span>`);
    return `<article class="anime-card" data-anime="${a.id}" tabindex="0" role="button" aria-label="打开 ${esc(a.title)}">
      ${cover}
      <div class="anime-card-body">
        <div class="anime-card-title">${esc(a.meta_name_cn || a.title)}</div>
        <div class="anime-card-sub" title="${esc(a.title)}">${esc(a.title)}</div>
        <div class="anime-card-stats">
          <span>${icon("film", "ic-xs")}${a.item_count} 条资源</span>
          <span>${icon("circle-check", "ic-xs")}已保存 ${a.saved_count}</span>
          ${a.episode_count ? `<span>${icon("tv", "ic-xs")}${a.episode_count} 集</span>` : ""}
        </div>
        ${tags.length ? `<div class="tags">${tags.join("")}</div>` : ""}
      </div>
    </article>`;
  }).join("");
  box.querySelectorAll("[data-anime]").forEach((card) => {
    const open = () => openAnimeDetail(Number(card.dataset.anime));
    card.onclick = open;
    card.onkeydown = (e) => { if (e.key === "Enter") open(); };
  });
  box.querySelectorAll("img.anime-card-cover").forEach((img) => {
    img.onerror = () => {
      const ph = document.createElement("div");
      ph.className = "anime-card-cover ph";
      ph.innerHTML = icon("film");
      ph.setAttribute("aria-hidden", "true");
      img.replaceWith(ph);
    };
  });
}

/* ---- 番剧详情 ---- */
let detailAnimeId = null;

function detailItemRow(it) {
  const badge = badgeInfo(it.status || "待下载");
  const retryable = ["待下载", "下载失败", "等待下载器"].includes(it.status);
  return `<div class="res-row" data-item="${it.id}">
    ${badgeHTML(badge)}
    <div class="res-info">
      <div class="res-title" title="${esc(it.title)}">${esc(it.title)}</div>
      <div class="res-meta">${esc(it.fansub_group || "未知字幕组")} · ${esc(it.quality || "未知清晰度")}${it.feed_name ? " · 来源 " + esc(it.feed_name) : ""}${it.manual ? " · 已纠错" : ""}</div>
      ${it.file_path ? `<div class="res-path" title="${esc(it.file_path)}">${esc(it.file_path)}</div>` : ""}
    </div>
    <div class="res-actions">
      ${retryable ? `<button class="small" data-act="download" data-id="${it.id}">${icon(it.status === "待下载" ? "download" : "rotate-cw", "ic-sm")}${it.status === "待下载" ? "下载" : "重试"}</button>` : ""}
      <button class="small" data-act="correct" data-id="${it.id}">${icon("pen-line", "ic-sm")}纠错</button>
      <button class="small danger" data-act="remove" data-id="${it.id}">${icon("trash-2", "ic-sm")}移除</button>
    </div>
  </div>`;
}

async function openAnimeDetail(id) {
  detailAnimeId = id;
  $("#ad-msg").textContent = "";
  $("#ad-body").innerHTML = `<div class="empty" style="padding:34px"><p>正在加载…</p></div>`;
  $("#anime-detail").classList.add("open");
  try {
    const d = await api("/api/animes/" + id);
    state.animeDetail = d;
    renderAnimeDetail(d);
  } catch (e) {
    $("#ad-body").innerHTML = "";
    $("#ad-msg").textContent = "加载失败：" + e.message;
  }
}

function closeAnimeDetail() { $("#anime-detail").classList.remove("open"); }
$("#ad-close").onclick = closeAnimeDetail;
$("#anime-detail").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeAnimeDetail(); });

function renderAnimeDetail(d) {
  $("#ad-title").textContent = d.meta_name_cn || d.title;
  const cover = $("#ad-cover");
  if (d.cover) { cover.src = d.cover; cover.hidden = false; cover.onerror = () => { cover.hidden = true; }; }
  else cover.hidden = true;
  const bits = [`识别名 ${esc(d.title)}`];
  if (d.meta_name && d.meta_name !== d.meta_name_cn) bits.push(esc(d.meta_name));
  if (d.season > 0) bits.push(`默认第 ${d.season} 季`);
  if (d.bangumi_id) bits.push(`Bangumi #${d.bangumi_id}`);
  bits.push(`${d.item_count} 条资源 · ${d.saved_count} 已保存`);
  if (d.sources && d.sources.length) bits.push(`来源 ${d.sources.map(esc).join("、")}`);
  $("#ad-meta").innerHTML = bits.map((b) => `<span>${b}</span>`).join('<span class="dot">·</span>');
  $("#ad-desc").textContent = d.include_rules
    ? `多来源规则：包含 ${d.include_rules}${d.exclude_rules ? " · 排除 " + d.exclude_rules : ""}`
    : "未设置多来源规则，仅按识别标题归并";
  const body = $("#ad-body");
  if (!d.seasons || !d.seasons.length) {
    body.innerHTML = `<div class="empty" style="padding:34px"><div class="e-ico">${icon("inbox", "ic-lg")}</div><p>该番剧下暂无资源</p></div>`;
    return;
  }
  body.innerHTML = d.seasons.map((s) => {
    const eps = s.episodes.map((ep) => {
      const rows = ep.items.map(detailItemRow).join("");
      const multi = ep.items.length > 1 ? `<span class="tag group">${ep.items.length} 个版本</span>` : "";
      return `<div class="ep-block">
        <div class="ep-title">第 ${esc(ep.episode)} 集 ${multi}</div>
        ${rows}
      </div>`;
    }).join("");
    const loose = s.loose.length
      ? `<div class="ep-block"><div class="ep-title">未识别集数</div>${s.loose.map(detailItemRow).join("")}</div>`
      : "";
    return `<div class="season-block">
      <div class="season-title">Season ${String(s.season).padStart(2, "0")}</div>
      ${eps}${loose}
    </div>`;
  }).join("");
}

/* 详情内资源操作（事件委托，弹窗内动态渲染） */
$("#ad-body").addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-act]");
  if (!btn) return;
  const id = Number(btn.dataset.id);
  const act = btn.dataset.act;
  const d = state.animeDetail;
  if (!d) return;
  let item = null;
  for (const s of d.seasons) {
    for (const ep of s.episodes) {
      const hit = ep.items.find((x) => x.id === id);
      if (hit) item = hit;
    }
    const hit = s.loose.find((x) => x.id === id);
    if (hit) item = hit;
  }
  if (!item) return;
  if (act === "correct") { openCorrectDialog(item, d); return; }
  if (act === "download") {
    btn.disabled = true;
    try { await api(`/api/items/${id}/download`, { method: "POST" }); toast("已加入下载队列"); }
    catch (err) { toast(err.message, true); }
    await openAnimeDetail(detailAnimeId);
    await load();
    return;
  }
  if (act === "remove") {
    if (!(await confirmDialog("移除记录", "只删除这条资源记录，不会删除 NAS 上已保存的文件。"))) return;
    try { await api(`/api/items/${id}`, { method: "DELETE" }); toast("已移除记录"); }
    catch (err) { toast(err.message, true); }
    await openAnimeDetail(detailAnimeId);
    await load();
  }
});

/* ---- 番剧编辑 ---- */
let editingAnimeId = null;
function openAnimeEdit(a) {
  editingAnimeId = a.id;
  $("#ae-anime").textContent = a.title;
  $("#ae-name").value = a.title;
  $("#ae-search").value = a.search_name || "";
  $("#ae-season").value = a.season > 0 ? a.season : 1;
  $("#ae-dir").value = a.storage_dir || "";
  $("#ae-include").value = a.include_rules || "";
  $("#ae-exclude").value = a.exclude_rules || "";
  $("#ae-msg").textContent = "";
  $("#anime-edit-dialog").classList.add("open");
}
$("#ad-edit").onclick = () => {
  if (state.animeDetail) openAnimeEdit(state.animeDetail);
};
function closeAnimeEdit() { $("#anime-edit-dialog").classList.remove("open"); }
$("#ae-cancel").onclick = closeAnimeEdit;
$("#anime-edit-dialog").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeAnimeEdit(); });

$("#ae-save").onclick = async () => {
  const msg = $("#ae-msg");
  msg.textContent = "";
  if (!$("#ae-name").value.trim()) { msg.textContent = "请填写番剧名称"; return; }
  const body = {
    title: $("#ae-name").value.trim(),
    search_name: $("#ae-search").value.trim(),
    season: parseInt($("#ae-season").value, 10) || 1,
    storage_dir: $("#ae-dir").value.trim(),
    include_rules: $("#ae-include").value.trim(),
    exclude_rules: $("#ae-exclude").value.trim(),
  };
  const btn = $("#ae-save");
  btn.disabled = true;
  try {
    await api("/api/animes/" + editingAnimeId, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    toast("番剧已保存");
    closeAnimeEdit();
    await load();
    if (detailAnimeId) await openAnimeDetail(detailAnimeId);
  } catch (e) { msg.textContent = e.message; }
  btn.disabled = false;
};

$("#ae-apply").onclick = async () => {
  const msg = $("#ae-msg");
  msg.textContent = "";
  if (!$("#ae-include").value.trim()) { msg.textContent = "请先填写包含规则关键词"; return; }
  // 先保存再应用，保证规则是最新值。
  const saveBody = {
    title: $("#ae-name").value.trim(),
    search_name: $("#ae-search").value.trim(),
    season: parseInt($("#ae-season").value, 10) || 1,
    storage_dir: $("#ae-dir").value.trim(),
    include_rules: $("#ae-include").value.trim(),
    exclude_rules: $("#ae-exclude").value.trim(),
  };
  const btn = $("#ae-apply");
  btn.disabled = true;
  try {
    await api("/api/animes/" + editingAnimeId, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(saveBody) });
    const r = await api(`/api/animes/${editingAnimeId}/apply-rules`, { method: "POST" });
    toast(`规则已应用，归并了 ${r.matched} 条现有条目`);
    closeAnimeEdit();
    await load();
    if (detailAnimeId) await openAnimeDetail(detailAnimeId);
  } catch (e) { msg.textContent = e.message; }
  btn.disabled = false;
};

/* ---- 元数据绑定 ---- */
let bindAnime = null;
function openBindDialog(a) {
  bindAnime = a;
  $("#bd-anime").textContent = a.title;
  $("#bd-query").value = a.search_name || "";
  $("#bd-results").innerHTML = a.bangumi_id
    ? `<div class="bind-current">当前绑定：Bangumi #${a.bangumi_id}${a.meta_name_cn ? " · " + esc(a.meta_name_cn) : ""}</div>`
    : `<div class="bind-current">当前未绑定，封面由自动识别获得</div>`;
  $("#bd-msg").textContent = "";
  $("#bind-dialog").classList.add("open");
  if (a.search_name) searchBangumiCandidates(a.search_name);
}
$("#ad-bind").onclick = () => { if (state.animeDetail) openBindDialog(state.animeDetail); };
function closeBindDialog() { $("#bind-dialog").classList.remove("open"); }
$("#bd-cancel").onclick = closeBindDialog;
$("#bind-dialog").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeBindDialog(); });

async function searchBangumiCandidates(q) {
  const box = $("#bd-results");
  box.innerHTML = `<div class="bind-current">搜索中…</div>`;
  try {
    const data = await api("/api/meta/search?q=" + encodeURIComponent(q));
    if (!data.items.length) {
      box.innerHTML = `<div class="bind-current">没有找到匹配的 Bangumi 条目，换个关键词试试</div>`;
      return;
    }
    box.innerHTML = data.items.map((c) => `
      <div class="bind-item">
        <div class="bind-names"><b>${esc(c.name_cn || c.name)}</b>${c.name_cn ? `<span>${esc(c.name)}</span>` : ""}</div>
        <button class="small" data-bind="${c.id}">${icon("link", "ic-sm")}绑定</button>
      </div>`).join("");
    box.querySelectorAll("[data-bind]").forEach((btn) => {
      btn.onclick = () => bindMetadata(Number(btn.dataset.bind));
    });
  } catch (e) {
    box.innerHTML = "";
    $("#bd-msg").textContent = e.message;
  }
}
$("#bd-search").onclick = () => {
  const q = $("#bd-query").value.trim();
  if (q.length < 2) { $("#bd-msg").textContent = "请输入至少 2 个字符的关键词"; return; }
  $("#bd-msg").textContent = "";
  searchBangumiCandidates(q);
};
$("#bd-query").addEventListener("keydown", (e) => { if (e.key === "Enter") $("#bd-search").click(); });

async function bindMetadata(bangumiID) {
  const btn = $("#bd-msg");
  btn.textContent = "";
  try {
    await api(`/api/animes/${bindAnime.id}/metadata`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ bangumi_id: bangumiID }) });
    toast(bangumiID > 0 ? "元数据绑定成功，封面已更新" : "已解除绑定");
    closeBindDialog();
    await load();
    if (detailAnimeId) await openAnimeDetail(detailAnimeId);
  } catch (e) { btn.textContent = e.message; }
}
$("#bd-unbind").onclick = () => bindMetadata(0);

/* ---- 手动纠错 ---- */
let correctingItem = null;
function openCorrectDialog(item, detail) {
  correctingItem = { item, detail: detail || null };
  $("#cd-original").textContent = item.title;
  const sel = $("#cd-anime");
  const options = state.animes.map((a) => `<option value="${a.id}">${esc(a.title)}</option>`).join("");
  sel.innerHTML = (item.anime_id ? `<option value="${item.anime_id}">${esc(item.anime_title || "当前番剧")}</option>` : `<option value="">（不更改番剧）</option>`) + options;
  if (item.anime_id) sel.value = String(item.anime_id);
  $("#cd-anime-new").value = "";
  $("#cd-season").value = item.season || 0;
  $("#cd-episode").value = item.episode || "";
  $("#cd-move").checked = false;
  $("#cd-msg").textContent = "";
  $("#correct-dialog").classList.add("open");
}
function closeCorrectDialog() { $("#correct-dialog").classList.remove("open"); }
$("#cd-cancel").onclick = closeCorrectDialog;
$("#correct-dialog").addEventListener("click", (e) => { if (e.target === e.currentTarget) closeCorrectDialog(); });

$("#cd-save").onclick = async () => {
  const msg = $("#cd-msg");
  msg.textContent = "";
  if (!correctingItem) return;
  const body = { season: parseInt($("#cd-season").value, 10) || 0, episode: $("#cd-episode").value.trim() };
  const newTitle = $("#cd-anime-new").value.trim();
  if (newTitle) body.anime_title = newTitle;
  else if ($("#cd-anime").value) body.anime_id = Number($("#cd-anime").value);
  else if (correctingItem.item.anime_id) body.anime_id = correctingItem.item.anime_id;
  else { msg.textContent = "请选择目标番剧或输入新番剧名"; return; }
  body.rearchive = $("#cd-move").checked;
  const btn = $("#cd-save");
  btn.disabled = true;
  try {
    const r = await api("/api/items/" + correctingItem.item.id, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    toast(r.cleanup_warning || (r.moved ? "纠错已保存，媒体文件已迁移" : "纠错已保存"));
    closeCorrectDialog();
    await load();
    if (detailAnimeId) await openAnimeDetail(detailAnimeId);
  } catch (e) { msg.textContent = e.message; }
  btn.disabled = false;
};

/* ---- 找番 ---- */
const discoverState = { sources: [], last: null };

function renderDiscoverMessage(text, ico = "compass") {
  $("#d-summary").hidden = true;
  $("#d-results").innerHTML = `<div class="empty"><div class="e-ico">${icon(ico, "ic-lg")}</div><p>${esc(text)}</p></div>`;
}

let discoverInitialHintDone = false;

// 每次进入找番视图都刷新搜索源：设置页新增/清空自定义模板后回到本视图立即生效。
async function initDiscoverView() {
  if (!discoverInitialHintDone) {
    discoverInitialHintDone = true;
    renderDiscoverMessage("输入关键词搜索资源站，从结果里把 RSS 地址添加为订阅");
  }
  const previous = $("#d-source").value;
  try {
    const data = await api("/api/discover/sources");
    discoverState.sources = data.sources || [];
  } catch (e) {
    $("#d-source").innerHTML = `<option value="">搜索源加载失败</option>`;
    return;
  }
  if (!discoverState.sources.length) {
    $("#d-source").innerHTML = `<option value="">无可用搜索源</option>`;
    if (!discoverState.last) renderDiscoverMessage("没有可用搜索源，请到设置页配置自定义 RSS 搜索模板", "triangle-alert");
    return;
  }
  $("#d-source").innerHTML = discoverState.sources.map((s) =>
    `<option value="${esc(s.id)}" title="${esc(s.hint || s.name)}">${esc(s.name)}</option>`).join("");
  // 之前选中的源仍存在时保持选择，否则回到第一个可用源。
  if (previous && discoverState.sources.some((s) => s.id === previous)) {
    $("#d-source").value = previous;
  }
}

async function runDiscoverSearch() {
  const keyword = $("#d-keyword").value.trim();
  const source = $("#d-source").value;
  if (!keyword) { renderDiscoverMessage("请输入搜索关键词"); $("#d-keyword").focus(); return; }
  if (!source) { renderDiscoverMessage("没有可用搜索源，请到设置页配置自定义 RSS 搜索模板", "triangle-alert"); return; }
  $("#d-summary").hidden = true;
  $("#d-results").innerHTML = `<div class="empty"><div class="e-ico">${icon("loader-circle", "ic-lg spin")}</div><p>正在搜索「${esc(keyword)}」…</p></div>`;
  $("#d-search-btn").disabled = true;
  try {
    const data = await api(`/api/discover/search?source=${encodeURIComponent(source)}&keyword=${encodeURIComponent(keyword)}`);
    discoverState.last = data;
    renderDiscoverResults(data);
  } catch (e) {
    renderDiscoverMessage(`搜索失败：${e.message}`, "cloud-off");
  }
  $("#d-search-btn").disabled = false;
}

async function copyDiscoverText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("RSS 地址已复制");
  } catch {
    const input = document.createElement("textarea");
    input.value = text;
    document.body.appendChild(input);
    input.select();
    try { document.execCommand("copy"); toast("RSS 地址已复制"); }
    catch { toast("复制失败，请手动复制", true); }
    input.remove();
  }
}

function renderDiscoverResults(data) {
  const summary = $("#d-summary");
  summary.hidden = false;
  summary.innerHTML = `
    <b>「${esc(data.keyword)}」</b>
    <span>${esc(data.source_name)} · ${data.count} 条结果</span>
    <span class="discover-rss" title="${esc(data.feed_url)}">${esc(data.feed_url)}</span>
    <button class="small" type="button" data-copy>${icon("copy", "ic-sm")}复制 RSS 地址</button>
    <button class="small primary" type="button" data-subscribe-feed>${icon("plus", "ic-sm")}把结果添加为订阅</button>`;
  summary.querySelector("[data-copy]").onclick = () => copyDiscoverText(data.feed_url);
  summary.querySelector("[data-subscribe-feed]").onclick = () =>
    openFeedDialog({ name: data.keyword, url: data.feed_url, include_words: data.keyword, exclude_words: "", auto_download: true, enabled: true });

  const box = $("#d-results");
  if (!data.items.length) {
    box.innerHTML = `<div class="empty"><div class="e-ico">${icon("search-x", "ic-lg")}</div><p>没有找到与「${esc(data.keyword)}」匹配的资源</p><small>换个关键词，或到设置页换一个搜索源</small></div>`;
    return;
  }
  box.innerHTML = data.items.map((it, i) => {
    const pageLink = /^https?:/i.test(it.link || "");
    return `<article class="item-row">
      <div class="item-main">
        <div class="info">
          <h3>${esc(it.anime_title || it.title)}${it.episode ? `<span class="ep">第 ${esc(it.episode)} 集</span>` : ""}</h3>
          <div class="meta-line">
            <span>${esc(it.fansub_group || "未知字幕组")}</span>
            ${it.quality ? `<span>${esc(it.quality)}</span>` : ""}
            <span>${esc(formatTime(it.published))}</span>
          </div>
          <div class="original">${esc(it.title)}</div>
        </div>
      </div>
      <div class="item-side">
        <div class="item-actions">
          ${pageLink ? `<button class="small" type="button" data-page="${i}">${icon("external-link", "ic-sm")}资源页</button>` : ""}
          <button class="small primary" type="button" data-subscribe="${i}">${icon("plus", "ic-sm")}添加订阅</button>
        </div>
      </div>
    </article>`;
  }).join("");
  box.querySelectorAll("[data-page]").forEach((btn) => {
    btn.onclick = () => window.open(discoverState.last.items[Number(btn.dataset.page)].link, "_blank", "noopener");
  });
  box.querySelectorAll("[data-subscribe]").forEach((btn) => {
    btn.onclick = () => {
      const it = discoverState.last.items[Number(btn.dataset.subscribe)];
      // 订阅的是搜索结果的 RSS 地址；包含关键词预填本次搜索词，与搜索口径一致，可在弹窗里修改。
      openFeedDialog({ name: it.anime_title || data.keyword, url: data.feed_url, include_words: data.keyword, exclude_words: "", auto_download: true, enabled: true });
    };
  });
}

$("#d-search-btn").onclick = runDiscoverSearch;
$("#d-keyword").addEventListener("keydown", (e) => { if (e.key === "Enter") runDiscoverSearch(); });

/* ---- 设置页 ---- */
const SETTINGS_MODE_VIEWS = { aria2: "st-dl-aria2", qbittorrent: "st-dl-qbit", transmission: "st-dl-tr" };
function syncSettingsMode() {
  for (const id of Object.values(SETTINGS_MODE_VIEWS)) $("#" + id).hidden = true;
  const view = SETTINGS_MODE_VIEWS[$("#st-mode").value];
  if (view) $("#" + view).hidden = false;
}
$("#st-mode").onchange = syncSettingsMode;

async function loadSettingsForm() {
  try {
    const s = await api("/api/settings");
    $("#st-user").value = s.web_user || "";
    $("#st-pass").value = "";
    $("#st-mode").value = s.download_mode || "direct";
    $("#st-aria2-url").value = s.aria2_url || "";
    $("#st-aria2-dir").value = s.aria2_dir || "";
    $("#st-aria2-local").value = s.aria2_local_dir || "";
    $("#st-qbit-url").value = s.qbit_url || "";
    $("#st-qbit-user").value = s.qbit_user || "";
    $("#st-qbit-save").value = s.qbit_save_path || "";
    $("#st-qbit-local").value = s.qbit_local_dir || "";
    $("#st-tr-url").value = s.transmission_url || "";
    $("#st-tr-user").value = s.transmission_user || "";
    $("#st-tr-dir").value = s.transmission_download_dir || "";
    $("#st-tr-local").value = s.transmission_local_dir || "";
    $("#st-proxy").value = s.proxy_url || "";
    $("#st-proxy-clear").checked = false;
    $("#st-proxy-auth-hint").textContent = s.proxy_credentials_set
      ? "已配置代理认证；地址不变时保存会保留，勾选上方选项可清除。"
      : "未配置代理认证；需要时可在代理 URL 中填写账号密码。";
    $("#st-poll").value = s.poll_minutes || 30;
    $("#st-discover").value = s.discover_search_template || "";
    syncSettingsMode();
    $("#st-msg").textContent = "";
  } catch (e) {
    $("#st-msg").textContent = "读取设置失败：" + e.message;
  }
}

$("#btn-save-settings").onclick = async () => {
  const msg = $("#st-msg");
  msg.textContent = "";
  const body = {
    web_user: $("#st-user").value.trim(),
    download_mode: $("#st-mode").value,
    aria2_url: $("#st-aria2-url").value.trim(),
    aria2_dir: $("#st-aria2-dir").value.trim(),
    aria2_local_dir: $("#st-aria2-local").value.trim(),
    qbit_url: $("#st-qbit-url").value.trim(),
    qbit_user: $("#st-qbit-user").value.trim(),
    qbit_save_path: $("#st-qbit-save").value.trim(),
    qbit_local_dir: $("#st-qbit-local").value.trim(),
    transmission_url: $("#st-tr-url").value.trim(),
    transmission_user: $("#st-tr-user").value.trim(),
    transmission_download_dir: $("#st-tr-dir").value.trim(),
    transmission_local_dir: $("#st-tr-local").value.trim(),
    proxy_url: $("#st-proxy").value.trim(),
    proxy_clear_credentials: $("#st-proxy-clear").checked,
    poll_minutes: parseInt($("#st-poll").value, 10) || 30,
    discover_search_template: $("#st-discover").value.trim(),
  };
  const pass = $("#st-pass").value;
  if (pass) {
    if (!body.web_user) { msg.textContent = "设置密码前请先填写用户名"; return; }
    if (pass.length < 6) { msg.textContent = "密码至少 6 位"; return; }
    body.web_password = pass;
  }
  const btn = $("#btn-save-settings");
  btn.disabled = true;
  try {
    const result = await api("/api/settings", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (result.reauth_required) {
      location.href = "/login";
      return;
    }
    toast("设置已保存并立即生效");
    await loadSettingsForm();
    await loadRuntime();
  } catch (e) { msg.textContent = e.message; }
  btn.disabled = false;
};

$("#f-preview-btn").onclick = async () => {
  const box = $("#f-preview");
  box.hidden = false;
  box.textContent = "正在读取 RSS…";
  try {
    const data = await api("/api/preview", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ url: $("#f-url").value.trim() }) });
    box.textContent = data.count
      ? `已读取 ${data.count} 条：\n` + data.items.slice(0, 3).map((i) => "· " + i.title).join("\n") + (data.count > 3 ? "\n…" : "")
      : "RSS 可以访问，但里面暂无条目";
  } catch (e) { box.textContent = "预览失败：" + e.message; }
};

$("#f-save").onclick = async () => {
  const msg = $("#f-msg");
  msg.textContent = "";
  const body = {
    name: $("#f-name").value.trim(),
    url: $("#f-url").value.trim(),
    include_words: $("#f-include").value.trim(),
    exclude_words: $("#f-exclude").value.trim(),
    auto_download: $("#f-auto").checked,
    enabled: state.editingFeedEnabled,
  };
  if (!body.name) { msg.textContent = "请填写订阅名称"; return; }
  const saveBtn = $("#f-save");
  saveBtn.disabled = true;
  try {
    if (state.editingFeedId) {
      await api("/api/feeds/" + state.editingFeedId, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      toast("订阅已保存");
    } else {
      const created = await api("/api/feeds", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (created.id) {
        try {
          await api("/api/sync/" + created.id, { method: "POST" });
          toast("订阅已创建并完成首次同步");
        } catch (e) { toast("订阅已保存，但首次同步失败：" + e.message, true); }
      }
    }
    closeFeedDialog();
    await load();
  } catch (e) { msg.textContent = e.message; }
  saveBtn.disabled = false;
};

/* ---- 备份 / 恢复 / OPML ---- */
$("#btn-backup").onclick = () => { window.open("/api/backup"); };
$("#btn-restore").onclick = () => $("#file-restore").click();
$("#file-restore").onchange = async (e) => {
  const file = e.target.files[0];
  if (!file) return;
  if (!(await confirmDialog("恢复订阅", "只导入订阅配置并按 RSS 地址去重，不会导入历史条目或下载文件。"))) { e.target.value = ""; return; }
  try {
    const r = await api("/api/restore", { method: "POST", headers: { "Content-Type": "application/json" }, body: await file.text() });
    toast(`已恢复 ${r.restored} 个订阅`);
    await load();
  } catch (err) { toast(err.message, true); }
  e.target.value = "";
};
$("#btn-opml").onclick = () => $("#file-opml").click();
$("#file-opml").onchange = async (e) => {
  const file = e.target.files[0];
  if (!file) return;
  try {
    const r = await api("/api/import-opml", { method: "POST", headers: { "Content-Type": "application/xml" }, body: await file.text() });
    toast(`已导入 ${r.imported} 个订阅`);
    await load();
  } catch (err) { toast(err.message, true); }
  e.target.value = "";
};

/* ---- 搜索 ---- */
let searchTimer = null;
$("#item-search").oninput = (e) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => { state.search = e.target.value.trim(); load(); }, 400);
};

/* ---- 数据加载与渲染 ---- */
async function load() {
  const query = state.search ? "?q=" + encodeURIComponent(state.search) : "";
  try {
    const [feeds, items, animes] = await Promise.all([api("/api/feeds"), api("/api/items" + query), api("/api/animes")]);
    state.feeds = feeds;
    state.items = items;
    state.animes = animes;
    renderFeeds();
    renderItems();
    renderAnimes();
  } catch (e) {
    $("#items").innerHTML = `<div class="empty"><div class="e-ico">${icon("server-off", "ic-lg")}</div><p>数据加载失败</p><small>${esc(e.message)}</small></div>`;
  }
}

async function loadRuntime() {
  const card = $("#runtime-card");
  try {
    const s = await api("/api/status", { cache: "no-store" });
    globalCounts = s.counts || {};
    $("#btn-logout").hidden = !s.auth_enabled;
    const countRows = Object.keys(globalCounts).length
      ? Object.entries(globalCounts).map(([k, v]) => `<div class="run-row"><span>${esc(k)}</span><b>${v}</b></div>`).join("")
      : `<div class="run-row"><span>下载任务</span><b>暂无</b></div>`;
    card.innerHTML = `<div class="run-head">${icon("activity")}<span>运行状态</span><i class="run-dot" aria-hidden="true"></i></div>` +
      `<div class="run-rows">` +
      `<div class="run-row"><span>下载模式</span><b>${esc(MODE_LABEL[s.download_mode] || s.download_mode)}</b></div>` +
      `<div class="run-row"><span>同步周期</span><b>每 ${s.poll_minutes} 分钟</b></div>` +
      (s.proxy ? `<div class="run-row"><span>出站代理</span><b title="${esc(s.proxy)}">${esc(s.proxy)}</b></div>` : "") +
      countRows +
      `</div>`;
    renderChips();
  } catch (e) {
    card.textContent = "运行状态不可用";
  }
}

function startPolling() {
  const tick = async () => { await load(); await loadRuntime(); };
  tick();
  setInterval(tick, 5000);
}
startPolling();
