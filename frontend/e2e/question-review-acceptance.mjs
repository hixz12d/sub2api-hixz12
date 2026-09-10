import { chromium, expect } from '@playwright/test'
import { createServer } from 'vite'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const output = resolve('handoff', `human-review-browser-${new Date().toISOString().replace(/[:.]/g, '-')}`)
await mkdir(output, { recursive: true })
const server = await createServer({ server: { host: '127.0.0.1', port: 0, strictPort: false }, optimizeDeps: { entries: ['handoff/manual-question-preview.html'] } })
let browser
const checks = []
try {
  await server.listen()
  const port = server.httpServer.address().port
  browser = await chromium.launch({ channel: 'chrome', headless: true })
  for (const width of [1280, 768, 375]) {
    const context = await browser.newContext({ viewport: { width, height: 950 } })
    const page = await context.newPage()
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`http://127.0.0.1:${port}/handoff/manual-question-preview.html`)
    await page.waitForFunction(() => typeof window.__openQuestionFixture === 'function')
    await page.evaluate(() => window.__openQuestionFixture(42))
    await page.getByText('常规请求', { exact: true }).click()
    await page.getByRole('option', { name: '人工问答', exact: true }).click()
    await expect(page.getByText('Offline stored answer', { exact: true })).toBeVisible()
    await page.locator('select').last().selectOption('degraded')
    await page.locator('textarea').last().fill('Browser fixture assessment')
    await page.getByRole('button', { name: '保存判定', exact: true }).click()
    const disable = page.getByRole('button', { name: '单独停用账号', exact: true })
    await expect(disable).toBeVisible()
    expect(await page.evaluate(() => window.__disabledRequests || 0)).toBe(0)
    await disable.click()
    await expect(page.getByText(/确认停用账号 #42/)).toBeVisible()
    await page.screenshot({ path: resolve(output, `confirm-${width}.png`) })
    await page.getByRole('button', { name: '取消', exact: true }).click()
    expect(await page.evaluate(() => window.__disabledRequests || 0)).toBe(0)
    await disable.click()
    await page.getByRole('button', { name: '确认', exact: true }).click()
    await expect(page.getByText('账号已停用', { exact: true })).toBeVisible()
    expect(await page.evaluate(() => window.__disabledRequests)).toBe(1)
    expect(await page.evaluate(() => window.__questionRequests.length)).toBe(0)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
    expect(overflow).toBe(false)
    expect(errors).toEqual([])
    await page.screenshot({ path: resolve(output, `disabled-${width}.png`) })
    checks.push({ width, cancelledWithoutWrite: true, confirmedWrites: 1, modelRequests: 0, overflow, errors })
    await context.close()
  }
  await writeFile(resolve(output, 'receipt.json'), JSON.stringify({ fixture: true, production: false, driver: 'Playwright Chrome', checks }, null, 2), { flag: 'wx' })
  console.log(JSON.stringify({ output, checks }, null, 2))
} finally {
  await browser?.close()
  await server.close()
}
