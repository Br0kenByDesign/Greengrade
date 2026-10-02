// Package mail sends transactional mail through Microsoft Graph, SMTP or the log (development).
// Recipient addresses are never logged or stored.
package mail

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

func New(c *config.Config) (Sender, error) {
	switch c.MailProvider {
	case "graph":
		return newGraph(c)
	case "smtp":
		return &smtpSender{c: c}, nil
	default:
		return logSender{}, nil
	}
}

// ---------- log (development only) ----------

type logSender struct{}

func (logSender) Send(_ context.Context, m Message) error {
	slog.Warn("MAIL_PROVIDER=log: Mail wird nicht verschickt, nur hier ausgegeben (nur für Entwicklung!)",
		"betreff", m.Subject, "text", m.Text)
	return nil
}

// ---------- Microsoft Graph (client credentials, certificate preferred) ----------

type graphSender struct {
	c      *config.Config
	key    *rsa.PrivateKey
	x5t    string
	http   *http.Client
	mu     sync.Mutex
	token  string
	expiry time.Time
}

func newGraph(c *config.Config) (*graphSender, error) {
	g := &graphSender{c: c, http: &http.Client{Timeout: 20 * time.Second}}
	if c.GraphCertFile != "" {
		certPEM, err := os.ReadFile(c.GraphCertFile)
		if err != nil {
			return nil, fmt.Errorf("GRAPH_CERT_FILE: %w", err)
		}
		keyPEM, err := os.ReadFile(c.GraphKeyFile)
		if err != nil {
			return nil, fmt.Errorf("GRAPH_KEY_FILE: %w", err)
		}
		cb, _ := pem.Decode(certPEM)
		if cb == nil {
			return nil, errors.New("GRAPH_CERT_FILE enthält kein PEM-Zertifikat")
		}
		if _, err := x509.ParseCertificate(cb.Bytes); err != nil {
			return nil, fmt.Errorf("GRAPH_CERT_FILE: %w", err)
		}
		sum := sha1.Sum(cb.Bytes) // x5t is defined as SHA-1 thumbprint of the DER certificate
		g.x5t = base64.RawURLEncoding.EncodeToString(sum[:])
		kb, _ := pem.Decode(keyPEM)
		if kb == nil {
			return nil, errors.New("GRAPH_KEY_FILE enthält keinen PEM-Schlüssel")
		}
		if k, err := x509.ParsePKCS8PrivateKey(kb.Bytes); err == nil {
			rk, ok := k.(*rsa.PrivateKey)
			if !ok {
				return nil, errors.New("GRAPH_KEY_FILE muss ein RSA-Schlüssel sein")
			}
			g.key = rk
		} else if rk, err := x509.ParsePKCS1PrivateKey(kb.Bytes); err == nil {
			g.key = rk
		} else {
			return nil, errors.New("GRAPH_KEY_FILE konnte nicht gelesen werden")
		}
	}
	return g, nil
}

func (g *graphSender) accessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token != "" && time.Until(g.expiry) > time.Minute {
		return g.token, nil
	}
	tokenURL := "https://login.microsoftonline.com/" + url.PathEscape(g.c.GraphTenantID) + "/oauth2/v2.0/token"
	form := url.Values{
		"client_id":  {g.c.GraphClientID},
		"scope":      {"https://graph.microsoft.com/.default"},
		"grant_type": {"client_credentials"},
	}
	if g.key != nil {
		now := time.Now()
		assertion := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{tokenURL},
			Issuer:    g.c.GraphClientID,
			Subject:   g.c.GraphClientID,
			ID:        strconv.FormatInt(now.UnixNano(), 36),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		})
		assertion.Header["x5t"] = g.x5t
		signed, err := assertion.SignedString(g.key)
		if err != nil {
			return "", err
		}
		form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		form.Set("client_assertion", signed)
	} else {
		form.Set("client_secret", g.c.GraphClientSecret)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := g.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error_description"`
	}
	json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out)
	if res.StatusCode != http.StatusOK || out.AccessToken == "" {
		return "", fmt.Errorf("graph token: %d %s", res.StatusCode, out.Error)
	}
	g.token = out.AccessToken
	g.expiry = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return g.token, nil
}

func (g *graphSender) Send(ctx context.Context, m Message) error {
	tok, err := g.accessToken(ctx)
	if err != nil {
		return err
	}
	body := map[string]any{
		"message": map[string]any{
			"subject":      m.Subject,
			"body":         map[string]string{"contentType": "HTML", "content": m.HTML},
			"toRecipients": []any{map[string]any{"emailAddress": map[string]string{"address": m.To}}},
		},
		// Important: no copy in "Sent Items" - otherwise the mailbox would collect every address.
		"saveToSentItems": false,
	}
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://graph.microsoft.com/v1.0/users/"+url.PathEscape(g.c.MailFrom)+"/sendMail", bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	res, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("graph sendMail: %d %s", res.StatusCode, b)
	}
	return nil
}

// ---------- SMTP (for self-hosters without Microsoft 365) ----------

type smtpSender struct{ c *config.Config }

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	addr := net.JoinHostPort(s.c.SMTPHost, strconv.Itoa(s.c.SMTPPort))
	tlsCfg := &tls.Config{ServerName: s.c.SMTPHost, MinVersion: tls.VersionTLS12}
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if s.c.SMTPPort == 465 {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	cl, err := smtp.NewClient(conn, s.c.SMTPHost)
	if err != nil {
		return err
	}
	defer cl.Close()
	if s.c.SMTPPort != 465 {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("SMTP-Server bietet kein STARTTLS an")
		}
		if err := cl.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if s.c.SMTPUser != "" {
		if err := cl.Auth(smtp.PlainAuth("", s.c.SMTPUser, s.c.SMTPPass, s.c.SMTPHost)); err != nil {
			return err
		}
	}
	if err := cl.Mail(s.c.MailFrom); err != nil {
		return err
	}
	if err := cl.Rcpt(m.To); err != nil {
		return err
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	boundary := "gg-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var b strings.Builder
	b.WriteString("From: greengrade <" + s.c.MailFrom + ">\r\n")
	b.WriteString("To: " + m.To + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
	for _, part := range [][2]string{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		b.WriteString("--" + boundary + "\r\nContent-Type: " + part[0] + "; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		enc := base64.StdEncoding.EncodeToString([]byte(part[1]))
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	if _, err := w.Write([]byte(b.String())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}
