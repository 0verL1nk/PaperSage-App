# Overleaf 编辑器实现详解

## 概述

Overleaf 采用 **CodeMirror 6** 作为编辑器核心，结合自定义的 **Lezer LaTeX** 语法解析器实现 LaTeX 编辑功能。实时协作基于 **Operational Transformation (OT)** 算法，通过 `overleaf-editor-core` 库和 ShareJS 实现。

## 1. CodeMirror 6 集成方式

### 1.1 核心架构

编辑器主入口位于 `services/web/frontend/js/features/source-editor/`:

```
source-editor/
├── components/          # React 组件层
│   ├── codemirror-editor.tsx    # 主编辑器组件
│   └── source-editor.tsx        # 编辑器包装器
├── extensions/          # CodeMirror 扩展
│   ├── index.ts         # 扩展入口，组合所有扩展
│   ├── realtime.ts      # 实时协作扩展
│   ├── auto-complete.ts # 自动补全扩展
│   └── history-ot.ts    # 追踪变更扩展
├── languages/
│   └── latex/           # LaTeX 语言支持
│       ├── latex-language.ts    # LanguageDefinition
│       └── complete.ts          # 补全逻辑
└── lezer-latex/         # Lezer 语法解析器
    ├── latex.grammar    # grammar.pegjs 语法定义
    └── tokens.mjs       # token 定义
```

### 1.2 扩展注册机制

Overleaf 使用模块化扩展系统，通过 `import-overleaf-module.macro` 宏动态加载扩展：

```typescript
// services/web/frontend/js/features/source-editor/extensions/index.ts
const moduleExtensions: Array<(options: Record<string, any>) => Extension> =
  importOverleafModules('sourceEditorExtensions').map(
    (item: { import: { extension: Extension } }) => item.import.extension
  )
```

所有扩展通过 `createExtensions()` 函数组合：

```typescript
export const createExtensions = (options: Record<string, any>): Extension[] => [
  lineNumbers(),
  highlightSpecialChars(options.visual.visual),
  history({ newGroupDelay: 250 }),
  foldGutter({...}),
  language(options.docName, options.metadata, options.settings),
  realtime(options.currentDoc, options.handleError),
  autoComplete({...}),
  // ... 更多扩展
]
```

### 1.3 主编辑器组件

**文件**: `services/web/frontend/js/features/source-editor/components/codemirror-editor.tsx`

```typescript
export function CodeMirrorEditor({
  viewOptions,
  onEditorReady,
}: CodeMirrorEditorProps) {
  const { view, viewRef } = useCodeMirrorView({
    ...viewOptions,
    parent: editorRef.current!,
  })

  // 挂载到 DOM 后的回调
  onEditorReady?.(view)

  return <div ref={editorRef} className="ol-cm-wrapper" />
}
```

## 2. LaTeX 语法高亮实现

### 2.1 Lezer 解析器架构

Overleaf 使用 **Lezer** (CodeMirror 6 的解析器系统) 解析 LaTeX：

**语法文件**: `services/web/frontend/js/features/source-editor/lezer-latex/latex.grammar`

```pegjs
// 基于 PEG.js 的语法定义
@top Document { (Element | Comment | NewLine)* }

Element {
  (Command | Math | Braces | Brackets | Text)*
}

Command {
  ("\\" | "\\\\") (CtrlSeq | Csname | MathShift | AlignmentTab)
}

CtrlSeq {
  /\\[a-zA-Z]+/      // 标准命令如 \section
  | /\\[^a-zA-Z]/    // 符号命令如 \{
}

Math {
  MathShift (MathText | MathChar)* MathShift?
}

Comment {
  "%" CommentContent*
}
```

### 2.2 Token 定义

**文件**: `services/web/frontend/js/features/source-editor/lezer-latex/tokens.mjs`

导出 LaTeX 特定的 token 类型：
- `ctrlSeq`: 控制序列 (`\section`, `\textbf`)
- `ctrlSym`: 控制符号 (`\{`, `\%`)
- `envName`: 环境名称 (`document`, `equation`)

### 2.3 语法高亮配置

