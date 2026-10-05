import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'
import { loadModules } from './helpers/modules.js'

const source = fs.readFileSync(new URL('../src/utils/urlState.js', import.meta.url), 'utf8')

test('legacy global directory scope fields are not parsed or serialized', () => {
  assert.doesNotMatch(source, /directoryFilterMode|enabledDirectoryIds/)
})

test('JAV list URL state round-trips the original directory filter', async (t) => {
  const [{ parseUrlState, buildUrlFromState }] = await loadModules(t, ['utils/urlState.js'])

  const videoParsed = parseUrlState('?view=video&unmatched=1&page=2')
  assert.equal(videoParsed.video.unmatched, true)
  assert.match(buildUrlFromState({ ...videoParsed, view: 'video' }, '/'), /unmatched=1/)
  const videoOff = parseUrlState('?view=video')
  assert.equal(videoOff.video.unmatched, false)
  assert.doesNotMatch(buildUrlFromState({ ...videoOff, view: 'video' }, '/'), /unmatched/)

  const parsed = parseUrlState('?view=jav&tab=list&directory_ids=3,5')
  assert.deepEqual(parsed.jav.directoryIds, [3, 5])
  const url = buildUrlFromState({ ...parsed, view: 'jav' }, '/')
  assert.match(url, /directory_ids=3(?:%2C|,)5/)

  const otherTab = parseUrlState('?view=jav&tab=idol&directory_ids=3')
  assert.deepEqual(otherTab.jav.directoryIds, [3])
  assert.doesNotMatch(buildUrlFromState({ ...otherTab, view: 'jav' }, '/'), /directory_ids/)

  const recent = parseUrlState('?view=jav&tab=recent&temp_sort=last_played&page=3')
  assert.equal(recent.jav.tab, 'recent')
  assert.equal(recent.jav.tempSort, 'last_played')
  assert.equal(recent.jav.page, 3)
  const recentUrl = buildUrlFromState({ ...recent, view: 'jav' }, '/')
  assert.match(recentUrl, /tab=recent/)
  assert.match(recentUrl, /temp_sort=last_played/)
  assert.match(recentUrl, /page=3/)
})
