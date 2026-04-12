# Overleaf 文件管理体系

## 1. 文件树结构与 MongoDB 模型

### 1.1 核心数据模型

Overleaf 使用 MongoDB 存储项目结构，核心模型位于 `/services/web/app/src/models/`:

#### Project 模型
**路径**: `/services/web/app/src/models/Project.mjs`

```javascript
const ProjectSchema = new Schema({
  name: { type: String, default: 'new project' },
  lastUpdated: { type: Date },
  lastUpdatedBy: { type: ObjectId, ref: 'User' },
  lastOpened: { type: Date },
  active: { type: Boolean, default: true },
  readOnly: { type: Boolean, default: false },
  owner_ref: { type: ObjectId, ref: 'User' },
  collaberator_refs: [{ type: ObjectId, ref: 'User' }],
  reviewer_refs: [{ type: ObjectId, ref: 'User' },
  readOnly_refs: [{ type: ObjectId, ref: 'User' }],
  rootDoc_id: { type: ObjectId },           // 主文档 ID
  rootFolder: [FolderSchema],                // 文件树根文件夹
  mainBibliographyDoc_id: { type: ObjectId },
  version: { type: Number },                 // 结构版本号，每次变更递增
  trashed: [{ type: ObjectId, ref: 'User' }], // 垃圾箱中的用户
  archived: { type: Schema.Types.Mixed },
  deletedDocs: [DeletedDocSchema],           // 已删除的文档
  // ... 其他字段
})
```

#### Folder 模型
**路径**: `/services/web/app/src/models/Folder.mjs`

```javascript
export const FolderSchema = new Schema({
  name: { type: String, default: 'new folder' },
})

FolderSchema.add({
  docs: [DocSchema],      // 子文档数组
  fileRefs: [FileSchema], // 文件引用数组
  folders: [FolderSchema]  // 子文件夹数组（递归）
})
```

#### Doc 模型
**路径**: `/services/web/app/src/models/Doc.mjs`

```javascript
export const DocSchema = new Schema({
  name: { type: String, default: 'new doc' },
}, { minimize: false })
```

#### File 模型
**路径**: `/services/web/app/src/models/File.mjs`

```javascript
export const FileSchema = new Schema({
  name: { type: String, default: '' },
  created: { type: Date, default: () => new Date() },
  rev: { type: Number, default: 0 },
  linkedFileData: { type: Schema.Types.Mixed },  // 链接文件数据
  hash: { type: String },                        // 文件内容哈希
}, { minimize: false })
```

### 1.2 文件树存储结构

项目文件树以嵌套文档形式存储在 MongoDB 的 `projects` 集合中：

```
Project {
  _id: ObjectId,
  name: "My Project",
  rootFolder: [
    {
      _id: ObjectId,
      name: "rootFolder",
      docs: [
        { _id: ObjectId, name: "main.tex", rev: 1 },
        { _id: ObjectId, name: "intro.tex", rev: 3 }
      ],
      fileRefs: [
        { _id: ObjectId, name: "figure.png", hash: "abc123", rev: 0 }
      ],
      folders: [
        { _id: ObjectId, name: "chapters", docs: [...], fileRefs: [...], folders: [...] }
      ]
    }
  ],
  version: 15  // 递增版本号
}
```

### 1.3 Docstore 文档存储

Docstore 服务（`/services/docstore/`）独立存储文档内容：

**MongoDB 集合**: `docstore.docs`

```javascript
// 文档结构
{
  _id: ObjectId(docId),
  project_id: ObjectId(projectId),
  lines: ["line1", "line2", ...],  // 文档行数组
  ranges: { comments: [], changes: [] },  // 批注和跟踪更改
  rev: 5,                              // 修订号
  version: 12,                         // 文档版本
  deleted: true,                       // 软删除标记
  deletedAt: ISODate(),
  inS3: false                          // 是否归档到 S3
}
```

**关键操作**:
- **路径**: `/services/docstore/app/js/MongoManager.js`
- **文档管理**: `/services/docstore/app/js/DocManager.js`
- **归档管理**: `/services/docstore/app/js/DocArchiveManager.js`

---

## 2. 文件夹操作

### 2.1 创建文件夹

**后端入口**: `/services/web/app/src/Features/Project/ProjectEntityUpdateHandler.mjs`

```javascript
const addFolder = wrapWithLock(
  async function (projectId, parentFolderId, folderName, userId) {
    if (!SafePath.isCleanFilename(folderName)) {
      throw new Errors.InvalidNameError('invalid element name')
    }
    return await ProjectEntityMongoUpdateHandler.promises.addFolder(
      projectId, parentFolderId, folderName, userId
    )
  }
)
```

**MongoDB 操作**: `/services/web/app/src/Features/Project/ProjectEntityMongoUpdateHandler.mjs`

