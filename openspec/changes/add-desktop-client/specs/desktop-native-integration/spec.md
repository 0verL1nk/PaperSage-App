## ADDED Requirements

### Requirement: 原生文件对话框
客户端 SHALL 使用操作系统原生对话框完成文件选择、文件夹选择和保存路径选择。选择的路径 SHALL 只用于用户明确发起的操作，不得在后台自动读取其他目录。

#### Scenario: 选择导入文件夹
- **WHEN** 用户触发“导入文件夹”
- **THEN** 打开系统文件夹选择器，用户取消时不产生任何上传或状态变更

#### Scenario: 选择导出位置
- **WHEN** 用户触发“导出项目”
- **THEN** 打开系统保存对话框，默认文件名为项目名，扩展名与导出格式一致

### Requirement: 系统文件拖放
客户端 SHALL 接受从操作系统拖入的文件和文件夹，且只在明确标记的放置区域内生效。拖入的文件 SHALL 复用现有上传流程与类型校验；不支持的类型 SHALL 给出明确提示而不是静默忽略。

#### Scenario: 拖入支持的文件
- **WHEN** 用户把 PDF 拖到参考资料页的放置区域
- **THEN** 文件进入现有上传流程，显示进度与结果

#### Scenario: 拖入不支持的文件
- **WHEN** 用户拖入不支持的文件类型
- **THEN** 应用提示不支持的类型，不发起上传

#### Scenario: 放置区域外释放
- **WHEN** 用户在未标记为放置区域的界面释放文件
- **THEN** 应用不处理该文件，也不会导航离开当前页面

### Requirement: 桌面通知
客户端 SHALL 在窗口未聚焦时，为编译完成、编译失败、Agent Run 完成和 Agent Run 失败发送系统通知。同一任务的结果 SHALL 只通知一次；窗口处于前台时 SHALL 不发送重复通知。

#### Scenario: 后台编译完成
- **WHEN** 编译任务在窗口失焦期间完成
- **THEN** 系统通知显示项目名与“编译完成”，点击通知可回到对应项目

#### Scenario: 前台不重复打扰
- **WHEN** 编译任务完成时窗口处于前台
- **THEN** 不发送系统通知，仅在界面内展示结果

#### Scenario: 同一任务只通知一次
- **WHEN** 编译状态轮询多次读到同一个终态
- **THEN** 系统通知只出现一次

### Requirement: 深链
客户端 SHALL 支持 `papersage://project/<projectId>` 和 `papersage://project/<projectId>/<stage>` 形式的深链，其中 `<stage>` 仅接受 `prepare`、`writing`、`review`。非法协议、缺失 ID 或未知阶段 SHALL 被忽略并记录原因，不得导航到任意 URL。

#### Scenario: 打开项目深链
- **WHEN** 用户在系统里点击 `papersage://project/abc/writing`
- **THEN** 客户端聚焦已有实例并导航到项目 `abc` 的写作页

#### Scenario: 非法深链
- **WHEN** 收到 `papersage://project/../../etc` 或未知阶段
- **THEN** 客户端拒绝导航，停留在当前页面并记录拒绝原因

### Requirement: 外部链接
客户端 SHALL 将页面中的外部链接交给系统默认浏览器打开，不得在应用 WebView 内直接导航离开应用。

#### Scenario: 打开帮助链接
- **WHEN** 用户点击帮助菜单中的文档链接
- **THEN** 系统默认浏览器打开该链接，应用窗口保持当前页面

#### Scenario: 非安全协议
- **WHEN** 页面尝试打开 `file://`、`javascript:` 或自定义非白名单协议
- **THEN** 客户端拒绝打开并提示不支持的链接类型

### Requirement: 原生能力的安全边界
所有原生能力 SHALL 通过显式暴露的 Go 服务调用。WebView SHALL NOT 获得任意文件系统、命令执行或任意 URL 打开能力。单实例传递的数据和拖入的文件路径 SHALL 按不可信输入校验。

#### Scenario: 外部 URL 白名单
- **WHEN** 前端请求打开一个 `https://` 链接
- **THEN** 使用系统浏览器打开；其他协议返回错误

#### Scenario: 任意命令执行不可用
- **WHEN** 页面尝试通过原生桥调用命令执行或读取任意路径
- **THEN** 该能力不存在，调用失败