**文件**: `services/web/frontend/js/features/source-editor/languages/latex/latex-language.ts`

```typescript
import { styleTags, tags as t } from '@lezer/highlight'

const Styles = {
  ctrlSeq: Object.fromEntries(
    Tokens.ctrlSeq.map(token => styleEntry(token, t.tagName))
  ),
  ctrlSym: Object.fromEntries(
    Tokens.ctrlSym.map(token => styleEntry(token, t.literal))
  ),
  envName: Object.fromEntries(
    Tokens.envName.map(token => styleEntry(token, t.attributeValue))
  ),
}

export const LaTeXLanguage = LRLanguage.define({
  name: 'latex',
  parser: parser.configure({
    props: [
      // 折叠节点属性
      foldNodeProp.add({
        Comment: (node, state) => {...},
        Group: foldInside,
        $Environment: node => node.getChild('Content'),
      }),
      // 样式标签
      styleTags({
        ...Styles.ctrlSeq,
        ...Styles.ctrlSym,
        ...Styles.envName,
        'HrefCommand/ShortTextArgument/...': t.link,
        Number: t.number,
        Comment: t.comment,
        // ...
      }),
    ],
  }),
})
```

### 2.4 节点组定义

通过 `NodeProp.group` 动态分组：

```typescript
NodeProp.group.add(type => {
  const types = []
  if (Tokens.ctrlSeq.includes(type.name)) {
    types.push('$CtrlSeq')
  } else if (type.name.endsWith('Argument')) {
    types.push('$Argument')
    if (type.name.endsWith('TextArgument')) {
      types.push('$TextArgument')
    }
  }
  return types.length > 0 ? types : undefined
})
```

## 3. 自动补全（Autocomplete）机制

### 3.1 补全扩展架构

**文件**: `services/web/frontend/js/features/source-editor/extensions/auto-complete.ts`

```typescript
export const autoComplete = ({ enabled, ...rest }: AutoCompleteOptions) =>
  autoCompleteConf.of(createAutoComplete({ enabled, ...rest }))

const createAutoComplete = ({ enabled, ...rest }: AutoCompleteOptions) => {
  if (!enabled) return []

  return [
    autocompleteTheme,
    autocompletion({
      icons: false,
      defaultKeymap: false,
      interactionDelay: 0,
      optionClass: (completion: Completion) => `ol-cm-completion-${completion.type}`,
    }),
    // 自定义按键绑定
    Prec.highest(keymap.of([
      { key: 'Escape', run: closeCompletion },
      { key: 'Enter', run: acceptCompletion },
      { key: 'Tab', run: acceptCompletion },
    ])),
  ]
}
```

### 3.2 LaTeX 特定补全源

**文件**: `services/web/frontend/js/features/source-editor/languages/latex/complete.ts`

Overleaf 为 LaTeX 实现了多个补全源：

```typescript
// 1. 命令补全
export const inCommandCompletionSource: CompletionSource = ifInType(
  '$CtrlSeq',
  context => {
    return context.explicit ? null : commandCompletionSource(context)
  }
)

// 2. 环境名称补全（\begin{}）
export const environmentNameCompletionSource = makeArgumentCompletionSource(
  ['EnvNameGroup'],
  ({ completions, context, matchBefore, before }) => {
    if (cursorIsAtBeginEnvironment(context.state, context.pos)) {
      buildAllCompletions(completions, context)
      return {
        from: matchBefore.from,
        validFor: /^\\begin{\S*/,
        options: [...completions.commands, ...customEnvironmentCompletions(context)],
      }
    }
    // ...
  }
)

// 3. 引用补全（\ref{}）
export const refArgumentCompletionSource = makeMultipleArgumentCompletionSource(
  ['RefArgument'],
  ({ completions, context, from, validFor, existingKeys }) => {
    buildLabelCompletions(completions, context)
    return {
      from,
      validFor,
      options: completions.labels.filter(item => !existingKeys.includes(item.label)),
    }
  }
)

// 4. 包/类补全
export const packageArgumentCompletionSource = makeMultipleArgumentCompletionSource(
  ['PackageArgument'],
  // ...
)
```