```javascript
async function addFolder(projectId, parentFolderId, folderName, userId) {
  const project = await ProjectGetter.promises.getProjectWithoutLock(projectId, {
    rootFolder: true, name: true, overleaf: true
  })
  parentFolderId = _confirmFolder(project, parentFolderId)
  const folder = new Folder({ name: folderName })
  await _putElement(project, parentFolderId, folder, 'folder', userId)
  return { folder, parentFolderId }
}

async function _putElement(project, folderId, element, type, userId) {
  // 检查元素数量限制
  if (_countElements(project) > Settings.maxEntitiesPerProject) {
    await CooldownManager.putProjectOnCooldown(project._id)
    throw new Error('project_has_too_many_files')
  }
  
  // 插入到指定文件夹
  const mongoPath = `${path.mongo}.${pathSegment}`
  const newProject = await Project.findOneAndUpdate(
    { _id: project._id, [path.mongo]: { $exists: true } },
    { $push: { [mongoPath]: element }, $inc: { version: 1 } },
    { new: true }
  )
}
```

### 2.2 删除文件夹

**入口**: `ProjectEntityUpdateHandler.deleteEntity`

```javascript
const deleteEntity = wrapWithLock(
  async function (projectId, entityId, entityType, userId, source, callback) {
    // 1. 刷新整个项目到 MongoDB
    await DocumentUpdaterHandler.promises.flushProjectToMongo(projectId)
    
    // 2. 从项目结构中删除实体
    const { entity, path, projectBeforeDeletion, newProject } =
      await ProjectEntityMongoUpdateHandler.promises.deleteEntity(
        projectId, entityId, entityType, userId
      )
    
    // 3. 清理实体（如果是文档则软删除）
    const subtreeListing = await ProjectEntityUpdateHandler._cleanUpEntity(...)
    
    // 4. 通知第三方数据存储
    await TpdsUpdateSender.promises.deleteEntity({
      projectId, path: path.fileSystem, projectName, entityId, entityType, subtreeEntityIds
    })
  }
)
```

**MongoDB 删除操作**:

```javascript
async function deleteEntity(projectId, entityId, entityType, userId) {
  // 不能删除根文件夹
  if (entityType === 'folder' && project.rootFolder.some(...)) {
    throw new Errors.NonDeletableEntityError('cannot delete root folder')
  }
  
  const { element, path } = await ProjectLocator.promises.findElement({
    project, element_id: entityId, type: entityType
  })
  
  const newProject = await _removeElementFromMongoArray(
    projectId, path.mongo, entityId, userId, deleteRootDoc
  )
}

async function _removeElementFromMongoArray(modelId, path, elementId, userId, deleteRootDoc = false) {
  const nonArrayPath = path.slice(0, path.lastIndexOf('.'))
  return Project.findOneAndUpdate(
    { _id: modelId },
    {
      $pull: { [nonArrayPath]: { _id: elementId } },
      $inc: { version: 1 },
      $set: { lastUpdated: new Date(), lastUpdatedBy: userId }
    },
    { new: true }
  )
}
```

**清理逻辑** (`_cleanUpEntity`):
- 递归列出所有子实体
- 文档执行软删除（设置 `deleted: true, deletedAt`）
- 文件从文件存储中删除

### 2.3 重命名文件夹

**入口**: `ProjectEntityUpdateHandler.renameEntity`

```javascript
const renameEntity = wrapWithLock(
  async function (projectId, entityId, entityType, newName, userId, source) {
    if (!SafePath.isCleanFilename(newName)) {
      throw new Errors.InvalidNameError('invalid element name')
    }
    
    const { project, startPath, endPath, rev, changes } =
      await ProjectEntityMongoUpdateHandler.promises.renameEntity(
        projectId, entityId, entityType, newName, userId
      )
    
    // 发送 TPDS 更新
    await TpdsUpdateSender.promises.moveEntity({
      projectId, projectName: project.name, startPath, endPath, rev, entityId, entityType
    })
    
    // 通知 DocumentUpdater
    await DocumentUpdaterHandler.promises.updateProjectStructure(...)
  }
)
```

**MongoDB 更新**:

```javascript
async function renameEntity(projectId, entityId, entityType, newName, userId) {
  const { element, path: entPath, folder: parentFolder } =
    await ProjectLocator.promises.findElement({ project, element_id: entityId, type })
  
  // 检查目标文件夹中是否已存在同名实体
  _checkValidElementName(parentFolder, newName)
  
  const newProject = await Project.findOneAndUpdate(
    { _id: projectId, [entPath.mongo]: { $exists: true } },
    {
      $set: { [`${entPath.mongo}.name`]: newName, lastUpdated: new Date(), lastUpdatedBy: userId },
      $inc: { version: 1 }
    },
    { new: true }
  )
}
```

