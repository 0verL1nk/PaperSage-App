# E2E Fixtures — Generation Recipe

`frontend/tests/e2e/fixtures/` 里的样例文件由 `scripts/gen_e2e_fixtures.py` 生成,用于:
- 真实 multipart 上传测试(`uploading/document-upload.spec.ts`)
- 任何需要"能成功打开"的样例文件的 spec

## 当前文件

| 文件 | 用途 | 大小 |
|---|---|---|
| `sample.pdf` | PDF 上传 → preview 走 PDF 渲染路径 | ~590 字节 |
| `sample.docx` | DOCX 上传 → preview 走 docx-preview 路径 | ~920 字节 |
| `main.tex` | LaTeX 上传 → writing 页可立刻编辑 | ~180 字节 |

## 重新生成

```bash
python3 scripts/gen_e2e_fixtures.py
```

(脚本是自包含的,只用标准库,无新增依赖。)

## 为什么不直接 commit 大文件

这些文件 < 1KB 就能满足"合法可解析"的需求。生产用真实 PDF 通常是 100KB+,会显著拖慢 `git clone` 和 CI 缓存命中。新增 fixture 时:
- 优先考虑能否用现成的 `sample.*` 满足
- 不行的话再用 `setInputFiles` 配合 `{name, mimeType, buffer}` 注入内存文件
- 真的需要大文件(>10KB)时单独讨论是否值得

## 扩展

如果需要更多类型(`.png`、`.cls`、`.bib`、…),直接在脚本里加一个生成函数 + 写入文件。保持每个文件 < 5KB。
