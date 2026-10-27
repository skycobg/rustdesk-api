package utils

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSkyHelpPeerId(t *testing.T) {
	for _, ok := range []string{"492776585", "niki-pc", "ABC_123", "123456", "abcdefghijklmnop"} {
		if !SkyHelpValidPeerId(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "12345", "abcdefghijklmnopq", "a b c d e", "кирилица1", "id;drop", "../../x"} {
		if SkyHelpValidPeerId(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestSkyHelpPhones(t *testing.T) {
	got := SkyHelpPhones("")
	if len(got) != 2 || got[0] != "+359 882 733 447" || got[1] != "+359 2 973 3447" {
		t.Fatalf("default phones: %v", got)
	}
	got = SkyHelpPhones(" 0888 1 , ,02 2 ")
	if len(got) != 2 || got[0] != "0888 1" || got[1] != "02 2" {
		t.Fatalf("configured phones: %v", got)
	}
}

func TestSkyHelpToken(t *testing.T) {
	a, ha, err := SkyHelpToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := SkyHelpToken()
	if len(a) != 32 || a == b {
		t.Fatalf("tokens: %q %q", a, b)
	}
	if ha != SkyHelpHash(a) || ha == a || len(ha) != 64 {
		t.Fatalf("hash: %q", ha)
	}
}

func TestSkyHelpEffective(t *testing.T) {
	now := int64(1_000_000)
	cases := []struct {
		status            string
		lastSeen, expires int64
		want              string
	}{
		{SkyHelpPending, now - 10, now + 100, SkyHelpPending},
		{SkyHelpPending, now - SkyHelpSeenTimeout - 1, now + 100, SkyHelpExpired}, // the customer closed it
		{SkyHelpPending, now - 5, now - 1, SkyHelpExpired},                        // nobody accepted in time
		{SkyHelpAccepted, now - 1000, now - 1000, SkyHelpAccepted},                // accepted stays
		{SkyHelpCancelled, now, now + 100, SkyHelpCancelled},
	}
	for i, c := range cases {
		if got := SkyHelpEffective(c.status, c.lastSeen, c.expires, now); got != c.want {
			t.Errorf("case %d: got %s, want %s", i, got, c.want)
		}
	}
}

func TestSkyHelpInfo(t *testing.T) {
	raw := json.RawMessage(`{"title":"  Иван Петров ","problem":"не\nнамира запера","rows":[{"k":"Запер","v":"80431001"},{"k":"","v":""},"junk",{"k":"Бр","v":5}],"data":{"mac":"80431001"},"product":7}`)
	got, err := SkyHelpInfo(raw, "203.0.113.7")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Title   string              `json:"title"`
		Problem string              `json:"problem"`
		Rows    []map[string]string `json:"rows"`
		Data    map[string]any      `json:"data"`
		Product any                 `json:"product"`
	}
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Иван Петров" || m.Problem != "не намира запера" || m.Product != nil {
		t.Fatalf("title/problem/product: %+v", m)
	}
	// the empty row and the junk go; a row with a non-string value keeps its key; the public IP comes last
	if len(m.Rows) != 3 || m.Rows[0]["v"] != "80431001" || m.Rows[1]["k"] != "Бр" || m.Rows[1]["v"] != "" ||
		m.Rows[2]["k"] != "Публичен IP" || m.Rows[2]["v"] != "203.0.113.7" {
		t.Fatalf("rows: %v", m.Rows)
	}
	if m.Data["mac"] != "80431001" {
		t.Fatalf("data: %v", m.Data)
	}

	if got, err := SkyHelpInfo(nil, ""); err != nil || got != `{"rows":[]}` {
		t.Fatalf("empty info: %q %v", got, err)
	}
	for _, bad := range []string{`[1,2]`, `"x"`, `{bad`, `{"a":"` + strings.Repeat("x", SkyHelpMaxInfo) + `"}`} {
		if _, err := SkyHelpInfo(json.RawMessage(bad), ""); err == nil {
			t.Errorf("%.20q should be refused", bad)
		}
	}
	long := strings.Repeat("я", 700)
	got, _ = SkyHelpInfo(json.RawMessage(`{"rows":[{"k":"x","v":"`+long+`"}]}`), "")
	if strings.Contains(got, long) || !strings.Contains(got, "…") {
		t.Fatalf("a long value should be cut")
	}
}

func TestSkyHelpLimiter(t *testing.T) {
	l := NewSkyHelpLimiter(3, 10*time.Minute)
	t0 := time.Unix(1_000_000, 0)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4", t0.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("request %d should pass", i)
		}
	}
	if l.Allow("1.2.3.4", t0.Add(time.Minute)) {
		t.Fatal("the 4th within the window should be refused")
	}
	if !l.Allow("5.6.7.8", t0.Add(time.Minute)) {
		t.Fatal("another address has its own count")
	}
	if !l.Allow("1.2.3.4", t0.Add(11*time.Minute)) {
		t.Fatal("after the window it passes again")
	}
}

func TestSkyHelpAlertMail(t *testing.T) {
	info := `{"title":"Иван Петров","problem":"не върви","rows":[{"k":"Запер","v":"80431001"},{"k":"Публичен IP","v":"203.0.113.7"}]}`
	created := time.Unix(1_000_000, 0)
	subject, body := SkyHelpAlertMail(17, "zapper", info, created, created.Add(150*time.Second), "https://remote.example/_admin/")
	if !strings.Contains(subject, "Иван Петров") {
		t.Fatalf("subject: %s", subject)
	}
	for _, want := range []string{"№17", "чака от 2 мин", "Проблем: не върви", "Запер: 80431001", "Публичен IP: 203.0.113.7", "Програма: zapper", "https://remote.example/_admin/"} {
		if !strings.Contains(body, want) {
			t.Errorf("body without %q:\n%s", want, body)
		}
	}
	subject, _ = SkyHelpAlertMail(1, "", "{}", created, created, "")
	if !strings.Contains(subject, "клиент") {
		t.Fatalf("subject without a title: %s", subject)
	}
}
