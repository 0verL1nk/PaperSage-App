/**
 * 查询键的单一来源。
 *
 * 单独成文件而不是放在 hooks 里：AI 助手面板在应用建议后要失效这些查询，
 * 若从 `@/hooks/use-latex` 导入会把 pdfjs / jszip 一并拉进不相关的组件与测试
 * （jsdom 下直接以 DOMMatrix 未定义崩掉）。
 */
export const latexKeys = {
  files: (projectId: string) => ['latex-files', projectId] as const,
  content: (projectId: string, filePath: string) =>
    ['latex-file-content', projectId, filePath] as const,
}