### 2.4 移动文件夹

**入口**: `ProjectEntityUpdateHandler.moveEntity`

```javascript
const moveEntity = wrapWithLock(
  async function (projectId, entityId, destFolderId, entityType, userId, source) {
    // 1. 刷新项目
    await DocumentUpdaterHandler.promises.flushProjectToMongo(projectId)
    
    // 2. 执行移动
    const { project, startPath, endPath, rev, changes } =
      await ProjectEntityMongoUpdateHandler.promises.moveEntity(
        projectId, entityId, destFolderId, entityType, userId
      )
    
    // 3. 验证移动有效性（不能移动到自身子文件夹）
    await _checkValidMove(project, entityType, entity, entityPath, destFolderId)
    
    // 4. 先插入到目标位置，再从原位置删除（安全策略）
    await _putElement(project, destFolderId, entity, entityType, userId)
    await _removeElementFromMongoArray(projectId, entityPath.mongo, entityId, userId)
  }
)
```

**移动验证** (`_checkValidMove`):

```javascript
async function _checkValidMove(project, entityType, entity, entityPath, destFolderId) {
  // 检查目标文件夹中是否已存在同名实体
  _checkValidElementName(destEntity, entity.name)
  
  // 如果是文件夹，不能移动到自身子文件夹中
  if (/folder/.test(entityType)) {
    _checkValidFolderPath(entityPath.fileSystem, destFolderPath.fileSystem)
  }
}

function _checkValidFolderPath(folderPath, destinationFolderPath) {
  if (destinationFolderPath.startsWith(folderPath)) {
    throw new Errors.InvalidNameError('destination folder is a child folder of me')
  }
}
```

---

## 3. 文件上传与下载

### 3.1 文件上传流程

**主要路径**:
1. `/services/web/app/src/Features/Project/ProjectEntityUpdateHandler.mjs` - `addFile`
2. `/services/web/app/src/Features/FileStore/FileStoreHandler.mjs` - 上传到存储
3. `/services/filestore/app/js/FileHandler.js` - 文件句柄

**上传流程**:

```javascript
// 1. ProjectEntityUpdateHandler.addFile
const addFile = wrapWithLock({
  async beforeLock(projectId, folderId, fileName, fsPath, linkedFileData, userId, source) {
    const { fileRef, createdBlob } = await ProjectEntityUpdateHandler._uploadFile(
      projectId, folderId, fileName, fsPath, linkedFileData
    )
    return { projectId, folderId, userId, fileRef, createdBlob, source }
  },
  async withLock({ projectId, folderId, userId, fileRef, createdBlob, source }) {
    // 添加到项目结构
    const { result, project } = await ProjectEntityUpdateHandler._addFileAndSendToTpds(...)
    // 通知 DocumentUpdater
    await DocumentUpdaterHandler.promises.updateProjectStructure(...)
  }
})

// 2. FileStoreHandler.uploadFileFromDisk
FileStoreHandler.uploadFileFromDisk = function(projectId, fileArgs, fsPath, callback) {
  // 计算文件哈希
  FileHashManager.computeHash(fsPath, function(err, hash) {
    // 上传到历史存储
    FileStoreHandler._uploadToHistory(historyId, hash, size, fsPath, callback)
  })
}

// 3. _uploadToHistory
Async.retry(FileStoreHandler.RETRY_ATTEMPTS, cb =>
  HistoryManager.uploadBlobFromDisk(historyId, hash, size, fsPath, cb)
)
```

### 3.2 文件下载

**主要路径**:
- `/services/filestore/app/js/FileController.js` - HTTP 控制器
- `/services/filestore/app/js/FileHandler.js` - 文件处理

```javascript
// FileController.getFile
function getFile(req, res, next) {
  const { key, bucket } = req
  
  // 获取重定向 URL（S3 预签名 URL）
  FileHandler.getRedirectUrl(bucket, key, options, function(err, redirectUrl) {
    if (redirectUrl) {
      return res.redirect(redirectUrl)  // 重定向到 S3
    }
    
    // 直接返回文件流
    FileHandler.getFile(bucket, key, options, function(err, fileStream) {
      pipeline(fileStream, res, ...)  // 流式传输
    })
  })
}
```

### 3.3 Docstore 文档存储

文档内容存储在 Docstore 服务中：

