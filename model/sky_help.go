package model

// SkyHelpRequest SKY: a customer's request for remote help (POST /api/sky/help), sent by SKY Support after the
// customer has phoned us and pressed the button. Technicians who are logged in see it and one of them accepts
// it; only that technician ever gets the customer's one-time password, and it is wiped as soon as it is not
// needed any more. Times are unix seconds.
type SkyHelpRequest struct {
	IdModel
	PeerId       string `json:"peer_id" gorm:"default:'';not null;index"`
	Uuid         string `json:"-" gorm:"default:'';not null;"`
	Password     string `json:"-" gorm:"default:'';not null;"`
	TokenHash    string `json:"-" gorm:"default:'';not null;index"`
	Product      string `json:"product" gorm:"default:'';not null;"`
	Info         string `json:"-" gorm:"type:text"`
	Ip           string `json:"ip" gorm:"default:'';not null;"`
	Status       string `json:"status" gorm:"default:'';not null;index"`
	AcceptedBy   string `json:"accepted_by" gorm:"default:'';not null;"`
	AcceptedById uint   `json:"-" gorm:"default:0;not null;"`
	AcceptedAt   int64  `json:"accepted_at" gorm:"default:0;not null;"`
	LastSeen     int64  `json:"-" gorm:"default:0;not null;"`
	ExpiresAt    int64  `json:"expires_at" gorm:"default:0;not null;"`
	CreatedUnix  int64  `json:"created_unix" gorm:"default:0;not null;index"`
	Alerted      bool   `json:"-" gorm:"default:0;not null;"`
	TimeModel
}
