/* Keep the worker URL beside the pinned Emscripten runtime so sibling-relative
 * importScripts() and locateFile() resolve to this immutable asset directory. */
importScripts(`/latex-engines/papersage-swift-worker.js${self.location.search}`)
