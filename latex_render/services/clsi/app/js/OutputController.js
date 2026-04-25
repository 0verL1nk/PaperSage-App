import OutputFileArchiveManager from './OutputFileArchiveManager.js'
import OutputCacheManager from './OutputCacheManager.js'
import Settings from '@overleaf/settings'
import { expressify } from '@overleaf/promise-utils'
import { pipeline } from 'node:stream/promises'
import { createReadStream } from 'node:fs'
import { join } from 'node:path'

function getContentDir(projectId, userId) {
  let subDir
  if (userId != null) {
    subDir = `${projectId}-${userId}`
  } else {
    subDir = projectId
  }
  return `${Settings.path.outputDir}/${subDir}/`
}

const MIME_TYPES = {
  pdf: 'application/pdf',
  log: 'text/plain',
  aux: 'text/plain',
  synctexgz: 'application/gzip',
  stdout: 'text/plain',
}

function getMimeType(filename) {
  if (filename.endsWith('.pdf')) return MIME_TYPES.pdf
  if (filename.endsWith('.log')) return MIME_TYPES.log
  if (filename.endsWith('.aux')) return MIME_TYPES.aux
  if (filename.endsWith('.synctex.gz')) return MIME_TYPES.synctexgz
  if (filename.endsWith('.stdout')) return MIME_TYPES.stdout
  return 'application/octet-stream'
}

async function createOutputZip(req, res) {
  const {
    project_id: projectId,
    user_id: userId,
    build_id: buildId,
  } = req.params

  const archive = await OutputFileArchiveManager.archiveFilesForBuild(
    projectId,
    userId,
    buildId
  )

  res.attachment('output.zip')
  res.setHeader('X-Content-Type-Options', 'nosniff')
  await pipeline(archive, res)
}

async function getOutputFile(req, res) {
  const {
    project_id: projectId,
    user_id: userId,
    build_id: buildId,
    file: filePath,
  } = req.params

  // Only allow known safe file names to prevent path traversal
  const allowedFiles = [
    'output.pdf', 'output.log', 'output.aux',
    'output.synctex.gz', 'output.stdout',
  ]
  if (!allowedFiles.includes(filePath)) {
    return res.status(400).send('File not allowed')
  }

  const contentDir = getContentDir(projectId, userId)
  const fullPath = join(contentDir, OutputCacheManager.path(buildId, filePath))

  try {
    res.setHeader('Content-Type', getMimeType(filePath))
    res.setHeader('X-Content-Type-Options', 'nosniff')
    const stream = createReadStream(fullPath)
    await pipeline(stream, res)
  } catch (error) {
    if (error.code === 'ENOENT' || error.code === 'ENOTDIR') {
      return res.status(404).send('File not found')
    }
    throw error
  }
}

export default {
  createOutputZip: expressify(createOutputZip),
  getOutputFile: expressify(getOutputFile),
}
