import * as monaco from 'monaco-editor'
import { parser } from './lezer-latex/latex.mjs'

export function registerLatexLsp() {
  monaco.languages.register({ id: 'latex' })

  // 1. Document Outline (Document Symbol Provider)
  monaco.languages.registerDocumentSymbolProvider('latex', {
    provideDocumentSymbols(model: monaco.editor.ITextModel) {
      const symbols: monaco.languages.DocumentSymbol[] = []
      const tree = parser.parse(model.getValue())
      const cursor = tree.cursor()

      const stack: { level: number, children: monaco.languages.DocumentSymbol[] }[] = [{ level: 0, children: symbols }]

      // Traverse the Lezer AST to build the outline
      do {
        const type = cursor.type.name
        let level = 0

        if (type === 'Part') level = 1
        else if (type === 'Chapter') level = 2
        else if (type === 'Section') level = 3
        else if (type === 'SubSection') level = 4
        else if (type === 'SubSubSection') level = 5
        else if (type === 'Paragraph') level = 6

        if (level > 0) {
          const text = model.getValueInRange({
            startLineNumber: model.getPositionAt(cursor.from).lineNumber,
            startColumn: model.getPositionAt(cursor.from).column,
            endLineNumber: model.getPositionAt(cursor.to).lineNumber,
            endColumn: model.getPositionAt(cursor.to).column,
          })

          // Extract the title from the command (e.g., \section{Title} -> Title)
          const titleMatch = text.match(/\\(part|chapter|section|subsection|subsubsection|paragraph)\*?\{(.*?)\}/)
          const name = titleMatch ? titleMatch[2] : text

          const symbol: monaco.languages.DocumentSymbol = {
            name,
            detail: type,
            kind: monaco.languages.SymbolKind.Module,
            range: {
              startLineNumber: model.getPositionAt(cursor.from).lineNumber,
              startColumn: model.getPositionAt(cursor.from).column,
              endLineNumber: model.getPositionAt(cursor.to).lineNumber,
              endColumn: model.getPositionAt(cursor.to).column,
            },
            selectionRange: {
              startLineNumber: model.getPositionAt(cursor.from).lineNumber,
              startColumn: model.getPositionAt(cursor.from).column,
              endLineNumber: model.getPositionAt(cursor.from).lineNumber,
              endColumn: model.getPositionAt(cursor.from).column + text.length,
            },
            tags: [],
            children: []
          }

          // Adjust hierarchy
          while (stack.length > 1 && stack[stack.length - 1].level >= level) {
            stack.pop()
          }
          stack[stack.length - 1].children.push(symbol)
          stack.push({ level, children: symbol.children! })
        }
      } while (cursor.next())

      return symbols
    }
  })

  // 2. Autocomplete (Completion Item Provider)
  monaco.languages.registerCompletionItemProvider('latex', {
    triggerCharacters: ['\\', '{'],
    provideCompletionItems: (model: monaco.editor.ITextModel, position: monaco.Position) => {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column
      })

      const suggestions: monaco.languages.CompletionItem[] = []
      const word = model.getWordUntilPosition(position)
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: word.endColumn
      }

      // Basic TeX Commands
      if (textUntilPosition.endsWith('\\') || word.word !== '') {
        const commands = [
          'begin', 'end', 'section', 'subsection', 'subsubsection', 
          'paragraph', 'textbf', 'textit', 'underline', 'cite', 'ref',
          'includegraphics', 'label', 'caption', 'item', 'frac', 'sum', 'int'
        ]

        commands.forEach(cmd => {
          suggestions.push({
            label: `\\${cmd}`,
            kind: monaco.languages.CompletionItemKind.Function,
            insertText: `${cmd}`,
            range: range
          })
        })
      }

      // Environment Auto-completion after \begin{
      const matchBegin = textUntilPosition.match(/\\begin\{([^}]*)$/)
      if (matchBegin) {
        const envs = ['document', 'itemize', 'enumerate', 'figure', 'table', 'equation', 'align', 'tabular', 'center']
        envs.forEach(env => {
          suggestions.push({
            label: env,
            kind: monaco.languages.CompletionItemKind.Struct,
            insertText: `${env}\}\n\t$0\n\\end{${env}`,
            insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
            range: { ...range, startColumn: position.column - matchBegin[1].length }
          })
        })
      }

      return { suggestions }
    }
  })
}