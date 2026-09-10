import { chromium, expect } from '@playwright/test'
import { spawn, execFileSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import { mkdir, mkdtemp, readFile, writeFile, rm } from 'node:fs/promises'
import { createWriteStream } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'
import { createServer } from 'node:net'
import assert from 'node:assert/strict'

const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repo = resolve(frontend, '..')
const stamp = new Date().toISOString().replace(/[:.]/g, '-')
const output = join(repo, 'handoff', `benchmark-browser-${stamp}`)
const temporary = await mkdtemp(join(tmpdir(), 'sub2api-browser-'))
await mkdir(output, { recursive: true })
const suffix = randomBytes(6).toString('hex')
const pg = `benchmark-browser-pg-${suffix}`
const redis = `benchmark-browser-redis-${suffix}`
const network = `benchmark-browser-${suffix}`
const containers = []
let server, browser, context, networkCreated = false
const password = randomBytes(24).toString('hex') + 'Aa1!'
const secrets = [password]
const checks = []
const delay = ms => new Promise(r => setTimeout(r, ms))
function docker(...args) { return execFileSync('docker', args, { encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] }).trim() }
function sql(query) { return execFileSync('docker', ['exec', '-i', pg, 'psql', '-U', 'postgres', '-d', 'sub2api', '-v', 'ON_ERROR_STOP=1', '-At'], { input: query, encoding: 'utf8' }).trim() }
async function freePort() {
  const socket = createServer()
  await new Promise(r => socket.listen(0, '127.0.0.1', r))
  const port = socket.address().port
  await new Promise(r => socket.close(r))
  return port
}
function sanitize(text) {
  for (const [path, label] of [[temporary, '[TEMP]'], [repo, '[REPO]'], [process.env.USERPROFILE, '[HOME]']]) {
    if (!path) continue
    for (const variant of [path, path.replaceAll('\\', '/'), encodeURI(path.replaceAll('\\', '/'))]) {
      text = text.split(JSON.stringify(variant).slice(1, -1)).join(label)
      text = text.split(variant).join(label)
    }
  }
  for (const secret of secrets) if (secret) text = text.split(secret).join('[REDACTED]')
  return text.replace(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '[REDACTED-JWT]')
}
async function redactTrace(source, target) {
  const require = createRequire(import.meta.url)
  const pw = createRequire(require.resolve('@playwright/test')).resolve('playwright')
  const core = createRequire(pw).resolve('playwright-core')
  const { yauzl, yazl } = require(join(dirname(core), 'lib/utilsBundle.js'))
  const zip = await new Promise((ok, fail) => yauzl.open(source, { lazyEntries: true }, (err, value) => err ? fail(err) : ok(value)))
  const out = new yazl.ZipFile()
  const destination = createWriteStream(target, { flags: 'wx' })
  const finished = new Promise((ok, fail) => { destination.on('close', ok); destination.on('error', fail); out.outputStream.on('error', fail) })
  out.outputStream.pipe(destination)
  await new Promise((ok, fail) => {
    zip.on('error', fail)
    zip.on('end', ok)
    zip.on('entry', entry => {
      if (entry.fileName.endsWith('/')) { zip.readEntry(); return }
      zip.openReadStream(entry, (err, stream) => {
        if (err) { fail(err); return }
        const chunks = []
        stream.on('error', fail)
        stream.on('data', c => chunks.push(c))
        stream.on('end', () => {
          let data = Buffer.concat(chunks)
          try { data = Buffer.from(sanitize(new TextDecoder('utf-8', { fatal: true }).decode(data))) } catch { /* Binary assets contain no textual credentials. */ }
          out.addBuffer(data, entry.fileName)
          zip.readEntry()
        })
      })
    })
    zip.readEntry()
  })
  out.end()
  await finished
}
async function login(page, email) {
  await page.goto(`${base}/login`)
  await page.locator('#email').fill(email)
  await page.locator('#password').fill(password)
  const response = page.waitForResponse(r => r.url().endsWith('/api/v1/auth/login') && r.request().method() === 'POST')
  await page.locator('button[type=submit]').click()
  const loginResponse = await response
  const submitted = loginResponse.request().postDataJSON()
  assert.equal(submitted.email === email, true, 'submitted email changed')
  assert.equal(submitted.password === password, true, 'submitted password changed')
  if (loginResponse.status() !== 200) {
    const failure = await loginResponse.json()
    throw new Error(`Form login rejected: ${loginResponse.status()} ${sanitize(JSON.stringify({ code: failure.code, message: failure.message, reason: failure.reason }))}`)
  }
  await page.waitForFunction(() => !!localStorage.getItem('auth_token'))
  secrets.push(...await page.evaluate(() => ['auth_token', 'refresh_token'].map(k => localStorage.getItem(k))))
  checks.push(`real-form-login:${email}`)
}
let base
try {
  docker('network', 'create', network); networkCreated = true
  docker('run', '--pull', 'never', '-d', '--name', pg, '--network', network, '-p', '127.0.0.1::5432', '-e', `POSTGRES_PASSWORD=${password}`, '-e', 'POSTGRES_DB=sub2api', 'postgres:18.1-alpine3.23'); containers.push(pg)
  docker('run', '--pull', 'never', '-d', '--name', redis, '--network', network, '-p', '127.0.0.1::6379', 'redis:8.4-alpine'); containers.push(redis)
  for (let i = 0; i < 40; i++) { try { docker('exec', pg, 'pg_isready', '-U', 'postgres'); break } catch { await delay(500) } }
  const port = await freePort()
  base = `http://127.0.0.1:${port}`
  const adapter = join(repo, 'workers/capability/meow_adapter.py')
  const adapterHash = createHash('sha256').update(await readFile(adapter)).digest('hex')
  const env = { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot, WINDIR: process.env.WINDIR, TEMP: process.env.TEMP, TMP: process.env.TMP,
    AUTO_SETUP: 'true', DATA_DIR: temporary, SERVER_HOST: '127.0.0.1', SERVER_PORT: String(port), SERVER_MODE: 'release',
    DATABASE_HOST: '127.0.0.1', DATABASE_PORT: docker('port', pg, '5432/tcp').split(':').at(-1), DATABASE_USER: 'postgres', DATABASE_PASSWORD: password, DATABASE_DBNAME: 'sub2api', DATABASE_SSLMODE: 'disable',
    REDIS_HOST: '127.0.0.1', REDIS_PORT: docker('port', redis, '6379/tcp').split(':').at(-1),
    ADMIN_EMAIL: 'benchmark-admin@example.test', ADMIN_PASSWORD: password, JWT_SECRET: randomBytes(32).toString('hex'),
    LLM_DETECTOR_ENGINE_ALLOWED: 'true', CHANNEL_MONITOR_PROBE_WORKER_ALLOWED: 'false',
    LLM_DETECTOR_PYTHON: join(repo, 'handoff/meow-adapter-venv/Scripts/python.exe'), LLM_DETECTOR_ADAPTER: adapter,
    LLM_DETECTOR_ENGINE_ROOT: join(repo, 'handoff/meow-fixed-engine'), LLM_DETECTOR_ADAPTER_SHA256: adapterHash,
    PRICING_REMOTE_URL: `${base}/offline-pricing`, TZ: 'UTC'
  }
  secrets.push(env.JWT_SECRET)
  const logs = []
  server = spawn(join(repo, 'handoff/sub2api-benchmark-local.exe'), [], { cwd: temporary, env, stdio: ['ignore', 'pipe', 'pipe'] })
  server.stdout.on('data', c => logs.push(c)); server.stderr.on('data', c => logs.push(c))
  let ready = false
  for (let i = 0; i < 180; i++) {
    try { if ((await fetch(`${base}/health`, { signal: AbortSignal.timeout(1000) })).ok) { ready = true; break } } catch { /* Startup includes migrations. */ }
    if (server.exitCode !== null) break
    await delay(1000)
  }
  if (!ready) { await writeFile(join(output, 'startup-error.log'), sanitize(Buffer.concat(logs).toString())); throw new Error('Isolated app startup failed') }
  await writeFile(join(output, 'startup.log'), sanitize(Buffer.concat(logs).toString()))
  assert.equal(sql("SELECT count(*) FROM users WHERE email='benchmark-admin@example.test' AND role='admin'"), '1', 'bootstrap administrator missing')
  // Synthetic fixtures only. No production connection or policy acknowledgement is used.
  sql(`INSERT INTO settings(key,value) SELECT 'admin_compliance_acknowledgement:'||id, '{"version":"v2026.06.10"}' FROM users WHERE role='admin' ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;
INSERT INTO settings(key,value) VALUES ('channel_monitor_enabled','true'),('channel_monitor_mode','v2') ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value;
INSERT INTO users(email,password_hash,role,status) SELECT 'benchmark-user@example.test',password_hash,'user','active' FROM users WHERE role='admin' LIMIT 1;`)
  browser = await chromium.launch({ executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe', headless: true, args: ['--disable-background-networking', '--disable-component-update', '--disable-sync'] })
  context = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'en-US' })
  context.setDefaultTimeout(20000)
  context.setDefaultNavigationTimeout(30000)
  await context.route('**/*', route => new URL(route.request().url()).origin === base ? route.continue() : route.abort())
  const page = await context.newPage()
  await page.addLocatorHandler(page.locator('.driver-popover-close-btn'), locator => locator.click())
  const errors = []
  page.on('pageerror', e => errors.push(e.message))
  await login(page, env.ADMIN_EMAIL)
  await context.tracing.start({ screenshots: true, snapshots: true, sources: false })
  await page.goto(`${base}/admin/channels/monitor`)
  await page.getByRole('tab', { name: /Benchmarks|基准版本/ }).click()
  await expect(page.getByText(/No benchmark versions|暂无基准版本/)).toBeVisible()
  const packagePath = join(repo, 'handoff/meow-fixed-engine/benchmarks/official/meow-gpt-baseline--4.5.0-rc4.meow.json')
  await page.locator('input[type=file]').setInputFiles(packagePath)
  await expect(page.getByRole('heading', { name: 'meow-gpt-baseline', exact: true })).toBeVisible({ timeout: 15000 })
  await page.getByRole('button', { name: /Validate and approve|验证并批准/ }).click()
  await expect(page.getByRole('button', { name: /^(Activate|激活)$/ })).toBeVisible({ timeout: 70000 })
  checks.push('real-upload-and-offline-engine-approval')
  await page.getByRole('button', { name: /^(Activate|激活)$/ }).click()
  await page.getByRole('button', { name: /^(Confirm|确认)$/ }).click()
  await expect(page.getByText(/Revision 1|修订版本 1/)).toBeVisible()
  checks.push('activate-with-cas')
  await page.getByRole('button', { name: /^(Activate|激活)$/ }).click()
  const changed = await page.evaluate(async () => {
    const headers = { Authorization: `Bearer ${localStorage.getItem('auth_token')}`, 'Content-Type': 'application/json' }
    const listing = await (await fetch('/api/v1/admin/monitor-benchmarks', { headers })).json()
    const id = listing.data.items[0].id
    return (await fetch(`/api/v1/admin/monitor-benchmarks/${id}/activate`, { method: 'POST', headers, body: JSON.stringify({ channel: 'meow-gpt-baseline', expected_revision: 1 }) })).status
  })
  assert.equal(changed, 200)
  await page.getByRole('button', { name: /^(Confirm|确认)$/ }).click()
  await expect(page.getByText(/Version conflict|版本冲突/).last()).toBeVisible()
  await page.getByRole('button', { name: /^(Cancel|取消)$/ }).click()
  await page.getByRole('button', { name: /^(Refresh|刷新)$/ }).click()
  await expect(page.getByText(/Revision 2|修订版本 2/)).toBeVisible()
  checks.push('real-concurrent-activation-conflict-and-refresh')
  for (const dark of [false, true]) {
    await page.evaluate(value => document.documentElement.classList.toggle('dark', value), dark)
    for (const width of [1280, 768, 375]) {
      await page.setViewportSize({ width, height: 900 })
      await page.waitForTimeout(200)
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `overflow ${width}`)
      await page.screenshot({ path: join(output, `benchmarks-${dark ? 'dark' : 'light'}-${width}.png`), fullPage: true })
    }
  }
  checks.push('light-dark-1280-768-375-no-overflow')
  await page.getByRole('button', { name: /^(Withdraw|撤回)$/ }).click()
  await page.locator('#benchmark-reason').fill('Offline acceptance withdrawal')
  await page.getByRole('button', { name: /^(Confirm|确认)$/ }).click()
  await expect(page.getByText(/^(Withdrawn|已撤回)$/)).toBeVisible()
  assert.equal(sql("SELECT state FROM monitor_benchmark_releases"), 'withdrawn')
  assert.equal(sql("SELECT count(*) FROM monitor_jobs"), '0')
  checks.push('withdraw-persisted-and-no-model-jobs')
  await context.tracing.stop({ path: join(temporary, 'trace.zip') })
  await redactTrace(join(temporary, 'trace.zip'), join(output, 'trace-redacted.zip'))
  await context.close(); context = null
  const userContext = await browser.newContext({ locale: 'en-US' })
  userContext.setDefaultTimeout(20000)
  await userContext.route('**/*', route => new URL(route.request().url()).origin === base ? route.continue() : route.abort())
  const userPage = await userContext.newPage()
  await login(userPage, 'benchmark-user@example.test')
  const denied = await userPage.evaluate(async () => (await fetch('/api/v1/admin/monitor-benchmarks', { headers: { Authorization: `Bearer ${localStorage.getItem('auth_token')}` } })).status)
  assert.equal(denied, 403)
  checks.push('real-user-admin-api-denied')
  await userContext.close()
  assert.deepEqual(errors, [])
  await writeFile(join(output, 'acceptance.json'), JSON.stringify({ driver: 'Playwright', isolated: true, productionChanged: false, modelRequests: 0, checks, pageErrors: errors }, null, 2))
  console.log(JSON.stringify({ output, checks, modelRequests: 0 }))
} catch (error) {
  await writeFile(join(output, 'failure.json'), JSON.stringify({ message: sanitize(error.message), checks }, null, 2))
  throw new Error(sanitize(error.message))
} finally {
  await context?.close().catch(() => {})
  await browser?.close().catch(() => {})
  if (server && server.exitCode === null) { server.kill(); await new Promise(resolve => { server.once('exit', resolve); setTimeout(resolve, 5000) }) }
  for (const name of containers.reverse()) { try { docker('rm', '-f', name) } catch { /* Only containers created by this run. */ } }
  if (networkCreated) { try { docker('network', 'rm', network) } catch { /* Names are unique to this run. */ } }
  await rm(temporary, { recursive: true, force: true })
}
