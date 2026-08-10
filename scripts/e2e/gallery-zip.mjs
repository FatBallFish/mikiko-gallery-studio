import { inflateRawSync } from 'node:zlib'

const DEFAULT_MAX_ARCHIVE_BYTES = 256 << 20

function fail(message, detail = {}) {
  const error = new Error(message)
  error.detail = detail
  throw error
}

function startsWith(content, signature) {
  const expected = Buffer.from(signature)
  return expected.length > 0 && content.length >= expected.length && content.subarray(0, expected.length).equals(expected)
}

function hasImageSignature(content) {
  return startsWith(content, Buffer.from('89504e470d0a1a0a', 'hex'))
    || startsWith(content, Buffer.from('ffd8ff', 'hex'))
    || (content.length >= 12 && content.toString('ascii', 0, 4) === 'RIFF' && content.toString('ascii', 8, 12) === 'WEBP')
    || startsWith(content, Buffer.from('GIF87a', 'ascii'))
    || startsWith(content, Buffer.from('GIF89a', 'ascii'))
}

export function readZipEntries(buffer, maxArchiveBytes = DEFAULT_MAX_ARCHIVE_BYTES) {
  if (!Buffer.isBuffer(buffer) || buffer.length < 22) {
    fail('Gallery export response was not a valid ZIP archive')
  }
  if (buffer.length > maxArchiveBytes) {
    fail('Gallery export ZIP exceeded the archive limit', { archiveBytes: buffer.length, maxArchiveBytes })
  }
  const minimumEOCDSize = 22
  const maximumCommentSize = 0xffff
  let eocdOffset = -1
  for (let offset = buffer.length - minimumEOCDSize; offset >= Math.max(0, buffer.length - minimumEOCDSize - maximumCommentSize); offset -= 1) {
    if (buffer.readUInt32LE(offset) === 0x06054b50) {
      eocdOffset = offset
      break
    }
  }
  if (eocdOffset < 0) fail('Gallery export response was not a valid ZIP archive')

  const entryCount = buffer.readUInt16LE(eocdOffset + 10)
  const directorySize = buffer.readUInt32LE(eocdOffset + 12)
  const directoryOffset = buffer.readUInt32LE(eocdOffset + 16)
  if (directoryOffset + directorySize > eocdOffset) {
    fail('Gallery export ZIP central directory exceeded the archive bounds')
  }

  const entries = []
  const names = new Set()
  let totalUncompressedSize = 0
  let offset = directoryOffset
  for (let index = 0; index < entryCount; index += 1) {
    if (offset + 46 > buffer.length || buffer.readUInt32LE(offset) !== 0x02014b50) {
      fail('Gallery export ZIP central directory was malformed', { index, offset })
    }
    const compressionMethod = buffer.readUInt16LE(offset + 10)
    const compressedSize = buffer.readUInt32LE(offset + 20)
    const uncompressedSize = buffer.readUInt32LE(offset + 24)
    const filenameLength = buffer.readUInt16LE(offset + 28)
    const extraLength = buffer.readUInt16LE(offset + 30)
    const commentLength = buffer.readUInt16LE(offset + 32)
    const localHeaderOffset = buffer.readUInt32LE(offset + 42)
    const nameStart = offset + 46
    const nameEnd = nameStart + filenameLength
    if (nameEnd > buffer.length) fail('Gallery export ZIP filename exceeded the archive bounds', { index })
    const name = buffer.toString('utf8', nameStart, nameEnd)
    if (!name || names.has(name)) fail('Gallery export ZIP contained an empty or duplicate filename', { name })
    names.add(name)
    totalUncompressedSize += uncompressedSize
    if (uncompressedSize > maxArchiveBytes || totalUncompressedSize > maxArchiveBytes) {
      fail('Gallery export ZIP exceeded the archive limit', { name, totalUncompressedSize, maxArchiveBytes })
    }
    entries.push({ name, compressionMethod, compressedSize, uncompressedSize, localHeaderOffset })
    offset = nameEnd + extraLength + commentLength
    if (offset > directoryOffset + directorySize) {
      fail('Gallery export ZIP central directory entry exceeded its bounds', { index, offset })
    }
  }
  if (offset !== directoryOffset + directorySize) {
    fail('Gallery export ZIP central directory size did not match its entries')
  }
  return entries
}

