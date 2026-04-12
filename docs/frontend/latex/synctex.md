# Overleaf SyncTeX 双向同步机制技术文档

## 目录

1. [SyncTeX 基础](#synctex-基础)
2. [代码 → PDF 同步（Code to PDF）](#代码--pdf-同步code-to-pdf)
3. [PDF → 代码同步（PDF to Code）](#pdf--代码同步pdf-to-code)
4. [前端事件总线](#前端事件总线)
5. [后端 API 架构](#后端-api-架构)
6. [边界情况处理](#边界情况处理)
7. [关键文件索引](#关键文件索引)

---

## SyncTeX 基础

### 什么是 SyncTeX

SyncTeX 是一个用于 LaTeX 文档的同步协议，允许在编辑器（TeX 源码）和 PDF 阅读器之间建立双向跳转关系。它由 LaTeX 编译时生成 `.synctex.gz` 文件来记录源码位置和 PDF 位置的映射关系。

### 编译时生成 SyncTeX

在 Overleaf 中，编译通过 `latexmk` 触发，SyncTeX 通过 `-synctex=1` 参数启用：

**文件**: `/tmp/overleaf/services/clsi/app/js/LatexRunner.js` (第 163-213 行)

```javascript
function _buildLatexCommand(mainFile, opts = {}) {
  const command = []
  command.push(
    'latexmk',
    '-cd',
    '-jobname=output',
    '-auxdir=$COMPILE_DIR',
    '-outdir=$COMPILE_DIR',
    '-synctex=1',              // <-- 启用 SyncTeX
    '-interaction=batchmode',
    '-time'
  )
  // ...
}
```

这会在编译输出目录生成 `output.synctex.gz` 文件。

### synctex 命令行工具

SyncTeX 提供两个核心命令：

1. **`synctex view`** - 从源码位置查询 PDF 位置
   ```bash
   synctex view -i line:column:input.tex -o output.pdf
   ```
   输出 PDF 的 page、h、v、width、height

2. **`synctex edit`** - 从 PDF 位置查询源码位置
   ```bash
   synctex edit -o page:h:v:output.pdf
   ```
   输出 Input（文件路径）、Line、Column

---

## 代码 → PDF 同步（Code to PDF）

### 完整流程

```
用户点击"跳转到 PDF"按钮
        ↓
cursor:editor:syncToPdf 事件触发
        ↓
useSynctex.syncToPdf() 获取当前光标位置
        ↓
GET /project/:id/sync/code?file=...&line=...&column=...
        ↓
CLSI 后端调用 synctex view
        ↓
返回 PDF 位置 [{page, h, v, width, height}]
        ↓
setHighlights() 高亮并滚动到该位置
```

### 光标位置转换为 Synctex 坐标

**文件**: `/tmp/overleaf/services/web/frontend/js/features/pdf-preview/hooks/use-synctex.ts` (第 152-166 行)

```typescript
const syncToPdf = useCallback(() => {
  const file = getCurrentFilePath()
  if (cursorPositionRef.current) {
    const { row, column } = cursorPositionRef.current
    const params = new URLSearchParams({
      file: file ?? '',
      line: String(row + 1),   // 行号从 1 开始
      column: String(column),
    }).toString()
    goToPdfLocation(params)
  }
}, [getCurrentFilePath, goToPdfLocation])
```

光标位置通过 `cursor:editor:update` 事件实时更新并存储在 `cursorPositionRef` 中。

### 后端处理

**文件**: `/tmp/overleaf/services/clsi/app/js/CompileManager.js` (第 474-504 行)

```javascript
async function syncFromCode(projectId, userId, filename, line, column, opts) {
  const compileName = getCompileName(projectId, userId)
  const baseDir = Settings.path.synctexBaseDir(compileName)
  const inputFilePath = Path.join(baseDir, filename)
  const outputFilePath = Path.join(baseDir, 'output.pdf')
  
  const command = [
    'synctex', 'view',
    '-i', `${line}:${column}:${inputFilePath}`,
    '-o', outputFilePath,
  ]
  const { stdout, downloadedFromCache } = await _runSynctex(...)
  return {
    codePositions: SynctexOutputParser.parseViewOutput(stdout),
    downloadedFromCache,
  }
}
```

### SynctexOutputParser 解析 view 输出

**文件**: `/tmp/overleaf/services/clsi/app/js/SynctexOutputParser.js`

```javascript
function parseViewOutput(output) {
  return _parseOutput(output, (record, label, value) => {
    switch (label) {
      case 'Page':   _setIntProp(record, 'page', value);  break;
      case 'h':      _setFloatProp(record, 'h', value);   break;
      case 'v':      _setFloatProp(record, 'v', value);   break;
      case 'W':      _setFloatProp(record, 'width', value); break;
      case 'H':      _setFloatProp(record, 'height', value); break;
    }
  })
}
```

返回格式：
```json
{
  "pdf": [{
    "page": 1,
    "h": 133.768356,
    "v": 134.764618,
    "height": 6.918498,
    "width": 343.71106
  }]
}
```

### PDF.js 高亮定位

**文件**: `/tmp/overleaf/services/web/frontend/js/features/pdf-preview/hooks/use-synctex.ts` (第 109-144 行)

```typescript
const goToPdfLocation = useCallback((params: string) => {
  // ...
  getJSON(`/project/${projectId}/sync/code?${params}`, { signal })
    .then(data => {
      setShowLogs(false)
      setHighlights(data.pdf)  // 设置高亮位置
    })
    // ...
}, [])
```

高亮元素构建和滚动逻辑在 `pdf-js-viewer.tsx` 的 `useEffect` 中处理（第 321-390 行）。

---

## PDF → 代码同步（PDF to Code）

### 完整流程

```
用户在 PDF 上双击
        ↓
synctex:sync-to-position 事件触发
        ↓
useSynctex._syncToCode() 计算 PDF 坐标
        ↓
GET /project/:id/sync/pdf?page=...&h=...&v=...
        ↓
CLSI 后端调用 synctex edit
        ↓
返回代码位置 [{file, line, column}]
        ↓
goToCodeLine() 打开文件并跳转到指定行
```

### PDF 坐标计算

**文件**: `/tmp/overleaf/services/web/frontend/js/features/pdf-preview/util/pdf-js-wrapper.ts` (第 139-160 行)

```typescript
clickPosition(event: MouseEvent, canvas: HTMLCanvasElement, page: number) {
  const { viewport } = this.viewer.getPageView(page)
  const pageRect = canvas.getBoundingClientRect()
  
  const dx = event.clientX - pageRect.left
  const dy = event.clientY - pageRect.top
  
  // 将 DOM 坐标转换为 PDF 坐标
  const [left, top] = viewport.convertToPdfPoint(dx, dy)
  
  return {
    page,
    offset: {
      left,
      top: viewport.viewBox[3] - top,  // Y 轴反转
    },
  }
}
```

### PDF 坐标到代码行号的转换（后端）

**文件**: `/tmp/overleaf/services/clsi/app/js/CompileManager.js` (第 506-527 行)

```javascript
async function syncFromPdf(projectId, userId, page, h, v, opts) {
  const compileName = getCompileName(projectId, userId)
  const baseDir = Settings.path.synctexBaseDir(compileName)
  const outputFilePath = `${baseDir}/output.pdf`
  
  const command = [
    'synctex', 'edit',
    '-o', `${page}:${h}:${v}:${outputFilePath}`,
  ]
  const { stdout, downloadedFromCache } = await _runSynctex(...)
  return {
    pdfPositions: SynctexOutputParser.parseEditOutput(stdout, baseDir),
    downloadedFromCache,
  }
}
```

### SynctexOutputParser 解析 edit 输出

**文件**: `/tmp/overleaf/services/clsi/app/js/SynctexOutputParser.js` (第 31-49 行)

```javascript
function parseEditOutput(output, baseDir) {
  return _parseOutput(output, (record, label, value) => {
    switch (label) {
      case 'Input':
        if (Path.isAbsolute(value)) {
          record.file = Path.relative(baseDir, value)  // 转为相对路径
        } else {
          record.file = value
        }
        break
      case 'Line':   _setIntProp(record, 'line', value);   break;
      case 'Column':  _setIntProp(record, 'column', value); break;
    }
  })
}
```

返回格式：
```json
{
  "code": [{
    "file": "main.tex",
    "line": 3,
    "column": -1
  }]
}
```

### 前端处理返回的代码位置

**文件**: `/tmp/overleaf/services/web/frontend/js/features/pdf-preview/hooks/use-synctex.ts` (第 180-251 行)

```typescript
const _syncToCode = useCallback(
  ({ position = positionRef.current, selectText, visualOffset = 0 }) => {
    if (!position) return
    
    const h = position.offset.left
    // PDF 坐标从页面顶部向下增长，需要反转 Y 轴
    let v = 0
    if (position.pageSize?.height) {
      v += position.pageSize.height - position.offset.top
    } else {
      v += position.offset.top
    }
    v += visualOffset  // 视觉偏移补偿
    
    const params = new URLSearchParams({
      page: position.page + 1,
      h: h.toFixed(2),
      v: v.toFixed(2),
    })
    
    getJSON(`/project/${projectId}/sync/pdf?${params}`, { signal })
      .then(data => {
        const [{ file, line }] = data.code
        goToCodeLine(file, line, selectText)
      })
      // ...
  }, []
)
```

---

## 前端事件总线

### 核心事件列表

| 事件名 | 方向 | 描述 |
|--------|------|------|
| `cursor:editor:update` | 编辑器 → 全局 | 光标位置更新 |
| `cursor:editor:syncToPdf` | 编辑器 → PDF | 请求跳转到 PDF |
| `synctex:sync-to-position` | PDF → 全局 | PDF 点击触发跳转 |
| `editor:gotoLine` | 全局 → 编辑器 | 跳转到指定行 |

### 事件定义

**文件**: `/tmp/overleaf/services/web/frontend/js/features/ide-react/create-ide-event-emitter.ts`

```typescript
export type IdeEvents = {
  'cursor:editor:update': [position: CursorPosition]
  'cursor:editor:syncToPdf': []
  'scroll:editor:update': [middleVisibleLine?: number]
  // ...
}
```

### 事件监听示例

**PDF 双击监听** (`pdf-js-viewer.tsx` 第 257-273 行):

```typescript
const doubleClickListener = (event: MouseEvent) => {
  const clickPosition = pdfJsWrapper.clickPosition(
    event,
    textLayerDiv.closest('.page').querySelector('canvas'),
    textLayer.pageNumber - 1
  )
  if (clickPosition) {
    window.dispatchEvent(
      new CustomEvent('synctex:sync-to-position', {
        detail: {
          position: clickPosition,
          selectText: window.getSelection()?.toString(),
        },
      })
    )
  }
}
```

**synctex:sync-to-position 处理** (`use-synctex.ts` 第 260-263 行):

```typescript
useEventListener(
  'synctex:sync-to-position',
  useCallback((event: CustomEvent) => syncToCode(event.detail), [syncToCode])
)
```

### Detach 模式下的跨窗口通信

**文件**: `/tmp/overleaf/services/web/frontend/js/shared/hooks/use-detach-action.js`

```javascript
export default function useDetachAction(
  actionName,
  actionFunction,
  senderRole,
  targetRole
) {
  const { role, broadcastEvent, addEventHandler } = useDetachContext()
  
  const triggerFn = useCallback(
    (...args) => {
      if (role === senderRole) {
        broadcastEvent(eventName, { args })  // 跨窗口广播
      } else {
        return actionFunction(...args)  // 本地执行
      }
    }, [role, senderRole, eventName, actionFunction, broadcastEvent]
  )
  // ...
}
```

当 PDF 查看器在分离窗口（detached）中运行时，`syncToCode` 通过 `broadcastEvent` 发送到主窗口执行。

---

## 后端 API 架构

### API 路由

**Web Server** (`/tmp/overleaf/services/web/app/src/router.mjs` 第 682-690 行):
```javascript
webRouter.get('/project/:Project_id/sync/code', ..., CompileController.proxySyncCode)
webRouter.get('/project/:Project_id/sync/pdf',  ..., CompileController.proxySyncPdf)
```

**CLSI Server** (`/tmp/overleaf/services/clsi/app.js` 第 82-83 行):
```javascript
app.get('/project/:project_id/sync/code', CompileController.syncFromCode)
app.get('/project/:project_id/sync/pdf', CompileController.syncFromPdf)
```

### 请求流程

```
┌─────────────────────────────────────────────────────────────────┐
│                        Web Server                                │
│  /project/:id/sync/code?file=main.tex&line=3&column=5           │
│           ↓                                                      │
│  CompileController.proxySyncCode()                               │
│           ↓                                                      │
│  CompileManager.syncTeX()                                       │
│           ↓                                                      │
│  ClsiManager.syncTeX()                                          │
│           ↓                                                      │
│  HTTP GET to CLSI: /project/:id/sync/code?...                   │
└─────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────┐
│                         CLSI Server                              │
│  CompileController.syncFromCode()                               │
│           ↓                                                      │
│  CompileManager.syncFromCode()                                  │
│           ↓                                                      │
│  synctex view -i 3:5:/path/to/main.tex -o output.pdf            │
│           ↓                                                      │
│  SynctexOutputParser.parseViewOutput(stdout)                    │
└─────────────────────────────────────────────────────────────────┘
```

### 关键验证参数

**proxySyncPdf** (`CompileController.mjs` 第 539-551 行):
```javascript
async proxySyncPdf(req, res) {
  const { page, h, v } = req.query
  if (!page?.match(/^\d+$/)) throw new Error('invalid page parameter')
  if (!h?.match(/^-?\d+\.\d+$/)) throw new Error('invalid h parameter')
  if (!v?.match(/^-?\d+\.\d+$/)) throw new Error('invalid v parameter')
  await _syncTeX(req, res, 'pdf', { page, h, v })
}
```

**proxySyncCode** (`CompileController.mjs` 第 553-574 行):
```javascript
async proxySyncCode(req, res) {
  const { file, line, column } = req.query
  if (file == null) throw new Error('missing file parameter')
  // 验证路径安全性
  const testPath = file.replace('/./', '/')
  if (Path.resolve('/', testPath) !== `/${testPath}`) {
    throw new Error('invalid file parameter')
  }
  if (!line?.match(/^\d+$/)) throw new Error('invalid line parameter')
  if (!column?.match(/^\d+$/)) throw new Error('invalid column parameter')
  await _syncTeX(req, res, 'code', { file, line, column })
}
```

---

## 边界情况处理

### 1. 滚动位置补偿

代码在 `use-synctex.ts` 中处理视觉偏移（第 195-199 行）：

```typescript
// FIXME: this actually works better if it's halfway across the
// page (or the visible part of the page). Synctex doesn't
// always find the right place in the file when the point is at
// the edge of the page, it sometimes returns the start of the
// next paragraph instead.
const h = position.offset.left
```

按钮点击时传入 72 像素的 `visualOffset` (`pdf-synctex-controls.tsx` 第 43 行):

```typescript
const syncToCodeWithButton = useCallback(() => {
  syncToCode({ visualOffset: 72 })  // 避免边缘定位问题
}, [syncToCode])
```

### 2. Y 轴坐标反转

PDF 坐标系从页面底部向上增长，而浏览器 DOM 坐标从顶部向下增长：

```typescript
// pdf-js-wrapper.ts 第 157 行
top: viewport.viewBox[3] - top,  // 反转 Y 坐标

// use-synctex.ts 第 207-212 行
let v = 0
if (position.pageSize?.height) {
  v += position.pageSize.height - position.offset.top  // 基于页面高度反转
} else {
  v += position.offset.top
}
v += visualOffset
```

### 3. 多页面定位

PDF 查看器需要找到当前可见页面：

```typescript
// pdf-js-wrapper.ts 第 163-191 行
get currentPosition() {
  const containerRect = this.container.getBoundingClientRect()
  
  let pageIndex = this.viewer.currentPageNumber - 1
  for (let i = pageIndex; i >= 0; i--) {
    const pageView = this.viewer.getPageView(i)
    const pageRect = pageView.div.getBoundingClientRect()
    if (pageRect.bottom < containerRect.top) {
      pageIndex = i + 1
      break
    }
    pageIndex = i
  }
  // ...
}
```

### 4. 缩放比例处理

PDF 查看器支持多种缩放模式：

```typescript
// pdf-js-wrapper.ts 第 193-217 行
scrollToPosition(position: Record<string, any>, scale = null) {
  const destArray = [
    null,
    { name: 'XYZ' },
    position.offset.left,
    position.offset.top,
    scale,  // null 时保持当前缩放
  ]
  this.viewer.scrollPageIntoView({ pageNumber: position.page + 1, destArray })
}
```

缩放值存储在 `localStorage` 中 (`pdf-js-viewer.tsx` 第 34-37 行)：

```typescript
const [scale, setScale] = usePersistedState(
  `pdf-viewer-scale:${projectId}`,
  'page-width'
)
```

### 5. Detach 模式状态同步

**文件**: `/tmp/overleaf/services/web/frontend/js/shared/hooks/use-detach-state.js`

分离窗口时，`syncToCodeInFlight` 等状态需要在窗口间同步：

```typescript
const [syncToCodeInFlight, setSyncToCodeInFlight] = useDetachState(
  'sync-to-code-inflight',
  false,
  'detacher',   // 主窗口角色
  'detached'    // 分离窗口角色
)
```

### 6. CLSI 缓存策略

**文件**: `/tmp/overleaf/services/clsi/app/js/CompileManager.js` (第 553-644 行)

当 `.synctex.gz` 文件不存在但 `compileFromClsiCache` 启用时，从缓存下载：

```javascript
if (err instanceof Errors.NotFoundError && compileFromClsiCache && editorId && buildId) {
  downloadedFromCache = await downloadOutputDotSynctexFromCompileCache(
    projectId, userId, editorId, buildId, directory
  )
}
```

---

## 关键文件索引

### 前端 (TypeScript/TSX)

| 文件路径 | 功能 |
|----------|------|
| `services/web/frontend/js/features/pdf-preview/hooks/use-synctex.ts` | SyncTeX 核心 Hook |
| `services/web/frontend/js/features/pdf-preview/components/pdf-js-viewer.tsx` | PDF 查看器组件 |
| `services/web/frontend/js/features/pdf-preview/util/pdf-js-wrapper.ts` | PDF.js 封装 |
| `services/web/frontend/js/features/pdf-preview/components/pdf-synctex-controls.tsx` | 同步控制按钮 |
| `services/web/frontend/js/features/pdf-preview/components/synctex-toasts.tsx` | 错误提示 |
| `services/web/frontend/js/features/ide-react/create-ide-event-emitter.ts` | 事件类型定义 |
| `services/web/frontend/js/features/source-editor/hooks/use-codemirror-scope.ts` | 光标事件发射 |
| `services/web/frontend/js/shared/hooks/use-detach-action.js` | 跨窗口动作分发 |
| `services/web/frontend/js/shared/context/detach-compile-context.tsx` | Detach 上下文 |

### 后端 (JavaScript)

| 文件路径 | 功能 |
|----------|------|
| `services/clsi/app/js/CompileManager.js` | 编译管理 + SyncTeX 入口 |
| `services/clsi/app/js/SynctexOutputParser.js` | synctex 输出解析器 |
| `services/clsi/app/js/LatexRunner.js` | LaTeX 编译命令构建 |
| `services/clsi/app.js` | CLSI 路由定义 |
| `services/web/app/src/Features/Compile/CompileController.mjs` | Web 层代理控制器 |
| `services/web/app/src/Features/Compile/CompileManager.mjs` | Web 层编译管理 |
| `services/web/app/src/Features/Compile/ClsiManager.mjs` | CLSI 通信层 |
| `services/web/app/src/router.mjs` | Web 路由定义 |

### 测试文件

| 文件路径 | 描述 |
|----------|------|
| `services/clsi/test/acceptance/js/SynctexTests.js` | SyncTeX 集成测试 |
| `services/web/test/frontend/components/pdf-preview/pdf-synctex-controls.spec.tsx` | 前端组件测试 |

---

## 数据流总结图

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                        Frontend                          │
                    │                                                          │
  Code Editor       │              PDF Viewer                                  │
       │            │                  │                                        │
       │ cursor:    │                  │ dblclick                               │
       │ update     │                  │                                        │
       │            │                  │ synctex:sync-to-position               │
       │            │                  │                                        │
       └──────────┬─┘                  └──────────┬────────────────────────────┘
                  │                                │
                  │ cursor:editor:syncToPdf         │ synctex:sync-to-position
                  │                                │
                  ▼                                ▼
        ┌─────────────────────────────────────────────────────────┐
        │              useSynctex Hook                             │
        │                                                          │
        │  syncToPdf() ──────────── _syncToCode()                  │
        │       │                            │                    │
        │       │ getJSON                    │ getJSON            │
        │       ▼                            ▼                    │
        │  /project/:id/sync/code     /project/:id/sync/pdf        │
        └─────────────────────────────────────────────────────────┘
                              │
                              ▼
        ┌─────────────────────────────────────────────────────────┐
        │              Web Server (CompileController)              │
        │                                                          │
        │  proxySyncCode() ─────────── proxySyncPdf()              │
        │       │                            │                    │
        │       └──────────┬─────────────────┘                    │
        │                 │                                        │
        │                 ▼                                        │
        │         ClsiManager.syncTeX()                            │
        └─────────────────────────────────────────────────────────┘
                              │
                              ▼
        ┌─────────────────────────────────────────────────────────┐
        │                    CLSI Server                          │
        │                                                          │
        │  syncFromCode() ─────────── syncFromPdf()               │
        │       │                            │                    │
        │       ▼                            ▼                    │
        │  synctex view                  synctex edit             │
        │       │                            │                    │
        │       └──────────┬─────────────────┘                    │
        │                 │                                        │
        │                 ▼                                        │
        │     SynctexOutputParser.parseViewOutput()                │
        │     SynctexOutputParser.parseEditOutput()                │
        └─────────────────────────────────────────────────────────┘
```
