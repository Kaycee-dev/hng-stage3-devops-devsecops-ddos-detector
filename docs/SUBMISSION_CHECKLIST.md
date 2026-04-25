# Submission Checklist

- [ ] Public GitHub repo is pushed and visible.
- [ ] Server has at least 2 vCPU and 2 GB RAM.
- [ ] `.env` contains real `SERVER_IP`, `DASHBOARD_HOST`, strong passwords, and Slack webhook.
- [ ] `docker pull kefaslungu/hng-nextcloud` succeeds on the VPS.
- [ ] `docker compose up -d --build` starts all services.
- [ ] `docker compose ps` shows the stack running.
- [ ] `curl http://SERVER_IP/status.php` reaches Nextcloud by IP.
- [ ] `curl http://DASHBOARD_HOST/health` reaches dashboard by DuckDNS.
- [ ] Nginx writes JSON logs to `/var/log/nginx/hng-access.log`.
- [ ] Detector audit log contains baseline recalculation entries.
- [ ] Synthetic IP attack creates Slack ban alert and `iptables DROP`.
- [ ] Scheduled unban creates Slack unban alert.
- [ ] Synthetic global spike creates Slack global alert only.
- [ ] Required screenshots are present under `screenshots/`.
- [ ] README has server IP, dashboard URL, language choice, algorithm explanations, setup steps, repo link, and blog link.
- [ ] Beginner-friendly blog post is published and linked.
- [ ] Server remains live for the announced 12-hour grading period.
- [ ] Submission form is completed before `2026-04-29 23:59 WAT`.
