# How I Built a Simple DDoS Detection Engine for Nextcloud

When a website is public, anyone on the internet can send it traffic. Most of that traffic is normal: users logging in, uploading files, or opening pages. Sometimes the traffic is hostile. One IP might send too many requests, or many different IPs might hit the service at the same time. This project watches HTTP traffic, learns what normal looks like, and reacts when traffic starts to look abnormal.

The protected app is Nextcloud. I did not change the Nextcloud image. Instead, I placed Nginx in front of it and built a Go daemon beside it.

## What the system does

The flow is:

```text
Internet -> Nginx -> Nextcloud
              |
              v
        JSON access log -> Go detector -> Slack / iptables / dashboard
```

Nginx writes one JSON line for every request. Each line includes the source IP, timestamp, HTTP method, path, status code, and response size. The detector reads that file continuously.

## Sliding windows

A sliding window answers this question: "how many requests happened recently?"

For this project, the recent window is 60 seconds. The detector uses a deque, which is a queue where old items can be removed efficiently from the front and new items can be added at the back.

Every time a request arrives:

1. Add its timestamp to the deque.
2. Remove timestamps older than 60 seconds.
3. Divide the remaining count by 60 to get requests per second.

There is one global deque for all requests and one deque for each source IP.

## Learning the baseline

A fixed threshold like "block anything over 100 requests per minute" is too simple. A quiet server and a busy server need different limits.

The detector keeps per-second request counts for the last 30 minutes. Every 60 seconds it calculates:

- average requests per second
- standard deviation
- average error requests per second

It also keeps per-hour slots. If the current hour has enough data, the detector prefers that hour's baseline because traffic at 9am may be different from traffic at 2am.

## Making a decision

The detector compares the current rate to the baseline in two ways.

First, it calculates a z-score:

```text
zscore = (current_rate - baseline_mean) / baseline_stddev
```

If the z-score is above 3.0, the traffic is unusual.

Second, it checks whether the current rate is more than 5 times the baseline mean.

If either rule fires for one IP, that IP is blocked. If the global traffic rate is abnormal, the detector sends a Slack alert but does not block everyone.

## Blocking with iptables

Linux can drop packets before they reach the application. The detector uses this command when one IP is abusive:

```bash
iptables -I INPUT -s <ip> -j DROP
```

Before adding a rule, the detector checks whether the IP is allowlisted and whether the rule already exists. Private ranges and loopback addresses are protected by default so the detector does not block Docker or local system traffic.

## Unbanning

Temporary bans are released automatically:

1. first ban: 10 minutes
2. second ban: 30 minutes
3. third ban: 2 hours
4. later bans: permanent

Every ban and unban sends a Slack notification and writes an audit log line.

## Dashboard

The dashboard refreshes every 3 seconds and shows:

- global requests per second
- top source IPs
- banned IPs
- CPU and memory usage
- effective mean and standard deviation
- baseline graph over time

That dashboard is the page submitted for grading.

## What I learned

The most important part was separating traffic measurement from response. Nginx only logs and proxies. The detector reads those logs, calculates baselines, makes decisions, and performs actions. That keeps the system easy to reason about and makes every scoring requirement testable.
