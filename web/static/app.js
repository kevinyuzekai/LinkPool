const $ = (id) => document.getElementById(id);
const errBox = $("errBox");

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

function renderAdapters(list) {
  const tb = $("adapterBody");
  tb.innerHTML = "";
  (list || []).forEach((a) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td><input type="checkbox" data-id="${a.id}" class="sel" ${a.selected ? "checked" : ""} /></td>
      <td>${escapeHtml(a.name)} <span class="muted" style="color:#8b9bb4;font-size:0.75rem">(${escapeHtml(a.ifaceName)})</span></td>
      <td><code>${escapeHtml(a.ipv4)}</code></td>
      <td class="type">${typeLabel(a.type)}</td>
      <td><input type="number" min="1" value="${a.weight || 1}" data-id="${a.id}" class="wt" /></td>
      <td><span class="badge ${a.up ? "up" : "down"}">${a.up ? "连通" : "断开"}</span></td>`;
    tb.appendChild(tr);
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
  if (!stats.length) {
    $("statsBox").textContent = "暂无流量统计";
    return;
  }
  const lines = stats.map(
    (s) =>
      `${s.adapterId}: 连接=${s.connections} 活跃=${s.active} ↑${fmtBytes(s.bytesSent)} ↓${fmtBytes(s.bytesRecv)} 错误=${s.errors}`
  );
  lines.push(`调度次数: ${status.schedulerPicks || 0}`);
  lines.push(`HTTP ${status.httpAddr} · SOCKS5 ${status.socksAddr}`);
  if (status.sysProxy) lines.push("系统代理: 已启用");
  $("statsBox").textContent = lines.join("\n");
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
