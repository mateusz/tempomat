# Tempomat

## The problem

Support gets woken up when SilverStripe servers become unavailable due to high load events caused by crawlers (which come in the night, as everyone who has been on call knows).

Even a single (legitimate or not) crawler can bring a server down because of SilverStripe's ready availability of URL endpoints that eat all the CPU.

## Solutions so far

WAF products can be used to provide an interface for support people to block certain IPs, or even IP classes. It works well but:

* it's reactive. Support still gets woken up, and it works only until next time some other crawler hits the site
* and then someone forgets to remove the blacklist and it locks out a legitimate user.

Another option is to use nginx to throttle access. However support still gets woken up, because:

* throttling level can only be configured in requests per second (RPS), and it must be tweaked manually depending on the individual site performance. Nobody does this in practice, and there is no single magic value of requests per second that'd fit all

* moreover, selecting values is made harder when static assets are co-located with dynamic requests. We can't decide by URL only which requests are dynamic and which static - people sometimes put PHP files in assets and media in modules.

* worse yet, even conservative values could still interfere with users making requests from behind a NAT. This happens on sites heavily used by a single organisation, where all requests come from their gateway.

* lastly, nginx [LimitReqModule](http://nginx.org/en/docs/http/ngx_http_limit_req_module.html) only permits throttling per IP out of the box. Or so I think. I guess it can be done with some hacking using geo module, but anyway we still have the aforementioned problems to deal with.

Yet another approach is limiting the amount of available Apache workers. This doesn't work in practice either: it favours clients making the largest amount of requests. "Slow lane" users (such as the actual humans) get starved out by crawlers - again. Dreaded "DOWN" text message arrives again.

## So what could be done?

Here is an observation:

> Response wall time for all web requests is readily available. It's easy to distinguish "slow" requests from "fast" requests.

Regular nginx ratelimiting is performed based on the *quantity* of requests. But what if instead we throttled based on the *CPU time consumed* by the request? Suddenly we would be able to see who is overloading the server, and we could partition the offenders away from other users automatically, without having to tweak the RPS values.

Every server has a limited, but well-known amount of CPU bandwidth available: it's 1 CPU-second per core. If we manage to limit specific IPs to use no more than N% of that bandwidth, we should be able to ensure there is always some residual capacity available to serve other requests too.

But how can we calculate the CPU bandwidth consumed by request? Well, we can approximate it by looking at the request wall time!

## Throttling model

The actual throttling can be done using a leaky token bucket implementation. This is similar to how t2 instances calculate their credit, and also the same how a [token bucket filter](http://www.tldp.org/HOWTO/html_single/Traffic-Control-HOWTO/#qs-tbf) works on Linux.

All classess of traffic will start with some predefined amount of credits, which are substracted as the class consumes CPU bandwidth. Credits regenerate over time at a predefined rate, to fit with the maximum desired bandwidth consumption.

### Example

Let's assume we have a 1 CPU machine, and we wish to limit certain class of users (say, partitioned by their IP) to consume at most 50% of the CPU.

The user performs a request that takes 2 seconds to serve - 2 CPU-seconds are substracted from their credit. User does further requests this way, until their credit falls to 0, and at this point they are throttled and must wait until the credit recovers.

The class' credit is restored continuously at 0.5 CPU-second per second. This ensures that the user consumes no more than 50% of the CPU, after they have extinguished their initial allowance.

### Another example

Say we want to permit at most 1 CPU-second per second server usage per IP. User starts making requests that take 1 second to process:

|Parallelism|Desired CPU-s/s|Non-throttled RPS|Throttled avg wait per req|Throttled average RPS|
|---|---|---|---|---|
|1|1|1|0s|1|
|2|2|2|1s|1|
|3|3|3|2s|1|

Note other IPs are throttled individually.

### Classification

We are not limited to looking at IP only when classifying requests. We can define any amount of heuristics and see what works best:

* subnets - we can limit CPU usage for entire subnet classes. For example /24 can get 25% of CPU, while /16 could get 50%

* headers - we can trivially classify by User-Agent. This would catch such straightforward repeat offenders as bespoke Java crawlers - which are legitimate, but ungraceful

* session - we can take it even further by connecting to the SilverStripe database, and verifying the user is `loggedInAs`. We can then increase the bandwidth available to logged in users

* URLs - we can monitor for high-intensity URLs, and corral these separately

* fingerprinting - for maximum coolness, it could be possible to classify users by looking at properties beyond HTTP request, such as IP header flags.

### Problem: can't read the future

It's unclear *how much* a user should be throttled because it's impossible to tell how much credit will the next request consume beforehand. Is it a static file request? 0.002 CPU-seconds may be consumed, so the theoretical throttling period should be very short. Is it a heavy listing request? 10.0 CPU-seconds may be consumed, and the user should be limited accordingly.

Current solution to this is "cooperative throttling": don't block processing, but do delay the response after it has been produced by the server. This will cause "friendly" clients to back off ("holding the caller"). Malicious bots can still bring down the server by simply starting large amounts of requests in parallel.

### Problem: computing accurate CPU-seconds

Additionally, an allowance needs to be made to estimate the CPU time consumed by a single request under >100% server load.

Currently I'm ignoring this problem, and assuming if the request took 1s to prodce, then it took 1 CPU-s, regardless of whether it was sleeping, busy on I/O, or the actual server load.

## Usage

Create config file in `/etc/tempomat.json`:

```json
{
	"debug": false,
	"delayThresholdSec": 0.1,
	"backend": "http://localhost:80",
	"listenPort": 8888,
	"graphite": "localhost:2003",
	"graphitePrefix": "some.place.prepend.{hostname}",
	"trustedProxies": "127.0.0.1",
	"cpuCount": 4.0,
	"slash32Share": 0.1,
	"slash24Share": 0.25,
	"slash16Share": 0.5,
	"userAgentShare": 0.1,
	"hashMaxLen": 100
}
```

Run server:

```
go install github.com/mateusz/tempomat
bin/tempomat
```

Connect the doctor:

```
go install github.com/mateusz/tempomat/doctor
bin/doctor
# Or watch:
watch -n 1 bin/doctor
```

Live-reload configuration without stopping:

```
ps # get the tempomat proc id
kill -SIGHUP 12345
```