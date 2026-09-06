/* 自动追番管理界面：订阅、条目、下载状态。全部请求走本地 API，无外部依赖。 */
"use strict";

const $ = (s) => document.querySelector(s);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const state = {
  view: "items",
  statusFilter: "全部",
  search: "",
  feeds: [],
  items: [],
  editingFeedId: null,
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
  if (status === "已保存") return { cls: "saved", text: "已保存" };
  if (status === "下载中" || status.startsWith("已发送到")) return { cls: "downloading", text: status === "下载中" ? "下载中" : status };
  if (status === "下载失败") return { cls: "failed", text: "下载失败" };
  if (status === "等待下载器") return { cls: "waiting", text: "等待下载器" };
  if (status === "等待下载" || status === "待下载") return { cls: "pending", text: "待下载" };
  return { cls: "pending", text: status };
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
  el.textContent = message;
  $("#toasts").appendChild(el);
  setTimeout(() => el.remove(), 4200);
}

async function api(url, options) {
  const res = await fetch(url, options);
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
  btn.textContent = light ? "☾" : "☀";
  const label = light ? "切换到暗色主题" : "切换到亮色主题";
  btn.title = label;
  btn.setAttribute("aria-label", label);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.content = light ? "#f4f6f8" : "#0e1216";
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

/* ---- 视图切换 ---- */
document.querySelectorAll(".nav-btn").forEach((btn) => {
  btn.onclick = () => {
    state.view = btn.dataset.view;
    document.querySelectorAll(".nav-btn").forEach((b) => b.classList.toggle("active", b === btn));
    document.querySelectorAll(".view").forEach((v) => v.classList.toggle("active", v.id === "view-" + state.view));
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
      ? `<div class="empty"><div class="big">🔍</div>当前筛选下没有条目</div>`
      : `<div class="empty"><div class="big">📺</div>暂无条目<br><small>左侧添加 RSS 订阅后，新番会出现在这里</small></div>`;
    return;
  }
  box.innerHTML = list.map((x) => {
    const badge = badgeInfo(x.status || "待下载");
    const downloading = badge.cls === "downloading";
    const progress = Math.max(0, Math.min(100, x.progress || 0));
    const retryable = ["待下载", "下载失败", "等待下载器"].includes(x.status);
    const cover = x.cover
      ? `<img class="cover" src="${esc(x.cover)}" alt="" loading="lazy">`
      : `<div class="cover ph" aria-hidden="true">📺</div>`;
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
          ${x.task_error ? `<div class="error-line">${esc(x.task_error)}</div>` : ""}
          ${downloading ? `<div class="progress"><div class="bar" style="width:${progress}%"></div></div>` : ""}
        </div>
      </div>
      <div class="item-side">
        <span class="badge ${badge.cls}">${esc(badge.text)}${downloading && progress > 0 ? " " + progress + "%" : ""}</span>
        <div class="item-actions">
          ${retryable ? `<button class="small" data-download="${x.id}">${x.status === "待下载" ? "下载" : "重试"}</button>` : ""}
          <button class="small danger" data-remove="${x.id}">移除</button>
        </div>
      </div>
    </article>`;
  }).join("");
  box.querySelectorAll("img.cover").forEach((img) => {
    img.onerror = () => {
      const ph = document.createElement("div");
      ph.className = "cover ph";
      ph.textContent = "📺";
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
    box.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="big">🛰️</div>还没有订阅<br><small>点击右上角“添加订阅”，粘贴一个 RSS 地址开始</small></div>`;
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
        <button class="small" data-sync="${f.id}">立即同步</button>
        <button class="small" data-edit="${f.id}">编辑</button>
        <button class="small" data-toggle="${f.id}">${f.enabled === false ? "启用" : "暂停"}</button>
        <button class="small danger" data-del="${f.id}">删除</button>
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
function openFeedDialog(feed) {
  state.editingFeedId = feed ? feed.id : null;
  $("#feed-dialog-title").textContent = feed ? "编辑订阅" : "添加订阅";
  $("#f-name").value = feed ? feed.name : "";
  $("#f-url").value = feed ? feed.url : "";
  $("#f-include").value = feed ? feed.include_words || "" : "";
  $("#f-exclude").value = feed ? feed.exclude_words || "" : "";
  $("#f-auto").checked = feed ? !!feed.auto_download : true;
  $("#f-url").disabled = !!feed;
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
});

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
    const [feeds, items] = await Promise.all([api("/api/feeds"), api("/api/items" + query)]);
    state.feeds = feeds;
    state.items = items;
    renderFeeds();
    renderItems();
  } catch (e) {
    $("#items").innerHTML = `<div class="empty">数据加载失败：${esc(e.message)}</div>`;
  }
}

async function loadRuntime() {
  const card = $("#runtime-card");
  try {
    const s = await api("/api/status", { cache: "no-store" });
    globalCounts = s.counts || {};
    card.innerHTML = `<b>${esc(MODE_LABEL[s.download_mode] || s.download_mode)}</b> · 每 ${s.poll_minutes} 分钟同步` +
      (Object.keys(globalCounts).length
        ? `<br>${Object.entries(globalCounts).map(([k, v]) => `${esc(k)} ${v}`).join(" · ")}`
        : "<br>暂无下载任务");
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
