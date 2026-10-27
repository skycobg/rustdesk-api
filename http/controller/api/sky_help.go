package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// SkyHelp SKY: help requests from SKY Support (docs/2026-10-27-help-request.md in the SKY Support project).
// The customer's SKY Support uses Config, Create, Status and Cancel without a login; Pending and Accept are for
// logged-in technicians (SKY Support or the internal SKY Zapper). Errors are {"error": "<text for people>"}.
type SkyHelp struct {
}

var skyHelpLimiter = utils.NewSkyHelpLimiter(5, 10*time.Minute)

// skyClientIp the caller's address: behind nginx the forwarded one, and with no trusted proxy set the socket's
// (gin would otherwise believe any X-Forwarded-For).
func skyClientIp(c *gin.Context) string {
	if global.Config.Gin.TrustProxy == "" {
		return c.RemoteIP()
	}
	return c.ClientIP()
}

func skyHelpWait() time.Duration {
	if w := global.Config.App.SkyHelpWait; w > 0 {
		return w
	}
	return 10 * time.Minute
}

func skyHelpError(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

// skyHelpTechnician the logged-in user, if he may see and accept help requests (else the answer is sent).
func skyHelpTechnician(c *gin.Context) *model.User {
	u := service.AllService.UserService.CurUser(c)
	group := global.Config.App.SkyHelpGroup
	if group == "" {
		group = "SKY"
	}
	if !service.AllService.SkyHelpService.Allowed(u, group) {
		skyHelpError(c, http.StatusForbidden, "Този акаунт не приема заявки за помощ.")
		return nil
	}
	return u
}

// Config GET /api/sky/help/config: the phones the customer calls and how long a request waits.
func (h *SkyHelp) Config(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"phones":       utils.SkyHelpPhones(global.Config.App.SkyHelpPhones),
		"wait_seconds": int(skyHelpWait().Seconds()),
	})
}

type skyHelpCreateForm struct {
	PeerId   string          `json:"peer_id"`
	Uuid     string          `json:"uuid"`
	Password string          `json:"password"`
	Product  string          `json:"product"`
	Info     json.RawMessage `json:"info"`
}

