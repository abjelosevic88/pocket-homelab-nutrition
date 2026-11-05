# Configuration

Environment variables; every one has a default. Personal settings (units, goals, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What |
|---|---|---|
| `PORT`, `HOST` | `8080`, `0.0.0.0` | Listen address |
| `BASE_URL` | – | Public URL (links in pushes, the OIDC callback) |
| `DATA_DIR` | `./data` | Database, Core token, backups |
| `DB_PATH` | `$DATA_DIR/pocket-nutrition.db` | SQLite file |
| `MAX_BODY_BYTES` | `67108864` | Largest JSON request, e.g. an export being imported (64 MB) |
| `TZ` | – | Time zone of the box: what "today" is, the messages, the week |
| `USER_AGENT` | `PocketNutrition/1.0 (+github…)` | Sent to the other apps and AI |
| `AUTH_MODE` | `local` | `local` (accounts), `proxy` (trust `AUTH_PROXY_HEADER`/`AUTH_PROXY_EMAIL_HEADER`), `none` |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | Seed the first admin while the database is empty |
| `SESSION_TTL`, `SECURE_COOKIES` | `720h`, `false` | Set `SECURE_COOKIES=true` behind HTTPS |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_NAME`, `OIDC_AUTO_CREATE` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL`, `NOTIFY_FORMAT` | – | Apprise (`…/notify/key`), ntfy topic URL or JSON webhook; format `apprise`/`ntfy`/`json` |
| `NOTIFY_HOUR` | `8` | Default hour of the morning messages (each person can change theirs) |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` | – | A meal from a sentence, your eating in plain words and meal ideas, without Pocket AI |
| `CORE_URL`, `CORE_REGISTRATION_KEY`, `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY`, `METRICS_ENABLED`, `LOG_LEVEL`, `LOG_FORMAT` | `true`, `true`, `info`, `text` | |
| `DEMO_DATA` | `false` | Seed four months of meals, water and two saved meals into the owner account and, without Pocket Core, made-up linked apps (weight and calories burned from "Pocket Health", recipes and the meal plan from "Pocket Kitchen", workouts, a restaurant) — for screenshots and trying it out, never for real use |

Commands: `pocket-nutrition backup [-o FILE]`, `pocket-nutrition reset-password [-email] [-password]`, `pocket-nutrition version`.

Connections to Pocket Health, Kitchen, Fitness, Habits and Places are made per person under Settings → Connected apps: through Pocket Core (nothing to paste) or directly with the app's URL and an API token. Open Food Facts (barcodes and searches) needs no key; each person can switch it off.

Background work: every minute the messages (at each person's hours); every ten minutes the days changed in the last week go to Pocket Health and the chosen habit is ticked (a change is also sent about 20 seconds after it is made).
