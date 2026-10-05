import assert from 'node:assert/strict'
import test from 'node:test'
import { createListResource } from '../../src/query/createListResource.js'

function fixture() {
  let state = { search: 'first', scope: 1, items: [], total: 0, page: 1, limit: 2 }
  const requests = []
  const get = () => state
  const set = (patch) => {
    state = { ...state, ...patch }
  }
  const resource = createListResource({
    get,
    set,
    fields: {
      items: 'items',
      total: 'total',
      loading: 'loading',
      loadingMore: 'loadingMore',
      error: 'error',
    },
    scope: (s) => s.scope,
    query: (s) => ({ search: s.search, limit: s.limit, offset: (s.page - 1) * s.limit }),
    fetcher: (params) =>
      new Promise((resolve, reject) => requests.push({ params, resolve, reject })),
  })
  return { get, set, requests, ...resource }
}

test('identical in-flight loads share work; successful loads are reused until invalidation', async () => {
  const f = fixture()
  const first = f.load()
  assert.equal(f.load(), first)
  assert.equal(f.requests.length, 1)
  f.requests[0].resolve({ items: [{ id: 1 }], total: 1 })
  await first
  await f.load()
  assert.equal(f.requests.length, 1)
  f.invalidate()
  const refresh = f.load()
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [], total: 0 })
  await refresh
})

test('a failed request can be retried without a force option', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].reject(new Error('offline'))
  await first
  assert.equal(f.get().error, 'offline')
  assert.equal(f.get().loading, false)
  const retry = f.load()
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [{ id: 2 }], total: 1 })
  await retry
  assert.equal(f.get().error, null)
  assert.deepEqual(f.get().items, [{ id: 2 }])
})

test('changing filters cancels the old request and rejects a late response even if abort is ignored', async () => {
  const f = fixture()
  const first = f.load()
  f.set({ search: 'second' })
  const second = f.load()
  assert.equal(f.requests[0].params.signal.aborted, true)
  f.requests[1].resolve({ items: [{ id: 2 }], total: 1 })
  await second
  f.requests[0].resolve({ items: [{ id: 1 }], total: 1 })
  await first
  assert.deepEqual(f.get().items, [{ id: 2 }])
  assert.equal(f.get().loading, false)
})

test('a directory change invalidates an in-flight response even before the next load starts', async () => {
  const f = fixture()
  const first = f.load()
  f.set({ scope: 2 })
  f.requests[0].resolve({ items: [{ id: 1 }], total: 1 })
  await first
  assert.deepEqual(f.get().items, [])
})

test('append uses the base page offset, joins concurrent loads and keeps edits made while loading', async () => {
  const f = fixture()
  f.set({ page: 3 })
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 5 }, { id: 6 }], total: 10 })
  await first
  const more = f.loadMore()
  assert.equal(f.loadMore(), more)
  assert.equal(f.requests[1].params.offset, 6)
  f.set({ items: [{ id: 5, title: 'edited' }, { id: 6 }] })
  f.requests[1].resolve({ items: [{ id: 7 }, { id: 8 }], total: 10 })
  await more
  assert.deepEqual(f.get().items, [{ id: 5, title: 'edited' }, { id: 6 }, { id: 7 }, { id: 8 }])
})

test('refresh cancels append work and stale completion cannot append to the replacement list', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }], total: 5 })
  await first
  const more = f.loadMore()
  const refresh = f.load({ force: true })
  assert.equal(f.requests[1].params.signal.aborted, true)
  f.requests[2].resolve({ items: [{ id: 3 }], total: 1 })
  await refresh
  f.requests[1].resolve({ items: [{ id: 2 }], total: 5 })
  await more
  assert.deepEqual(f.get().items, [{ id: 3 }])
  assert.equal(f.get().loadingMore, false)
})

test('an empty append stops repeated requests even when the server reports an outdated total', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }], total: 5 })
  await first
  const more = f.loadMore()
  f.requests[1].resolve({ items: [], total: 5 })
  await more
  await f.loadMore()
  assert.equal(f.requests.length, 2)
})

