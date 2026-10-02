// Package oauth implements the authorization code flow (with PKCE) for social logins.
// Only the provider's stable user id is requested - no e-mail, no profile.
package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/config"
)

type Provider struct {
	Name, Label            string
	AuthURL, TokenURL, API string
	Scope                  string
	IDField                string
	ClientID, Secret       string
}

var defs = map[string]Provider{
	"google": {Name: "google", Label: "Google",
		AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
		API: "https://openidconnect.googleapis.com/v1/userinfo", Scope: "openid", IDField: "sub"},
	"github": {Name: "github", Label: "GitHub",
		AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token",
		API: "https://api.github.com/user", Scope: "", IDField: "id"},
	"discord": {Name: "discord", Label: "Discord",
		AuthURL: "https://discord.com/oauth2/authorize", TokenURL: "https://discord.com/api/oauth2/token",
		API: "https://discord.com/api/users/@me", Scope: "identify", IDField: "id"},
}

type Registry struct {
	providers map[string]Provider
	http      *http.Client
}

func NewRegistry(c *config.Config) *Registry {
	r := &Registry{providers: map[string]Provider{}, http: &http.Client{Timeout: 15 * time.Second}}
	for name, creds := range c.OAuth {
		p := defs[name]
		p.ClientID, p.Secret = creds.ClientID, creds.ClientSecret
		r.providers[name] = p
	}
	return r
}

func (r *Registry) Get(name string) (Provider, bool) { p, ok := r.providers[name]; return p, ok }

func (r *Registry) Enabled() []map[string]string {
	out := []map[string]string{}
	for _, n := range []string{"google", "github", "discord"} {
		if p, ok := r.providers[n]; ok {
			out = append(out, map[string]string{"id": p.Name, "label": p.Label})
		}
	}
	return out
}

func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (p Provider) AuthCodeURL(state, verifier, redirect string) string {
	q := url.Values{
		"client_id":             {p.ClientID},
		"redirect_uri":          {redirect},
		"response_type":         {"code"},
		"state":                 {state},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	if p.Scope != "" {
		q.Set("scope", p.Scope)
	}
	if p.Name == "google" {
		q.Set("prompt", "select_account")
	}
	return p.AuthURL + "?" + q.Encode()
}

// Subject exchanges the code and returns the provider's stable user id.
func (r *Registry) Subject(ctx context.Context, p Provider, code, verifier, redirect string) (string, error) {
	form := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.Secret},
		"code":          {code},
		"redirect_uri":  {redirect},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := r.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("%s token: status %d %s", p.Name, res.StatusCode, tok.Error)
	}
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, p.API, nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "greengrade")
	res2, err := r.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s userinfo: status %d", p.Name, res2.StatusCode)
	}
	var info map[string]any
	dec := json.NewDecoder(io.LimitReader(res2.Body, 1<<20))
	dec.UseNumber()
	if err := dec.Decode(&info); err != nil {
		return "", err
	}
	switch v := info[p.IDField].(type) {
	case string:
		if v != "" {
			return v, nil
		}
	case json.Number:
		if _, err := strconv.ParseInt(v.String(), 10, 64); err == nil {
			return v.String(), nil
		}
	}
	return "", errors.New(p.Name + ": keine Nutzer-ID erhalten")
}
