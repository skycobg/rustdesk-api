package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SKY: help requests from SKY Support. The customer phones us and presses the button; SKY Support sends its ID,
// its one-time password and what it knows about the computer and the program that started it. A technician who
// is logged in accepts the request and connects without any digits or questions on the customer's side.

const (
	SkyHelpPending   = "pending"
	SkyHelpAccepted  = "accepted"
	SkyHelpConnected = "connected" // the technician got in (the customer's one-time password changed)
	SkyHelpCancelled = "cancelled"
	SkyHelpExpired   = "expired"

	// SkyHelpSeenTimeout seconds without a status call from the customer's SKY Support: it is gone.
	SkyHelpSeenTimeout = 45
	// SkyHelpShowAccepted seconds an accepted request stays on the technicians' list ("taken by ...").
	SkyHelpShowAccepted = 120
	// SkyHelpKeepPassword seconds an accepted request keeps the password for the technician to connect again.
	SkyHelpKeepPassword = 600
	// SkyHelpMaxInfo the largest info object a client may send.
	SkyHelpMaxInfo = 32 << 10
	// SkyHelpDefaultPhones the phones shown to the customer when the config has none (sky.bg, 27.10.2026).
	SkyHelpDefaultPhones = "+359 882 733 447, +359 2 973 3447"
)

var skyHelpPeerId = regexp.MustCompile(`^[A-Za-z0-9_-]{6,16}$`)

// SkyHelpValidPeerId a SKY Support ID: 6 to 16 of a-z A-Z 0-9 _ - (as our hbbs takes them).
func SkyHelpValidPeerId(id string) bool {
	return skyHelpPeerId.MatchString(id)
}

// SkyHelpPhones the phones shown to the customer: the configured comma-separated list, or ours.
func SkyHelpPhones(conf string) []string {
	if strings.TrimSpace(conf) == "" {
		conf = SkyHelpDefaultPhones
	}
	var out []string
	for _, p := range strings.Split(conf, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SkyHelpToken a new random token for the customer's SKY Support and the hash that is stored.
func SkyHelpToken() (token, hash string, err error) {
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, SkyHelpHash(token), nil
}

// SkyHelpHash what is stored for a token.
func SkyHelpHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// SkyHelpEffective the status a request has at `now`: a waiting request expires when its time is over or when
// the customer's SKY Support has not called for SkyHelpSeenTimeout seconds.
func SkyHelpEffective(status string, lastSeen, expiresAt, now int64) string {
	if status == SkyHelpPending && (now > expiresAt || now-lastSeen > SkyHelpSeenTimeout) {
		return SkyHelpExpired
	}
	return status
}

func skyCut(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// SkyHelpInfo checks the client's info object (title, problem, rows of {k, v}, data), trims it and adds the
// public IP as the last row. It returns the JSON that is stored and shown to the technicians.
func SkyHelpInfo(raw json.RawMessage, ip string) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	if len(raw) > SkyHelpMaxInfo {
		return "", errors.New("info is too big")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return "", errors.New("info is not an object")
	}
	for key, n := range map[string]int{"title": 200, "problem": 1000, "product": 32} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			m[key] = skyCut(s, n)
		} else {
			delete(m, key)
		}
	}
	rows, _ := m["rows"].([]any)
	clean := make([]any, 0, len(rows)+1)
	for _, r := range rows {
		o, ok := r.(map[string]any)
		if !ok {
			continue
		}
		k, _ := o["k"].(string)
		v, _ := o["v"].(string)
		k, v = skyCut(k, 60), skyCut(v, 500)
		if k == "" && v == "" {
			continue
		}
		if len(clean) == 60 {
			break
		}
		clean = append(clean, map[string]any{"k": k, "v": v})
	}
	if ip != "" {
		clean = append(clean, map[string]any{"k": "Публичен IP", "v": ip})
	}
	m["rows"] = clean
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SkyHelpLimiter allows at most n requests per key (the IP) within a window.
type SkyHelpLimiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	hits   map[string][]time.Time
}

func NewSkyHelpLimiter(n int, window time.Duration) *SkyHelpLimiter {
	return &SkyHelpLimiter{n: n, window: window, hits: map[string][]time.Time{}}
}

// Allow counts a request from key at now, unless key has used up its n requests in the window.
func (l *SkyHelpLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 {
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > l.window {
				delete(l.hits, k)
			}
		}
	}
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.n {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// SkyHelpAlertMail the email when nobody has accepted a request in time: subject and body (plain text).
func SkyHelpAlertMail(id uint, product, info string, created time.Time, now time.Time, panel string) (string, string) {
	var m struct {
		Title   string `json:"title"`
		Problem string `json:"problem"`
		Rows    []struct {
			K string `json:"k"`
			V string `json:"v"`
		} `json:"rows"`
	}
	_ = json.Unmarshal([]byte(info), &m)
	who := m.Title
	if who == "" {
		who = "клиент"
	}
	waited := int(now.Sub(created).Minutes())
	if waited < 1 {
		waited = 1
	}
	subject := fmt.Sprintf("SKY Support: никой не е приел заявката за помощ от %s", who)
	var b strings.Builder
	fmt.Fprintf(&b, "Заявка за помощ №%d чака от %d мин. и никой не я е приел.\n\n", id, waited)
	if m.Title != "" {
		b.WriteString(m.Title + "\n")
	}
	if m.Problem != "" {
		b.WriteString("Проблем: " + m.Problem + "\n")
	}
	for _, r := range m.Rows {
		fmt.Fprintf(&b, "%s: %s\n", r.K, r.V)
	}
	if product != "" {
		b.WriteString("Програма: " + product + "\n")
	}
	fmt.Fprintf(&b, "Изпратена: %s\n", created.Local().Format("02-01-06 15:04"))
	b.WriteString("\nПриема се от SKY Support (влезли с акаунта си) или от вътрешния SKY Zapper (Табло).\n")
	if panel != "" {
		b.WriteString(panel + "\n")
	}
	return subject, b.String()
}
