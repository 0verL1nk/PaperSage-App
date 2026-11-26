// Package netguard 约束服务端发往「用户可配置地址」的出站连接。
//
// 模型 provider 的 base_url 由用户填写，连接检查与模型调用都会请求它；
// 不加限制时，任何登录用户都能借服务端探测内网、回环与云元数据端点（SSRF）。
// 这里做两层校验：入参先校验 scheme/host（ValidateBaseURL），
// 真正拨号时再按解析出的 IP 拒绝非公网目标（Control）。后者才是权威控制，
// 因为它看到的是即将连接的真实地址，DNS 结果变化也绕不过去。
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// AllowPrivateEnv 显式放开「本地模型」开关。设为 true/1 时允许回环与私网
// 目标（ollama、LM Studio 等本机 provider）。默认关闭：生产环境的 base_url
// 由普通用户填写，不能默认允许访问服务端所在网络。
const AllowPrivateEnv = "AGENT_ALLOW_PRIVATE_PROVIDER_URLS"

// ErrInvalidBaseURL 标记「用户填写的 base URL 不可用」，调用方应回 4xx 而不是 5xx。
var ErrInvalidBaseURL = errors.New("invalid base_url")

// ValidateBaseURL 校验用户填写的 base URL，返回去掉结尾斜杠的规范形式。
// allowPrivate 为 false 时，字面量 IP 不能是回环/私网/链路本地/云元数据地址。
// 域名在这里不解析：解析结果由拨号期的 Control 检查，避免校验与连接之间的
// DNS 变化（TOCTOU）绕开限制。
func ValidateBaseURL(raw string, allowPrivate bool) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: must be a valid http(s) URL", ErrInvalidBaseURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: must be a valid http(s) URL", ErrInvalidBaseURL)
	}
	if u.User != nil {
		return "", fmt.Errorf("%w: must not contain credentials", ErrInvalidBaseURL)
	}
	if !allowPrivate {
		if ip := net.ParseIP(u.Hostname()); ip != nil && isBlockedIP(ip) {
			return "", fmt.Errorf("%w: must not point to a private or loopback address", ErrInvalidBaseURL)
		}
	}
	return base, nil
}

// Control 返回 net.Dialer.Control：在建立连接前检查目标 IP。
// allowPrivate 为 false 时拒绝内网、回环、链路本地（含 169.254.169.254
// 云元数据）、CGNAT 与组播地址。
func Control(allowPrivate bool) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return errors.New("provider host did not resolve to an IP")
		}
		if isBlockedIP(ip) {
			return fmt.Errorf("provider host %s is not a public address", ip)
		}
		return nil
	}
}

func isBlockedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10：CGNAT，云上常用于内部服务。
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}