### 3.3 补全数据构建

**文件**: `services/web/frontend/js/features/source-editor/languages/latex/completions/`

```
completions/
├── environments.ts   # 环境模板
├── doc-commands.ts   # 文档命令
├── references.ts    # 引用标签
├── packages.ts      # LaTeX 包
├── labels.ts        # 定义的标签
├── include.ts       # 文件引用
└── bibliography-styles.ts
```

补全数据结构：

```typescript
export type Completions = {
  bibliographies: Completion[]
  bibliographyStyles: Completion[]
  classes: Completion[]
  commands: Completion[]
  graphics: Completion[]
  includes: Completion[]
  labels: Completion[]
  packages: Completion[]
  references: Completion[]
}
```

## 4. 符号/模板插入

### 4.1 符号面板

**文件**: `services/web/frontend/js/features/source-editor/extensions/symbol-palette.ts`

提供数学符号面板，用户可以点击插入符号。

### 4.2 Snippet 模板

**文件**: `services/web/frontend/js/features/source-editor/languages/latex/snippets.ts`

环境模板使用 snippet 格式：

```typescript
import { snippet } from './completions/data/environments'

// snippet 示例
export const snippet = (name: string) => {
  return `\\begin{${name}}\n  $0\n\\end{${name}}`
}
```

### 4.3 Command Tooltip

**文件**: `services/web/frontend/js/features/source-editor/extensions/command-tooltip.ts`

当用户输入特定命令时，显示工具提示：

```typescript
const COMMAND_TOOLTIPS: Record<string, string> = {
  '\\frac': '\\\\frac{num}{denom} - 分数',
  '\\section': '\\\\section{title} - 章节标题',
  '\\ref': '\\\\ref{label} - 引用标签',
}
```

## 5. 实时协作编辑实现

### 5.1 整体架构

Overleaf 实时协作系统由多个服务组成：

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend                              │
│  CodeMirror 6 → ShareJsDoc → Socket.io Client               │
└─────────────────────┬───────────────────────────────────────┘
                      │ WebSocket
┌─────────────────────▼───────────────────────────────────────┐
│                    real-time Service                          │
│  Socket.IO → Router → WebsocketController                    │
└──────┬───────────────────────────────────────────┬────────────┘
       │                                           │
┌──────▼──────────────┐                  ┌─────────▼─────────────┐
│ document-updater    │                  │     web (API)        │
│ - OT Client         │                  │  - Project Meta       │
│ - Redis (ops)       │                  │  - Authorization      │
└─────────────────────┘                  └──────────────────────┘
```

### 5.2 OT 算法核心

**文件**: `libraries/overleaf-editor-core/`

```javascript
// 核心操作类型
const { InsertOp, RemoveOp, RetainOp } = require('overleaf-editor-core')

// TextOperation 示例
const op = new TextOperation()
op.retain(5)      // 保留前5个字符
op.insert('hello') // 插入文本
op.remove(3)      // 删除3个字符
op.retain(10)     // 保留剩余10个字符
```

### 5.3 前端 OT 集成

**文件**: `services/web/frontend/js/features/source-editor/extensions/realtime.ts`

```typescript
export class EditorFacade extends EventEmitter {
  attachShareJs(shareDoc: ShareDoc, maxDocLength?: number) {
    this.otAdapter =
      shareDoc.otType === 'history-ot'
        ? new HistoryOTAdapter(this, shareDoc, maxDocLength)
        : new ShareLatexOTAdapter(this, shareDoc, maxDocLength)
    this.otAdapter.attachShareJs()
  }

  // 处理来自 CodeMirror 的更新
  handleUpdateFromCM(transactions: readonly Transaction[]) {
    this.otAdapter.handleUpdateFromCM(transactions)
  }
}

class ShareLatexOTAdapter {
  attachShareJs() {
    const shareDoc = this.shareDoc

    // 监听远程插入
    shareDoc.on('insert', (pos: number, text: string) => {
      this.editor.cmInsert(pos, text, 'remote')
    })

    // 监听远程删除
    shareDoc.on('delete', (pos: number, text: string) => {
      this.editor.cmDelete(pos, text, 'remote')
    })
  }