```javascript
// DocManager.getFullDoc
async getFullDoc(projectId, docId) {
  const doc = await DocManager._getDoc(projectId, docId, {
    lines: true, rev: true, deleted: true, version: true, ranges: true, inS3: true
  })
  
  // 如果文档已归档到 S3，先恢复
  if (doc.inS3) {
    await DocArchive.unarchiveDoc(projectId, docId)
    return await DocManager._getDoc(projectId, docId, ...)
  }
  return doc
}

// DocManager.updateDoc - 带乐观锁
async updateDoc(projectId, docId, lines, version, ranges) {
  const MAX_ATTEMPTS = 2
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      return await DocManager._tryUpdateDoc(projectId, docId, lines, version, ranges)
    } catch (err) {
      if (err instanceof Errors.DocRevValueError && attempt < MAX_ATTEMPTS) {
        // 检测到并发更新，重试
        await setTimeout(100 + Math.random() * 100)
        continue
      }
      throw err
    }
  }
}
```

---

## 4. 项目模板

### 4.1 模板类型

**路径**: `/services/web/app/src/Features/Templates/`

Overleaf 支持以下模板类型：

1. **空白项目** - 仅包含根文件夹
2. **基础项目** - 包含示例 LaTeX 文件 (`mainbasic.tex`)
3. **示例项目** - 包含完整示例文档、图片和参考文献
4. **V1 模板** - 从旧版 Overleaf 导入的模板

### 4.2 创建示例项目

**路径**: `/services/web/app/src/Features/Project/ProjectCreationHandler.mjs`

```javascript
async function createExampleProject(ownerId, projectName, attributes = {}) {
  const project = await _createBlankProject(ownerId, projectName, attributes)
  
  // 添加示例文件
  const { fileEntries, docEntries } = await _addExampleProjectFiles(ownerId, projectName, project)
  
  // 预热 CLSI 缓存
  await populateClsiCacheForExampleProject(ownerId, project, fileEntries, docEntries)
  
  return project
}

async function _addExampleProjectFiles(ownerId, projectName, project) {
  const mainDocLines = await _buildTemplate('example-project/main.tex', ownerId, projectName)
  const rootDoc = await _createRootDoc(project, ownerId, mainDocLines)
  
  const bibDocLines = await _buildTemplate('example-project/sample.bib', ownerId, projectName)
  const bibDoc = await ProjectEntityUpdateHandler.promises.addDoc(...)
  
  const frogPath = path.join(templatesDir, 'example-project/frog.jpg')
  const { fileRef } = await ProjectEntityUpdateHandler.promises.addFile(
    project._id, project.rootFolder[0]._id, 'frog.jpg', frogPath, null, ownerId, null
  )
  
  return { fileEntries, docEntries }
}
```

### 4.3 V1 模板导入

**路径**: `/services/web/app/src/Features/Templates/TemplatesManager.mjs`

```javascript
async createProjectFromV1Template(brandVariationId, compiler, mainFile, 
                                   templateId, templateName, templateVersionId, userId, imageName) {
  // 1. 从 V1 API 获取模板 ZIP
  const zipUrl = `${settings.apis.v1.url}/api/v1/overleaf/templates/${templateVersionId}`
  const zipReq = await fetchStreamWithResponse(zipUrl, { basicAuth: {...} })
  
  // 2. 下载并保存到临时文件
  const dumpPath = `${settings.path.dumpFolder}/${crypto.randomUUID()}`
  await pipeline(zipReq.stream, fs.createWriteStream(dumpPath))
  
  // 3. 从 ZIP 创建项目
  const { fileEntries, docEntries, project } =
    await ProjectUploadManager.promises.createProjectFromZipArchiveWithName(
      userId, projectName, dumpPath, attributes
    )
  
  // 4. 设置主文件
  await TemplatesManager._setMainFile(project, mainFile)
}
```

### 4.4 项目复制

**路径**: `/services/web/app/src/Features/Project/ProjectDuplicator.mjs`

```javascript
async function duplicate(owner, originalProjectId, newProjectName, tags, isDebugCopy) {
  // 1. 刷新原项目到 MongoDB
  await DocumentUpdaterHandler.promises.flushProjectToMongo(originalProjectId)
  
  // 2. 获取原项目结构
  const originalProject = await ProjectGetter.promises.getProject(originalProjectId, {...})
  const originalEntries = _getFolderEntries(originalProject.rootFolder[0])
  
  // 3. 创建新项目
  const newProject = await ProjectCreationHandler.promises.createBlankProject(
    owner._id, newProjectName, attributes
  )
  
  // 4. 复制文档内容
  const [docEntries, fileEntries] = await Promise.all([
    _copyDocs(originalEntries.docEntries, originalProject, newProject),
    _copyFiles(originalEntries.fileEntries, originalProject, newProject)
  ])
  
  // 5. 创建文件夹结构
  await ProjectEntityMongoUpdateHandler.promises.createNewFolderStructure(
    newProject._id, docEntries, fileEntries
  )
}
```

---

## 5. 垃圾箱与版本历史

