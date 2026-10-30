# chatgpt-api

Anonymous `chatgpt.com` as an OpenAI-compatible API. No account, no key, no token.

```
POST /v1/chat/completions   OpenAI chat completions + SSE streaming + tools
GET  /v1/models             gpt-6
GET  /health                health check for Render
GET  /                      same as /health
```

## Upstream chain

```
GET  /                                          data-build="prod-…"  -> OAI-Client-Version
POST /backend-anon/sentinel/chat-requirements    token + proofofwork{seed,difficulty}
     solve proofofwork                           FNV-1a(seed + base64(config)) -> gAAAAAB…~S
POST /backend-anon/f/conversation/prepare       conduit_token
POST /backend-anon/f/conversation               SSE  (data: lines, no event: names)
     OpenAI-Sentinel-Chat-Requirements-Token
     OpenAI-Sentinel-Proof-Token
     X-Conduit-Token
     OAI-Echo-Logs: 0,<t1>,1,<t2>
     OAI-Device-Id, OAI-Client-Version
```

`/backend-anon/conversation` (the URL most older scripts use) is dead — it answers
401/403. The chain above is what works today.

## Run

```bash
go build -o chatgpt-api .
./chatgpt-api -port 8082
```

```bash
curl -s localhost:8082/v1/chat/completions -H 'Content-Type: application/json' -d '{
  "model": "gpt-6",
  "messages": [{"role": "user", "content": "hi"}]
}'
```

```python
from openai import OpenAI
c = OpenAI(base_url="http://localhost:8082/v1", api_key="none")
for ch in c.chat.completions.create(model="gpt-6", stream=True,
        messages=[{"role": "user", "content": "write a haiku"}]):
    print(ch.choices[0].delta.content or "", end="")
```

## Deploy on Render (Web Service)

No Blueprint, no payment method. Render → **New +** → **Web Service** → connect
the repo, then:

| field | value |
| --- | --- |
| Language / Runtime | **Docker** |
| Branch | `main` |
| Root Directory | *(leave empty)* |
| Dockerfile Path | `./Dockerfile` |
| Docker Command | *(leave empty)* |
| Health Check Path | `/health` |
| Instance Type | **Free** |

**Do not set `PORT`.** Render injects it and the binary reads it automatically
(`PORT` env → `-port` flag → `config.json`). Setting it to 8082 makes Render's
router look on a port nothing listens on.

Optional env vars (Advanced → Add Environment Variable):

| var | value |
| --- | --- |
| `IMPERSONATE` | `chrome_146` |
| `DEFAULT_MODEL` | `gpt-6` |
| `LOG_REQUESTS` | `true` |
| `API_KEYS` | `key1,key2` — when set, `Authorization: Bearer <key>` is required |
| `CHATGPT_PROXIES` | `http://u:p@ip:port,http://...` — one per egress slot |

### The Blueprint trap

`render.yaml` is intentionally **not** in this repo. Render's Blueprint flow
(`New +` → **Blueprint**) asks for billing details even when `plan: free` is set,
and the free-tier Blueprint will not deploy without it. A plain **Web Service**
with the table above deploys on the free plan with no card.

### Free plan behaviour

| | |
| --- | --- |
| Sleep | after 15 min idle; first request after that takes ~30 s (cold start) |
| Egress IP | changes on restart/sleep — a fresh IP gets a fresh anonymous quota, which helps here |
| Bandwidth | 100 GB/month |
| Build minutes | 500/month shared across services |
| Disk | ephemeral — `session*.json` does not survive a deploy |

Keep it warm (free, no card) with any uptime pinger on `https://<app>.onrender.com/health`.

## Config file

`config.json` next to the binary (or `-config path`). Env vars override it.

```json
{
  "port": 8082,
  "host": "0.0.0.0",
  "default_model": "gpt-6",
  "request_timeout_sec": 300,
  "retry_attempts": 4,
  "retry_delay_sec": 2,
  "log_requests": true,
  "api_keys": [],
  "chatgpt_base": "https://chatgpt.com",
  "chatgpt_default_model": "gpt-6",
  "chatgpt_user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
  "chatgpt_proxies": [],
  "sessions": [],
  "session_file": "",
  "impersonate": "chrome_146"
}
```

## TLS fingerprint

`internal/fingerprint` replays a real browser handshake through
`bogdanfinn/tls-client`, so Cloudflare's `cf-mitigated` layer is cleared before
the request is even scored on headers.

```
chrome_120 chrome_124 chrome_131 chrome_133 chrome_144 chrome_146
firefox_120 firefox_123 firefox_147 safari_16_0 safari_ios_17_0
```

```bash
./chatgpt-api -impersonate chrome_131
./chatgpt-api -impersonate ""          # stock Go TLS
```

## Layout

```
main.go                     flags, env, config, router, graceful shutdown
internal/config/            config.json loader
internal/chatgpt/client.go  egress rotation, cookies, retries
internal/chatgpt/sentinel.go build id, sentinel token, proof, conduit
internal/chatgpt/generate.go request shaping, Generate / GenerateStream
internal/chatgpt/stream.go  SSE parser
internal/chatgpt/session.go cookie-file reload
internal/pow/               FNV-1a proof-of-work solver
internal/fingerprint/       browser TLS/H2 handshake
internal/server/            OpenAI handlers, auth, logging
```

## License

MIT
