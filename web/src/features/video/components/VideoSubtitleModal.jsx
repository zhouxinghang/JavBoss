import { useEffect, useId, useRef, useState } from 'react'
import CloseOutlinedIcon from '@mui/icons-material/CloseOutlined'
import AppModal from '@/shared/ui/AppModal'
import { deleteVideoSubtitle, fetchVideoSubtitle, uploadVideoSubtitle } from '@/features/video/api'
import { formatBytes, getVideoDisplayName } from '@/utils/display'
import { getErrorMessage } from '@/utils/errors'
import { zh } from '@/utils/i18n'

export default function VideoSubtitleModal({ video, onClose }) {
  const titleId = useId()
  const inputId = useId()
  const inputRef = useRef(null)
  const [subtitle, setSubtitle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [reload, setReload] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    fetchVideoSubtitle(video.id, { signal: controller.signal })
      .then((file) => setSubtitle(file))
      .catch((err) => {
        if (!controller.signal.aborted) setError(getErrorMessage(err))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [video.id, reload])

  const handleUpload = async (event) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    setError('')
    setMessage('')
    if (!/\.(srt|ass|ssa|vtt)$/i.test(file.name)) {
      setError(
        zh('请选择 SRT、ASS、SSA 或 VTT 字幕文件', 'Choose an SRT, ASS, SSA or VTT subtitle')
      )
      return
    }
    if (file.size === 0 || file.size > 10 * 1024 * 1024) {
      setError(
        zh(
          '字幕文件不能为空，且不能超过 10 MB',
          'Subtitle files must be nonempty and 10 MB or smaller'
        )
      )
      return
    }
    setBusy(true)
    try {
      setSubtitle(await uploadVideoSubtitle(video.id, file))
      setMessage(
        zh(
          '上传成功，下次打开 MPV 时自动加载。',
          'Uploaded. The subtitle will load the next time you open MPV.'
        )
      )
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const handleDelete = async () => {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      await deleteVideoSubtitle(video.id)
      setSubtitle(null)
      setMessage(zh('已移除上传的字幕。', 'The uploaded subtitle has been removed.'))
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <AppModal
      ariaLabelledby={titleId}
      onClose={onClose}
      closeDisabled={busy}
      contentClassName="mx-4 max-h-[calc(100dvh-2rem)] w-full max-w-md overflow-y-auto rounded-xl bg-white p-5 shadow-xl"
    >
      <div className="flex items-center justify-between gap-3">
        <h2 id={titleId} className="text-lg font-semibold text-gray-900">
          {zh('视频字幕', 'Video subtitle')}
        </h2>
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          aria-label={zh('关闭', 'Close')}
          className="rounded p-1 text-gray-500 hover:bg-gray-100 disabled:opacity-50"
        >
          <CloseOutlinedIcon fontSize="small" />
        </button>
      </div>
      <p className="mt-2 break-all text-sm text-gray-600">{getVideoDisplayName(video)}</p>
      <p className="mt-3 text-sm leading-6 text-gray-600">
        {zh(
          '支持 SRT、ASS、SSA、VTT，最大 10 MB。每个视频保留一份当前字幕，上传新文件会替换当前选择；下次使用 MPV 播放时自动加载。',
          'Supports SRT, ASS, SSA and VTT up to 10 MB. Each video has one selected subtitle. A new upload replaces the selection and loads the next time you play with MPV.'
        )}
      </p>
      <div
        className="mt-4 rounded-lg border border-gray-200 bg-gray-50 p-3"
        aria-busy={loading || busy}
      >
        {loading ? (
          <p role="status" className="text-sm text-gray-500">
            {zh('正在加载…', 'Loading...')}
          </p>
        ) : subtitle ? (
          <div>
            <p className="break-all text-sm font-medium text-gray-800">{subtitle.name}</p>
            <p className="mt-1 text-xs text-gray-500">{formatBytes(subtitle.size)}</p>
          </div>
        ) : (
          <p className="text-sm text-gray-500">{zh('尚未上传字幕', 'No subtitle uploaded')}</p>
        )}
      </div>
      {error ? (
        <div role="alert" className="mt-3 text-sm text-red-600">
          <p>{error}</p>
          {!subtitle && !busy ? (
            <button
              type="button"
              onClick={() => setReload((value) => value + 1)}
              className="mt-1 underline"
            >
              {zh('重新加载', 'Reload')}
            </button>
          ) : null}
        </div>
      ) : null}
      <p role="status" className="mt-3 text-sm text-green-700">
        {busy ? zh('正在保存字幕…', 'Saving subtitle...') : message}
      </p>
      <div className="mt-4 flex flex-wrap justify-end gap-2">
        {subtitle ? (
          <button
            type="button"
            onClick={handleDelete}
            disabled={loading || busy}
            className="rounded-lg border border-red-200 px-3 py-2 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
          >
            {zh('移除字幕', 'Remove subtitle')}
          </button>
        ) : null}
        <input
          ref={inputRef}
          id={inputId}
          type="file"
          accept=".srt,.ass,.ssa,.vtt"
          aria-label={zh('选择字幕文件', 'Choose subtitle file')}
          className="hidden"
          onChange={handleUpload}
          disabled={loading || busy}
        />
        <button
          type="button"
          onClick={() => inputRef.current?.click()}
          disabled={loading || busy}
          className="rounded-lg bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {subtitle ? zh('替换字幕', 'Replace subtitle') : zh('上传字幕', 'Upload subtitle')}
        </button>
      </div>
    </AppModal>
  )
}
