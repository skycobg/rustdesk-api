package api

import (
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	request "github.com/lejianwen/rustdesk-api/v2/http/request/api"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
	"strconv"
	"time"
)

type Audit struct {
}

// AuditConn
// @Tags 审计
// @Summary 审计连接
// @Description 审计连接
// @Accept  json
// @Produce  json
// @Param body body request.AuditConnForm true "审计连接"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/conn [post]
func (a *Audit) AuditConn(c *gin.Context) {
	af := &request.AuditConnForm{}
	err := c.ShouldBindBodyWith(af, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	/*ttt := &gin.H{}
	c.ShouldBindBodyWith(ttt, binding.JSON)
	fmt.Println(ttt)*/
	ac := af.ToAuditConn()
	if af.Action == model.AuditActionNew {
		service.AllService.AuditService.CreateAuditConn(ac)
	} else if af.Action == model.AuditActionClose {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			ex.CloseTime = time.Now().Unix()
			service.AllService.AuditService.UpdateAuditConn(ex)
		}
	} else if af.Action == "" {
		ex := service.AllService.AuditService.InfoByPeerIdAndConnId(af.Id, af.ConnId)
		if ex.Id != 0 {
			up := &model.AuditConn{
				IdModel:   model.IdModel{Id: ex.Id},
				FromPeer:  ac.FromPeer,
				FromName:  ac.FromName,
				SessionId: ac.SessionId,
				Type:      ac.Type,
			}
			service.AllService.AuditService.UpdateAuditConn(up)
		}
	}
	// SKY: the client takes only an empty body as stored and posted every record three times otherwise.
	c.Status(http.StatusOK)
}

// AuditFile
// @Tags 审计
// @Summary 审计文件
// @Description 审计文件
// @Accept  json
// @Produce  json
// @Param body body request.AuditFileForm true "审计文件"
// @Success 200 {string} string ""
// @Failure 500 {object} response.Response
// @Router /audit/file [post]
func (a *Audit) AuditFile(c *gin.Context) {
	aff := &request.AuditFileForm{}
	err := c.ShouldBindBodyWith(aff, binding.JSON)
	if err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	//ttt := &gin.H{}
	//c.ShouldBindBodyWith(ttt, binding.JSON)
	//fmt.Println(ttt)
	af := aff.ToAuditFile()
	service.AllService.AuditService.CreateAuditFile(af)
	// SKY: the client takes only an empty body as stored and posted every record three times otherwise.
	c.Status(http.StatusOK)
}

// SKY: the RustDesk ID of the device the caller logged in from (stored with its access token).
func callerDeviceId(c *gin.Context) string {
	tk, _ := c.Get("token")
	s, _ := tk.(string)
	if s == "" {
		return ""
	}
	_, ut := service.AllService.UserService.InfoByAccessToken(s)
	if ut == nil {
		return ""
	}
	return ut.DeviceId
}

// ConnActive
// SKY: the controlling client asks for the log row of its connection, to attach a note at the end:
// GET /api/audit/conn/active?id=<controlled id>&session_id=<its session id>&conn_type=<type>. The answer is a
// JSON string: the row id, or "" while the controlled side has not reported the connection yet (the client
// retries for a few seconds). Only for the device that controls the connection (the row's from_peer).
func (a *Audit) ConnActive(c *gin.Context) {
	peerId := c.Query("id")
	sessionId := c.Query("session_id")
	connType, _ := strconv.Atoi(c.Query("conn_type"))
	dev := callerDeviceId(c)
	if peerId == "" || sessionId == "" || sessionId == "0" || dev == "" {
		c.JSON(http.StatusOK, "")
		return
	}
	ex := service.AllService.AuditService.ActiveBySession(peerId, sessionId, connType)
	if ex.Id == 0 || ex.FromPeer != dev {
		c.JSON(http.StatusOK, "")
		return
	}
	c.JSON(http.StatusOK, strconv.FormatUint(uint64(ex.Id), 10))
}

// UpdateNote
// SKY: PUT /api/audit {"guid": "<row id from ConnActive>", "note": "..."}: the technician's note at the end
// of the session, kept with the connection log together with the user who wrote it. Only from the device
// that controlled the connection, while it is open or up to an hour after it closed, and a note of another
// user is never overwritten.
func (a *Audit) UpdateNote(c *gin.Context) {
	f := &request.AuditNoteForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Error(c, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	id, err := strconv.ParseUint(f.Guid, 10, 64)
	if err != nil || id == 0 {
		response.Error(c, response.TranslateMsg(c, "ParamsError"))
		return
	}
	ex := service.AllService.AuditService.ConnInfoById(uint(id))
	u := service.AllService.UserService.CurUser(c)
	if ex.Id == 0 || u == nil {
		response.Error(c, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	dev := callerDeviceId(c)
	late := ex.CloseTime > 0 && time.Now().Unix()-ex.CloseTime > 3600
	if dev == "" || ex.FromPeer != dev || late || (ex.Note != "" && ex.NoteBy != u.Username) {
		response.Error(c, response.TranslateMsg(c, "NoAccess"))
		return
	}
	service.AllService.AuditService.UpdateAuditConn(&model.AuditConn{IdModel: model.IdModel{Id: ex.Id}, Note: f.Note, NoteBy: u.Username})
	c.Status(http.StatusOK)
}