export function readZipEntry(buffer, entry, maxArchiveBytes = DEFAULT_MAX_ARCHIVE_BYTES) {
  if (entry.uncompressedSize > maxArchiveBytes) {
    fail('Gallery export ZIP entry exceeded the archive limit', { name: entry.name, maxArchiveBytes })
  }
  const offset = entry.localHeaderOffset
  if (offset + 30 > buffer.length || buffer.readUInt32LE(offset) !== 0x04034b50) {
    fail('Gallery export ZIP local header was malformed', { name: entry.name })
  }
  const compressionMethod = buffer.readUInt16LE(offset + 8)
  const filenameLength = buffer.readUInt16LE(offset + 26)
  const extraLength = buffer.readUInt16LE(offset + 28)
  const contentStart = offset + 30 + filenameLength + extraLength
  const contentEnd = contentStart + entry.compressedSize
  if (contentEnd > buffer.length) fail('Gallery export ZIP entry exceeded the archive bounds', { name: entry.name })
  if (compressionMethod !== entry.compressionMethod) fail('Gallery export ZIP compression metadata did not match', { name: entry.name })
  const localName = buffer.toString('utf8', offset + 30, offset + 30 + filenameLength)
  if (localName !== entry.name) fail('Gallery export ZIP filename metadata did not match', { name: entry.name, localName })
  const compressed = buffer.subarray(contentStart, contentEnd)
  let content
  if (entry.compressionMethod === 0) {
    content = compressed
  } else if (entry.compressionMethod === 8) {
    try {
      content = inflateRawSync(compressed, { maxOutputLength: maxArchiveBytes })
    } catch (error) {
      fail('Gallery export ZIP entry exceeded the archive limit or was invalid', { name: entry.name, message: error.message })
    }
  } else {
    fail('Gallery export ZIP used an unsupported compression method', { name: entry.name, method: entry.compressionMethod })
  }
  if (content.length !== entry.uncompressedSize) {
    fail('Gallery export ZIP entry size did not match its metadata', { name: entry.name, expected: entry.uncompressedSize, actual: content.length })
  }
  return content
}

export function assertGalleryZip(result, expectedImageIDs, options = {}) {
  const maxArchiveBytes = options.maxArchiveBytes ?? DEFAULT_MAX_ARCHIVE_BYTES
  if (result.status !== 200 || !String(result.headers.get('content-type') || '').toLowerCase().startsWith('application/zip')) {
    fail('Gallery batch download did not return a ZIP archive', {
      status: result.status,
      contentType: result.headers.get('content-type'),
      body: result.buffer.toString('utf8', 0, 600),
    })
  }
  const entries = readZipEntries(result.buffer, maxArchiveBytes)
  const entriesByName = new Map(entries.map(entry => [entry.name, entry]))
  const manifestEntry = entriesByName.get('manifest.json')
  if (!manifestEntry || entries.length !== expectedImageIDs.length + 1) {
    fail('Gallery export ZIP did not contain the selected images plus manifest', {
      expectedImageCount: expectedImageIDs.length,
      entries: entries.map(entry => entry.name),
    })
  }
  let manifest
  try {
    manifest = JSON.parse(readZipEntry(result.buffer, manifestEntry, maxArchiveBytes).toString('utf8'))
  } catch (error) {
    fail('Gallery export ZIP manifest was invalid JSON', { message: error.message })
  }
  const successfulFiles = Array.isArray(manifest.files) ? manifest.files.filter(item => item.status === 'succeeded') : []
  const manifestIDs = successfulFiles.map(item => String(item.id)).sort()
  const expectedIDs = expectedImageIDs.map(String).sort()
  if (JSON.stringify(manifestIDs) !== JSON.stringify(expectedIDs)) {
    fail('Gallery export ZIP manifest did not match the selected assets', { manifestIDs, expectedIDs, manifest })
  }

  const successfulFilenames = successfulFiles.map(file => String(file.filename || ''))
  const archiveImageFilenames = entries.filter(entry => entry.name !== 'manifest.json').map(entry => entry.name).sort()
  if (successfulFilenames.some(filename => !filename || filename === 'manifest.json')
    || new Set(successfulFilenames).size !== successfulFilenames.length
    || JSON.stringify([...successfulFilenames].sort()) !== JSON.stringify(archiveImageFilenames)) {
    fail('Gallery export ZIP manifest filenames did not match the image entries', { successfulFilenames, archiveImageFilenames })
  }

  const expectedSignatures = (options.expectedImageSignatures || []).map(signature => Buffer.from(signature))
  let matchedExpectedSignature = expectedSignatures.length === 0
  for (const file of successfulFiles) {
    const filename = String(file.filename || '')
    const entry = entriesByName.get(filename)
    if (!filename || filename === 'manifest.json' || !entry) {
      fail('Gallery export ZIP manifest filename did not match an image entry', { file, entries: entries.map(item => item.name) })
    }
    const content = readZipEntry(result.buffer, entry, maxArchiveBytes)
    if (content.length === 0) fail('Gallery export ZIP contained an empty image entry', { id: file.id, filename })
    if (!Number.isSafeInteger(file.size_bytes) || file.size_bytes !== content.length) {
      fail('Gallery export ZIP manifest size did not match the image entry', { id: file.id, filename, manifestSize: file.size_bytes, actual: content.length })
    }
    if (!hasImageSignature(content)) {
      fail('Gallery export ZIP image signature was invalid', { id: file.id, filename })
    }
    if (expectedSignatures.some(signature => startsWith(content, signature))) matchedExpectedSignature = true
  }
  if (!matchedExpectedSignature) {
    fail('Gallery export ZIP did not contain an expected image signature')
  }
  return { archiveBytes: result.buffer.length, entries: entries.map(entry => entry.name) }
}