  handleUpdateFromCM(transactions: readonly Transaction[]) {
    for (const transaction of transactions) {
      if (transaction.docChanged) {
        const origin = chooseOrigin(transaction)
        if (origin === 'remote') return  // 跳过远程变更

        transaction.changes.iterChanges((fromA, toA, fromB, toB, inserted) => {
          if (removed > 0) {
            shareDoc.del(pos, removedLength, fromUndo)
          }
          if (inserted > 0) {
            shareDoc.insert(pos, insertedText.toString(), fromUndo)
          }
        })
      }
    }
  }
}
```

### 5.4 History OT (追踪变更)

**文件**: `services/web/frontend/js/features/source-editor/extensions/history-ot.ts`

用于 track changes 功能：

```typescript
export const historyOT = (currentDoc: DocumentContainer) => {
  return [
    updateSender,                    // 发送更新到服务器
    trackChangesUserIdState,         // 追踪用户 ID
    shareDocState.init(() => currentDoc?.doc?._doc ?? null),
    rangesState.init(() => ({
      trackedChanges,
      comments,
      decorations: buildRangesDecorations({ trackedChanges, comments }),
    })),
    trackedChangesTheme,             // 高亮样式
  ]
}

// 追踪插入
opBuilder.trackedInsert(pos, inserted.toString(), trackingUserId, timestamp)

// 追踪删除
opBuilder.trackedDelete(start, end - start, trackingUserId, timestamp)
```

### 5.5 ShareJsDoc 类

**文件**: `services/web/frontend/js/features/ide-react/editor/share-js-doc.ts`

```typescript
export class ShareJsDoc extends EventEmitter {
  constructor(
    readonly doc_id: string,
    docLines: string[],
    version: number,
    readonly socket: Socket,
    private readonly globalEditorWatchdogManager: EditorWatchdogManager,
    private readonly eventEmitter: IdeEventEmitter,
    readonly type: OTType = 'sharejs-text-ot'
  ) {
    // 初始化 ShareJS Doc
    this._doc = new Doc(this.connection, this.doc_id, {
      type: sharejsType,
    })

    // 监听变更
    this._doc.on('change', (...args) => this.trigger('change', ...args))
    this._doc.on('acknowledge', () => {/* ... */})
    this._doc.on('remoteop', (...args) => {/* 远程操作 */})
  }

