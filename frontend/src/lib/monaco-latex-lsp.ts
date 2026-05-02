import * as Monaco from 'monaco-editor'
// @ts-expect-error lezer-generator output has no type declarations
import { parser } from './lezer-latex/latex.mjs'

const LATEX_TOKENIZER: Monaco.languages.IMonarchLanguage = {
  defaultToken: '',
  tokenPostfix: '.latex',

  brackets: [
    { open: '{', close: '}', token: 'delimiter.bracket' },
    { open: '[', close: ']', token: 'delimiter.square' },
  ],

  keywords: [
    'document', 'itemize', 'enumerate', 'figure', 'table', 'equation',
    'align', 'tabular', 'center', 'abstract', 'verbatim', 'tikzpicture',
    'array', 'matrix', 'bmatrix', 'pmatrix', 'cases',
  ],

  tokenizer: {
    root: [
      { regex: '%.*$', action: { token: 'comment' } },
      { regex: '\\\\begin\\{', action: { token: 'keyword', next: '@envBody' } },
      { regex: '\\\\end\\{', action: { token: 'keyword' } },
      { regex: '\\\\(documentclass|usepackage|input|include|includeonly)\\b', action: { token: 'keyword' } },
      { regex: '\\\\(part|chapter|section|subsection|subsubsection|paragraph|subparagraph)\\*?', action: { token: 'string' } },
      { regex: '\\\\(textbf|textit|underline|emph|texttt|textrm|textsf|textsc)\\b', action: { token: 'type' } },
      { regex: '\\\\(cite|citep|citet|citeauthor|citeyear|nocite|bibliography|bibliographystyle)\\b', action: { token: 'variable' } },
      { regex: '\\\\(ref|eqref|pageref|label|autoref|cref|Cref)\\b', action: { token: 'variable' } },
      { regex: '\\\\(frac|sum|prod|int|sqrt|alpha|beta|gamma|delta|epsilon|theta|lambda|mu|pi|sigma|omega|infty|cdot|times|leq|geq|neq|approx)\\b', action: { token: 'number' } },
      { regex: '\\\\(title|author|date|maketitle|tableofcontents|newpage|clearpage|linebreak|pagebreak)\\b', action: { token: 'tag' } },
      { regex: '\\\\(centering|raggedright|raggedleft|hfill|vfill|hrule|vrule|noindent|bigskip|medskip|smallskip)\\b', action: { token: 'tag' } },
      { regex: '\\\\(item)\\b', action: { token: 'keyword' } },
      { regex: '\\\\[a-zA-Z]+', action: { token: 'identifier' } },
      { regex: '\\$\\$', action: { token: 'string', next: '@mathDisplay' } },
      { regex: '\\$', action: { token: 'string', next: '@mathInline' } },
      { regex: '[{}\\[\\]]', action: { token: 'delimiter.bracket' } },
      { regex: '\\\\[\\[\\]{}\\\\]', action: { token: 'string.escape' } },
    ],

    envBody: [
      { regex: '\\}', action: { token: 'keyword', next: '@pop' } },
      { regex: '[a-zA-Z*]+', action: { token: 'type' } },
    ],

    mathInline: [
      { regex: '\\$', action: { token: 'string', next: '@pop' } },
      { regex: '\\\\[a-zA-Z]+', action: { token: 'number' } },
      { regex: '[{}\\[\\]]', action: { token: 'delimiter.bracket' } },
      { regex: '[^$\\\\{}]+', action: { token: 'string' } },
    ],

    mathDisplay: [
      { regex: '\\$\\$', action: { token: 'string', next: '@pop' } },
      { regex: '\\\\[a-zA-Z]+', action: { token: 'number' } },
      { regex: '[{}\\[\\]]', action: { token: 'delimiter.bracket' } },
      { regex: '[^$\\\\{}]+', action: { token: 'string' } },
    ],
  },
}

export function registerLatexLsp(monaco: typeof Monaco) {
  monaco.languages.register({ id: 'latex' })
  monaco.languages.setMonarchTokensProvider('latex', LATEX_TOKENIZER)

  // 1. Document Outline (Document Symbol Provider)
  monaco.languages.registerDocumentSymbolProvider('latex', {
    provideDocumentSymbols(model: Monaco.editor.ITextModel) {
      const symbols: Monaco.languages.DocumentSymbol[] = []
      const tree = parser.parse(model.getValue())
      const cursor = tree.cursor()

      const stack: { level: number, children: Monaco.languages.DocumentSymbol[] }[] = [{ level: 0, children: symbols }]

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

          const symbol: Monaco.languages.DocumentSymbol = {
            name,
            detail: type,
            kind: Monaco.languages.SymbolKind.Module,
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
    provideCompletionItems: (model: Monaco.editor.ITextModel, position: Monaco.Position) => {
      const textUntilPosition = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column
      })

      const suggestions: Monaco.languages.CompletionItem[] = []
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
            kind: Monaco.languages.CompletionItemKind.Function,
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
            kind: Monaco.languages.CompletionItemKind.Struct,
            insertText: `${env}}\n\t$0\n\\end{${env}`,
            insertTextRules: Monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
            range: { ...range, startColumn: position.column - matchBegin[1].length }
          })
        })
      }

      return { suggestions }
    }
  })
}