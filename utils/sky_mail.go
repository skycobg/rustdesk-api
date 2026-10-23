package utils

import (
	"crypto/tls"
	"encoding/base64"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// MailServer SKY: the SMTP server for the alert emails: STARTTLS on 587 or TLS on 465, as the SKY Zapper's crash
// reports and the SKY SmartHome alerts send theirs (smtp.gmail.com with an App Password).
type MailServer struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string // empty: User
}

// SendMail sends a plain-text UTF-8 email and gives up after a minute.
func SendMail(s MailServer, to []string, subject, body string) error {
	from := s.From
	if from == "" {
		from = s.User
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	var conn net.Conn
	var err error
	if s.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: s.Host})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(time.Minute)); err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if s.Port != 465 {
		if err = c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return err
		}
	}
	if s.User != "" {
		if err = c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, t := range to {
		if err = c.Rcpt(t); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(mailMessage(from, to, subject, body, time.Now())); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// mailMessage: the subject encoded for the header (RFC 2047, so nothing in it can start another header line),
// the text in base64.
func mailMessage(from string, to []string, subject, body string, date time.Time) []byte {
	var b strings.Builder
	b.WriteString("From: " + (&mail.Address{Name: "SKY Support", Address: from}).String() + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.BEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + date.Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return []byte(b.String())
}