  // 连接到 CodeMirror 6
  attachToCM6(cm6: EditorFacade) {
    cm6.attachShareJs(this._doc, getMeta('ol-maxDocLength'))
  }
}
```

### 5.6 WebSocket 路由

**文件**: `services/real-time/app/js/Router.js`

```javascript
export default Router = {
  configure(app, io, sessionSockets) {
    // 客户端加入项目
    socket.on('joinProject', (client, projectId, callback) => {
      WebsocketController.joinProject(client, user, projectId, callback)
    })

    // 客户端加入文档
    socket.on('joinDoc', (client, docId, fromVersion, options, callback) => {
      WebsocketController.joinDoc(client, docId, fromVersion, options, callback)
    })

    // 应用 OT 更新
    socket.on('applyOtUpdate', (client, docId, update, callback) => {
      WebsocketController.applyOtUpdate(client, docId, update, callback)
    })
  }
}
```

## 6. 光标/选区同步

### 6.1 Cursor Updates

**文件**: `services/web/frontend/js/features/source-editor/extensions/cursor-position.ts`

```typescript
export const cursorPosition = (currentDoc: DocumentContainer) => {
  return EditorView.updateListener.of(update => {
    if (update.selectionSet || update.docChanged) {
      const { anchor, head } = update.state.selection.main
      const position = update.state.doc.lineAt(head)

      // 发送光标位置到服务器
      currentDoc.emit('cursor', {
        line: position.number,
        column: head - position.from,
      })
    }
  })
}
```

### 6.2 Cursor Highlights（远程用户光标）

**文件**: `services/web/frontend/js/features/source-editor/extensions/cursor-highlights.ts`

显示其他用户的光标位置：

```typescript
export const cursorHighlights = () => {
  return Decoration.mark({
    class: 'ol-cursor-highlight',
    attributes: {
      style: `--cursor-color: ${userColor}`,
      title: userName,
    },
  })
}
```

### 6.3 SelectionListener

**文件**: `services/web/frontend/js/features/source-editor/extensions/selection-listener.ts`

监听选区变化并通知其他客户端：

```typescript
export const selectionListener = (setEditorSelection: SetEditorSelection) => {
  return EditorView.updateListener.of(update => {
    if (update.selectionSet) {
      const selection = update.state.selection
      setEditorSelection({
        ranges: selection.ranges.map(r => ({
          from: r.from,
          to: r.to,
        })),
        primaryIndex: selection.primaryIndex,
      })
    }
  })
}
```

## 7. 关键文件路径总结

| 功能 | 路径 |
|------|------|
| 主编辑器组件 | `services/web/frontend/js/features/source-editor/components/codemirror-editor.tsx` |
| 扩展入口 | `services/web/frontend/js/features/source-editor/extensions/index.ts` |
| LaTeX 语法 | `services/web/frontend/js/features/source-editor/lezer-latex/latex.grammar` |
| 语言定义 | `services/web/frontend/js/features/source-editor/languages/latex/latex-language.ts` |
| 自动补全 | `services/web/frontend/js/features/source-editor/extensions/auto-complete.ts` |
| LaTeX 补全 | `services/web/frontend/js/features/source-editor/languages/latex/complete.ts` |
| 实时协作 | `services/web/frontend/js/features/source-editor/extensions/realtime.ts` |
| History OT | `services/web/frontend/js/features/source-editor/extensions/history-ot.ts` |
| ShareJsDoc | `services/web/frontend/js/features/ide-react/editor/share-js-doc.ts` |
| OT 核心库 | `libraries/overleaf-editor-core/` |
| WebSocket 路由 | `services/real-time/app/js/Router.js` |
| WebSocket 控制器 | `services/real-time/app/js/WebsocketController.js` |
| Document Updater | `services/document-updater/` |

## 8. 技术栈总结

- **编辑器核心**: CodeMirror 6
- **语法解析**: Lezer (PEG.js based)
- **实时通信**: Socket.IO
- **OT 算法**: ShareJS + overleaf-editor-core
- **前端框架**: React + TypeScript
- **状态管理**: CodeMirror StateField + ViewPlugin
- **会话存储**: Redis

---

## 9. CodeMirror 扩展详解

### 9.1 扩展注册机制

**文件**: `services/web/frontend/js/features/source-editor/extensions/index.ts`

Overleaf 使用模块化扩展系统，所有扩展通过 `createExtensions()` 函数组合：

```typescript
export const createExtensions = (options: Record<string, any>): Extension[] => [
  lineNumbers(),
  highlightSpecialChars(options.visual.visual),
  history({ newGroupDelay: 250 }),
  foldGutter({ openText: '▾', closedText: '▸' }),
  drawSelection(),
  EditorState.allowMultipleSelections.of(true),
  EditorView.lineWrapping,
  indentOnInput(),
]
```

扩展通过 `importOverleafModules` 宏动态加载：

```typescript
const moduleExtensions: Array<(options: Record<string, any>) => Extension> =
  importOverleafModules('sourceEditorExtensions').map(
    (item: { import: { extension: Extension } }) => item.import.extension
  )
