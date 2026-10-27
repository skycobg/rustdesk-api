package service

import (
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/utils"
)

// SkyHelpService SKY: help requests from SKY Support (utils/sky_help.go, http/controller/api/sky_help.go).
type SkyHelpService struct {
}

var skyHelpOpen = []string{utils.SkyHelpPending, utils.SkyHelpAccepted}

// Create stores a new request. A waiting request of the same machine (the same ID and uuid) is cancelled: the
// customer pressed again. Someone who only knows the ID cannot cancel the customer's request this way.
func (s *SkyHelpService) Create(r *model.SkyHelpRequest) error {
	if r.Uuid != "" {
		DB.Model(&model.SkyHelpRequest{}).Where("peer_id = ? and uuid = ? and status in ?", r.PeerId, r.Uuid, skyHelpOpen).
			Updates(map[string]any{"status": utils.SkyHelpCancelled, "password": ""})
	}
	return DB.Create(r).Error
}

// Allowed whether user u may see and accept help requests: an admin, or a member of the group named `group`
// (app.sky-help-group, default "SKY"). Accounts are made by the admin, but a password must never reach anyone else.
func (s *SkyHelpService) Allowed(u *model.User, group string) bool {
	if u == nil || u.Id == 0 {
		return false
	}
	if u.IsAdmin != nil && *u.IsAdmin {
		return true
	}
	if group == "" || u.GroupId == 0 {
		return false
	}
	g := &model.Group{}
	DB.Where("id = ?", u.GroupId).First(g)
	return g.Id != 0 && g.Name == group
}

// ByToken the request of the customer's SKY Support that holds `token` (empty Id when there is none).
func (s *SkyHelpService) ByToken(id uint, token string) *model.SkyHelpRequest {
	r := &model.SkyHelpRequest{}
	if id == 0 || token == "" {
		return r
	}
	DB.Where("id = ? and token_hash = ?", id, utils.SkyHelpHash(token)).First(r)
	return r
}

// Seen the customer's SKY Support is still there, with its current one-time password, and tells whether the
// technician is in. r.Status is set to the status after the call.
func (s *SkyHelpService) Seen(r *model.SkyHelpRequest, password string, connected bool, now int64) {
	st := utils.SkyHelpEffective(r.Status, r.LastSeen, r.ExpiresAt, now)
	up := map[string]any{}
	// The one-time password changes as soon as it has let someone in: a new password after the request was
	// accepted means the technician got in, even if the customer's SKY Support has not noticed it yet.
	rotated := st == utils.SkyHelpAccepted && password != "" && r.Password != "" && password != r.Password
	switch {
	case st == utils.SkyHelpAccepted && (connected || rotated):
		st = utils.SkyHelpConnected
		up["status"], up["password"] = st, ""
	case st == utils.SkyHelpPending || st == utils.SkyHelpAccepted:
		up["last_seen"] = now
		if password != "" && len(password) <= 64 {
			up["password"] = password
		}
	case st != r.Status:
		up["status"], up["password"] = st, ""
	}
	r.Status = st
	if len(up) > 0 {
		DB.Model(&model.SkyHelpRequest{}).Where("id = ?", r.Id).Updates(up)
	}
}

// Cancel the customer does not want help any more.
func (s *SkyHelpService) Cancel(r *model.SkyHelpRequest) {
	DB.Model(&model.SkyHelpRequest{}).Where("id = ? and status in ?", r.Id, skyHelpOpen).
		Updates(map[string]any{"status": utils.SkyHelpCancelled, "password": ""})
	r.Status = utils.SkyHelpCancelled
}

// expire marks the waiting requests nobody can accept any more and forgets passwords that are not needed.
func (s *SkyHelpService) expire(now int64) {
	DB.Model(&model.SkyHelpRequest{}).
		Where("status = ? and (expires_at < ? or last_seen < ?)", utils.SkyHelpPending, now, now-utils.SkyHelpSeenTimeout).
		Updates(map[string]any{"status": utils.SkyHelpExpired, "password": ""})
	DB.Model(&model.SkyHelpRequest{}).
		Where("status = ? and accepted_at < ? and password <> ''", utils.SkyHelpAccepted, now-utils.SkyHelpKeepPassword).
		Update("password", "")
}

// Board the technicians' list: the waiting requests and the ones accepted in the last SkyHelpShowAccepted s.
func (s *SkyHelpService) Board(now int64) []*model.SkyHelpRequest {
	s.expire(now)
	var list []*model.SkyHelpRequest
	DB.Where("status = ? or (status in ? and accepted_at >= ?)", utils.SkyHelpPending,
		[]string{utils.SkyHelpAccepted, utils.SkyHelpConnected}, now-utils.SkyHelpShowAccepted).
		Order("id desc").Limit(50).Find(&list)
	return list
}

// SkyHelpName how a technician is shown: the nickname, or the username.
func SkyHelpName(u *model.User) string {
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Username
}

// Accept gives request `id` to user u if nobody has taken it. The same user may accept it again (to connect
// again) while its password is kept. On refusal it returns an HTTP status and the reason for the technician.
func (s *SkyHelpService) Accept(id uint, u *model.User, now int64) (*model.SkyHelpRequest, int, string) {
	s.expire(now)
	res := DB.Model(&model.SkyHelpRequest{}).Where("id = ? and status = ?", id, utils.SkyHelpPending).
		Updates(map[string]any{"status": utils.SkyHelpAccepted, "accepted_by": SkyHelpName(u),
			"accepted_by_id": u.Id, "accepted_at": now})
	r := &model.SkyHelpRequest{}
	DB.Where("id = ?", id).First(r)
	if r.Id == 0 {
		return nil, 404, "Няма такава заявка."
	}
	if res.RowsAffected == 1 {
		return r, 0, ""
	}
	mine := r.AcceptedById == u.Id
	switch r.Status {
	case utils.SkyHelpAccepted:
		if !mine {
			return nil, 409, "Вече я пое " + r.AcceptedBy + "."
		}
		if r.Password == "" {
			return nil, 410, "Паролата вече не важи. Помолете клиента да натисне бутона пак."
		}
		return r, 0, ""
	case utils.SkyHelpConnected:
		if !mine {
			return nil, 409, "Вече я пое " + r.AcceptedBy + "."
		}
		return nil, 410, "Вече сте влезли при клиента. За нова връзка клиентът да натисне бутона пак."
	case utils.SkyHelpCancelled:
		return nil, 410, "Клиентът отказа заявката."
	default:
		return nil, 410, "Заявката изтече: клиентът вече не чака."
	}
}

// Unanswered the waiting requests older than `after` seconds with no email about them yet.
func (s *SkyHelpService) Unanswered(now, after int64) []*model.SkyHelpRequest {
	s.expire(now)
	var list []*model.SkyHelpRequest
	DB.Where("status = ? and alerted = ? and created_unix <= ?", utils.SkyHelpPending, false, now-after).Find(&list)
	return list
}

// MarkAlerted an email about request id has gone out (or failed: it is not repeated).
func (s *SkyHelpService) MarkAlerted(id uint) {
	DB.Model(&model.SkyHelpRequest{}).Where("id = ?", id).Update("alerted", true)
}
