# PaperSage Vendor Notes

This directory keeps only the Overleaf files required to run `services/clsi` for PaperSage.
Source: `../.ref/overleaf`

PaperSage-specific integration code should stay outside this directory where possible.

## Patch: ResourceWriter.checkPath Windows 分隔符

上游 `checkPath` 要求规范化路径以 `basePath + '/'` 开头；Windows 上
`Path.normalize` 产出反斜杠，所有资源必然被判为 "outside root
directory"。补丁保持逃逸防护语义不变，接受平台分隔符（`/` 或 `\`）。
仅影响 Windows 主机直跑（`make clsi-dev` + 本机 TeX）；Linux 容器路径
行为不变。
## Patch: LocalCommandRunner 不在 Windows 上 detach

`detached: true` 在 Windows 解析为 DETACHED_PROCESS；MiKTeX 的
latexmk.exe 在该环境下 fail-fast 崩溃（exit 0xC0000409）。POSIX 上
保留原语义（进程组用于 kill）；win32 跳过 detach。
