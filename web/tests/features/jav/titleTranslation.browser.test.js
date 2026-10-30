import assert from 'node:assert/strict'
import test from 'node:test'
import { writeFile } from 'node:fs/promises'
import { browserUnavailable, openBrowser } from '../../helpers/browser.js'

test(
  'DeepSeek settings and title translation preserve originals and save Chinese display titles',
  { skip: browserUnavailable, timeout: 60000 },
  async (t) => {
    const { origin, command, evaluate, waitFor } = await openBrowser(t)
    await command('Page.navigate', { url: `${origin}/tests/fixtures/app.html?view=jav` })
    await waitFor(`document.querySelector('button[aria-label="Edit JAV"]')`)
    await evaluate(`{
      const originalFetch = window.fetch;
      const config = {...window.testStore.getState().config};
      window.translationRequests = [];
      window.translationFails = false;
      window.fetch = async (input, init = {}) => {
        const pathname = new URL(input, location.origin).pathname;
        if (pathname === '/config' && init.method === 'PATCH') {
          const payload = JSON.parse(init.body);
          window.translationRequests.push({pathname, payload});
          Object.assign(config, payload);
          if ('deepseek_api_key' in payload) config.deepseek_api_key_configured = String(Boolean(payload.deepseek_api_key));
          delete config.deepseek_api_key;
          return Response.json(config);
        }
        if (pathname === '/jav/title-translations') {
          window.translationRequests.push({pathname, payload: JSON.parse(init.body)});
          return window.translationFails
            ? Response.json({error_en: 'Translation unavailable'}, {status: 502})
            : Response.json({zh_title: '宁静的午后'});
        }
        if (pathname === '/jav/items/1' && init.method === 'PUT') {
          const payload = JSON.parse(init.body);
          window.translationRequests.push({pathname, payload});
          return Response.json({...window.testStore.getState().javItems[0], ...payload});
        }
        return originalFetch(input, init);
      };
      document.querySelector('aside button[aria-label="Settings"]').click();
    }`)
    const settings = `document.querySelector('[aria-labelledby="global-settings-title"]')`
    await waitFor(settings)
    await evaluate(
      `Array.from(${settings}.querySelectorAll('button')).find(b => b.textContent.includes('Title Translation')).click()`
    )
    await waitFor(`document.querySelector('#deepseek-api-key')`)
    for (const [width, height] of [
      [375, 812],
      [812, 375],
      [1280, 1000],
    ]) {
      await command('Emulation.setDeviceMetricsOverride', {
        width,
        height,
        deviceScaleFactor: 1,
        mobile: false,
      })
      await waitFor(`${settings}.querySelector('form').getBoundingClientRect().right <= innerWidth`)
      if (process.env.TITLE_TRANSLATION_SCREENSHOT_PREFIX) {
        const screenshot = await command('Page.captureScreenshot', { format: 'png' })
        await writeFile(
          `${process.env.TITLE_TRANSLATION_SCREENSHOT_PREFIX}-${width}.png`,
          Buffer.from(screenshot.result.data, 'base64')
        )
      }
    }
    const setInput = async (selector, value) => {
      await evaluate(`{
        const input = document.querySelector(${JSON.stringify(selector)});
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, ${JSON.stringify(value)});
        input.dispatchEvent(new Event('input', {bubbles: true}));
      }`)
    }
    await setInput('#deepseek-api-key', 'test-private-key')
    await evaluate(`${settings}.querySelector('input[type="checkbox"]').click()`)
    await evaluate(`${settings}.querySelector('form').requestSubmit()`)
    await waitFor(`${settings}.querySelector('[role="status"]')`)
    assert.equal(await evaluate(`document.querySelector('#deepseek-api-key').value`), '')
    assert.equal(await evaluate('window.testStore.getState().config.deepseek_api_key'), undefined)
    assert.equal(await evaluate('window.testStore.getState().config.deepseek_auto_translate'), true)
    await evaluate(
      `${settings}.querySelector('button[aria-label="Close global settings"]').click()`
    )
    await waitFor(`!${settings}`)
    await evaluate(`document.querySelector('button[aria-label="Edit JAV"]').click()`)
    const editor = `document.querySelector('[role="dialog"][aria-label="Edit JAV info"]')`
    await waitFor(`document.querySelector('#jav-title-1')`)
    assert.equal(await evaluate(`document.querySelector('#jav-title-1').value`), 'Test JAV')
    const translateButton = `Array.from(${editor}.querySelectorAll('button')).find(b => b.textContent.includes('Translate with DeepSeek'))`
    await evaluate(`${translateButton}.click()`)
    await waitFor(`document.querySelector('#jav-zh-title-1').value === '宁静的午后'`)
    assert.equal(await evaluate(`document.querySelector('#jav-title-1').value`), 'Test JAV')
    assert.equal(
      await evaluate(`window.translationRequests.some(r => r.pathname === '/jav/items/1')`),
      false
    )
    // A failed retry retains the successful draft and lets the user retry/save.
    await evaluate(`window.translationFails = true; ${translateButton}.click()`)
    await waitFor(`${editor}.querySelector('[role="alert"]')`)
    assert.equal(await evaluate(`document.querySelector('#jav-zh-title-1').value`), '宁静的午后')
    await evaluate(
      `Array.from(${editor}.querySelectorAll('button')).find(b => b.textContent === 'Save').click()`
    )
    await waitFor(`!${editor}`)
    assert.equal(await evaluate('window.testStore.getState().javItems[0].title'), 'Test JAV')
    assert.equal(await evaluate('window.testStore.getState().javItems[0].zh_title'), '宁静的午后')
    await waitFor(
      `Array.from(document.querySelectorAll('span')).some(a => a.textContent === '宁静的午后')`
    )
    await evaluate(`document.querySelector('button[aria-label="View ABC-001 details"]').click()`)
    await waitFor(`document.title === 'ABC-001 宁静的午后'`)
    assert.deepEqual(await evaluate('window.appErrors'), [])
  }
)
