# API

Everything is under `/api/v1`, JSON in and out, with a session cookie, a personal token (`Authorization: Bearer pnu_…`, Settings → API & shortcuts) or a Pocket Core link token. Energy is in **kcal**, protein/carbs/fat/fibre/sugar/saturated fat in **grams**, sodium in **milligrams**, water in **millilitres**. `date` is the local `YYYY-MM-DD`; `meal` is `breakfast`, `lunch`, `dinner` or `snacks`.

A **food** has its numbers per 100 g (`unit: "g"`), per 100 ml (`"ml"`) or per serving (`"serving"`), and `servings` (`[{label: "1 egg", amount: 50}]`, the amount in the unit); the plain unit (`"g"`, `"ml"`, `"serving"`) is always a serving too. An **entry** is what was eaten: `qty` × `serving` = `amount` (in the food's unit) and its own numbers (`kcal`, `protein`, `carbs`, `fat`, `fiber`, `sugar`, `sat_fat`, `sodium`; the last four `null` when unknown), worked out when logged — correcting a food later does not rewrite the diary.

## Reading (`nutrition:read` for linked apps)

| | |
|---|---|
| `GET /summary` | the short version (Pocket AI, Pocket Day, Homepage): today's `totals`, `entries`, `water_ml`, `goals` (`kcal, protein, carbs, fat, fiber, water, burned, budget`), `remaining`, `week` (see below), `streak` (days in a row logged), a one-line `text` and `url` |
| `GET /day/YYYY-MM-DD` | one date: `meals` (`[{meal, label, totals, entries}]`), `totals`, `entries` (count), `water_ml`, `goals`, `remaining`, `note`, and a `text` ("1,850 kcal of 2,300; protein 112 g, carbs 190 g, fat 64 g. Breakfast: oats, banana…") |
| `GET /overview[?date=]` | the diary page: `{day (as above), week, streak, first_day}` |
| `GET /week[?date=]` | the week a date is in: `days` (`[{date, entries, kcal, protein, …, water_ml}]`), `logged`, `on_goal`, `avg_kcal`, `avg_protein`, `avg_carbs`, `avg_fat`, `goal` |
| `GET /trends[?days=30]` | up to a year day by day, `average`, `split` (% of energy from protein/carbs/fat), `top` (foods that brought the most energy), `by_meal`, `weight` (Pocket Health), `on_goal`, `streak` |
| `GET /entries[?date=\|from=&to=&meal=&limit=]` | the diary in order (day, meal, position); the last 7 days by default |
| `GET /entries/{id}` | one entry |
| `GET /foods[?archived=1]`, `GET /foods/{id}` | your foods (with `uses`, `last_used`), one with its recent entries |
| `GET /foods/search?q=&meal=` | `{foods}`: yours first (most logged, favourites), then the built-in list (`id: 0`, `builtin` set); without `q` the ones you log most at that meal |
| `GET /meals`, `GET /meals/{id}` | saved meals with their `items` (food, `qty`, `serving`, `amount`, numbers) and `totals` |

## Logging (`nutrition:write` for linked apps)

`POST /entries/log` logs one or more foods by name — matched to the person's foods, then the built-in list (with its aliases: `"jaje"` → Egg); a food not known needs its numbers (`kcal`, optional `protein`, `carbs`, `fat`, `fiber`, `sugar`) for the whole item, and `save: true` keeps it as a food:

```json
{
  "date": "2026-11-05", "meal": "breakfast", "ref": "voice-memo-812",
  "items": [
    {"name": "egg", "qty": 2},
    {"name": "White bread", "serving": "1 slice", "qty": 2},
    {"name": "oats", "amount": 60, "unit": "g"},
    {"name": "Grandma's pie", "kcal": 420, "protein": 12, "save": true}
  ]
}
```

`qty` counts servings (the food's first serving when none is named); `amount` + `unit` gives grams or millilitres instead. `food_id` or `builtin` pick a food exactly. `ref` is the caller's own id: logging the same `ref` again changes nothing. The answer is `{entries, count, day: {totals, goals, remaining, text}}`. A linked app may change (`PUT /entries/{id}`: `qty`, `serving`, `amount`, `meal`, `date`, `note`) or remove (`DELETE /entries/{id}`) only the entries it logged itself (403 otherwise).

`POST /water/YYYY-MM-DD/add` `{"ml": 250}` adds water (a negative amount takes some off; without `ml`, one glass of the person's size); answers `{date, ml}` with the day's total.

## The diary (the person themselves)

| | |
|---|---|
| `POST /entries` | log one: `{date, meal, food_id \| builtin, qty, serving}` or `amount` instead of `qty`; a quick add is `{name, kcal, protein, carbs, fat}` without a food |
| `PUT /entries/{id}`, `DELETE /entries/{id}` | change (a new amount works the numbers out again from the food), remove |
| `POST /entries/copy` | `{from, to, meal?, to_meal?, ids?}`: a meal or a whole day to another day |
| `POST /entries/order` | `{ids}`: the order within a meal |
| `PUT /water/YYYY-MM-DD` | `{ml}`: set the day's water |
| `PUT /days/YYYY-MM-DD/note` | `{note}` |
| `POST /foods`, `PUT /foods/{id}`, `DELETE /foods/{id}` | your own foods (`name, brand, barcode, category, unit, kcal, protein, carbs, fat, fiber, sugar, sat_fat, sodium, servings, favorite, archived, notes`) |
| `POST /foods/builtin/{key}` | your own copy of a built-in food (to correct it) |
| `GET /foods/barcode/{code}` | `{found, food, saved}`: yours, else Open Food Facts (unsaved, `source: "off"`) when allowed in Settings |
| `GET /foods/online?q=` | search Open Food Facts by name |
| `POST /scan` | a photo (JPEG/PNG body) → `{found: {format, value}}` (EAN/UPC), for browsers that can't read barcodes |
| `POST /meals`, `PUT /meals/{id}`, `DELETE /meals/{id}` | saved meals: `{name, meal, notes, items: [{food_id \| builtin, qty, serving}]}`, or `{name, from_date, from_meal}` to save what was logged |
| `POST /meals/{id}/log` | `{date, meal, times}` |
| `POST /goals/suggest` | the calculator (`sex, birth_year, height_cm, weight_kg, activity, aim, aim_rate`; missing ones from Settings and Pocket Health's weight) → `{goals: {kcal, protein, carbs, fat, fiber, water, bmr, tdee, lines}}`; nothing is saved |

## The other Pocket apps

| | |
|---|---|
| `GET /apps/day?date=` | `{planned (Pocket Kitchen's meal plan, with per_serving numbers and logged), training (Pocket Fitness), ate_out (Pocket Places restaurants and cafés), apps: {kitchen\|fitness\|places: {state, detail}}}` |
| `GET /apps/recipes?q=` | Pocket Kitchen recipes with `per_serving` numbers (from the recipe's nutrition) |
| `POST /apps/recipes/{id}/log` | `{date, meal, servings, plan_id?}`: log servings of a recipe (a planned one only once) |
| `GET /apps/habits` | Pocket Habits habits, for Settings |
| `POST /apps/health/sync` | send the last week's changed days to Pocket Health now (it also happens by itself) |

Each day's totals go to Pocket Health as `dietary_energy`, `protein`, `carbs`, `fat`, `fiber`, `sugar`, `sodium` and `water`, at noon of that day with source `nutrition` (sent again when the day changes).

## AI (Pocket AI or your own OpenAI-compatible key)

| | |
|---|---|
| `POST /ai/parse` | `{text, meal?, date?}`: "two eggs, toast with butter and a coffee" → `{date, meal, items: [{name, food_id \| builtin, food, qty, serving, amount, unit, nutrients, estimated}]}`; nothing is saved |
| `POST /ai/review` | `{period: week\|month, fresh?}`: the period in plain words (kept for the day) |
| `POST /ai/suggest` | `{date, meal, ask?}`: three ideas that fit what is left, from Kitchen recipes, saved meals and usual foods |
| `POST /ai/estimate` | `{name, brand?}`: a food's typical numbers per 100 g/ml (unsaved) |

## Events (Pocket Core bus)

| Topic | When | Payload |
|---|---|---|
| `nutrition.meal_logged` | food is logged | `date, meal, foods, kcal, protein, carbs, fat, text, url` |
| `nutrition.day_summary` | the next morning, for a day with entries | `date, kcal, protein, carbs, fat, water_ml, goal_kcal, entries, text, url` |
| `nutrition.goal_met` | the next morning, for a day on goal (within −10%/+5% of the calories, 95% of the protein) | `date, kcal, goal_kcal, protein, goal_protein, text` |
| `nutrition.streak` | 7, 30, 100, 365 days in a row logged | `days, date, text` |

Every payload has `user_email`.

## Box

`GET /export.json` (everything of yours), `GET /export.csv` (the diary), `POST /import` (a Pocket Nutrition `.json` or `.csv`, MyFitnessPal's "Nutrition Summary" CSV, Cronometer's "Servings" CSV; importing again adds nothing), `GET /backup` (admin: the whole database). `GET /pocket/portable`: Pocket Move can copy your food diary out and bring it in (`move:export` / `move:import` links).
