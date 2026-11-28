# AI 调用计量与健康度看板

一次真实模型调用一行，落在 `ai_model_calls`（只追加、按保留期硬删）。管理端看板与个人用量都读这张表，`agent_runs` 不再维护另一套 token/耗时账。

## 采集点

唯一埋点是 `server/internal/agent/usage.go` 的 ChatModel 装饰器，在 `ExecuteRun` 构建模型后立即包装：

- `Generate`：记录总耗时；`usage` 取 `ResponseMeta.Usage`。
- `Stream`：逐块转发，记录首个「有内容或工具调用」chunk 的首字延迟；`usage` 取末帧 `ResponseMeta.Usage`；消费者提前关闭按 `cancelled` 记录；先落库再向消费者发 EOF。
- 主 agent、子 agent、上下文压缩/摘要共用同一实例，全部计入同一 Run。

归属（`user_id` / `project_id` / `run_id` / `feature_kind`）来自 `runContext`，不接受客户端上报。

## feature_kind 政策

`feature_kind` 是**发起 Run 的产品入口**，由入口显式传值、服务端白名单校验（`server/internal/agent/runtime.go`）：

| 值 | 入口 |
|---|---|
| `chat` | AI 助手对话（默认） |
| `cross_chapter_review` | 跨章节检查（检查页固定提示词 / review-runs 引擎） |

- **禁止**按用户输入做关键词/正则分类——违反项目「严禁伪智能」红线。
- 新增一键大纲/润色/报错诊断入口时，在同一个 PR 注册枚举值，否则看板不会出现对应分类。
- 历史 Run 重试复制原 Run 的 `feature_kind`。

## 隐私边界

不存 prompt/回复正文、API Key、base_url、请求头、原始错误消息。`provider`/`model` 是用户可影响文本，入库前去控制字符并截断。`error_code` 只存枚举：

`auth` / `rate_limit` / `bad_request` / `provider_5xx` / `network` / `stream_interrupted` / `unknown`。

provider 不返回 usage 时 `prompt_tokens`/`completion_tokens` 存 **NULL**，看板显示「—」并按 `usage_missing_calls` 计数，绝不补 0。

## 接口

| 方法 | 路径 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/ai-usage?from&to` | 登录 | 个人用量，服务端强制 `user_id = 当前用户` |
| GET | `/api/v1/admin/ai-usage?from&to&model&feature_kind` | superuser | 全站汇总 |
| GET | `/api/v1/admin/ai-usage/export?format=csv\|json&from&to` | superuser | 日 × provider × model × feature 聚合，不含个人维度 |

- 窗口默认：管理端 7 天、个人 30 天；上限 90 天；`from`/`to` 支持 RFC3339 与 `YYYY-MM-DD`（`to` 含当天）。
- 响应统一为 `totals` + `by_day` + `by_model` + `by_feature` + `by_error`。
- 指标：调用数、成功/失败/超时/取消、成功率、输入/输出 Token、`usage_missing_calls`、P50/P95 延迟（仅 success）、首字 P50（仅 success 且流式）。分位数为精确 rank（`ceil(p·n)`），不是估算直方图。
- 管理端不提供按用户下钻：个人维度消耗仅本人可见。

## 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `AI_USAGE_RETENTION_DAYS` | 180 | clamp 7–730；每小时惰性清理 |
| `AI_USAGE_TIMEZONE` | `Asia/Shanghai` | `created_day` 按此时区计算；非法回退 UTC 并告警 |

## 排查

```sql
-- 最近 24 小时按状态/错误码
SELECT status, error_code, COUNT(*), AVG(latency_ms)
FROM ai_model_calls
WHERE created_at >= now() - interval '24 hours'
GROUP BY status, error_code ORDER BY 3 DESC;

-- 某天按模型
SELECT model, COUNT(*), SUM(prompt_tokens), SUM(completion_tokens)
FROM ai_model_calls WHERE created_day = '2026-11-28'
GROUP BY model;
```

## 范围外

嵌入调用（`rag.HTTPEmbedder`）当前不解析 usage，未计入；费用估算缺价格数据源，不做；多实例指标导出（OTel）后续按需接入。