### 5.1 垃圾箱机制

**路径**: `/services/web/app/src/Features/Project/ProjectDeleter.mjs`

Overleaf 使用软删除机制：

```javascript
// 项目软删除
async function trashProject(projectId, userId) {
  await Project.updateOne(
    { _id: projectId },
    {
      $addToSet: { trashed: new ObjectId(userId) },
      $pull: { archived: new ObjectId(userId) }
    }
  )
}

// 恢复项目
async function untrashProject(projectId, userId) {
  await Project.updateOne(
    { _id: projectId },
    { $pull: { trashed: new ObjectId(userId) } }
  )
}

// 永久删除项目
async function deleteProject(projectId, options = {}) {
  const project = await Project.findOne({ _id: projectId }).exec()
  
  // 1. 刷新文档到 MongoDB 并删除
  await DocumentUpdaterHandler.promises.flushProjectToMongoAndDelete(projectId)
  
  // 2. 归档文档（可选）
  await DocstoreManager.promises.archiveProject(projectId)
  
  // 3. 保存到 DeletedProject 集合
  await DeletedProject.updateOne(
    { 'deleterData.deletedProjectId': projectId },
    { project, deleterData },
    { upsert: true }
  )
  
  // 4. 从 projects 集合删除
  await Project.deleteOne({ _id: projectId }).exec()
}
```

### 5.2 文档软删除

**路径**: `/services/docstore/app/js/DocManager.js`

```javascript
async patchDoc(projectId, docId, meta) {
  if (meta.deleted && Settings.docstore.archiveOnSoftDelete) {
    // 异步归档到 S3
    DocArchive.archiveDoc(projectId, docId).catch(err => {...})
  }
  await MongoManager.patchDoc(projectId, docId, meta)
}
```

### 5.3 DeletedProject 恢复

```javascript
async function undeleteProject(projectId, options = {}) {
  const deletedProject = await DeletedProject.findOne({
    'deleterData.deletedProjectId': projectId
  }).exec()
  
  // 恢复项目数据
  const restored = new Project(deletedProject.project)
  restored.name = await ProjectDetailsHandler.promises.generateUniqueName(
    deletedProject.deleterData.deletedProjectOwnerId,
    restored.name + ' (Restored)'
  )
  
  // 恢复已删除的文档上下文
  for (const deletedDoc of restored.deletedDocs) {
    await DocstoreManager.promises.deleteDoc(projectId, docId, name, deletedAt)
  }
  restored.deletedDocs = []
  
  // 重新插入到 projects 集合
  await db.projects.insertOne(restored)
  await DeletedProject.deleteOne({ _id: deletedProject._id }).exec()
}
```

### 5.4 文档归档机制

**路径**: `/services/docstore/app/js/DocArchiveManager.js`

```javascript
async function archiveDoc(projectId, docId) {
  const doc = await MongoManager.getDocForArchiving(projectId, docId)
  
  // 序列化为 JSON
  const json = JSON.stringify({
    lines: doc.lines,
    ranges: doc.ranges,
    rev: doc.rev,
    schema_v: 1
  })
  
  // 上传到 S3
  const key = `${projectId}/${docId}`
  await PersistorManager.sendStream(Settings.docstore.bucket, key, stream)
  
  // 标记为已归档
  await MongoManager.markDocAsArchived(projectId, docId, doc.rev)
}

// 恢复归档文档
async function unarchiveDoc(projectId, docId) {
  const archivedDoc = await getDoc(projectId, docId)
  await MongoManager.restoreArchivedDoc(projectId, docId, archivedDoc)
}
```

---

## 6. 并发控制

### 6.1 Redis 分布式锁

**路径**: `/services/web/app/src/infrastructure/LockManager.mjs`

Overleaf 使用 Redis 实现分布式锁：

```javascript
function createLockManager(options) {
  return new RedisWebLocker({
    rclient,  // Redis 客户端
    getKey(namespace, id) {
      return `lock:web:${namespace}:${id}`  // 锁键格式
    },
    options
  })
}

const LockManager = createLockManager(settings.lockManager)

LockManager.withTimeout = function(timeout) {
  return createLockManager({
    redisLockExpiry: timeout,
    slowExecutionThreshold: 0.5 * timeout * 1000
  })
}
```

### 6.2 项目结构锁

所有文件树操作都使用锁保护：

**路径**: `/services/web/app/src/Features/Project/ProjectEntityUpdateHandler.mjs`

