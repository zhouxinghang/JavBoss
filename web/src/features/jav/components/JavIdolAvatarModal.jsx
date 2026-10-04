import { useEffect, useMemo, useRef, useState } from 'react'
import CloseOutlinedIcon from '@mui/icons-material/CloseOutlined'
import CheckCircleRoundedIcon from '@mui/icons-material/CheckCircleRounded'
import RefreshRoundedIcon from '@mui/icons-material/RefreshRounded'
import UploadFileRoundedIcon from '@mui/icons-material/UploadFileRounded'
import {
  fetchJavIdolAvatarOptions,
  javIdolAvatarCandidateUrl,
  refreshJavIdolAvatar,
  setJavIdolAvatarURL,
  updateJavIdolAvatar,
  uploadJavIdolAvatar,
} from '@/features/jav/api'
import AppModal from '@/shared/ui/AppModal'
import { zh } from '@/utils/i18n'
import { getErrorMessage } from '@/utils/errors'
import { getIdolDisplayName } from '@/utils/javIdol'

const AUTO_KEY = ''

export default function JavIdolAvatarModal({
  open,
  item,
  preferChineseName = false,
  onClose,
  onSaved,
}) {
  const idolId = Number(item?.id)
  const hasIdolId = Number.isFinite(idolId) && idolId > 0
  const [options, setOptions] = useState([])
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [addingUrl, setAddingUrl] = useState(false)
  const [urlInput, setUrlInput] = useState('')
  const [error, setError] = useState('')
  const [selectedKey, setSelectedKey] = useState(AUTO_KEY)
  const [cacheVersion, setCacheVersion] = useState(0)
  const fileInputRef = useRef(null)

  const loadOptions = useMemo(() => {
    return async (cancelledRef) => {
      setLoading(true)
      setError('')
      try {
        const data = await fetchJavIdolAvatarOptions(idolId)
        if (cancelledRef.cancelled) return
        setOptions(data.items)
        const selected = data.items.find((option) => option?.selected)
        setSelectedKey(selected ? String(selected.key) : AUTO_KEY)
      } catch (err) {
        if (!cancelledRef.cancelled) setError(getErrorMessage(err))
      } finally {
        if (!cancelledRef.cancelled) setLoading(false)
      }
    }
  }, [idolId])

  useEffect(() => {
    if (!open || !hasIdolId) return undefined
    const cancelledRef = { cancelled: false }
    setOptions([])
    setSelectedKey(AUTO_KEY)
    setCacheVersion(0)
    setUrlInput('')
    loadOptions(cancelledRef)
    return () => {
      cancelledRef.cancelled = true
    }
  }, [open, hasIdolId, loadOptions])

  const handleRefresh = async () => {
    if (!hasIdolId || refreshing) return
    setRefreshing(true)
    setError('')
    try {
      await refreshJavIdolAvatar(idolId)
      setCacheVersion((current) => current + 1)
      const cancelledRef = { cancelled: false }
      await loadOptions(cancelledRef)
      // Reload the card image with the freshly downloaded file.
      onSaved?.(selectedKey)
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setRefreshing(false)
    }
  }

  const handleUpload = async (event) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file || !hasIdolId || uploading || saving || refreshing) return
    setUploading(true)
    setError('')
    try {
      await uploadJavIdolAvatar(idolId, file)
      setCacheVersion((current) => current + 1)
      await loadOptions({ cancelled: false })
      onSaved?.()
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setUploading(false)
    }
  }

  const handleAddURL = async () => {
    const url = urlInput.trim()
    if (!url || !hasIdolId || addingUrl || saving || refreshing || uploading) return
    setAddingUrl(true)
    setError('')
    try {
      await setJavIdolAvatarURL(idolId, url)
      setUrlInput('')
      setCacheVersion((current) => current + 1)
      await loadOptions({ cancelled: false })
      onSaved?.()
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setAddingUrl(false)
    }
  }

  const handleSave = async () => {
    if (!hasIdolId || saving) return
    setSaving(true)
    setError('')
    try {
      await updateJavIdolAvatar(idolId, selectedKey)
      onSaved?.(selectedKey)
      onClose?.()
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  if (!open) return null

  const displayName = getIdolDisplayName(item, preferChineseName)
  const busy = saving || refreshing || uploading || addingUrl

  return (
    <AppModal
      ariaLabel={zh('选择女优头像', 'Select idol avatar')}
      className="px-4 py-6"
      closeDisabled={busy}
      contentClassName="flex max-h-[90vh] w-full max-w-3xl flex-col overflow-hidden rounded-lg bg-white shadow-2xl"
      onClose={onClose}
      zIndex={1600}
    >
      <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
        <div className="min-w-0">
          <div className="truncate text-base font-semibold text-slate-950">
            {zh('选择女优头像', 'Select idol avatar')}
          </div>
          <div className="truncate text-xs text-slate-500">{displayName}</div>
        </div>
        <button
          type="button"
          className="flex h-8 w-8 items-center justify-center rounded-full text-slate-500 hover:bg-slate-100"
          onClick={onClose}
          aria-label={zh('关闭', 'Close')}
        >
          <CloseOutlinedIcon sx={{ fontSize: 18 }} />
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded border border-slate-200 bg-slate-50 p-2">
          <input
            ref={fileInputRef}
            type="file"
            accept="image/jpeg,image/png"
            className="hidden"
            onChange={handleUpload}
          />
          <button
            type="button"
            className="flex items-center gap-1.5 rounded border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 disabled:cursor-not-allowed disabled:text-slate-400"
            onClick={() => fileInputRef.current?.click()}
            disabled={!hasIdolId || busy}
          >
            <UploadFileRoundedIcon sx={{ fontSize: 16 }} />
            {uploading ? zh('上传中…', 'Uploading...') : zh('上传图片', 'Upload image')}
          </button>
          <div className="flex min-w-[12rem] flex-1 items-center gap-2">
            <input
              value={urlInput}
              onChange={(event) => setUrlInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault()
                  handleAddURL()
                }
              }}
              placeholder={zh('粘贴图片链接', 'Paste an image URL')}
              className="min-w-0 flex-1 rounded border border-slate-300 bg-white px-3 py-1.5 text-sm outline-none focus:border-slate-900"
              disabled={busy}
            />
            <button
              type="button"
              className="rounded bg-slate-950 px-3 py-1.5 text-sm text-white hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300"
              onClick={handleAddURL}
              disabled={!hasIdolId || !urlInput.trim() || busy}
            >
              {addingUrl ? zh('添加中…', 'Adding...') : zh('添加链接', 'Add URL')}
            </button>
          </div>
        </div>
        {loading ? (
          <div className="flex h-40 items-center justify-center text-sm text-slate-500">
            {zh('加载中…', 'Loading...')}
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4">
            <AvatarOption
              label={zh('自动选择', 'Auto')}
              description={zh('默认优选 AI 高清头像', 'Best AI-enhanced avatar by default')}
              selected={selectedKey === AUTO_KEY}
              onSelect={() => setSelectedKey(AUTO_KEY)}
            />
            {options.map((option) => (
              <AvatarOption
                key={option.key}
                label={avatarOptionLabel(option)}
                description={option?.kind ? zh('自定义头像', 'Custom avatar') : ''}
                imageSrc={
                  javIdolAvatarCandidateUrl(idolId, option.key) +
                  (cacheVersion ? `?v=${cacheVersion}` : '')
                }
                selected={selectedKey === String(option.key)}
                onSelect={() => setSelectedKey(String(option.key))}
              />
            ))}
          </div>
        )}
        {!loading && options.length === 0 ? (
          <div className="mt-3 text-center text-sm text-slate-500">
            {zh('未在 Gfriends 中找到更多头像', 'No additional avatars found in Gfriends')}
          </div>
        ) : null}
        {error ? <div className="mt-3 text-sm text-red-600">{error}</div> : null}
      </div>

      <div className="flex items-center justify-between gap-2 border-t border-slate-200 px-4 py-3">
        <button
          type="button"
          className="flex items-center gap-1.5 rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:text-slate-400"
          onClick={handleRefresh}
          disabled={!hasIdolId || busy}
        >
          <RefreshRoundedIcon sx={{ fontSize: 16 }} />
          {refreshing ? zh('刷新中…', 'Refreshing...') : zh('重新下载', 'Re-download')}
        </button>
        <div className="flex gap-2">
          <button
            type="button"
            className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50"
            onClick={onClose}
            disabled={saving}
          >
            {zh('取消', 'Cancel')}
          </button>
          <button
            type="button"
            className="rounded bg-slate-950 px-3 py-1.5 text-sm text-white hover:bg-slate-800 disabled:cursor-not-allowed disabled:bg-slate-300"
            onClick={handleSave}
            disabled={!hasIdolId || busy}
          >
            {saving ? zh('保存中…', 'Saving...') : zh('保存', 'Save')}
          </button>
        </div>
      </div>
    </AppModal>
  )
}

