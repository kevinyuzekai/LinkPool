const $ = (id) => document.getElementById(id);
const errBox = $("errBox");

let lastRates = {}; // adapterId -> rateBps from status
let dlPollTimer = null;

function showErr(msg) {
  errBox.textContent = msg || "";
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || res.statusText);
  }
  return data;
}

function typeLabel(t) {
  const map = {
    ethernet: "有线",
    wifi: "Wi‑Fi",
    cellular: "蜂窝",
    vpn: "VPN",
    loopback: "回环",
    other: "其他",
  };
  return map[t] || t;
}

function fmtRate(bps) {
  if (!bps || bps < 1) return "—";
  if (bps < 1024) return bps.toFixed(0) + " B/s";
  if (bps < 1048576) return (bps / 1024).toFixed(1) + " KB/s";
  return (bps / 1048576).toFixed(2) + " MB/s";
}

function renderAdapters(list) {
  const tb = $("adapterBody");
  tb.innerHTML = "";
  (list || []).forEach((a) => {
    const rate = lastRates[a.id];
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><input type="checkbox" data-id="${a.id}" class="sel" ${a.selected ? "checked" : ""} /></td>
      <td>${escapeHtml(a.name)} <span class="muted" style="color:#8b9bb4;font-size:0.75rem">(${escapeHtml(a.ifaceName)})</span></td>
      <td><code>${escapeHtml(a.ipv4)}</code></td>
      <td class="type">${typeLabel(a.type)}</td>
      <td><input type="number" min="1" value="${a.weight || 1}" data-id="${a.id}" class="wt" /></td>
      <td class="rate" data-rate-id="${escapeHtml(a.id)}">${fmtRate(rate)}</td>
      <td><span class="badge ${a.up ? "up" : "down"}">${a.up ? "连通" : "断开"}</span></td>`;
    tb.appendChild(tr);
  });
}

function updateRateCells() {
  document.querySelectorAll("[data-rate-id]").forEach((el) => {
    const id = el.getAttribute("data-rate-id");
    el.textContent = fmtRate(lastRates[id]);
  });
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

async function collectAndSaveAdapters() {
  const sels = [...document.querySelectorAll(".sel")];
  const adapters = sels.map((cb) => {
    const id = cb.dataset.id;
    const wt = document.querySelector(`.wt[data-id="${CSS.escape(id)}"]`);
    return {
      id,
      selected: cb.checked,
      weight: parseInt(wt?.value || "1", 10) || 1,
    };
  });
  const data = await api("/api/adapters/update", {
    method: "POST",
    body: JSON.stringify({ adapters }),
  });
  renderAdapters(data.adapters);
  return data;
}

function setStatusPill(st) {
  const pill = $("statusPill");
  const map = {
    stopped: "已停止",
    starting: "启动中…",
    running: "运行中",
    stopping: "停止中…",
    error: "错误",
  };
  pill.textContent = map[st] || st;
  pill.className = "status-pill" + (st === "running" ? " running" : st === "error" ? " error" : "");
}

function renderStats(status) {
  const stats = status.stats || [];
  lastRates = {};
  stats.forEach((s) => {
    lastRates[s.adapterId] = s.rateBps || 0;
  });
  updateRateCells();

  if (!stats.length) {
    $("statsBox").textContent = "暂无流量统计";
  } else {
    const ew = status.effectiveWeights || {};
    const lines = stats.map((s) => {
      const w = ew[s.adapterId] != null ? ` 有效权重=${ew[s.adapterId]}` : "";
      return `${s.adapterId}: 连接=${s.connections} 活跃=${s.active} ↑${fmtBytes(s.bytesSent)} ↓${fmtBytes(s.bytesRecv)} 速率=${fmtRate(s.rateBps)} 错误=${s.errors}${w}`;
    });
    lines.push(`调度: ${status.schedulerMode || "?"} · 次数 ${status.schedulerPicks || 0}`);
    lines.push(`HTTP ${status.httpAddr} · SOCKS5 ${status.socksAddr}`);
    if (status.sysProxy) lines.push("系统代理: 已启用");
    if (status.downloadDir) lines.push(`下载目录: ${status.downloadDir}`);
    $("statsBox").textContent = lines.join("\n");
  }

  const mode = status.schedulerMode || "adaptive";
  if (mode === "static") {
    $("modeStatic").checked = true;
  } else {
    $("modeAdaptive").checked = true;
  }
  const ew = status.effectiveWeights || {};
  const parts = Object.keys(ew).map((k) => `${k}:${ew[k]}`);
  $("effWeights").textContent = parts.length ? "有效权重 " + parts.join(" ") : "";
}

function fmtBytes(n) {
  if (n < 1024) return n + "B";
  if (n < 1048576) return (n / 1024).toFixed(1) + "KB";
  return (n / 1048576).toFixed(1) + "MB";
}

async function refreshStatus() {
  try {
    const st = await api("/api/status");
    setStatusPill(st.status);
    renderStats(st);
    if (st.httpAddr) $("httpAddr").value = st.httpAddr;
    if (st.socksAddr) $("socksAddr").value = st.socksAddr;
    if (st.error) showErr(st.error);
  } catch (e) {
    showErr(e.message);
  }
}

async function loadAll() {
  showErr("");
  try {
    const data = await api("/api/adapters");
    renderAdapters(data.adapters || []);
    if (data.warning) showErr(data.warning);
    const bypass = await api("/api/bypass");
    $("bypass").value = bypass.text || "";
    await refreshStatus();
  } catch (e) {
    showErr(e.message);
  }
}

async function setSchedMode(mode) {
  try {
    await api("/api/scheduler", {
      method: "POST",
      body: JSON.stringify({ mode }),
    });
    await refreshStatus();
  } catch (e) {
    showErr(e.message);
  }
}

$("modeAdaptive").onchange = () => {
  if ($("modeAdaptive").checked) setSchedMode("adaptive");
};
$("modeStatic").onchange = () => {
  if ($("modeStatic").checked) setSchedMode("static");
};

function renderDlJob(job) {
  if (!job) {
    $("dlBox").textContent = "尚未开始";
    return;
  }
  const pct =
    job.totalSize > 0 ? ((100 * job.downloaded) / job.totalSize).toFixed(1) : "?";
  const lines = [
    `#${job.id} ${job.status}  ${fmtBytes(job.downloaded || 0)} / ${fmtBytes(job.totalSize || 0)} (${pct}%)`,
    `文件: ${job.fileName}  并发: ${job.concurrency}`,
  ];
  if (job.error) lines.push("错误: " + job.error);
  (job.parts || []).forEach((p) => {
    lines.push(
      `  段${p.index} [${p.start}-${p.end}] → ${p.adapterId || "?"}  ${fmtBytes(p.done || 0)}${p.error ? " ✗ " + p.error : ""}`
    );
  });
  if (job.status === "done") {
    lines.push(`完成 → 下载文件: /api/download/${job.id}/file`);
    lines.push(`（或打开 ${job.filePath || ""}）`);
  }
  $("dlBox").textContent = lines.join("\n");
}

function pollDownload(id) {
  if (dlPollTimer) clearInterval(dlPollTimer);
  dlPollTimer = setInterval(async () => {
    try {
      const data = await api(`/api/download/${id}`);
      renderDlJob(data.job);
      if (data.job && (data.job.status === "done" || data.job.status === "failed" || data.job.status === "canceled")) {
        clearInterval(dlPollTimer);
        dlPollTimer = null;
        if (data.job.status === "done") {
          // offer browser download
          const a = document.createElement("a");
          a.href = `/api/download/${id}/file`;
          a.download = data.job.fileName || "download";
          a.textContent = "点击保存文件";
          // auto-click once
          a.click();
        }
      }
    } catch (e) {
      $("dlBox").textContent = e.message;
      clearInterval(dlPollTimer);
      dlPollTimer = null;
    }
  }, 800);
}

$("btnDownload").onclick = async () => {
  showErr("");
  try {
    await collectAndSaveAdapters();
    const url = $("dlUrl").value.trim();
    const conc = parseInt($("dlConc").value || "0", 10) || 0;
    if (!url) {
      showErr("请填写下载 URL");
      return;
    }
    $("dlBox").textContent = "启动中…";
    const data = await api("/api/download", {
      method: "POST",
      body: JSON.stringify({ url, concurrency: conc }),
    });
    renderDlJob(data.job);
    pollDownload(data.job.id);
  } catch (e) {
    showErr(e.message);
    $("dlBox").textContent = e.message;
  }
};

$("btnRefresh").onclick = () => loadAll();
$("btnStart").onclick = async () => {
  showErr("");
  try {
    await collectAndSaveAdapters();
    await api("/api/listen", {
      method: "POST",
      body: JSON.stringify({ http: $("httpAddr").value, socks: $("socksAddr").value }),
    });
    const st = await api("/api/start", {
      method: "POST",
      body: JSON.stringify({ sysProxy: $("sysProxy").checked }),
    });
    setStatusPill(st.status);
    renderStats(st);
    if (st.error) showErr(st.error);
  } catch (e) {
    showErr(e.message);
    refreshStatus();
  }
};
$("btnStop").onclick = async () => {
  try {
    const st = await api("/api/stop", { method: "POST", body: "{}" });
    setStatusPill(st.status);
    renderStats(st);
    showErr("");
  } catch (e) {
    showErr(e.message);
  }
};
$("btnApplyListen").onclick = async () => {
  try {
    await api("/api/listen", {
      method: "POST",
      body: JSON.stringify({ http: $("httpAddr").value, socks: $("socksAddr").value }),
    });
    showErr("");
  } catch (e) {
    showErr(e.message);
  }
};
$("btnSaveBypass").onclick = async () => {
  try {
    await api("/api/bypass", {
      method: "POST",
      body: JSON.stringify({ text: $("bypass").value }),
    });
    showErr("");
  } catch (e) {
    showErr(e.message);
  }
};
$("btnResetStats").onclick = async () => {
  await api("/api/stats/reset", { method: "POST", body: "{}" });
  refreshStatus();
};
$("btnDiag").onclick = async () => {
  $("diagBox").textContent = "检测中…";
  try {
    await collectAndSaveAdapters();
    const data = await api("/api/diag");
    const lines = (data.results || []).map((r) => {
      if (r.ok) return `✓ ${r.name} (${r.localIp}) → 公网 ${r.publicIp}  ${r.latencyMs}ms`;
      return `✗ ${r.name} (${r.localIp}) → ${r.error || "失败"}`;
    });
    $("diagBox").textContent = lines.length ? lines.join("\n") : "没有已选网卡";
  } catch (e) {
    $("diagBox").textContent = e.message;
  }
};

loadAll();
setInterval(refreshStatus, 2000);
