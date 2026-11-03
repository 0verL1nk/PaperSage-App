## Why

稿件当前原位覆盖R2，元数据更新失败可能仍返回成功；正文抽取默认48KB，旧证据只有摘录哈希而非完整来源版本。现有代次、确认建议和权限检查需要接成真正的版本/失败/失效闭环，才能可靠承载后续科研产物。

## What Changes

- 完整抽取获支持资料，区分完整/不完整/不支持，提供绑定版本、UTF-8安全边界的可续读接口；资源安全限制保留为明确错误而非伪完整内容。
- 引入项目来源身份、不可变来源版本、抽取结果及稳定定位；复用现有Document/LatexFile与索引代次，兼容旧证据但不伪造历史页码。
- 稿件新对象先准备、主库CAS切换当前版本；保存/恢复/建议应用使用基准修订，同事务更新建议状态，失败不污染旧稿或报假成功。
- 来源变更真实驱动索引新代次构建/校验/原子激活；主库是权限和活动代次权威，Neon/R2不能假设同事务。
- 删除/撤权/排除立即约束检索、生成、建议应用和派生结果；历史展示区分失效与不可读取。
- 编译绑定实际输入manifest和保存/草稿状态，过期产物不得冒充最新稿件结果。
- **BREAKING**：版本化写入激活后，已有文件更新/恢复要求基准修订；缺失前置条件返回428，冲突409。先升级调用方并停写旧API，禁止保留无条件写入后门。

## Capabilities

### New Capabilities
- `complete-source-processing`: 完整正文、不可变来源/抽取身份、续读/定位、活动索引代次与来源失效。

### Modified Capabilities
- `latex-source-storage`: 版本化存储、条件写入、历史/恢复、失败语义与旧客户端切换。
- `agent-evidence-provenance`: 来源修订绑定、真实定位、引用有效性、历史兼容与再授权。
- `agent-evidence-citations`: 可定位的依据、版本/失效/权限状态，不暴露内部标识。
- `browser-latex-compilation`: 增加输入快照/草稿标识及过期结果判定。

## Impact

- `server/internal/{store,storage,documents,extract,rag,latex,agent,bootstrap}`、SQL truth/PG/MySQL migrations及相应测试；复用现有`AgentRun`、`AgentChangeProposal`、`RagIndexGeneration`。
- 前端文件API/自动保存/导入/建议/编译/预览等所有写入与原文回查调用方同时更新。
- 现有云库、向量库、R2，无新增必需服务；基线回填独立于SQL迁移，不能伪造已丢失历史。
- 前置为 `production-release-readiness`；后续组织产物/审阅引用本变更的数据身份。
