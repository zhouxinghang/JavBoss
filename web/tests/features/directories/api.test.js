import assert from 'node:assert/strict'
import test from 'node:test'
import { loadModules } from '../../helpers/modules.js'

test('scanDirectory requests one manual scan without force by default', async (t) => {
  const [api] = await loadModules(t, ['features/directories/api.js'])
  const requests = []
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    requests.push({ url, init })
    return Response.json({ work_status: 'scanning' })
  })

  await api.scanDirectory(7)

  assert.equal(requests.length, 1)
  assert.equal(requests[0].url, '/directories/7/scan')
  assert.equal(requests[0].init.method, 'POST')
})

test('scanDirectory forwards force so the server ignores cached failures', async (t) => {
  const [api] = await loadModules(t, ['features/directories/api.js'])
  const requests = []
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    requests.push({ url, init })
    return Response.json({ work_status: 'scanning' })
  })

  await api.scanDirectory(7, { force: true })

  assert.equal(requests.length, 1)
  assert.equal(requests[0].url, '/directories/7/scan?force=true')
  assert.equal(requests[0].init.method, 'POST')
})
