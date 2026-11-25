import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, LoaderCircle, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSetPasswordMutation } from '@/hooks/auth-hooks'
import { getCurrentUser } from '@/services/auth-api'
import { passwordFormSchema, type PasswordFormValues } from '@/types/auth'

const fieldLabelClass = 'text-[12.5px] font-medium text-foreground/80'

/**
 * 账号与安全：验证码自动注册的账号（hasPassword=false）在此免旧密码设置
 * 初始密码；已设密码的账号需校验旧密码后修改。
 */
export function AccountSettings() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useQuery({
    queryKey: ['auth-me'],
    queryFn: getCurrentUser,
    staleTime: 30_000,
  })
  const setPasswordMutation = useSetPasswordMutation()
  const [saveError, setSaveError] = useState('')

  // 旧后端不返回 hasPassword 时按「已有密码」处理（要求旧密码），避免降级绕过。
  const hasPassword = currentUser.data?.hasPassword !== false
  const form = useForm<PasswordFormValues>({
    resolver: zodResolver(passwordFormSchema()),
    defaultValues: { currentPassword: '', newPassword: '', confirmPassword: '' },
  })

  const onSubmit = form.handleSubmit(async (values) => {
    setSaveError('')
    if (hasPassword && !values.currentPassword) {
      form.setError('currentPassword', { message: t('settings.currentPasswordRequired') })
      return
    }
    try {
      await setPasswordMutation.mutateAsync({
        currentPassword: hasPassword ? values.currentPassword : undefined,
        newPassword: values.newPassword,
      })
      toast.success(t('settings.passwordSaved'), {
        description: t('settings.passwordSavedDesc'),
      })
      form.reset()
      await queryClient.invalidateQueries({ queryKey: ['auth-me'] })
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : t('settings.passwordSaveFailed'))
    }
  })

  if (currentUser.isPending) {
    return (
      <Card>
        <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <LoaderCircle className="size-4 animate-spin" />
          {t('settings.accountLoading')}
        </CardContent>
      </Card>
    )
  }

  if (currentUser.isError || !currentUser.data) {
    return (
      <Card>
        <CardContent className="space-y-3 py-6">
          <Alert variant="destructive" className="rounded-lg py-2.5">
            <ShieldCheck className="size-4" />
            <AlertDescription className="text-[12px]">{t('settings.accountLoadFailed')}</AlertDescription>
          </Alert>
          <Button variant="outline" className="h-11" onClick={() => void currentUser.refetch()}>
            {t('settings.accountRetry')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('settings.accountTitle')}</CardTitle>
          <CardDescription>{t('settings.accountDesc')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">{t('settings.accountUsername')}</span>
            <span className="font-medium">{currentUser.data.username}</span>
          </div>
          <div className="flex items-center justify-between gap-4">
            <span className="text-muted-foreground">{t('settings.accountEmail')}</span>
            <span className="font-medium">
              {currentUser.data.email ?? t('settings.accountNoEmail')}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            {hasPassword ? t('settings.changePasswordTitle') : t('settings.setPasswordTitle')}
          </CardTitle>
          <CardDescription>
            {hasPassword ? t('settings.changePasswordDesc') : t('settings.setPasswordDesc')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {saveError && (
            <Alert variant="destructive" className="mb-3 rounded-lg py-2.5">
              <ShieldCheck className="size-4" />
              <AlertDescription className="text-[12px]">{saveError}</AlertDescription>
            </Alert>
          )}
          <form onSubmit={onSubmit} className="space-y-3.5">
            {hasPassword && (
              <div className="space-y-1.5">
                <Label htmlFor="current-password" className={fieldLabelClass}>
                  {t('settings.currentPassword')}
                </Label>
                <Input
                  id="current-password"
                  type="password"
                  autoComplete="current-password"
                  className="h-11"
                  {...form.register('currentPassword')}
                />
                {form.formState.errors.currentPassword && (
                  <p className="ml-1 text-[11.5px] text-destructive">
                    {form.formState.errors.currentPassword.message}
                  </p>
                )}
              </div>
            )}
            <div className="space-y-1.5">
              <Label htmlFor="new-password" className={fieldLabelClass}>
                {t('settings.newPassword')}
              </Label>
              <Input
                id="new-password"
                type="password"
                autoComplete="new-password"
                placeholder={t('settings.passwordPlaceholder')}
                className="h-11"
                {...form.register('newPassword')}
              />
              {form.formState.errors.newPassword && (
                <p className="ml-1 text-[11.5px] text-destructive">
                  {form.formState.errors.newPassword.message}
                </p>
              )}
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="confirm-new-password" className={fieldLabelClass}>
                {t('settings.confirmNewPassword')}
              </Label>
              <Input
                id="confirm-new-password"
                type="password"
                autoComplete="new-password"
                className="h-11"
                {...form.register('confirmPassword')}
              />
              {form.formState.errors.confirmPassword && (
                <p className="ml-1 text-[11.5px] text-destructive">
                  {form.formState.errors.confirmPassword.message}
                </p>
              )}
            </div>
            <Button
              type="submit"
              disabled={setPasswordMutation.isPending}
              className="h-11 gap-2 rounded-[10px] px-5 text-[13px] font-semibold"
            >
              {setPasswordMutation.isPending
                ? <LoaderCircle className="size-4 animate-spin" />
                : <KeyRound className="size-4" />}
              {hasPassword ? t('settings.changePasswordAction') : t('settings.setPasswordAction')}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
