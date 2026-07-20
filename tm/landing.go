package tm

import "fmt"

// landingHTML renders the human-friendly status page served at "/".
func landingHTML(db string) string {
	badge, color := "Connected", "#16a34a"
	if db != "connected" {
		badge, color = "Disconnected", "#dc2626"
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Ticketmaster API</title>
<style>
  :root { color-scheme: light dark; }
  body { margin:0; font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
         background:#0b1120; color:#e2e8f0; display:flex; justify-content:center; padding:40px 16px; }
  .card { width:100%%; max-width:760px; }
  h1 { font-size:28px; margin:0 0 4px; }
  .sub { color:#94a3b8; margin:0 0 24px; }
  .status { display:inline-flex; align-items:center; gap:8px; background:#111827;
            border:1px solid #1f2937; border-radius:999px; padding:6px 14px; font-size:14px; margin-bottom:28px; }
  .dot { width:10px; height:10px; border-radius:50%%; background:%s; }
  h2 { font-size:14px; text-transform:uppercase; letter-spacing:.06em; color:#64748b; margin:28px 0 12px; }
  .ep { display:flex; gap:12px; align-items:center; padding:9px 12px; background:#111827;
        border:1px solid #1f2937; border-radius:10px; margin-bottom:8px; font-size:14px; }
  .m { font-weight:700; font-size:12px; padding:2px 8px; border-radius:6px; min-width:52px; text-align:center; }
  .GET{background:#052e2b;color:#5eead4} .POST{background:#0c2a4d;color:#7dd3fc}
  .DELETE{background:#3f1d1d;color:#fca5a5}
  code { font-family: ui-monospace, Menlo, Consolas, monospace; color:#e2e8f0; }
  a { color:#7dd3fc; }
  .foot { color:#64748b; font-size:13px; margin-top:28px; }
</style>
</head>
<body>
  <div class="card">
    <h1>🎟️ Ticketmaster API</h1>
    <p class="sub">Backend service is running.</p>
    <div class="status"><span class="dot"></span> Database: %s</div>

    <h2>Browse (no login needed)</h2>
    %s

    <h2>Account &amp; Bookings</h2>
    %s

    <p class="foot">This is a JSON API — use these endpoints from Postman or your app.
       Health check: <a href="/health"><code>/health</code></a></p>
  </div>
</body>
</html>`, color, badge,
		rows([][2]string{
			{"GET", "/discovery/v2/events"}, {"GET", "/discovery/v2/events/{id}"}, {"POST", "/discovery/v2/events"},
			{"GET", "/discovery/v2/venues"}, {"GET", "/discovery/v2/venues/{id}"}, {"POST", "/discovery/v2/venues"},
			{"GET", "/discovery/v2/attractions"}, {"GET", "/discovery/v2/attractions/{id}"}, {"POST", "/discovery/v2/attractions"},
			{"GET", "/discovery/v2/classifications"}, {"GET", "/discovery/v2/classifications/{id}"},
		}),
		rows([][2]string{
			{"POST", "/api/register"}, {"POST", "/api/login"},
			{"POST", "/api/bookings"}, {"GET", "/api/bookings"}, {"GET", "/api/bookings/{id}"}, {"DELETE", "/api/bookings/{id}"},
		}),
	)
}

func rows(eps [][2]string) string {
	out := ""
	for _, e := range eps {
		out += fmt.Sprintf(`<div class="ep"><span class="m %s">%s</span><code>%s</code></div>`, e[0], e[0], e[1])
	}
	return out
}