```javascript
const LOCK_NAMESPACE = 'sequentialProjectStructureUpdateLock'

function wrapWithLock(methodWithoutLock, lockManager = LockManager) {
  const methodWithLock = async (projectId, ...rest) => {
    return lockManager.promises.runWithLock(
      LOCK_NAMESPACE, projectId,
      () => methodWithoutLock(projectId, ...rest)
    )
  }
  methodWithLock.withoutLock = methodWithoutLock
  return methodWithLock
}

// 所有文件树操作都通过此装饰器包装
const addDoc = wrapWithLock({ beforeLock, withLock })
const addFile = wrapWithLock({ beforeLock, withLock })
const deleteEntity = wrapWithLock(deleteEntity)
const moveEntity = wrapWithLock(moveEntity)
const renameEntity = wrapWithLock(renameEntity)
```

### 6.3 MongoDB 事务锁

**路径**: `/services/web/app/src/Features/Project/ProjectEntityMongoUpdateHandler.mjs`

```javascript
const LOCK_NAMESPACE = 'mongoTransaction'

function wrapWithLock(methodWithoutLock) {
  async function methodWithLock(projectId, ...rest) {
    return LockManager.promises.runWithLock(
      LOCK_NAMESPACE, projectId,
      () => methodWithoutLock(projectId, ...rest)
    )
  }
  return methodWithLock
}
```

### 6.4 文档修订控制

**路径**: `/services/docstore/app/js/MongoManager.js`

```javascript
async function upsertIntoDocCollection(projectId, docId, previousRev, updates) {
  if (previousRev) {
    // 乐观锁 - 只有 revision 匹配才能更新
    const result = await db.docs.updateOne(
      { _id: new ObjectId(docId), project_id: new ObjectId(projectId), rev: previousRev },
      { $set: updates, $unset: { inS3: true }, $inc: { rev: 1 } }
    )
    if (result.matchedCount !== 1) {
      throw new Errors.DocRevValueError()  // 并发冲突
    }
  } else {
    // 新建文档
    await db.docs.insertOne({ _id: new ObjectId(docId), project_id, rev: 1, ...updates })
  }
}

// 检查 revision 是否改变（用于检测并发修改）
async function checkRevUnchanged(doc) {
  const currentRev = await getDocRev(doc._id)
  if (doc.rev !== currentRev) {
    throw new Errors.DocModifiedError('doc rev has changed', {...})
  }
}
```

### 6.5 重试机制

```javascript
// DocManager.updateDoc 带重试
async updateDoc(projectId, docId, lines, version, ranges) {
  const MAX_ATTEMPTS = 2
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      return await DocManager._tryUpdateDoc(projectId, docId, lines, version, ranges)
    } catch (err) {
      if (err instanceof Errors.DocRevValueError && attempt < MAX_ATTEMPTS) {
        // 检测到并发冲突，等待后重试
        await setTimeout(100 + Math.random() * 100)
        continue
      }
      throw err
    }
  }
}
```

### 6.6 版本号机制

每个项目有一个全局 `version` 字段，每次文件树结构变更时递增：

```javascript
// Project.version 递增
await Project.findOneAndUpdate(
  { _id: projectId },
  { $inc: { version: 1 }, $set: { lastUpdated: new Date(), lastUpdatedBy: userId } }
)
```

---

## 7. 前端文件树组件

**路径**: `/services/web/frontend/js/features/file-tree/`

### 7.1 组件结构

```
file-tree/
  components/
    file-tree-root.tsx          # 根组件
    file-tree-folder.tsx         # 文件夹组件
    file-tree-folder-list.tsx   # 文件夹列表
    file-tree-doc.tsx            # 文档项
    file-tree-context-menu.tsx   # 右键菜单
    file-tree-action-buttons.tsx # 操作按钮
    modals/                      # 模态框
      create-folder-modal.tsx
      rename-modal.tsx
      delete-entity-modal.tsx
  hooks/
    use-project-entities.ts     # 获取项目实体
    use-collapsible-file-tree.tsx # 可折叠状态
  contexts/
    file-tree-data-context.tsx   # 文件树数据上下文
  util/
    mutate-in-tree.js            # 树操作工具
```

### 7.2 获取项目实体

**路径**: `/services/web/frontend/js/features/file-tree/hooks/use-project-entities.ts`

```typescript
export function useProjectEntities(projectId?: string) {
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<Entity[] | null>(null)
  const [error, setError] = useState<any>(false)

  useEffect(() => {
    if (projectId) {
      setLoading(true)
      getJSON(`/project/${projectId}/entities`, { signal })
        .then(data => {
          setData(data.entities.sort(alphabetical))
        })
        .catch(error => setError(error))
        .finally(() => setLoading(false))
    }
  }, [projectId, signal])

  return { loading, data, error }
}
```

### 7.3 Socket 监听

**路径**: `/services/web/frontend/js/features/file-tree/hooks/file-tree-socket-listener.ts`

实时接收来自服务器的更新：

