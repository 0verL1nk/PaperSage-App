# 参与贡献

感谢你愿意改进 PaperSage。本文说明本地环境、提交规范和 PR 要求；项目结构与领域约定见 [AGENTS.md](AGENTS.md)，安全问题请按 [SECURITY.md](SECURITY.md) 私密报告，不要开公开 issue。

## 本地环境

需要 Node.js、pnpm 与 Go（版本以 `server/go.mod` 为准）。

```bash
make install   # 安装前端与 LaTeX 服务依赖
make dev       # 启动本地服务
make check     # 类型检查与静态检查
make test      # Go 单元测试 + 前端 Vitest
```

前端与后端也可以分别运行：

```bash
cd frontend && pnpm dev
cd server && go run ./cmd/papersage
```

后端配置以 `server/internal/config/` 为准。AI、数据库、对象存储与 LaTeX 编译等能力可能需要额外服务或环境变量；**不要把开发默认值当作生产配置，也不要把任何真实凭据提交进仓库。**

## 提交规范

提交信息使用 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>: <描述>

<可选正文>
```

常用 `type`：`feat`、`fix`、`refactor`、`docs`、`test`、`chore`、`perf`、`ci`。

- 一个提交只做一件事，便于回滚与评审。
- 不提交构建产物、`node_modules/`、本地数据库或导出的 PDF。
- 新增依赖前先确认是否已有同类实现；确实需要时在 PR 里说明原因。

## PR 要求

- 标题同样使用 Conventional Commits 格式（CI 会校验）。
- 描述里写清：**改了什么、为什么、怎么验证、还有什么没覆盖**。
- 修 bug 请附复现步骤；改界面请附截图或录屏。
- 提交前本地跑过相关检查（`make check`、`make test`，或对应子集），并在 PR 里贴命令与结果。
- 不要在同一 PR 里混入无关重构。

## 代码约定

- 沿用仓库既有分层与命名，不引入新框架。
- 业务逻辑要有测试；测试文件与被测代码同目录（`*.test.ts` / `*_test.go`）。
- 面向用户的文案用使用者能懂的话，不出现内部术语、堆栈或裸 ID。
- 破坏性操作要有确认或可撤销；会丢数据的路径必须显式提示。

## 报告问题

普通 bug 与功能建议请用 issue 模板。安全漏洞请走 [SECURITY.md](SECURITY.md) 的私密渠道。
