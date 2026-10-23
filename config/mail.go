package config

// Mail SKY: the SMTP server for the alert emails (587: STARTTLS, 465: TLS). Its password lives only in the
// server's config.yaml.
type Mail struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"` // empty: user
}
