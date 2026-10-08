// Package config reads all settings from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type OAuthProvider struct {
	ClientID     string
	ClientSecret string
}

type Config struct {
	ListenAddr     string
	AppOrigin      string // e.g. https://app.example.com
	RPID           string // e.g. example.com - never change after launch
	DatabaseURL    string
	DataDir        string
	AppSecret      []byte // signs JWTs etc. Rotating it logs everyone out.
	EmailPepper    []byte // HMAC key for e-mail hashes. Never change it.
	AdminEmails    []string
	ModerationMail string
	TrustedProxies []*net.IPNet
	Secure         bool // cookies with Secure flag / HSTS
	PrivacyURL     string
	ImprintURL     string
	TermsURL       string

	MailProvider string // graph | smtp | log
	MailFrom     string

	GraphTenantID     string
	GraphClientID     string
	GraphClientSecret string
	GraphCertFile     string
	GraphKeyFile      string

	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string

	OAuth map[string]OAuthProvider
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	if f := strings.TrimSpace(os.Getenv(k + "_FILE")); f != "" {
		if b, err := os.ReadFile(f); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return def
}

func secret(k string) ([]byte, error) {
	raw := env(k, "")
	if raw == "" {
		return nil, fmt.Errorf("%s fehlt (erzeugen mit: openssl rand -base64 32)", k)
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s ist kein gültiges Base64: %w", k, err)
	}
	if len(b) < 32 {
		return nil, fmt.Errorf("%s muss mindestens 32 Byte lang sein", k)
	}
	return b, nil
}

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:     env("LISTEN_ADDR", ":8080"),
		AppOrigin:      strings.TrimRight(env("APP_ORIGIN", ""), "/"),
		RPID:           env("RP_ID", ""),
		DatabaseURL:    env("DATABASE_URL", ""),
		DataDir:        env("DATA_DIR", "/data"),
		ModerationMail: env("MODERATION_EMAIL", ""),
		PrivacyURL:     env("PRIVACY_URL", ""),
		ImprintURL:     env("IMPRINT_URL", ""),
		TermsURL:       env("TERMS_URL", ""),
		MailProvider:   env("MAIL_PROVIDER", "log"),
		MailFrom:       env("MAIL_FROM", ""),

		GraphTenantID:     env("GRAPH_TENANT_ID", ""),
		GraphClientID:     env("GRAPH_CLIENT_ID", ""),
		GraphClientSecret: env("GRAPH_CLIENT_SECRET", ""),
		GraphCertFile:     env("GRAPH_CERT_FILE", ""),
		GraphKeyFile:      env("GRAPH_KEY_FILE", ""),

		SMTPHost: env("SMTP_HOST", ""),
		SMTPUser: env("SMTP_USER", ""),
		SMTPPass: env("SMTP_PASSWORD", ""),
		OAuth:    map[string]OAuthProvider{},
	}
	var err error
	if c.AppSecret, err = secret("APP_SECRET"); err != nil {
		return nil, err
	}
	if c.EmailPepper, err = secret("EMAIL_PEPPER"); err != nil {
		return nil, err
	}
	if c.SMTPPort, err = strconv.Atoi(env("SMTP_PORT", "587")); err != nil {
		return nil, errors.New("SMTP_PORT ist keine Zahl")
	}
	u, err := url.Parse(c.AppOrigin)
	if err != nil || u.Host == "" || u.Path != "" {
		return nil, errors.New("APP_ORIGIN muss eine URL ohne Pfad sein, z. B. https://app.example.com")
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" {
		return nil, errors.New("APP_ORIGIN muss https verwenden (nur localhost darf http sein)")
	}
	c.Secure = u.Scheme == "https" || u.Hostname() == "localhost"
	if c.RPID == "" {
		c.RPID = u.Hostname()
	}
	if host := u.Hostname(); host != c.RPID && !strings.HasSuffix(host, "."+c.RPID) {
		return nil, errors.New("RP_ID muss die Domain von APP_ORIGIN oder eine übergeordnete Domain sein")
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL fehlt")
	}
	for _, e := range strings.Split(env("ADMIN_EMAILS", ""), ",") {
		if e = strings.TrimSpace(e); e != "" {
			c.AdminEmails = append(c.AdminEmails, e)
		}
	}
	for _, cidr := range strings.Split(env("TRUSTED_PROXIES", "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1/32,::1/128"), ",") {
		if cidr = strings.TrimSpace(cidr); cidr == "" {
			continue
		}
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
		}
		c.TrustedProxies = append(c.TrustedProxies, n)
	}
	for _, p := range []string{"google", "github", "discord"} {
		up := strings.ToUpper(p)
		id, sec := env("OAUTH_"+up+"_CLIENT_ID", ""), env("OAUTH_"+up+"_CLIENT_SECRET", "")
		if id != "" && sec != "" {
			c.OAuth[p] = OAuthProvider{ClientID: id, ClientSecret: sec}
		}
	}
	switch c.MailProvider {
	case "graph":
		if c.GraphTenantID == "" || c.GraphClientID == "" || c.MailFrom == "" ||
			(c.GraphClientSecret == "" && (c.GraphCertFile == "" || c.GraphKeyFile == "")) {
			return nil, errors.New("MAIL_PROVIDER=graph braucht GRAPH_TENANT_ID, GRAPH_CLIENT_ID, MAIL_FROM und entweder GRAPH_CERT_FILE+GRAPH_KEY_FILE oder GRAPH_CLIENT_SECRET")
		}
	case "smtp":
		if c.SMTPHost == "" || c.MailFrom == "" {
			return nil, errors.New("MAIL_PROVIDER=smtp braucht SMTP_HOST und MAIL_FROM")
		}
	case "log":
	default:
		return nil, errors.New("MAIL_PROVIDER muss graph, smtp oder log sein")
	}
	return c, nil
}
