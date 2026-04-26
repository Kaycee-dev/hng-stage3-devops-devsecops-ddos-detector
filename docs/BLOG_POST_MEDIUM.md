How I Built a Simple DDoS Detection Engine for Nextcloud


When a website is public, anyone on the internet can send it traffic. Most of that traffic is normal: people logging in, uploading files, opening pages. Some of it is hostile. One IP might send hundreds of requests in a few seconds, or many different IPs might hit the service at the same time. This project watches HTTP traffic in real time, learns what normal looks like, and reacts when traffic starts to look abnormal.

The protected app is Nextcloud, a self-hosted file-sharing platform. I did not change the Nextcloud image. Instead, I placed Nginx in front of it and built a small Go daemon beside it. The repository for everything I describe below is at https://github.com/Kaycee-dev/hng-stage3-devops-devsecops-ddos-detector and the live dashboard runs at http://hng-stage3-kelechi.duckdns.org.


How the pieces fit together

Every visitor request flows through Nginx first. Nginx forwards the request to Nextcloud and writes a single JSON line to a shared log file describing what just happened: source IP, timestamp, HTTP method, URL path, status code, and response size. That log file lives on a Docker volume named HNG-nginx-logs that the detector also mounts, read-only. The detector follows the file the same way the Linux command tail -f follows it. Whenever a new line appears, the detector reads it, parses it, updates its counters, and decides whether anything is wrong.

There are no rate-limiting libraries involved. The whole detector is plain Go using only the standard library. That was a deliberate choice: rate limiters apply blanket caps, but the goal here is to learn what is normal and respond to deviations.


Sliding windows in plain English

A sliding window answers a single question: how many requests happened recently? For this project, recently means the last 60 seconds.

The detector uses a structure called a deque, which is just a queue where you can add new items to the back and remove old items from the front efficiently. Every time a request arrives, the detector adds the request timestamp to the back of the deque. Before reading the count, it removes timestamps older than 60 seconds from the front. The current request rate is just the remaining count divided by 60.

There is one global deque for all requests and a separate deque for every source IP. That gives the detector two views of the world at once: how loud is the whole site, and how loud is each individual visitor.


Learning the baseline

A fixed rule like "block anything over 100 requests per minute" sounds clean but breaks the moment your traffic shape changes. A quiet server and a busy server need different limits. So instead of guessing, the detector measures.

It keeps per-second request counts for the last 30 minutes. Every 60 seconds it recalculates the average requests per second, the standard deviation, and the average rate of error responses (any HTTP status of 400 or higher). It also keeps a separate set of counts for each hour of the day. If the current hour has collected enough samples, the detector prefers that hour's baseline because traffic at 9am can look very different from traffic at 2am.

There are tiny floor values for the mean and standard deviation in the config file. They prevent silly division-by-zero edges and stop the detector from going into red alert when the server has only been running for a few seconds. The config is the only place where these numbers live; the detector code never hardcodes them.


How a decision is made

When a new request comes in, the detector compares the current rate to the baseline in two ways.

The first is a z-score, which sounds fancy but is just this:

zscore = (current_rate - baseline_mean) / baseline_stddev

It tells you, in plain language, how many standard deviations away from normal the current rate is. If the z-score is above 3.0, the traffic is unusual.

The second check is simpler: is the current rate more than 5 times the baseline mean? If yes, the traffic is also unusual.

If either check fires for a single IP, that IP gets blocked. If either check fires for the global rate, the detector sends a Slack alert but does not block anyone. Blocking everyone during a global spike would also block legitimate users.

There is one extra rule for error traffic. If a single IP is producing errors at three times the baseline error rate, the detector tightens its thresholds for that IP. This catches credential-stuffing-style attacks where the request rate alone might not be alarming but the failure rate is.


Blocking at the kernel with iptables

When the detector decides to ban an IP, it does not ask Nginx or the application to drop traffic. It tells the Linux kernel directly using iptables. The command looks like this:

sudo iptables -I INPUT -s 198.51.100.23 -j DROP

[ image — Iptables-banned.png ]

Before adding the rule, the detector checks two things. It checks whether the IP is on an allowlist (private ranges, loopback, IPv6 link-local, the operator's own admin IP) and refuses to touch those. It also checks whether a DROP rule for that IP already exists, which keeps the rule list clean if the same IP keeps being detected.

Bans are not permanent on the first offense. The schedule is 10 minutes for the first ban, 30 minutes for the second, 2 hours for the third, and only then permanent. After the time runs out, the detector removes the iptables rule, sends a Slack notification, and writes an audit line. The same IP coming back will hit the next step on the ladder.


Slack notifications and the audit log

Every ban, every unban, and every baseline recalculation is recorded twice: once as a Slack message in a channel of the operator's choice and once as a structured line in an audit log file inside the detector container. A single ban event in the log looks like this:

[2026-04-26T01:13:05Z] BAN 198.51.100.23 | zscore 3.00 > 3.00 | rate=0.40 | baseline=mean=0.10,stddev=0.10,source=floor | duration=10m

[ image — Audit-log.png ]

The audit log is what graders and incident responders read after the fact. The Slack message is what wakes someone up.

[ image — Ban-slack.png ]


The live dashboard

The detector also serves a tiny web dashboard that refreshes every three seconds. It shows the global requests per second, the top ten source IPs ranked by recent activity, the currently banned IPs with their conditions and durations, CPU and memory usage of the host, the effective baseline mean and standard deviation, and a chart of the baseline over time.

[ image — Tool-running.png ]

The chart at the bottom of the dashboard is the most useful piece for understanding what the detector is doing. Each point is a baseline recalculation, taken every 60 seconds. The horizontal axis is time, labelled in hours. The vertical axis is the effective baseline mean. When traffic is calm, the line stays flat near the floor. When a real attack hits, the line jumps and the labelled hour bucket noticeably exceeds adjacent buckets.

[ image — Baseline-graph.png ]


What I would change next

The hardest part of this project was not the math. It was the discipline of keeping every threshold, floor, and duration in a single config file and refusing the urge to sprinkle magic numbers through the detection code. Once that habit settled, every scoring requirement turned into something I could test by editing one file and re-running the detector.

If I extended this in a real product, the next step would be sharing baselines across instances using something like Redis, so a multi-region detector cluster sees the same picture of normal. The deque and the per-second counters work fine in memory for one host; the moment you horizontally scale, you need a shared store. The math stays exactly the same.

The full code, including the Dockerfile, the Compose stack, the Nginx config, and the helper scripts that produce the screenshots above, is open source at https://github.com/Kaycee-dev/hng-stage3-devops-devsecops-ddos-detector.