```typescript
socket.on('reciveNewFolder', (folderId, folder, currentUserId) => {...})
socket.on('reciveNewFile', (folderId, fileRef, source, folder, currentUserId) => {...})
socket.on('reciveNewDoc', (folderId, doc, currentUserId) => {...})
socket.on('removeEntity', (entityId, source, currentUserId) => {...})
socket.on('renameEntity', (entityId, newName, currentUserId) => {...})
socket.on('moveEntity', (entityId, folderId, currentUserId) => {...})
```

---

## 8. 关键 API 端点

| 操作 | HTTP 方法 | 端点 |
|------|----------|------|
| 获取项目实体 | GET | `/project/:projectId/entities` |
| 创建文件夹 | POST | `/project/:projectId/folder` |
| 删除实体 | DELETE | `/project/:projectId/entity/:entityId` |
| 重命名实体 | POST | `/project/:projectId/entity/:entityId/rename` |
| 移动实体 | POST | `/project/:projectId/entity/:entityId/move` |
| 上传文件 | POST | `/project/:projectId/upload` |
| 下载文件 | GET | `/files/:fileId` |
| 项目垃圾箱 | POST | `/project/:projectId/trash` |
| 恢复项目 | POST | `/project/:projectId/untrash` |
| 永久删除 | DELETE | `/project/:projectId` |

---

## 10. 文件锁机制 (LockManager)

**路径**: `/services/web/app/src/infrastructure/LockManager.mjs`

Overleaf 使用 Redis 实现分布式锁，保护并发访问项目结构。

### 10.1 Redis 分布式锁核心实现

```javascript
import RedisWrapper from './RedisWrapper.mjs'
import RedisWebLocker from '@overleaf/redis-wrapper/RedisWebLocker.js'

const rclient = RedisWrapper.client('lock')

function createLockManager(options) {
  return new RedisWebLocker({
    rclient,
    getKey(namespace, id) {
      return `lock:web:${namespace}:${id}`
    },
    options,
  })
}

const LockManager = createLockManager(settings.lockManager)

LockManager.withTimeout = function(timeout) {
  const overrides = {
    redisLockExpiry: timeout,
    slowExecutionThreshold: 0.5 * timeout * 1000,
  }
  return createLockManager(Object.assign({}, settings.lockManager, overrides))
}
```

### 10.2 项目结构锁

**路径**: `/services/web/app/src/Features/Project/ProjectEntityUpdateHandler.mjs`

```javascript
const LOCK_NAMESPACE = 'sequentialProjectStructureUpdateLock'

function wrapWithLock(methodWithoutLock, lockManager = LockManager) {
  const methodWithLock = async (projectId, ...rest) => {
    return lockManager.promises.runWithLock(
      LOCK_NAMESPACE, projectId,
      () => methodWithoutLock(projectId, ...rest)
    )
  }
  methodWithLock.withoutLock = methodWithoutLock
  return methodWithLock
}

const addDoc = wrapWithLock({ beforeLock, withLock })
const addFile = wrapWithLock({ beforeLock, withLock })
const deleteEntity = wrapWithLock(deleteEntity)
const moveEntity = wrapWithLock(moveEntity)
const renameEntity = wrapWithLock(renameEntity)
```

### 10.3 锁粒度与超时策略

| 锁命名空间 | 保护内容 | 典型超时 |
|-----------|---------|---------|
| `sequentialProjectStructureUpdateLock` | 项目文件树结构变更 | 30s |
| `mongoTransaction` | MongoDB 原子操作 | 10s |

---

## 11. 版本历史存储

### 11.1 ProjectHistoryManager

```javascript
const ProjectHistoryHandler = {
  async ensureHistoryExistsForProject(projectId) {
    let historyId = await ProjectHistoryHandler.getHistoryId(projectId)
    if (historyId != null) return

    historyId = await HistoryManager.promises.initializeProject(projectId)
    await ProjectHistoryHandler.setHistoryId(projectId, historyId)
    await ProjectEntityUpdateHandler.promises.resyncProjectHistory(projectId, {})
    await HistoryManager.promises.flushProject(projectId)
  },
}
```

### 11.2 增量存储机制

```javascript
const MAX_TIME_BETWEEN_UPDATES = 60 * 1000
const MAX_UPDATE_SIZE = 2 * 1024 * 1024

const mergeUpdatesWithOp = function(firstUpdate, secondUpdate, op) {
  const update = Object.assign({}, firstUpdate)
  update.meta = Object.assign({}, update.meta)
  update.op = op
  if (secondUpdate.v != null) update.v = secondUpdate.v
  return update
}
```

---

## 12. ZIP 导入/导出

### 12.1 ZIP 导入流程

