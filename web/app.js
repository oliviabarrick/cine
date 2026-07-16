// Cine — client-side cartelera. Fetches the server's normalized, cached feed
// once and does all filtering in the browser: by movie, date, format
// (subtituladas/dobladas), sala, and cadena. No framework, no build step.

const KNOWN_CHAINS = ["Cinépolis", "Cinemark", "CCM", "Nova Cinemas", "Sala Garbo"];
const FORMAT_LABEL = { sub: "SUB", dub: "DOB", unknown: "?" };

const state = {
  q: "",
  format: "sub", // default to subtituladas — CR preference
  chains: new Set(KNOWN_CHAINS),
  cinema: "all",
  date: "all",
};

let DATA = { showtimes: [], chains: [], cinemas: [], errors: {} };

const $ = (sel) => document.querySelector(sel);

function crToday() {
  // en-CA renders as YYYY-MM-DD; pin to Costa Rica regardless of the viewer's TZ.
  return new Date().toLocaleDateString("en-CA", { timeZone: "America/Costa_Rica" });
}

async function load() {
  try {
    const r = await fetch("/api/showtimes", { headers: { Accept: "application/json" } });
    if (!r.ok) throw new Error("HTTP " + r.status);
    DATA = await r.json();
  } catch (e) {
    $("#results").innerHTML =
      `<div class="empty">No se pudo cargar la cartelera.<br><small>${escape(String(e))}</small></div>`;
    return;
  }
  buildFacets();
  bindControls();
  render();
}

function buildFacets() {
  // Status + notices.
  const st = $("#status");
  st.textContent = DATA.updatedAt
    ? "Actualizado " + new Date(DATA.updatedAt).toLocaleString("es-CR", { timeZone: "America/Costa_Rica", dateStyle: "medium", timeStyle: "short" })
    : "Recolectando cartelera…";

  const notices = $("#notices");
  notices.innerHTML = "";
  if (DATA.stale) notices.appendChild(mk("div", "notice warn", "Los datos podrían estar desactualizados."));
  for (const [chain, err] of Object.entries(DATA.errors || {})) {
    notices.appendChild(mk("div", "notice", `No se pudo leer ${chain}: ${err}`));
  }

  // Chain toggles.
  const chainsBox = $("#chains");
  chainsBox.innerHTML = "";
  const present = new Set(DATA.chains || []);
  for (const c of KNOWN_CHAINS) {
    if (!present.has(c)) continue; // only offer chains we actually have data for
    const el = mk("span", "chip-toggle on", c);
    el.dataset.chain = c;
    el.onclick = () => {
      el.classList.toggle("on");
      if (el.classList.contains("on")) state.chains.add(c);
      else state.chains.delete(c);
      render();
    };
    chainsBox.appendChild(el);
  }

  // Cinema select.
  const cinemaSel = $("#cinema");
  cinemaSel.innerHTML = "";
  cinemaSel.appendChild(opt("all", "Todas"));
  for (const c of DATA.cinemas || []) cinemaSel.appendChild(opt(c, c));

  // Date select.
  const dates = [...new Set(DATA.showtimes.map((s) => s.date))].sort();
  const today = crToday();
  const dateSel = $("#date");
  dateSel.innerHTML = "";
  dateSel.appendChild(opt("all", "Todas las fechas"));
  for (const d of dates) dateSel.appendChild(opt(d, dateLabel(d, today)));
  state.date = dates.includes(today) ? today : dates[0] || "all";
  dateSel.value = state.date;

  // Footer sources.
  $("#sources").innerHTML = KNOWN_CHAINS.map((c) =>
    present.has(c)
      ? `<span class="src ok">${c} ✓</span>`
      : `<span class="src todo">${c} (próximamente)</span>`
  ).join(" · ");
}