```

### 9.2 主要扩展分类

| 扩展文件 | 功能描述 |
|---------|---------|
| `realtime.ts` | OT 实时协作核心 |
| `history-ot.ts` | Track Changes 变更追踪 |
| `auto-complete.ts` | 自动补全 UI |
| `cursor-position.ts` | 光标位置同步 |
| `cursor-highlights.ts` | 远程用户光标显示 |
| `symbol-palette.ts` | 符号面板 |
| `search.ts` | 搜索功能 |

### 9.3 StateField 与 ViewPlugin 模式

**StateField** - 用于持久状态管理：

```typescript
export const rangesState = StateField.define<{
  comments: CommentList
  trackedChanges: TrackedChangeList
  decorations: DecorationSet
}>({
  create() {
    return { trackedChanges, comments, decorations }
  },

  update(value, transaction) {
    if (transaction.docChanged) {
      const shareDoc = transaction.state.field(shareDocState)
      return buildRangesDecorations(...)
    }
    return value
  },

  provide(field) {
    return EditorView.decorations.from(field, value => value.decorations)
  },
})
```

**ViewPlugin** - 用于视图相关逻辑：

```typescript
export const realtime = ({ currentDoc }, handleError) => {
  return ViewPlugin.define(view => {
    const editor = new EditorFacade(view)
    currentDoc.attachToCM6(editor)

    return {
      update(update) {
        if (update.docChanged) {
          editor.handleUpdateFromCM(update.transactions, currentDoc.ranges)
        }
      },
      destroy() {
        currentDoc.detachFromCM6()
      },
    }
  })
}
```

### 9.4 Transaction Extender 模式

用于拦截和转换事务：

```typescript
const updateSender = EditorState.transactionExtender.of(tr => {
  if (!tr.docChanged || tr.annotation(Transaction.remote)) {
    return {}
  }

  const opBuilder = new OperationBuilder(...)
  tr.changes.iterChanges((fromA, toA, fromB, toB, inserted) => {
    // 处理插入和删除
  })

  const op = opBuilder.finish()
  const shareDoc = tr.startState.field(shareDocState)
  if (shareDoc != null) {
    shareDoc.submitOp([op])
  }

  return {}
})
```

---

## 10. 自动补全详细机制

### 10.1 补全扩展架构

**文件**: `services/web/frontend/js/features/source-editor/extensions/auto-complete.ts`

```typescript
const createAutoComplete = ({ enabled, ...rest }) => {
  if (!enabled) return []

  return [
    autocompleteTheme,
    autocompletion({
      icons: false,
      defaultKeymap: false,
      interactionDelay: 0,
      optionClass: (completion: Completion) => `ol-cm-completion-${completion.type}`,
      addToOptions: [{
        render: completion => {
          const span = document.createElement('span')
          span.classList.add('ol-cm-completionType')
          span.textContent = completion.type || ''
          return span
        },
        position: 400,
      }],
    }),
    Prec.highest(keymap.of([
      { key: 'Escape', run: closeCompletion },
      { key: 'Enter', run: acceptCompletion },
      { key: 'Tab', run: acceptCompletion },
    ])),
  ]
}
```

### 10.2 补全源分类

1. **命令补全** (`inCommandCompletionSource`)
   - 触发条件：在 `$CtrlSeq` 节点类型中

2. **环境名称补全** (`environmentNameCompletionSource`)
   - 触发条件：在 `EnvNameGroup` 节点中

3. **引用补全** (`refArgumentCompletionSource`)
   - 触发条件：在 `RefArgument` 节点中

4. **包/类补全** (`packageArgumentCompletionSource`)
   - 触发条件：在 `PackageArgument` 节点中

### 10.3 补全匹配算法

**`getCompletionMatches`** 函数处理文本匹配：

```typescript
export function getCompletionMatches(context: CompletionContext) {
  const matchBefore = context.explicit
    ? context.matchBefore(/(?:^|\\)[^\\]*(\[[^\]]*])?[^\\]*/)
    : context.matchBefore(/\\?\\[^\\]*(\[[^\]]*])?[^\\]*/)

  if (!matchBefore) return null

  // 忽略某些匹配
  if (!context.explicit) {
    if (/\\\\$/.test(matchBefore.text)) return null
    if (/[^,\s]\s+$/.test(matchBefore.text)) return null
  }

  // 多参数匹配
  const multipleArgumentMatcher =
    /^(?<before>\\(?<command>\w+)\*?(?<arguments>(\[[^\]]*?]|\{[^}]*?})+)?{)(?<existing>([^}]+\s*,\s*)+)?(?<prefix>[^}]+)?$/

  const match = matchBefore.text.match(multipleArgumentMatcher)
  return { match, matchBefore }
}
```

### 10.4 buildAllCompletions 流程

**文件**: `services/web/frontend/js/features/source-editor/languages/latex/completions/`

```typescript
export const buildAllCompletions = (completions, context) => {
  buildSnippetCompletions(completions)      // Snippet 补全
  buildEnvironmentCompletions(completions)  // 环境补全
  buildClassCompletions(completions)        // 文档类补全
  buildBibliographyStyleCompletions(completions)
  buildIncludeCompletions(completions, context)  // 文件引用
  buildReferenceCompletions(completions, context) // 引用
  buildLabelCompletions(completions, context)    // 标签
  buildPackageCompletions(completions, context) // 包
  return completions
}
```

---

## 11. Snippet 模板系统

### 11.1 Snippet 定义格式

**文件**: `services/web/frontend/js/features/languages/latex/completions/data/environments.ts`

```typescript
export const snippet = (name: string) => `\\begin{${name}}
\t$1
\\end{${name}}`

