import { loader } from '@monaco-editor/react'
import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

// Serve Monaco from the app bundle instead of @monaco-editor/react's default
// jsDelivr CDN: the desktop client must edit offline and not run remote code.
;(self as unknown as { MonacoEnvironment: { getWorker: () => Worker } }).MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
}

loader.config({ monaco })
