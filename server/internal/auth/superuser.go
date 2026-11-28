package auth

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// superuserEmails 解析 SUPERUSER_EMAILS（逗号分隔）：逐项 trim + 转全小写，
// 丢弃空项。全邮箱大小写无关是部署侧的硬要求——.env 里的大小写不该决定提权成败。
func superuserEmails(list string) []string {
	emails := make([]string, 0)
	for _, e := range strings.Split(list, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			emails = append(emails, e)
		}
	}
	return emails
}

// isSuperuserEmail 报告 email 是否命中白名单（全邮箱 trim + 小写比较）。
func isSuperuserEmail(list, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range superuserEmails(list) {
		if e == email {
			return true
		}
	}
	return false
}

// SyncSuperusers 启动时把 SUPERUSER_EMAILS 命中的既有账号幂等提权。
// 认证中间件每个请求都重读用户行，所以改完 env 重启后，已登录用户刷新
// 页面立即生效，无需退出重登。只提权不降权：移出名单不回收已授予权限。
func SyncSuperusers(ctx context.Context, db *gorm.DB, list string) (int64, error) {
	emails := superuserEmails(list)
	if len(emails) == 0 {
		return 0, nil
	}
	res := db.WithContext(ctx).Model(&store.User{}).
		Where("is_superuser = ?", false).
		Where("LOWER(email) IN ?", emails).
		Update("is_superuser", true)
	return res.RowsAffected, res.Error
}
