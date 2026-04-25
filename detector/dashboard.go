package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type DashboardServer struct {
	cfg     Config
	engine  *Engine
	sampler *SystemSampler
	server  *http.Server
}

func NewDashboardServer(cfg Config, engine *Engine) *DashboardServer {
	mux := http.NewServeMux()
	dashboard := &DashboardServer{
		cfg:     cfg,
		engine:  engine,
		sampler: &SystemSampler{},
		server:  &http.Server{Addr: cfg.DashboardAddr, Handler: mux},
	}
	mux.HandleFunc("/", dashboard.handleIndex)
	mux.HandleFunc("/health", dashboard.handleHealth)
	mux.HandleFunc("/api/metrics", dashboard.handleMetrics)
	return dashboard
}

func (d *DashboardServer) ListenAndServe() error {
	return d.server.ListenAndServe()
}

func (d *DashboardServer) Shutdown(ctxDone <-chan struct{}) {
	<-ctxDone
	_ = d.server.Close()
}

func (d *DashboardServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (d *DashboardServer) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	cpu, memory := d.sampler.Sample()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(d.engine.Snapshot(cpu, memory)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *DashboardServer) handleIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dashboardTemplate.Execute(w, map[string]interface{}{
		"Title": d.cfg.Dashboard.Title,
	})
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <style>
    :root { color-scheme: light; --ink:#172026; --muted:#64727d; --line:#d8dee4; --bg:#f6f8fa; --panel:#ffffff; --accent:#0f766e; --danger:#b42318; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--bg); color:var(--ink); }
    header { padding:20px 28px; background:#102a43; color:#fff; display:flex; justify-content:space-between; gap:18px; align-items:center; flex-wrap:wrap; }
    h1 { margin:0; font-size:24px; font-weight:720; letter-spacing:0; }
    .status { font-size:14px; color:#d9e2ec; }
    main { padding:24px 28px 32px; max-width:1280px; margin:0 auto; }
    .grid { display:grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap:14px; }
    .panel { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:16px; min-width:0; }
    .metric-label { color:var(--muted); font-size:13px; margin-bottom:8px; }
    .metric-value { font-size:28px; font-weight:760; white-space:nowrap; }
    .wide { grid-column: span 2; }
    .full { grid-column: 1 / -1; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th, td { text-align:left; border-bottom:1px solid var(--line); padding:9px 6px; vertical-align:top; }
    th { color:var(--muted); font-weight:650; }
    .danger { color:var(--danger); font-weight:700; }
    .ok { color:var(--accent); font-weight:700; }
    canvas { width:100%; height:260px; border:1px solid var(--line); border-radius:6px; background:#fff; display:block; }
    @media (max-width: 900px) { .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .wide { grid-column: 1 / -1; } }
    @media (max-width: 560px) { header, main { padding-left:16px; padding-right:16px; } .grid { grid-template-columns: 1fr; } .metric-value { font-size:24px; } }
  </style>
</head>
<body>
  <header>
    <h1>{{ .Title }}</h1>
    <div class="status" id="status">Loading live metrics...</div>
  </header>
  <main>
    <section class="grid">
      <div class="panel"><div class="metric-label">Global req/s</div><div class="metric-value" id="globalRate">0.00</div></div>
      <div class="panel"><div class="metric-label">Effective mean</div><div class="metric-value" id="mean">0.00</div></div>
      <div class="panel"><div class="metric-label">Effective stddev</div><div class="metric-value" id="stddev">0.00</div></div>
      <div class="panel"><div class="metric-label">Banned IPs</div><div class="metric-value" id="banCount">0</div></div>
      <div class="panel"><div class="metric-label">CPU usage</div><div class="metric-value" id="cpu">0%</div></div>
      <div class="panel"><div class="metric-label">Memory usage</div><div class="metric-value" id="memory">0%</div></div>
      <div class="panel"><div class="metric-label">Tracked sources</div><div class="metric-value" id="tracked">0</div></div>
      <div class="panel"><div class="metric-label">Uptime</div><div class="metric-value" id="uptime">0s</div></div>
      <div class="panel wide">
        <div class="metric-label">Top 10 source IPs</div>
        <table><thead><tr><th>IP</th><th>Req/s</th><th>Window hits</th></tr></thead><tbody id="topIps"></tbody></table>
      </div>
      <div class="panel wide">
        <div class="metric-label">Banned IPs</div>
        <table><thead><tr><th>IP</th><th>Duration</th><th>Condition</th></tr></thead><tbody id="bans"></tbody></table>
      </div>
      <div class="panel full">
        <div class="metric-label">Baseline over time</div>
        <canvas id="baselineCanvas" width="1100" height="260"></canvas>
      </div>
    </section>
  </main>
  <script>
    const fmt = (n, digits = 2) => Number(n || 0).toFixed(digits);
    const uptime = (seconds) => {
      seconds = Number(seconds || 0);
      const h = Math.floor(seconds / 3600);
      const m = Math.floor((seconds % 3600) / 60);
      const s = seconds % 60;
      return h > 0 ? h + "h " + m + "m" : (m > 0 ? m + "m " + s + "s" : s + "s");
    };
    function rows(items, empty, mapper) {
      if (!items || items.length === 0) return "<tr><td colspan=\"3\">" + empty + "</td></tr>";
      return items.map(mapper).join("");
    }
    function drawBaseline(points) {
      const canvas = document.getElementById("baselineCanvas");
      const ctx = canvas.getContext("2d");
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      ctx.strokeStyle = "#d8dee4";
      ctx.lineWidth = 1;
      for (let i = 0; i < 5; i++) {
        const y = 24 + i * 46;
        ctx.beginPath(); ctx.moveTo(44, y); ctx.lineTo(canvas.width - 16, y); ctx.stroke();
      }
      const data = (points || []).slice(-120);
      ctx.fillStyle = "#64727d";
      ctx.font = "12px system-ui";
      if (data.length < 2) {
        ctx.fillText("Waiting for baseline recalculations", 48, 132);
        return;
      }
      const max = Math.max(...data.map(p => p.mean || 0), 1);
      const xStep = (canvas.width - 72) / Math.max(data.length - 1, 1);
      ctx.strokeStyle = "#0f766e";
      ctx.lineWidth = 3;
      ctx.beginPath();
      data.forEach((p, i) => {
        const x = 44 + i * xStep;
        const y = canvas.height - 28 - ((p.mean || 0) / max) * (canvas.height - 58);
        if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
      });
      ctx.stroke();
      ctx.fillStyle = "#172026";
        ctx.fillText("max mean " + max.toFixed(2), 48, 18);
      ctx.fillText(data[0].window_name || "", 44, canvas.height - 8);
      ctx.fillText(data[data.length - 1].window_name || "", canvas.width - 72, canvas.height - 8);
    }
    async function load() {
      const res = await fetch("/api/metrics", { cache: "no-store" });
      const data = await res.json();
      document.getElementById("status").textContent = "Last baseline " + (data.last_baseline_at || "pending") + " via " + (data.baseline_source || "floor");
      document.getElementById("globalRate").textContent = fmt(data.global_req_per_sec);
      document.getElementById("mean").textContent = fmt(data.effective_mean);
      document.getElementById("stddev").textContent = fmt(data.effective_stddev);
      document.getElementById("banCount").textContent = (data.banned_ips || []).length;
      document.getElementById("cpu").textContent = fmt(data.cpu_percent, 1) + "%";
      document.getElementById("memory").textContent = fmt(data.memory_percent, 1) + "%";
      document.getElementById("tracked").textContent = data.tracked_source_count || 0;
      document.getElementById("uptime").textContent = uptime(data.uptime_seconds);
      document.getElementById("topIps").innerHTML = rows(data.top_ips, "No active sources", ip => "<tr><td>" + ip.ip + "</td><td>" + fmt(ip.rate) + "</td><td>" + ip.requests + "</td></tr>");
      document.getElementById("bans").innerHTML = rows(data.banned_ips, "No active bans", ban => "<tr><td class=\"danger\">" + ban.ip + "</td><td>" + ban.duration + "</td><td>" + ban.condition + "</td></tr>");
      drawBaseline(data.baseline_history);
    }
    load();
    setInterval(load, 3000);
  </script>
</body>
</html>`))

type SystemSampler struct {
	mu        sync.Mutex
	lastTotal uint64
	lastIdle  uint64
}

func (s *SystemSampler) Sample() (float64, float64) {
	return s.sampleCPU(), sampleMemory()
}

func (s *SystemSampler) sampleCPU() float64 {
	total, idle, err := readCPUStat()
	if err != nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastTotal == 0 {
		s.lastTotal = total
		s.lastIdle = idle
		return 0
	}
	totalDelta := total - s.lastTotal
	idleDelta := idle - s.lastIdle
	s.lastTotal = total
	s.lastIdle = idle
	if totalDelta == 0 {
		return 0
	}
	return 100 * (1 - float64(idleDelta)/float64(totalDelta))
}

func readCPUStat() (uint64, uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, fmt.Errorf("empty /proc/stat")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("unexpected /proc/stat")
	}
	var total uint64
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		total += value
	}
	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	return total, idle, nil
}

func sampleMemory() float64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()
	values := map[string]float64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseFloat(fields[1], 64)
		if err == nil {
			values[key] = value
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total <= 0 {
		return 0
	}
	return 100 * (1 - available/total)
}
