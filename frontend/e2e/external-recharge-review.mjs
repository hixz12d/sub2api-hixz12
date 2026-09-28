import { chromium, expect } from '@playwright/test'
import { createServer } from 'vite'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

const output = resolve('../output/external-recharge-verification')
await mkdir(output, { recursive: true })
const server = await createServer({ server: { host: '127.0.0.1', port: 0 }, optimizeDeps: { entries: ['handoff/external-recharge-preview.html'] } })
let browser
const checks = []
try {
  await server.listen()
  browser = await chromium.launch({ channel: 'chrome', headless: true })
  const port = server.httpServer.address().port
  for (const [width, dark] of [[1280, true], [375, false]]) {
    const context = await browser.newContext({ viewport: { width, height: 1000 } })
    const page = await context.newPage()
    const errors = []
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(`http://127.0.0.1:${port}/handoff/external-recharge-preview.html`, { waitUntil: 'domcontentloaded', timeout: 120000 })
    const toggle = page.getByRole('switch', { name: '展示链动小铺充值' })
    await expect(toggle).toBeVisible({ timeout: 120000 })
    await page.evaluate(dark => document.documentElement.classList.toggle('dark', dark), dark)
    await expect(page.getByTestId('external-recharge-method')).toHaveAttribute('href', '/custom/ldxp-recharge')
    await page.screenshot({ path: resolve(output, `external-recharge-${width}.png`), fullPage: true, animations: 'disabled' })
    await toggle.click()
    await expect(toggle).toHaveAttribute('aria-checked', 'false')
    await expect(page.getByTestId('external-recharge-method')).toHaveCount(0)
    await expect(page.getByTestId('external-recharge-url')).toHaveValue('https://example.com/shop-recharge/')
    await expect(page.getByTestId('perpay-fixture')).toBeVisible()
    await toggle.focus()
    await page.keyboard.press('Space')
    await expect(toggle).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByTestId('external-recharge-method')).toHaveCount(1)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
    expect(overflow).toBe(false)
    expect(errors).toEqual([])
    checks.push({ width, dark, toggleAndKeyboard: true, originalUrlPreserved: true, overflow, errors })
    await context.close()
  }
  await writeFile(resolve(output, 'browser.json'), JSON.stringify({ fixture: true, production: false, checks }, null, 2))
  console.log(JSON.stringify(checks))
} finally {
  await browser?.close()
  await server.close()
}
