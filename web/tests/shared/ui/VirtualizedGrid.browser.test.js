import assert from 'node:assert/strict'
import test from 'node:test'
import { browserUnavailable, openBrowser } from '../../helpers/browser.js'

test(
  'virtualized video grid mounts only nearby rows and reveals later items on scroll',
  { skip: browserUnavailable, timeout: 60000 },
  async (t) => {
    const { origin, command, evaluate, waitFor } = await openBrowser(t, {
      cacheDir: 'node_modules/.vite-virtual-grid-test',
    })

    await command('Page.navigate', {
      url: `${origin}/tests/fixtures/virtualizedGrid.html`,
    })
    await waitFor(`document.querySelectorAll('.video-card').length > 0`)

    assert.equal(await evaluate('window.fixtureVideoCount'), 200)

    const mountedAtTop = await evaluate(`document.querySelectorAll('.video-card').length`)
    assert.ok(
      mountedAtTop > 0 && mountedAtTop < 80,
      `expected only a window of cards to mount, got ${mountedAtTop}`
    )

    const scrollHeight = await evaluate('document.documentElement.scrollHeight')
    assert.ok(scrollHeight > 2000, `expected a tall virtual scroll area, got ${scrollHeight}`)

    // The final item must not be mounted until the list is scrolled near it.
    assert.equal(await evaluate(`document.querySelectorAll('img[src^="/videos/200/"]').length`), 0)

    await evaluate('window.scrollTo(0, document.documentElement.scrollHeight)')
    await waitFor(`document.querySelectorAll('img[src^="/videos/200/"]').length > 0`)

    const mountedAtBottom = await evaluate(`document.querySelectorAll('.video-card').length`)
    assert.ok(
      mountedAtBottom < 80,
      `expected only a window of cards after scrolling, got ${mountedAtBottom}`
    )
  }
)
