import assert from 'node:assert/strict'
import test from 'node:test'
import { loadModules } from '../helpers/modules.js'

test('toggling a waterfall mode persists the choice and remembers it on reload', async (t) => {
  const [{ useStore }, { emptyWaterfallModes }] = await loadModules(t, [
    'store.js',
    'state/model.js',
  ])
  let patchPayload = null
  t.mock.method(globalThis, 'fetch', async (_url, init = {}) => {
    if (init?.body) {
      patchPayload = JSON.parse(init.body)
      return Response.json({ ...patchPayload })
    }
    return Response.json({ video_waterfall_default: 'true' })
  })

  await useStore.getState().setWaterfallMode('video', true)
  assert.deepEqual(patchPayload, { video_waterfall_default: true })
  assert.equal(useStore.getState().waterfallModes.video, true)

  // Simulate a fresh page load reading the stored preference.
  useStore.setState({ waterfallModes: emptyWaterfallModes(), config: {} })
  await useStore.getState().loadConfig()
  assert.equal(useStore.getState().waterfallModes.video, true)
})

test('a failed waterfall toggle rolls the choice back', async (t) => {
  const [{ useStore }, { emptyWaterfallModes }] = await loadModules(t, [
    'store.js',
    'state/model.js',
  ])
  t.mock.method(globalThis, 'fetch', async () =>
    Response.json({ error_en: 'Save failed' }, { status: 500 })
  )

  useStore.setState({ waterfallModes: emptyWaterfallModes(), javError: null })
  await useStore.getState().setWaterfallMode('jav', true)
  assert.equal(useStore.getState().waterfallModes.jav, false)
  assert.ok(useStore.getState().javError)
})

test('syncing an already-saved default skips the config write', async (t) => {
  const [{ useStore }, { emptyWaterfallModes }] = await loadModules(t, [
    'store.js',
    'state/model.js',
  ])
  let requests = 0
  t.mock.method(globalThis, 'fetch', async () => {
    requests += 1
    return Response.json({})
  })

  useStore.setState({ waterfallModes: emptyWaterfallModes() })
  useStore.getState().syncWaterfallMode('idol', true)
  assert.equal(useStore.getState().waterfallModes.idol, true)
  assert.equal(requests, 0)
})
