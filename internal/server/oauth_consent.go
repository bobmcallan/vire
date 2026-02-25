package server

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
)

// oauthConsentData holds the template data for the consent page.
type oauthConsentData struct {
	ClientName          string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ResponseType        string
	Error               string
	DenyURL             string // pre-built, properly URL-encoded deny redirect
}

var consentTemplate = template.Must(template.New("consent").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Authorize {{.ClientName}} — Vire</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0f1117;color:#e1e4e8;min-height:100vh;display:flex;align-items:center;justify-content:center}
.card{background:#161b22;border:1px solid #30363d;border-radius:12px;padding:2rem;width:100%;max-width:400px}
h1{font-size:1.25rem;margin-bottom:.5rem;color:#f0f6fc}
p.desc{color:#8b949e;margin-bottom:1.5rem;font-size:.9rem}
label{display:block;font-size:.85rem;color:#8b949e;margin-bottom:.25rem}
input[type=email],input[type=password]{width:100%;padding:.6rem .75rem;background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#e1e4e8;font-size:.9rem;margin-bottom:1rem}
input:focus{outline:none;border-color:#58a6ff}
.actions{display:flex;gap:.75rem;margin-top:.5rem}
button{flex:1;padding:.6rem;border:none;border-radius:6px;font-size:.9rem;cursor:pointer;font-weight:500}
button[type=submit]{background:#238636;color:#fff}
button[type=submit]:hover{background:#2ea043}
a.deny{flex:1;display:flex;align-items:center;justify-content:center;padding:.6rem;border:1px solid #30363d;border-radius:6px;color:#8b949e;text-decoration:none;font-size:.9rem}
a.deny:hover{border-color:#8b949e}
.error{background:#3d1f1f;border:1px solid #6e3630;color:#f85149;padding:.5rem .75rem;border-radius:6px;margin-bottom:1rem;font-size:.85rem}
.scope{background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:.5rem .75rem;margin-bottom:1rem;font-size:.85rem;color:#8b949e}
</style>
</head>
<body>
<div class="card">
<h1>Authorize {{.ClientName}}</h1>
<p class="desc">{{.ClientName}} wants to access your Vire account.</p>
{{if .Scope}}<div class="scope">Scope: {{.Scope}}</div>{{end}}
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<form method="POST" action="/oauth/authorize">
<label for="email">Email</label>
<input type="email" id="email" name="email" required autocomplete="email">
<label for="password">Password</label>
<input type="password" id="password" name="password" required autocomplete="current-password">
<input type="hidden" name="client_id" value="{{.ClientID}}">
<input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
<input type="hidden" name="state" value="{{.State}}">
<input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
<input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
<input type="hidden" name="scope" value="{{.Scope}}">
<input type="hidden" name="response_type" value="{{.ResponseType}}">
<div class="actions">
<button type="submit">Grant Access</button>
<a class="deny" href="{{.DenyURL}}">Deny</a>
</div>
</form>
</div>
</body>
</html>`))

// buildDenyURL constructs a properly URL-encoded deny redirect URL.
func buildDenyURL(redirectURI, state string) string {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return redirectURI
	}
	q := u.Query()
	q.Set("error", "access_denied")
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// renderConsentPage renders the OAuth consent/login page.
func renderConsentPage(w http.ResponseWriter, data oauthConsentData) {
	data.DenyURL = buildDenyURL(data.RedirectURI, data.State)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if data.Error != "" {
		w.WriteHeader(http.StatusOK)
	}
	if err := consentTemplate.Execute(w, data); err != nil {
		log.Printf("oauth consent template error: %v", err)
	}
}
