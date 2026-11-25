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

test('scanDirectoryCleanup posts selected categories and duplicate flag', async (t) => {
  const [api] = await loadModules(t, ['features/directories/api.js'])
  const requests = []
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    requests.push({ url, init })
    return Response.json({ junk_items: [], duplicate_groups: [] })
  })

  await api.scanDirectoryCleanup(4, {
    categories: ['system_junk', 'torrent'],
    includeDuplicates: true,
  })

  assert.equal(requests.length, 1)
  assert.equal(requests[0].url, '/directories/4/cleanup/scan')
  assert.equal(requests[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(requests[0].init.body), {
    categories: ['system_junk', 'torrent'],
    include_duplicates: true,
  })
})

test('deleteDirectoryCleanupFiles posts paths and remove_empty_dirs', async (t) => {
  const [api] = await loadModules(t, ['features/directories/api.js'])
  const requests = []
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    requests.push({ url, init })
    return Response.json({ deleted_count: 1, deleted_paths: ['a/b.txt'], failed: [] })
  })

  await api.deleteDirectoryCleanupFiles(4, {
    paths: ['a/b.txt'],
    removeEmptyDirs: true,
  })

  assert.equal(requests.length, 1)
  assert.equal(requests[0].url, '/directories/4/cleanup/delete')
  assert.equal(requests[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(requests[0].init.body), {
    paths: ['a/b.txt'],
    remove_empty_dirs: true,
  })
})

test('directoryCleanupPreviewUrl encodes relative path query param', async (t) => {
  const [api] = await loadModules(t, ['features/directories/api.js'])
  assert.equal(
    api.directoryCleanupPreviewUrl(9, 'sub/扫码 推广.png'),
    '/directories/9/cleanup/preview?path=sub%2F%E6%89%AB%E7%A0%81+%E6%8E%A8%E5%B9%BF.png'
  )
})
