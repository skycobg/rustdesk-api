package utils

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// SKY: an email when wrong passwords pile up, in place of a ban by IP (the office goes out through one IP, so a
// ban would lock everyone there out). All wrong passwords count together: every user and IP, in the program
// (/api/login) and in the web panel (/api/admin/login). The threshold-th of them within the window sends an
// email that lists them. The next email waits for the cooldown and lists everything that came meanwhile, so an
// attack of any size costs one email per cooldown and none of it goes unreported.

// FailedLogin is one wrong password.
type FailedLogin struct {
	At       time.Time
	Username string
	Known    bool // such a user exists
	Ip       string
	Where    string // the program, the web client or the web panel
	Device   string // what the program or the browser says about itself; may be empty
}

// LoginAlert collects wrong passwords and hands the emails to send, in a goroutine of its own.
type LoginAlert struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	cooldown  time.Duration
	server    string // shown in the email, e.g. https://remote.sky.bg
	send      func(subject, body string)
	pending   []FailedLogin // not in an email yet, oldest first
	dropped   int           // pending ones beyond loginAlertKeep: counted, not kept
	lastSent  time.Time
	followUp  bool // a follow-up email is due at the end of the cooldown
}

const (
	loginAlertKeep = 500 // the most wrong passwords kept for one email (an attack can make millions)
	loginAlertList = 25  // the email lists the newest ones, up to this many
	loginAlertTop  = 10  // and counts them per user and per IP, the top this many
)

func NewLoginAlert(threshold int, window, cooldown time.Duration, server string, send func(subject, body string)) *LoginAlert {
	threshold = max(1, min(threshold, loginAlertKeep))
	return &LoginAlert{threshold: threshold, window: window, cooldown: cooldown, server: server, send: send}
}

// Failed records a wrong password. It never waits for the email.
func (a *LoginAlert) Failed(f FailedLogin) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending) == loginAlertKeep {
		a.pending = a.pending[1:]
		a.dropped++
	}
	a.pending = append(a.pending, f)
	switch {
	case a.followUp:
		// it goes into the follow-up
	case !a.lastSent.IsZero() && f.At.Sub(a.lastSent) < a.cooldown:
		// soon after an email: everything until the end of the cooldown goes into one follow-up
		a.followUp = true
		time.AfterFunc(a.lastSent.Add(a.cooldown).Sub(f.At), a.flush)
	default:
		cut := f.At.Add(-a.window)
		i := 0
		for i < len(a.pending) && a.pending[i].At.Before(cut) {
			i++
		}
		a.pending = a.pending[i:]
		if len(a.pending) >= a.threshold {
			a.mail(false, f.At)
		}
	}
}

func (a *LoginAlert) flush() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.followUp = false
	if len(a.pending) > 0 {
		a.mail(true, time.Now())
	}
}

func (a *LoginAlert) mail(followUp bool, now time.Time) {
	subject, body := a.message(followUp)
	a.pending, a.dropped, a.lastSent = nil, 0, now
	go a.send(subject, body)
}

func (a *LoginAlert) message(followUp bool) (subject, body string) {
	total := len(a.pending) + a.dropped
	users := tally(a.pending, func(f FailedLogin) string { return show(f.Username, 32) })
	ips := tally(a.pending, func(f FailedLogin) string { return show(f.Ip, 45) })
	unknown := map[string]bool{}
	for _, f := range a.pending {
		if !f.Known {
			unknown[show(f.Username, 32)] = true
		}
	}

	var names []string
	for i, u := range users {
		if i == 3 {
			names = append(names, "…")
			break
		}
		names = append(names, u.key)
	}
	what := fmt.Sprintf("%d грешни опита за вход", total)
	if followUp {
		what = "още " + what
	}
	subject = "SKY Support: " + what + " (" + strings.Join(names, ", ") + ")"

	var b strings.Builder
	where := "в SKY Support"
	if a.server != "" {
		where += " (" + a.server + ")"
	}
	first, last := a.pending[0].At, a.pending[len(a.pending)-1].At
	span := "от " + first.Format("02.01.2006 15:04:05") + " до "
	if first.Format("02.01.2006") == last.Format("02.01.2006") {
		span += last.Format("15:04:05")
	} else {
		span += last.Format("02.01.2006 15:04:05")
	}
	if followUp {
		fmt.Fprintf(&b, "Още %d грешни опита за вход %s след предишното писмо, %s.\n\n", total, where, span)
	} else {
		fmt.Fprintf(&b, "%d грешни опита за вход %s %s.\n\n", total, where, span)
	}
	b.WriteString("По потребител: " + counts(users, unknown) + "\n")
	b.WriteString("По IP: " + counts(ips, nil) + "\n")
	if a.dropped > 0 {
		fmt.Fprintf(&b, "(броят по потребител и IP е за последните %d)\n", len(a.pending))
	}
	b.WriteString("\n")

	shown := a.pending[max(0, len(a.pending)-loginAlertList):]
	for _, f := range shown {
		line := f.At.Format("02.01.2006 15:04:05") + "  " + show(f.Username, 32) + "  " + show(f.Ip, 45) + "  " + show(f.Where, 20)
		if f.Device != "" {
			line += " (" + show(f.Device, 160) + ")"
		}
		b.WriteString(line + "\n")
	}
	if total > len(shown) {
		fmt.Fprintf(&b, "(показани са последните %d)\n", len(shown))
	}

	fmt.Fprintf(&b, "\nПисмо има при %d грешни опита за %s; следващото е най-рано след %s и изброява всичко дотогава.\n",
		a.threshold, durText(a.window), durText(a.cooldown))
	if a.server != "" {
		b.WriteString("Ако опитите не са от наш човек, сменете паролата на потребителя в панела: " + a.server + "/_admin/\n")
	}
	return subject, b.String()
}

type tallied struct {
	key string
	n   int
}

// tally counts the keys, the most frequent first (equal ones in the order they came).
func tally(list []FailedLogin, key func(FailedLogin) string) []tallied {
	at := map[string]int{}
	var out []tallied
	for _, f := range list {
		k := key(f)
		if i, ok := at[k]; ok {
			out[i].n++
			continue
		}
		at[k] = len(out)
		out = append(out, tallied{k, 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	return out
}

func counts(list []tallied, unknown map[string]bool) string {
	var parts []string
	for i, c := range list {
		if i == loginAlertTop {
			parts = append(parts, fmt.Sprintf("и още %d", len(list)-i))
			break
		}
		p := fmt.Sprintf("%s %d", c.key, c.n)
		if unknown[c.key] {
			p += " (няма такъв)"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

// show keeps what a user or a bot sent printable and short: it goes into the email and its subject.
func show(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return "(празно)"
	}
	if len(r) > limit {
		r = append(r[:limit:limit], '…')
	}
	for i, c := range r {
		if unicode.IsControl(c) {
			r[i] = '?'
		}
	}
	return string(r)
}

func durText(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%d ч", int(d/time.Hour))
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%d мин", int(d/time.Minute))
	}
	return d.String()
}
