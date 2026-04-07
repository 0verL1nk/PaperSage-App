## ADDED Requirements

### Requirement: Redis 连接
应用 SHALL 提供 Redis 连接工具。

#### Scenario: Redis 连接配置
- **WHEN** 配置 REDIS_URL 环境变量
- **THEN** 应用使用该连接串连接 Redis

### Requirement: 缓存操作
应用 SHALL 提供基本的缓存 get/set 操作。

#### Scenario: 设置缓存
- **WHEN** 调用 cache.set(key, value, ttl)
- **THEN** 数据写入 Redis，TTL 生效

#### Scenario: 获取缓存
- **WHEN** 调用 cache.get(key)
- **THEN** 返回缓存值，不存在返回 None

### Requirement: 健康检查
Redis 连接 SHALL 支持健康检查。

#### Scenario: 连接测试
- **WHEN** 调用 redis.ping()
- **THEN** 返回 True 表示连接正常