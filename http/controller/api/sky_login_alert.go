package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// SkyLoginFailed SKY: tells the login alert (global.LoginAlert, nil when it is off) about a wrong password.
// where: the program, the web client or the web panel; device: what it says about itself (may be empty).
func SkyLoginFailed(c *gin.Context, username, where, device string) {
	if global.LoginAlert == nil {
		return
	}
	known := service.AllService.UserService.InfoByUsername(username).Id > 0
	ip := c.ClientIP()
	if global.Config.Gin.TrustProxy == "" {
		ip = c.RemoteIP() // with no trusted proxy set, gin believes any X-Forwarded-For: the socket's address then
	}
	global.LoginAlert.Failed(utils.FailedLogin{At: time.Now(), Username: username, Known: known, Ip: ip,
		Where: where, Device: device})
}

// skyDevice SKY: what the program says about the device it logs in from.
func skyDevice(f *api.LoginForm) string {
	var parts []string
	if f.Id != "" {
		parts = append(parts, "ID "+f.Id)
	}
	for _, s := range []string{f.DeviceInfo.Os, f.DeviceInfo.Name} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}
