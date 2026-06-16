# GoHttpAuto

HTTP-first automation platform with a professional light-theme dashboard.

**Live panel:** [panel.1clkaccess.store](https://panel.1clkaccess.store)  
**GitHub:** [sagartiwari-net/newhttpgoauto](https://github.com/sagartiwari-net/newhttpgoauto)

## Structure

```
gohttpauto/
├── database/schema.sql    ← MySQL schema
├── server/                ← Go API + automation engine
├── dashboard/             ← React light-theme UI
└── deploy/                ← Server & worker install scripts
```

## Architecture

| Machine | Role | `ENABLE_SCHEDULER` |
|---------|------|-------------------|
| VPS (`panel.1clkaccess.store`) | Dashboard + API | `false` |
| Mac worker | Runs automations | `true` |

Both connect to the same MySQL database `newhttpgoauto`.

---

## Deploy Panel (VPS)

Path: `/www/wwwroot/panel.1clkaccess.store`

```bash
cd /www/wwwroot/panel.1clkaccess.store
git clone https://github.com/sagartiwari-net/newhttpgoauto.git .
cp deploy/.env.panel.example server/.env
nano server/.env   # set DB password, JWT_SECRET, master password
chmod +x deploy/install-panel.sh
./deploy/install-panel.sh
```

**aaPanel nginx** — add reverse proxy to `127.0.0.1:4010` (see `deploy/nginx-panel.conf.example`).

---

## Automation Worker (Mac)

```bash
git clone https://github.com/sagartiwari-net/newhttpgoauto.git
cd newhttpgoauto
cp deploy/.env.worker.example server/.env
nano server/.env   # DB credentials; use tunnel if needed

# Terminal 1 — SSH tunnel (if remote MySQL blocked)
ssh -L 3307:127.0.0.1:3306 root@74.208.99.161

# Terminal 2 — worker
chmod +x deploy/install-worker.sh
./deploy/install-worker.sh
cd server && ./gohttpauto
```

---

## Local Development

```bash
# server
cd server && cp .env.example .env && go run ./cmd

# dashboard (separate terminal)
cd dashboard && npm install && npm run dev
```

Open http://localhost:5173 — login `admin` / password from `.env`

---

## API Trigger (external servers)

```bash
curl -X POST https://panel.1clkaccess.store/api/tasks/run \
  -H "X-API-Key: your_api_key" \
  -H "Content-Type: application/json" \
  -d '{"task_uid":"nox_runSemrush"}'
```

## Features

- Dashboard stats, cron control, manual run
- Sessions: JSON, Netscape, Header, LocalStorage, IndexedDB
- Portal credentials + scraped logins (masked passwords)
- 2-day log retention
- Nox Semrush HTTP automation
