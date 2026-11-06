import assert from 'node:assert/strict'
import test from 'node:test'
import { browserUnavailable, openBrowser } from '../../helpers/browser.js'

test(
  'video subtitles upload, replace, recover from failed uploads and remove through the card menu',
  { skip: browserUnavailable, timeout: 60000 },
  async (t) => {
    const { origin, command, evaluate, waitFor } = await openBrowser(t)
    await command('Page.navigate', { url: `${origin}/tests/fixtures/app.html?view=video` })
    await waitFor(
      `window.requests?.some(r => r.url.startsWith('/videos?')) && !window.testStore.getState().loading`
    )
    await evaluate(`{
      const originalFetch = window.fetch;
      window.selectedSubtitle = null;
      window.subtitleUploads = [];
      window.failUpload = false;
      window.fetch = async (input, init = {}) => {
        if (new URL(input, location.origin).pathname !== '/videos/42/subtitle') return originalFetch(input, init);
        if (init.method === 'PUT') {
          const file = init.body.get('file');
          window.subtitleUploads.push({name: file.name, content: await file.text()});
          if (window.failUpload) return Response.json({error_en: 'Upload failed; try again'}, {status: 503});
          await new Promise(resolve => { window.finishUpload = resolve; });
          window.selectedSubtitle = {name: file.name, size: file.size};
        }
        if (init.method === 'DELETE') window.selectedSubtitle = null;
        return Response.json({subtitle: window.selectedSubtitle});
      };
      window.testStore.setState({videos: [{id: 42, location_id: 7, path: 'movie.mp4', filename: 'movie.mp4', directory: {path: '/media'}}], total: 1});
    }`)
    await waitFor(`document.querySelector('.video-card button[aria-label="Edit video"]')`)
    await evaluate(`document.querySelector('.video-card button[aria-label="Edit video"]').click()`)
    await waitFor(
      `Array.from(document.querySelectorAll('button')).some(b => b.textContent === 'Subtitle')`
    )
    await evaluate(
      `Array.from(document.querySelectorAll('button')).find(b => b.textContent === 'Subtitle').click()`
    )
    const dialog = `document.querySelector('[role="dialog"]')`
    await waitFor(`${dialog}?.textContent.includes('No subtitle uploaded')`)
    const choose = async (name, content = '1\n00:00:00,000 --> 00:00:10,000\n字幕\n') => {
      await evaluate(`{
        const transfer = new DataTransfer();
        transfer.items.add(new File([${JSON.stringify(content)}], ${JSON.stringify(name)}));
        const input = ${dialog}.querySelector('input[type="file"]');
        input.files = transfer.files;
        input.dispatchEvent(new Event('change', {bubbles: true}));
      }`)
    }
    await choose('wrong.txt')
    await waitFor(`${dialog}.querySelector('[role="alert"]')`)
    assert.equal(await evaluate('window.subtitleUploads.length'), 0)
    await choose('中文字幕.srt')
    await waitFor('window.finishUpload')
    assert.equal(
      await evaluate(`${dialog}.querySelector('button[aria-label="Close"]').disabled`),
      true
    )
    await evaluate('window.finishUpload(); window.finishUpload = null')
    await waitFor(
      `${dialog}.textContent.includes('中文字幕.srt') && ${dialog}.textContent.includes('Uploaded.')`
    )
    assert.match(await evaluate('window.subtitleUploads[0].content'), /字幕/)
    await evaluate('window.failUpload = true')
    await choose('replacement.srt')
    await waitFor(
      `${dialog}.querySelector('[role="alert"]')?.textContent.includes('Upload failed')`
    )
    assert.equal(await evaluate(`${dialog}.textContent.includes('中文字幕.srt')`), true)
    await evaluate('window.failUpload = false')
    await choose('replacement.vtt', 'WEBVTT\n\n00:00.000 --> 00:10.000\nReplacement\n')
    await waitFor('window.finishUpload')
    await evaluate('window.finishUpload(); window.finishUpload = null')
    await waitFor(`${dialog}.textContent.includes('replacement.vtt')`)
    await command('Emulation.setDeviceMetricsOverride', {
      width: 390,
      height: 844,
      deviceScaleFactor: 1,
      mobile: true,
    })
    assert.equal(await evaluate(`${dialog}.getBoundingClientRect().width <= innerWidth`), true)
    await evaluate(
      `Array.from(${dialog}.querySelectorAll('button')).find(b => b.textContent === 'Remove subtitle').click()`
    )
    await waitFor(`${dialog}.textContent.includes('No subtitle uploaded')`)
    await evaluate(`${dialog}.querySelector('button[aria-label="Close"]').click()`)
    await waitFor(`!${dialog}`)
    assert.deepEqual(await evaluate('window.appErrors'), [])
  }
)