function bindControls() {
  $("#q").oninput = (e) => { state.q = e.target.value.trim().toLowerCase(); render(); };
  $("#cinema").onchange = (e) => { state.cinema = e.target.value; render(); };
  $("#date").onchange = (e) => { state.date = e.target.value; render(); };
  for (const b of document.querySelectorAll("#formatSeg button")) {
    b.onclick = () => {
      document.querySelectorAll("#formatSeg button").forEach((x) => x.classList.remove("on"));
      b.classList.add("on");
      state.format = b.dataset.format;
      render();
    };
  }
}

function visible() {
  return DATA.showtimes.filter((s) => {
    if (state.format !== "all" && s.format !== state.format) return false;
    if (!state.chains.has(s.chain)) return false;
    if (state.cinema !== "all" && s.cinema !== state.cinema) return false;
    if (state.date !== "all" && s.date !== state.date) return false;
    if (state.q && !s.movie.toLowerCase().includes(state.q)) return false;
    return true;
  });
}

function render() {
  const rows = visible();
  const out = $("#results");
  out.innerHTML = "";

  if (rows.length === 0) {
    out.appendChild(mk("div", "empty", "Sin funciones para estos filtros."));
    return;
  }

  // Group by movie, then by cinema.
  const byMovie = groupBy(rows, (s) => s.movie);
  const movies = [...byMovie.keys()].sort((a, b) => a.localeCompare(b, "es"));
  const now = Date.now();

  for (const movie of movies) {
    const list = byMovie.get(movie);
    const card = mk("article", "movie");
    card.appendChild(mk("h2", null, `${movie} <span class="count">· ${list.length} func.</span>`, true));

    const byCinema = groupBy(list, (s) => cinemaLabel(s));
    const cinemas = [...byCinema.keys()].sort((a, b) => a.localeCompare(b, "es"));
    for (const cinema of cinemas) {
      const venue = mk("div", "venue");
      venue.appendChild(mk("div", "name", cinema));
      const times = mk("div", "times");
      for (const s of byCinema.get(cinema).sort((a, b) => a.start.localeCompare(b.start))) {
        times.appendChild(timeChip(s, now));
      }
      venue.appendChild(times);
      card.appendChild(venue);
    }
    out.appendChild(card);
  }
}

function timeChip(s, now) {
  const past = new Date(s.start).getTime() < now;
  const el = mk(s.buyUrl ? "a" : "span", `time ${s.format}${past ? " past" : ""}`);
  if (s.buyUrl) { el.href = s.buyUrl; el.target = "_blank"; el.rel = "noopener"; }
  el.title = `${s.chain} · ${FORMAT_LABEL[s.format] || "?"}${s.screen ? " · " + s.screen : ""}${s.language ? " · " + s.language : ""}`;
  el.appendChild(mk("span", "dot"));
  el.appendChild(document.createTextNode(s.time));
  if (s.screen) el.appendChild(mk("span", "screen", s.screen));
  if (s.language) el.appendChild(mk("span", "lang", s.language));
  return el;
}

// The cinema field may be blank for chains that don't expose it; fall back to the
// chain name so a screening is never orphaned.
function cinemaLabel(s) { return s.cinema || s.chain; }

function dateLabel(d, today) {
  if (d === today) return "Hoy";
  const t = new Date(today + "T12:00:00-06:00");
  const dd = new Date(d + "T12:00:00-06:00");
  if (Math.round((dd - t) / 86400000) === 1) return "Mañana";
  return dd.toLocaleDateString("es-CR", { weekday: "short", day: "numeric", month: "short" });
}

// --- tiny DOM helpers ---
function mk(tag, cls, html, isHTML) {
  const el = document.createElement(tag);
  if (cls) el.className = cls;
  if (html != null) { if (isHTML) el.innerHTML = html; else el.textContent = html; }
  return el;
}
function opt(value, label) {
  const o = document.createElement("option");
  o.value = value; o.textContent = label; return o;
}
function groupBy(arr, keyFn) {
  const m = new Map();
  for (const x of arr) {
    const k = keyFn(x);
    (m.get(k) || m.set(k, []).get(k)).push(x);
  }
  return m;
}
function escape(s) { return s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c])); }

load();
