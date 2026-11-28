import { apiClient } from './client'
import { SYSTEM } from './api-routes'

/** 检索能力：semantic = 有嵌入模型的语义检索；keyword = 未配置模型，按关键词匹配。 */
export type RetrievalMode = 'semantic' | 'keyword'

interface HealthResponse {
  /** 旧后端不返回该字段；新增取值时同步这里的收窄，否则界面按“未知”处理。 */
  retrievalMode?: string
}

/**
 * 读取后端公布的检索模式。
 *
 * /health 是裸 JSON（不带 {code,data,info} 信封），因此不走 getJson；
 * 读不到或值不认识时返回 null——这是说明性信息，不能因此让页面报错。
 */
export async function getRetrievalMode(): Promise<RetrievalMode | null> {
  const response = await apiClient.get<HealthResponse>(SYSTEM.health, { timeout: 10_000 })
  const mode = response.data?.retrievalMode
  return mode === 'semantic' || mode === 'keyword' ? mode : null
}