export const environments = new Map([
  ['abstract', snippet('abstract')],
  ['align', snippet('align')],
  ['equation', snippet('equation')],
  [
    'figure',
    `\\begin{figure}
\t\\centering
\t\\includegraphics[width=0.5\\linewidth]{$1}
\t\\caption{\${2:Caption}}
\t\\label{\${3:fig:placeholder}}
\\end{figure}`,
  ],
])
```

### 11.2 Snippet 占位符格式

Overleaf 使用两种占位符格式：

1. **CodeMirror 原生格式**: `${1}`, `${2}`, `#{1}`
2. **Ace 旧格式**: `$1`, `$2` (需转换)

```typescript
export const prepareSnippetTemplate = (template: string): string => {
  return template.replace(/\$(\d+)/g, '#{$1}') + '${}'
}
```

### 11.3 智能扩展机制

**extendOverUnpairedClosingBrace** - 处理未闭合括号：

```typescript
export const extendOverUnpairedClosingBrace = (state, change) => {
  if (nextChar(state.doc, change.to) === '}') {
    const unclosedBraces = countUnclosedBraces(doc, change.from, change.to)
    if (unclosedBraces < 0) {
      change.to++
    }
  }
  return change
}
```

---

## 12. 符号面板 (Symbol Palette)

### 12.1 符号面板架构

**文件**: `services/web/frontend/js/features/source-editor/extensions/symbol-palette.ts`

```typescript
export const symbolPalette = () => {
  return ViewPlugin.define(view => {
    const listener = (event: Event) => {
      const symbol = (event as CustomEvent<{ command: string }>).detail
      view.focus()

      const spec = view.state.changeByRange(range => {
        const changeSet = view.state.changes([
          { from: range.from, to: range.to, insert: symbol.command },
        ])
        return {
          range: EditorSelection.cursor(changeSet.mapPos(range.to, 1)),
          changes: changeSet,
        }
      })

      view.dispatch(spec, { scrollIntoView: true })
    }

    window.addEventListener('editor:insert-symbol', listener)

    return {
      destroy() {
        window.removeEventListener('editor:insert-symbol', listener)
      },
    }
  })
}
```

### 12.2 符号插入事件

UI 层通过自定义事件触发符号插入：

```typescript
const handleSymbolClick = (symbol: string) => {
  window.dispatchEvent(new CustomEvent('editor:insert-symbol', {
    detail: { command: symbol }
  }))
}
```

---

## 13. 历史记录 (History)

### 13.1 History 与 OT 的冲突处理

**关键问题**：CodeMirror 的本地 undo/redo 与 OT 的远程操作可能冲突。

**解决方案** - 远程操作标记：

```typescript
// services/web/frontend/js/features/source-editor/extensions/realtime.ts
const onInsert = (pos: number, text: string) => {
  editor.cmInsert(pos, text, 'remote')  // 'remote' origin
}

// cmChange 实现
cmChange(changes: ChangeSpec, origin?: string) {
  const isRemote = origin === 'remote'
  this.view.dispatch({
    changes,
    annotations: [
      Transaction.remote.of(isRemote),
      Transaction.addToHistory.of(!isRemote),  // 远程操作不加入本地历史
    ],
  })
}
```

