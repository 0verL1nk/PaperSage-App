import { useQuery } from '@tanstack/react-query'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { fetchBackendBuildInfo, frontendBuildInfo } from '@/lib/build-info'

export function VersionInfo() {
  const { data: backend, isError } = useQuery({
    queryKey: ['backend-build-info'],
    queryFn: fetchBackendBuildInfo,
    retry: false,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>版本信息</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 text-sm">
        <section aria-label="前端构建信息" className="space-y-1">
          <p className="font-medium">前端</p>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-muted-foreground">
            <dt>版本</dt>
            <dd className="break-all">{frontendBuildInfo.version}</dd>
            <dt>提交</dt>
            <dd className="break-all">{frontendBuildInfo.commit}</dd>
            <dt>构建时间</dt>
            <dd className="break-all">{frontendBuildInfo.buildTime}</dd>
          </dl>
        </section>

        <section aria-label="后端构建信息" className="space-y-1">
          <p className="font-medium">后端</p>
          {isError ? (
            <p role="status" className="text-muted-foreground">后端版本暂不可用</p>
          ) : backend ? (
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-muted-foreground">
              <dt>状态</dt>
              <dd>{backend.status}</dd>
              <dt>版本</dt>
              <dd className="break-all">{backend.version}</dd>
              <dt>提交</dt>
              <dd className="break-all">{backend.commit}</dd>
              <dt>构建时间</dt>
              <dd className="break-all">{backend.buildTime}</dd>
              <dt>Go</dt>
              <dd className="break-all">{backend.goVersion}</dd>
            </dl>
          ) : (
            <p className="text-muted-foreground">正在读取后端版本…</p>
          )}
        </section>
      </CardContent>
    </Card>
  )
}
