package utils

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"
)

type sentMail struct{ subject, body string }

func newTestAlert(window, cooldown time.Duration) (*LoginAlert, chan sentMail) {
	ch := make(chan sentMail, 10)
	a := NewLoginAlert(5, window, cooldown, "https://remote.example", func(s, b string) { ch <- sentMail{s, b} })
	return a, ch
}

func wrong(user string, at time.Time) FailedLogin {
	return FailedLogin{At: at, Username: user, Known: user != "root", Ip: "203.0.113.7", Where: "програмата", Device: "ID 123456789, windows, PC"}
}

func expectNoMail(t *testing.T, ch chan sentMail) {
	t.Helper()
	select {
	case m := <-ch:
		t.Fatalf("unexpected email: %s", m.subject)
	case <-time.After(80 * time.Millisecond):
	}
}

func expectMail(t *testing.T, ch chan sentMail) sentMail {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no email")
	}
	return sentMail{}
}

func TestSkyLoginAlertThreshold(t *testing.T) {
	a, ch := newTestAlert(24*time.Hour, time.Hour)
	now := time.Now()
	for i := 0; i < 4; i++ {
		a.Failed(wrong("vesko", now))
	}
	expectNoMail(t, ch)
	a.Failed(wrong("root", now))
	m := expectMail(t, ch)
	if m.subject != "SKY Support: 5 грешни опита за вход (vesko, root)" {
		t.Fatal(m.subject)
	}
	for _, want := range []string{"5 грешни опита за вход в SKY Support (https://remote.example)", "По потребител: vesko 4, root 1 (няма такъв)",
		"По IP: 203.0.113.7 5", "  vesko  203.0.113.7  програмата (ID 123456789, windows, PC)", "Писмо има при 5 грешни опита за 24 ч",
		"най-рано след 1 ч", "https://remote.example/_admin/"} {
		if !strings.Contains(m.body, want) {
			t.Fatalf("no %q in\n%s", want, m.body)
		}
	}
	a.Failed(wrong("vesko", now)) // within the hour after the email: no new one now
	expectNoMail(t, ch)
}

func TestSkyLoginAlertWindow(t *testing.T) {
	a, ch := newTestAlert(time.Hour, time.Hour)
	now := time.Now()
	for i := 0; i < 4; i++ {
		a.Failed(wrong("vesko", now.Add(-2*time.Hour)))
	}
	a.Failed(wrong("vesko", now)) // the four are out of the window by now
	expectNoMail(t, ch)
	for i := 0; i < 4; i++ {
		a.Failed(wrong("vesko", now))
	}
	if m := expectMail(t, ch); !strings.HasPrefix(m.subject, "SKY Support: 5 грешни") {
		t.Fatal(m.subject)
	}
}

func TestSkyLoginAlertFollowUp(t *testing.T) {
	a, ch := newTestAlert(24*time.Hour, 300*time.Millisecond)
	for i := 0; i < 5; i++ {
		a.Failed(wrong("vesko", time.Now()))
	}
	expectMail(t, ch)
	for i := 0; i < 30; i++ {
		a.Failed(wrong("admin", time.Now())) // during the cooldown: no email now...
	}
	expectNoMail(t, ch)
	m := expectMail(t, ch) // ...but one at its end, with all thirty
	if m.subject != "SKY Support: още 30 грешни опита за вход (admin)" {
		t.Fatal(m.subject)
	}
	if !strings.Contains(m.body, "Още 30 грешни опита за вход в SKY Support (https://remote.example) след предишното писмо") ||
		!strings.Contains(m.body, "(показани са последните 25)") || strings.Count(m.body, "  admin  ") != 25 {
		t.Fatal(m.body)
	}
	time.Sleep(400 * time.Millisecond) // a quiet cooldown: a new email needs the threshold again
	for i := 0; i < 4; i++ {
		a.Failed(wrong("vesko", time.Now()))
	}
	expectNoMail(t, ch)
}

func TestSkyLoginAlertKeepsTheCount(t *testing.T) {
	a, ch := newTestAlert(24*time.Hour, 200*time.Millisecond)
	for i := 0; i < 5; i++ {
		a.Failed(wrong("vesko", time.Now()))
	}
	expectMail(t, ch)
	for i := 0; i < loginAlertKeep+20; i++ {
		a.Failed(wrong("bot", time.Now()))
	}
	m := expectMail(t, ch)
	if !strings.Contains(m.subject, "още 520 грешни") || !strings.Contains(m.body, "(броят по потребител и IP е за последните 500)") {
		t.Fatal(m.subject, m.body)
	}
}

func TestSkyShow(t *testing.T) {
	if got := show(" ad\r\nmin ", 32); got != "ad??min" {
		t.Fatalf("%q", got)
	}
	if got := show(strings.Repeat("я", 40), 32); got != strings.Repeat("я", 32)+"…" {
		t.Fatalf("%q", got)
	}
	if got := show("  ", 32); got != "(празно)" {
		t.Fatalf("%q", got)
	}
}

func TestSkyMailMessage(t *testing.T) {
	subject := "SKY Support: 5 грешни опита за вход (vesko)\r\nBcc: someone@example.com"
	body := "Ред 1\nРед 2: " + strings.Repeat("ж", 100)
	raw := mailMessage("info@example.com", []string{"niki@example.com"}, subject, body, time.Now())
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Get("Bcc") != "" {
		t.Fatal("a header came in through the subject")
	}
	if got, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); err != nil || got != subject {
		t.Fatalf("%q %v", got, err)
	}
	if got := m.Header.Get("From"); got != `"SKY Support" <info@example.com>` {
		t.Fatal(got)
	}
	got, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, m.Body))
	if err != nil || string(got) != body {
		t.Fatalf("%q %v", got, err)
	}
}
