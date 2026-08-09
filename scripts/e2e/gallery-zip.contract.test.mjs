import assert from 'node:assert/strict'
import { deflateRawSync } from 'node:zlib'
import test from 'node:test'

import { assertGalleryZip } from './gallery-zip.mjs'

const PNG = Buffer.concat([Buffer.from('89504e470d0a1a0a', 'hex'), Buffer.from('e2e-image')])

function zip(files) {
  const locals = []
  const centrals = []
  let localOffset = 0
  for (const file of files) {
    const name = Buffer.from(file.name)
    const content = Buffer.from(file.content)
    const compressed = file.deflate ? deflateRawSync(content) : content
    const method = file.deflate ? 8 : 0
    const local = Buffer.alloc(30)
    local.writeUInt32LE(0x04034b50, 0)
    local.writeUInt16LE(20, 4)
    local.writeUInt16LE(method, 8)
    local.writeUInt32LE(compressed.length, 18)
    local.writeUInt32LE(content.length, 22)
    local.writeUInt16LE(name.length, 26)
    locals.push(local, name, compressed)

    const central = Buffer.alloc(46)
    central.writeUInt32LE(0x02014b50, 0)
    central.writeUInt16LE(20, 4)
    central.writeUInt16LE(20, 6)
    central.writeUInt16LE(method, 10)
    central.writeUInt32LE(compressed.length, 20)
    central.writeUInt32LE(content.length, 24)
    central.writeUInt16LE(name.length, 28)
    central.writeUInt32LE(localOffset, 42)
    centrals.push(central, name)
    localOffset += local.length + name.length + compressed.length
  }
  const directory = Buffer.concat(centrals)
  const eocd = Buffer.alloc(22)
  eocd.writeUInt32LE(0x06054b50, 0)
  eocd.writeUInt16LE(files.length, 8)
  eocd.writeUInt16LE(files.length, 10)
  eocd.writeUInt32LE(directory.length, 12)
  eocd.writeUInt32LE(localOffset, 16)
  return Buffer.concat([...locals, directory, eocd])
}

function resultFor(image, manifest, options = {}) {
  return {
    status: 200,
    headers: new Headers({ 'content-type': 'application/zip' }),
    buffer: zip([
      { name: 'image.png', content: image, deflate: options.deflate },
      { name: 'manifest.json', content: JSON.stringify(manifest) },
    ]),
  }
}

function manifest(sizeBytes = PNG.length) {
  return { files: [{ id: 'image-1', filename: 'image.png', status: 'succeeded', size_bytes: sizeBytes }] }
}

test('accepts a non-empty image matching its manifest and expected signature', () => {
  const detail = assertGalleryZip(resultFor(PNG, manifest()), ['image-1'], { expectedImageSignatures: [PNG.subarray(0, 8)] })
  assert.deepEqual(detail.entries.sort(), ['image.png', 'manifest.json'])
})

test('rejects an empty image entry even when the manifest IDs match', () => {
  assert.throws(
    () => assertGalleryZip(resultFor(Buffer.alloc(0), manifest(0)), ['image-1'], { expectedImageSignatures: [PNG.subarray(0, 8)] }),
    /empty image entry/,
  )
})

test('rejects an image whose bytes do not match its manifest size', () => {
  assert.throws(
    () => assertGalleryZip(resultFor(PNG, manifest(PNG.length + 1)), ['image-1'], { expectedImageSignatures: [PNG.subarray(0, 8)] }),
    /manifest size/,
  )
})

test('rejects image bytes without a recognized or expected signature', () => {
  const text = Buffer.from('not an image')
  assert.throws(
    () => assertGalleryZip(resultFor(text, manifest(text.length)), ['image-1'], { expectedImageSignatures: [PNG.subarray(0, 8)] }),
    /image signature/,
  )
})

test('rejects duplicate manifest filenames that leave an image entry unverified', () => {
  const duplicateManifest = {
    files: [
      { id: 'image-1', filename: 'image-one.png', status: 'succeeded', size_bytes: PNG.length },
      { id: 'image-2', filename: 'image-one.png', status: 'succeeded', size_bytes: PNG.length },
    ],
  }
  const result = {
    status: 200,
    headers: new Headers({ 'content-type': 'application/zip' }),
    buffer: zip([
      { name: 'image-one.png', content: PNG },
      { name: 'image-two.png', content: PNG },
      { name: 'manifest.json', content: JSON.stringify(duplicateManifest) },
    ]),
  }
  assert.throws(
    () => assertGalleryZip(result, ['image-1', 'image-2'], { expectedImageSignatures: [PNG.subarray(0, 8)] }),
    /manifest filename/,
  )
})

test('bounds deflate output by the configured archive limit', () => {
  const oversized = Buffer.concat([PNG, Buffer.alloc(64, 1)])
  assert.throws(
    () => assertGalleryZip(resultFor(oversized, manifest(oversized.length), { deflate: true }), ['image-1'], {
      expectedImageSignatures: [PNG.subarray(0, 8)],
      maxArchiveBytes: 32,
    }),
    /archive limit/,
  )
})