### 13.2 OperationBuilder 类

将 CodeMirror 变更转换为 OT 操作：

```typescript
class OperationBuilder {
  private docLength: number
  private pos: number
  private op: TextOperation

  constructor(docLength: number) {
    this.docLength = docLength
    this.op = new TextOperation()
    this.pos = 0
  }

  insert(pos: number, text: string) {
    this.retainUntil(pos)
    this.op.insert(text)
  }

  delete(pos: number, length: number) {
    this.retainUntil(pos)
    this.op.remove(length)
    this.pos += length
  }

  finish() {
    this.retainUntil(this.docLength)
    return this.op
  }
}
```

---

## 14. 快捷键绑定

### 14.1 快捷键配置

**文件**: `services/web/frontend/js/features/source-editor/extensions/keymaps.ts`

```typescript
import { keymap } from '@codemirror/view'
import { defaultKeymap, historyKeymap } from '@codemirror/commands'
import { lintKeymap } from '@codemirror/lint'

export const keymaps = keymap.of([
  ...filteredDefaultKeymap,
  ...historyKeymap,
  ...lintKeymap,
])
```

### 14.2 禁用某些默认快捷键

```typescript
const ignoredDefaultKeybindings = new Set([
  'Mod-Enter',           // 用于编译
  'Alt-ArrowLeft',       // Windows/Linux 特殊行为
  'Alt-ArrowRight',
  'Mod-Alt-\\',          // 与 AltGr 输入冲突
])
```

### 14.3 第三方快捷键支持

**文件**: `services/web/frontend/js/features/source-editor/extensions/keybindings.ts`

支持 Vim 和 Emacs 模式：

```typescript
const options = [
  { name: 'default', load: async () => [] },
  {
    name: 'vim',
    load: () => import('@replit/codemirror-vim').then(m => {
      customiseVimOnce(m.Vim, m.CodeMirror)
      return m.vim()
    }),
  },
  {
    name: 'emacs',
    load: () => import('@replit/codemirror-emacs').then(m => {
      return [m.emacs()]
    }),
  },
]
```

---

## 15. 粘贴处理

### 15.1 粘贴处理架构

**文件**: `services/web/frontend/js/features/source-editor/extensions/visual/paste-html.ts`

Overleaf 使用 DOM 事件处理器拦截粘贴：

```typescript
export const pasteHtml = [
  Prec.highest(
    EditorView.domEventHandlers({
      paste(event, view) {
        const { clipboardData } = event

        if (!clipboardData.types.includes('text/html')) {
          return false
        }

        const html = clipboardData.getData('text/html').trim()
        const text = clipboardData.getData('text/plain').trim()

        try {
          const latex = convertHtmlStringToLatex(html, clipboardData.files.length)

          if (latex === null || (latex === text && clipboardData.files.length === 0)) {
            return false
          }

          view.dispatch(insertPastedContent(view, { latex, text }))
          return true
        } catch (error) {
          return false
        }
      },
    })
  ),
]
```

### 15.2 HTML 元素到 LaTeX 映射

```typescript
const selectors = [
  // 粗体
  { selector: 'b', start: () => '\\textbf{', end: () => '}' },
  { selector: 'strong', start: () => '\\textbf{', end: () => '}' },
  // 斜体
  { selector: 'i', start: () => '\\textit{', end: () => '}' },
  // 上标
  { selector: 'sup', start: () => '\\textsuperscript{', end: () => '}' },
  // 下标
  { selector: 'sub', start: () => '\\textsubscript{', end: () => '}' },
  // 链接
  { selector: 'a', start: (el) => `\\href{${protectUrlCharacters(el.href)}}{`, end: () => '}' },
  // 标题
  { selector: 'h1', start: () => '\n\n\\section{', end: () => '}\n\n' },
  { selector: 'h2', start: () => '\n\n\\subsection{', end: () => '}\n\n' },
]
```

---

## 16. 参考资料

- CodeMirror 6: https://codemirror.net/
- Lezer: https://lezer.handcrafted.email/
- ShareJS OT: https://github.com/share/ShareJS
