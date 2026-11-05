// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true: four months of
// meals, water, two saved meals, and made-up linked apps: weight and calories burned from "Pocket Health",
// recipes and the meal plan from "Pocket Kitchen", workouts, a restaurant):
//
//   DATA_DIR=/tmp/pnudemo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Amar PORT=18321 ./pocket-nutrition
//   CDP=http://127.0.0.1:9337 BASE=http://127.0.0.1:18321 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18321'
const shots = process.env.SHOTS !== '0'
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1000) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name, full = false) {
  if (!shots) return
  for (let i = 0; ; i++) {
    try { await page.screenshot({ path: dir + name + '.png', fullPage: full }); return } catch (e) { if (i >= 3) throw e; await page.waitForTimeout(800) }
  }
}

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1500)
  assert.ok(await page.locator('.meal-card').count() === 4, 'four meals on the diary')
  assert.ok(await page.getByText('Chicken and vegetable stir-fry').count() > 0, 'Kitchen plan shows')
  await shot(page, 'desktop-diary')
  // the Log food sheet: search, then an amount
  await page.locator('.create-btn').click()
  await page.waitForTimeout(500)
  await page.getByLabel('Search foods').fill('jaje')
  await page.waitForTimeout(700)
  assert.ok(await page.locator('.food-row').filter({ hasText: 'Egg' }).count() > 0, 'alias search finds Egg')
  await shot(page, 'desktop-search')
  await page.locator('.food-row').filter({ hasText: 'Egg' }).first().click()
  await page.waitForTimeout(400)
  await shot(page, 'desktop-amount')
  await page.keyboard.press('Escape')
  await go(page, '/trends', 1500)
  await shot(page, 'desktop-trends')
  await go(page, '/foods', 800)
  await shot(page, 'desktop-foods')
  await go(page, '/meals', 800)
  await shot(page, 'desktop-meals')
  await go(page, '/settings/goals', 800)
  await page.getByText('Work out my goals').click()
  await page.waitForTimeout(700)
  await shot(page, 'desktop-settings-goals')
  await go(page, '/settings/apps', 800)
  await shot(page, 'desktop-settings-apps')
  await ctx.close()
}
// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1500)
  await shot(page, 'phone-diary')
  assert.ok(!await page.evaluate(() => { const c = document.querySelector('.content'); return !!c && c.scrollWidth > c.clientWidth + 1 }), 'the phone diary does not scroll sideways')
  await page.locator('.fab').click()
  await page.waitForTimeout(600)
  await shot(page, 'phone-add')
  await page.keyboard.press('Escape')
  await go(page, '/?date=' + new Date(Date.now() - 86400e3).toISOString().slice(0, 10), 1000)
  await shot(page, 'phone-yesterday', true)
  const overflow = await page.evaluate(() => { const c = document.querySelector('.content'); return document.documentElement.scrollWidth > window.innerWidth + 1 || (!!c && c.scrollWidth > c.clientWidth + 1) })
  assert.ok(!overflow, 'the phone page does not scroll sideways')
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true, dark: true })
  await go(page, '/trends', 1500)
  await shot(page, 'phone-trends-dark')
  await go(page, '/meals', 800)
  await shot(page, 'phone-meals-dark')
  await ctx.close()
}
await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