test('prefetches the next page so switching to it needs no request', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  assert.equal(f.requests.length, 2)
  assert.equal(f.requests[1].params.offset, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  f.set({ page: 2 })
  await f.load()
  assert.equal(f.requests.length, 2)
  assert.deepEqual(f.get().items, [{ id: 3 }, { id: 4 }])
  assert.equal(f.get().total, 6)
})

test('prefetches the previous page for backward navigation', async () => {
  const f = fixture()
  f.set({ page: 3 })
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 5 }, { id: 6 }], total: 6 })
  await first

  const prefetch = f.prefetchPrev()
  assert.equal(f.requests.length, 2)
  assert.equal(f.requests[1].params.offset, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  f.set({ page: 2 })
  await f.load()
  assert.equal(f.requests.length, 2)
  assert.deepEqual(f.get().items, [{ id: 3 }, { id: 4 }])
})

test('append consumes a prefetched slice instead of requesting it', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  await f.loadMore()
  assert.equal(f.requests.length, 2)
  assert.deepEqual(f.get().items, [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }])
  assert.equal(f.get().loadingMore, false)
})

test('a page switch joins an in-flight prefetch instead of duplicating it', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  f.set({ page: 2 })
  const next = f.load()
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await Promise.all([prefetch, next])
  assert.deepEqual(f.get().items, [{ id: 3 }, { id: 4 }])
})

test('append joins an in-flight prefetch', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  const more = f.loadMore()
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await Promise.all([prefetch, more])
  assert.deepEqual(f.get().items, [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }])
  assert.equal(f.get().loadingMore, false)
})

test('concurrent prefetches share one request', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const a = f.prefetchNext()
  const b = f.prefetchNext()
  assert.equal(a, b)
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await a
})

test('forced loads ignore and clear prefetched pages', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  f.set({ page: 2 })
  const forced = f.load({ force: true })
  assert.equal(f.requests.length, 3)
  assert.equal(f.requests[2].params.offset, 2)
  f.requests[2].resolve({ items: [{ id: 30 }], total: 6 })
  await forced
  assert.deepEqual(f.get().items, [{ id: 30 }])
})

test('invalidate drops prefetched pages', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  f.invalidate()
  f.set({ page: 2 })
  const next = f.load()
  assert.equal(f.requests.length, 3)
  f.requests[2].resolve({ items: [{ id: 7 }, { id: 8 }], total: 6 })
  await next
  assert.deepEqual(f.get().items, [{ id: 7 }, { id: 8 }])
})

test('a prefetch that settles after invalidation is discarded', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const prefetch = f.prefetchNext()
  f.invalidate()
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await prefetch

  f.set({ page: 2 })
  const next = f.load()
  assert.equal(f.requests.length, 3)
  f.requests[2].resolve({ items: [{ id: 5 }, { id: 6 }], total: 6 })
  await next
  assert.deepEqual(f.get().items, [{ id: 5 }, { id: 6 }])
})

test('prefetching stops at the end of the list', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 2 })
  await first

  await f.prefetchNext()
  assert.equal(f.requests.length, 1)
})

test('prefetching is skipped while the list is loading', async () => {
  const f = fixture()
  const first = f.load()
  await f.prefetchNext()
  assert.equal(f.requests.length, 1)
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first
})

test('prefetching is skipped while an append is in flight', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const more = f.loadMore()
  assert.equal(f.requests.length, 2)
  await f.prefetchNext()
  assert.equal(f.requests.length, 2)
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await more
})

test('prefetch advances to the next slice after an append', async () => {
  const f = fixture()
  const first = f.load()
  f.requests[0].resolve({ items: [{ id: 1 }, { id: 2 }], total: 6 })
  await first

  const more = f.loadMore()
  f.requests[1].resolve({ items: [{ id: 3 }, { id: 4 }], total: 6 })
  await more

  const prefetch = f.prefetchNext()
  assert.equal(f.requests.length, 3)
  assert.equal(f.requests[2].params.offset, 4)
  f.requests[2].resolve({ items: [{ id: 5 }, { id: 6 }], total: 6 })
  await prefetch

  await f.loadMore()
  assert.equal(f.requests.length, 3)
  assert.deepEqual(
    f.get().items.map((item) => item.id),
    [1, 2, 3, 4, 5, 6]
  )
})
