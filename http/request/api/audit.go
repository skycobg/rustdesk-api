package api

import (
	"encoding/json"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

type AuditConnForm struct {
	Action    string   `json:"action"`
	ConnId    int64    `json:"conn_id"`
	Id        string   `json:"id"`
	Peer      []string `json:"peer"`
	Ip        string   `json:"ip"`
	SessionId json.Number `json:"session_id"` // SKY: a u64; as float64 it lost digits and could not be matched
	Type      int      `json:"type"`
	Uuid      string   `json:"uuid"`
}

func (a *AuditConnForm) ToAuditConn() *model.AuditConn {
	fp := ""
	fn := ""
	if len(a.Peer) >= 1 {
		fp = a.Peer[0]
		if len(a.Peer) == 2 {
			fn = a.Peer[1]
		}
	}
	ssid := a.SessionId.String()
	if ssid == "" {
		ssid = "0"
	}
	return &model.AuditConn{
		Action:    a.Action,
		ConnId:    a.ConnId,
		PeerId:    a.Id,
		FromPeer:  fp,
		FromName:  fn,
		Ip:        a.Ip,
		SessionId: ssid,
		Type:      a.Type,
		Uuid:      a.Uuid,
	}
}

type AuditFileForm struct {
	Id     string `json:"id"`
	Info   string `json:"info"`
	IsFile bool   `json:"is_file"`
	Path   string `json:"path"`
	PeerId string `json:"peer_id"`
	Type   int    `json:"type"`
	Uuid   string `json:"uuid"`
}
type AuditFileInfo struct {
	Ip   string `json:"ip"`
	Name string `json:"name"`
	Num  int    `json:"num"`
}

func (a *AuditFileForm) ToAuditFile() *model.AuditFile {
	fi := &AuditFileInfo{}
	err := json.Unmarshal([]byte(a.Info), fi)
	if err != nil {
		global.Logger.Warn("ToAuditFile", err)
	}

	return &model.AuditFile{
		PeerId:   a.Id,
		Info:     a.Info,
		IsFile:   a.IsFile,
		FromPeer: a.PeerId,
		Path:     a.Path,
		Type:     a.Type,
		Uuid:     a.Uuid,
		FromName: fi.Name,
		Ip:       fi.Ip,
		Num:      fi.Num,
	}
}

// AuditNoteForm SKY: PUT /api/audit from the controlling client at the end of a session.
type AuditNoteForm struct {
	Guid string `json:"guid"`
	Note string `json:"note"`
}
