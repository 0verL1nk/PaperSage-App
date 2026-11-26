import { compileLatexInBrowser, type BrowserLatexProgress } from '@/lib/latex/swiftlatex-browser-compiler'

/**
 * Warmup compiles a real document instead of downloading a hardcoded resource
 * list: package names and Kpathsea format numbers must match the served
 * profile exactly, and a template change must not silently invalidate them.
 */
export interface WarmupSample {
  id: 'xelatex' | 'pdflatex'
  name: string
  compiler: 'xelatex' | 'pdflatex'
  source: string
}

const XELATEX_SAMPLE = String.raw`\documentclass[UTF8,a4paper]{ctexart}
\usepackage{amsmath,amssymb}
\usepackage{graphicx}
\usepackage{geometry}
\usepackage{booktabs}
\usepackage{longtable}
\usepackage{multirow}
\usepackage{array}
\usepackage{caption}
\usepackage{xcolor}
\usepackage{hyperref}
\begin{document}
\section{中文排版预热}
这是用于预热浏览器缓存的示例段落，包含中文标点：“测试”、（括号）与数字 2026。
\begin{table}[htbp]
\centering
\begin{tabular}{lcr}
\toprule
项目 & 数量 & 比例 \\
\midrule
示例 & 1 & 100\% \\
\bottomrule
\end{tabular}
\caption{预热表格}
\end{table}
\begin{equation}
E = mc^2
\end{equation}
\end{document}
`

const PDFLATEX_SAMPLE = String.raw`\documentclass[UTF8,a4paper]{article}
\usepackage{amsmath,amssymb}
\usepackage{graphicx}
\usepackage{geometry}
\usepackage{booktabs}
\usepackage{longtable}
\usepackage{multirow}
\usepackage{array}
\usepackage{caption}
\usepackage{xcolor}
\usepackage{hyperref}
\begin{document}
\section{Warmup}
This sample warms the common English paper packages in the browser cache.
\begin{table}[htbp]
\centering
\begin{tabular}{lcr}
\toprule
Item & Count & Share \\
\midrule
Sample & 1 & 100\% \\
\bottomrule
\end{tabular}
\caption{Warmup table}
\end{table}
\begin{equation}
E = mc^2
\end{equation}
\end{document}
`

export const WARMUP_SAMPLES: readonly WarmupSample[] = [
  { id: 'xelatex', name: '中文论文环境（XeLaTeX）', compiler: 'xelatex', source: XELATEX_SAMPLE },
  { id: 'pdflatex', name: '英文论文环境（pdfLaTeX）', compiler: 'pdflatex', source: PDFLATEX_SAMPLE },
]

export async function warmupBrowserLatex(
  sample: WarmupSample,
  options: { signal?: AbortSignal; onProgress?: (progress: BrowserLatexProgress) => void } = {},
): Promise<void> {
  await compileLatexInBrowser(
    [{ path: 'main.tex', content: sample.source }],
    'main.tex',
    sample.compiler,
    options,
  )
}