function avatarOptionLabel(option) {
  switch (option?.kind) {
    case 'upload':
      return zh('本地上传', 'Uploaded')
    case 'url':
      return zh('图片链接', 'Image URL')
    default:
      return option?.source || zh('未命名来源', 'Unknown source')
  }
}

function AvatarOption({ label, description = '', imageSrc = '', selected = false, onSelect }) {
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    setFailed(false)
  }, [imageSrc])

  return (
    <button
      type="button"
      onClick={onSelect}
      className={`group relative flex flex-col overflow-hidden rounded-lg border text-left transition ${
        selected
          ? 'border-slate-900 ring-2 ring-slate-900/20'
          : 'border-slate-200 hover:border-slate-400'
      }`}
    >
      <div className="relative w-full overflow-hidden bg-slate-100 pt-[150%]">
        {imageSrc && !failed ? (
          <img
            src={imageSrc}
            alt={label}
            loading="lazy"
            className="absolute inset-0 h-full w-full object-cover object-top"
            onError={() => setFailed(true)}
            draggable={false}
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center px-2 text-center text-xs font-medium text-slate-500">
            {label}
          </div>
        )}
        {selected ? (
          <CheckCircleRoundedIcon
            className="absolute right-1.5 top-1.5 text-white drop-shadow"
            sx={{ fontSize: 20 }}
          />
        ) : null}
      </div>
      <div className="px-2 py-1.5">
        <div className="truncate text-xs font-semibold text-slate-800" title={label}>
          {label}
        </div>
        {description ? (
          <div className="truncate text-[10px] text-slate-500" title={description}>
            {description}
          </div>
        ) : null}
      </div>
    </button>
  )
}