```javascript
async function createProjectFromZipArchive(ownerId, defaultName, zipPath) {
  const contentsPath = await _extractZip(zipPath)
  const { path, content } = await ProjectRootDocManager.promises
    .findRootDocFileFromDirectory(contentsPath)
  const projectName = DocumentHelper.getTitleFromTexContent(content || '') || defaultName
  const project = await ProjectCreationHandler.promises.createBlankProject(ownerId, uniqueName)
  await _initializeProjectWithZipContents(ownerId, project, contentsPath)
  await fs.promises.rm(contentsPath, { recursive: true, force: true })
}
```

### 12.2 ZIP 验证

```javascript
const ArchiveManager = {
  _isZipTooLarge(source, callback) {
    yauzl.open(source, { lazyEntries: true }, function(err, zipfile) {
      if (zipfile.entryCount > Settings.maxEntitiesPerProject) {
        return callback(null, true)  // 文件过多
      }
      zipfile.on('end', function() {
        const isTooLarge = totalSizeInBytes > ONE_MEG * 300  // 300MB 限制
        return callback(null, isTooLarge)
      })
    })
  },
}
```

---

## 13. 链接文件 (Linked Files)

### 13.1 linkedFileData 数据结构

```javascript
{
  provider: 'url' | 'project_file' | 'project_output',
  url: 'https://example.com/image.png',
  source_project_id: '...',
  source_doc_id: '...',
  importedAt: '2024-01-01T00:00:00.000Z',
}
```

### 13.2 链接文件类型

| Provider | 说明 | Agent |
|----------|------|-------|
| `url` | 从外部 URL 导入 | `UrlAgent` |
| `project_file` | 引用其他项目文件 | `ProjectFileAgent` |
| `project_output` | 引用其他项目输出 | `ProjectOutputFileAgent` |

---

## 14. 搜索实现

```typescript
export const searchSnapshot = async (
  projectSnapshot: ProjectSnapshot,
  searchQuery: SearchQuery
) => {
  const matchedFiles = new Map<string, MatchedFile>()
  const docPaths = projectSnapshot.getDocPaths()

  for (const path of docPaths) {
    const content = projectSnapshot.getDocContents(path)
    if (content) {
      const lines = content.split('\n')
      const text = Text.of(lines)

      const cursor = searchQuery.regexp
        ? new RegExpCursor(text, searchQuery.search, { ignoreCase: !searchQuery.caseSensitive })
        : new SearchCursor(text, searchQuery.search, ...)

      while (!cursor.next().done) {
        const { lineIndex, matchIndex, length } = cursor.value
        // 记录匹配
      }
    }
  }
  return matchedFiles
}
```

---

## 15. 前端文件树 Socket 同步

```typescript
export function useFileTreeSocketListener(onDelete) {
  const { socket } = useIdeContext()
  const { dispatchRename, dispatchDelete, dispatchMove } = useFileTreeData()

  useEffect(() => {
    if (!socket) return
    socket.on('reciveEntityRename', (entityId, name) => dispatchRename(entityId, name))
    socket.on('reciveEntityDelete', (entityId, origin) => dispatchDelete(entityId))
    socket.on('reciveEntityMove', (entityId, folderId) => dispatchMove(entityId, folderId))
    socket.on('reciveNewFolder', (...args) => dispatchCreateFolder(...))
    socket.on('reciveNewDoc', (...args) => dispatchCreateDoc(...))
  }, [socket])
}
```

---

## 16. 关键 API 端点

| 操作 | HTTP 方法 | 端点 |
|------|----------|------|
| 链接文件创建 | POST | `/project/:projectId/linked_file` |
| 链接文件刷新 | POST | `/project/:projectId/linked_file/:fileId/refresh` |
| 移动实体 | POST | `/project/:projectId/entity/:entityId/move` |
| ZIP 导出 | GET | `/project/:projectId/download/zip` |
| 全文搜索 | POST | `/project/:projectId/search` |

---

## 17. 总结

Overleaf 文件管理体系的核心设计：

1. **分层存储**: 项目结构在 MongoDB (web 服务)，文档内容在 Docstore，文件在 Filestore/S3
2. **软删除机制**: 文档和项目使用软删除，支持恢复
3. **分布式锁**: 使用 Redis 锁保护并发操作
4. **乐观锁**: 文档修订使用乐观锁检测并发冲突
5. **实时同步**: 通过 Socket.IO 实时推送更新到前端
6. **归档机制**: 不活跃的文档自动归档到 S3 节省 MongoDB 空间
7. **增量历史**: 基于 diff-match-patch 的增量压缩存储，支持任意版本重建
8. **链接文件**: 支持 URL、项目文件、项目输出三种外部引用

---

## 18. 参考资料

- Overleaf 官方仓库: https://github.com/overleaf/overleaf
- MongoDB 文档: https://docs.mongodb.com/
