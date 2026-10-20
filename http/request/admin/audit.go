package admin

type AuditQuery struct {
	PeerId   string `form:"peer_id"`
	FromPeer string `form:"from_peer"`
	FromName string `form:"from_name"` // SKY
	From     string `form:"from"`      // SKY: YYYY-MM-DD, inclusive (server time zone)
	To       string `form:"to"`        // SKY: YYYY-MM-DD, inclusive
	PageQuery
}

type AuditConnLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
type AuditFileLogIds struct {
	Ids []uint `json:"ids" validate:"required"`
}