// Create POST /api/sky/help: a new request from the customer's SKY Support.
func (h *SkyHelp) Create(c *gin.Context) {
	f := &skyHelpCreateForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	f.PeerId = strings.TrimSpace(f.PeerId)
	if !utils.SkyHelpValidPeerId(f.PeerId) || f.Password == "" || len(f.Password) > 64 || len(f.Uuid) > 128 {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	ip := skyClientIp(c)
	if !skyHelpLimiter.Allow(ip, time.Now()) {
		skyHelpError(c, http.StatusTooManyRequests, "Твърде много заявки. Опитайте след няколко минути или ни се обадете.")
		return
	}
	info, err := utils.SkyHelpInfo(f.Info, ip)
	if err != nil {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	token, hash, err := utils.SkyHelpToken()
	if err != nil {
		skyHelpError(c, http.StatusInternalServerError, "Сървърът не можа да приеме заявката.")
		return
	}
	product := strings.TrimSpace(f.Product)
	if len(product) > 32 {
		product = product[:32]
	}
	now := time.Now().Unix()
	r := &model.SkyHelpRequest{
		PeerId:      f.PeerId,
		Uuid:        f.Uuid,
		Password:    f.Password,
		TokenHash:   hash,
		Product:     product,
		Info:        info,
		Ip:          ip,
		Status:      utils.SkyHelpPending,
		LastSeen:    now,
		ExpiresAt:   now + int64(skyHelpWait().Seconds()),
		CreatedUnix: now,
	}
	if err := service.AllService.SkyHelpService.Create(r); err != nil {
		global.Logger.Error("SKY help: storing a request failed: ", err)
		skyHelpError(c, http.StatusInternalServerError, "Сървърът не можа да приеме заявката.")
		return
	}
	global.Logger.Info(fmt.Sprintf("SKY help: request %d from %s (%s, %s)", r.Id, r.PeerId, r.Product, ip))
	c.JSON(http.StatusOK, gin.H{"id": r.Id, "token": token, "expires_at": r.ExpiresAt})
}

type skyHelpStatusForm struct {
	Id        uint   `json:"id"`
	Token     string `json:"token"`
	Password  string `json:"password"`
	Connected bool   `json:"connected"`
}

// Status POST /api/sky/help/status: the customer's SKY Support is still waiting (with its current password)
// and asks what has happened; connected=true tells that the technician got in.
func (h *SkyHelp) Status(c *gin.Context) {
	f := &skyHelpStatusForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	r := service.AllService.SkyHelpService.ByToken(f.Id, f.Token)
	if r.Id == 0 {
		skyHelpError(c, http.StatusNotFound, "Няма такава заявка.")
		return
	}
	service.AllService.SkyHelpService.Seen(r, f.Password, f.Connected, time.Now().Unix())
	c.JSON(http.StatusOK, gin.H{"status": r.Status, "accepted_by": r.AcceptedBy, "expires_at": r.ExpiresAt})
}

// Cancel POST /api/sky/help/cancel: the customer does not want help any more.
func (h *SkyHelp) Cancel(c *gin.Context) {
	f := &skyHelpStatusForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	r := service.AllService.SkyHelpService.ByToken(f.Id, f.Token)
	if r.Id == 0 {
		skyHelpError(c, http.StatusNotFound, "Няма такава заявка.")
		return
	}
	service.AllService.SkyHelpService.Cancel(r)
	c.JSON(http.StatusOK, gin.H{"status": r.Status})
}

type skyHelpItem struct {
	Id         uint            `json:"id"`
	PeerId     string          `json:"peer_id"`
	Product    string          `json:"product"`
	Status     string          `json:"status"`
	AcceptedBy string          `json:"accepted_by"`
	AcceptedAt int64           `json:"accepted_at"`
	CreatedAt  int64           `json:"created_at"`
	ExpiresAt  int64           `json:"expires_at"`
	Ip         string          `json:"ip"`
	Info       json.RawMessage `json:"info"`
}

// Pending GET /api/sky/help/pending (logged in): the waiting requests and the recently accepted ones,
// without passwords.
func (h *SkyHelp) Pending(c *gin.Context) {
	if skyHelpTechnician(c) == nil {
		return
	}
	now := time.Now().Unix()
	list := service.AllService.SkyHelpService.Board(now)
	items := make([]skyHelpItem, 0, len(list))
	for _, r := range list {
		info := json.RawMessage(r.Info)
		if !json.Valid(info) {
			info = json.RawMessage("{}")
		}
		items = append(items, skyHelpItem{Id: r.Id, PeerId: r.PeerId, Product: r.Product, Status: r.Status,
			AcceptedBy: r.AcceptedBy, AcceptedAt: r.AcceptedAt, CreatedAt: r.CreatedUnix, ExpiresAt: r.ExpiresAt,
			Ip: r.Ip, Info: info})
	}
	c.JSON(http.StatusOK, gin.H{"now": now, "list": items})
}

// Accept POST /api/sky/help/accept {"id": 17} (logged in): the request is this technician's; the answer has
// the customer's ID and one-time password.
func (h *SkyHelp) Accept(c *gin.Context) {
	f := &struct {
		Id uint `json:"id"`
	}{}
	if err := c.ShouldBindJSON(f); err != nil || f.Id == 0 {
		skyHelpError(c, http.StatusBadRequest, "Невалидна заявка.")
		return
	}
	u := skyHelpTechnician(c)
	if u == nil {
		return
	}
	r, code, msg := service.AllService.SkyHelpService.Accept(f.Id, u, time.Now().Unix())
	if code != 0 {
		skyHelpError(c, code, msg)
		return
	}
	global.Logger.Info(fmt.Sprintf("SKY help: request %d (%s) accepted by %s", r.Id, r.PeerId, u.Username))
	c.JSON(http.StatusOK, gin.H{"id": r.Id, "peer_id": r.PeerId, "password": r.Password, "accepted_by": r.AcceptedBy})
}

// SkyHelpAlerts SKY: every 30 s, an email about each request nobody has accepted within `after` (once per request).
func SkyHelpAlerts(after time.Duration, send func(subject, body string) error) {
	panel := strings.TrimRight(global.Config.Rustdesk.ApiServer, "/")
	if panel != "" {
		panel += "/_admin/"
	}
	go func() {
		for range time.Tick(30 * time.Second) {
			now := time.Now()
			for _, r := range service.AllService.SkyHelpService.Unanswered(now.Unix(), int64(after.Seconds())) {
				service.AllService.SkyHelpService.MarkAlerted(r.Id)
				subject, body := utils.SkyHelpAlertMail(r.Id, r.Product, r.Info, time.Unix(r.CreatedUnix, 0), now, panel)
				if err := send(subject, body); err != nil {
					global.Logger.Error("SKY help: the email about request ", r.Id, " failed: ", err)
					continue
				}
				global.Logger.Info("SKY help: emailed about request ", r.Id)
			}
		}
	}()
}
