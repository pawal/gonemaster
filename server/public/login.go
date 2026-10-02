package public

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/http"
	"strconv"
)

//go:embed login.css
var loginCSS []byte

// loginStrings are the visible texts of the admin token form.
type loginStrings struct {
	Title, Label, Placeholder, Submit, Invalid string
}

var loginText = map[string]loginStrings{
	"cs": {"Vyžaduje se ověření správce", "Token správce", "Vložte svůj token správce", "Přihlásit se", "Neplatný token"},
	"da": {"Administratorgodkendelse kræves", "Administratortoken", "Indsæt din administratortoken", "Log ind", "Ugyldig token"},
	"de": {"Admin-Authentifizierung erforderlich", "Admin-Token", "Admin-Token einfügen", "Anmelden", "Ungültiger Token"},
	"en": {"Admin authentication required", "Admin token", "Paste your admin token", "Log in", "Invalid token"},
	"es": {"Se requiere autenticación de administrador", "Token de administrador", "Pega tu token de administrador", "Iniciar sesión", "Token no válido"},
	"fi": {"Ylläpitäjän todennus vaaditaan", "Ylläpitotunniste", "Liitä ylläpitotunnisteesi", "Kirjaudu sisään", "Virheellinen tunniste"},
	"fr": {"Authentification administrateur requise", "Jeton administrateur", "Collez votre jeton administrateur", "Se connecter", "Jeton invalide"},
	"ja": {"管理者認証が必要です", "管理者トークン", "管理者トークンを貼り付けてください", "ログイン", "無効なトークンです"},
	"nb": {"Administratorpålogging kreves", "Administratortoken", "Lim inn administratortokenet ditt", "Logg inn", "Ugyldig token"},
	"nl": {"Beheerdersauthenticatie vereist", "Beheerderstoken", "Plak uw beheerderstoken", "Inloggen", "Ongeldig token"},
	"sl": {"Zahtevana je skrbniška prijava", "Skrbniški žeton", "Prilepite svoj skrbniški žeton", "Prijava", "Neveljaven žeton"},
	"sv": {"Administratörsinloggning krävs", "Administratörstoken", "Klistra in din administratörstoken", "Logga in", "Ogiltig token"},
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="{{.Lang}}">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="robots" content="noindex, nofollow" />
    <title>{{.T.Title}} - Gonemaster</title>
    <link rel="stylesheet" href="{{.CSSHref}}" />
  </head>
  <body>
    <main>
      <form class="login-card" method="post">
        <h1>{{.T.Title}}</h1>
        <label for="token">{{.T.Label}}</label>
        <input id="token" name="token" type="password" autocomplete="off" required autofocus placeholder="{{.T.Placeholder}}" />
        {{- if .Failed}}
        <p class="login-error" role="alert">{{.T.Invalid}}</p>
        {{- end}}
        <button type="submit">{{.T.Submit}}</button>
      </form>
    </main>
  </body>
</html>
`))

type loginData struct {
	Lang    string
	CSSHref string
	Failed  bool
	T       loginStrings
}

// RenderLogin writes the admin token form with status 401.
func RenderLogin(w http.ResponseWriter, r *http.Request, cssHref string, failed bool) {
	locale := negotiateLocale(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	var buf bytes.Buffer
	_ = loginTemplate.Execute(&buf, loginData{Lang: locale, CSSHref: cssHref, Failed: failed, T: loginText[locale]})
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(buf.Len()))
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex")
	h.Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	if r.Method != http.MethodHead {
		_, _ = w.Write(buf.Bytes())
	}
}

// ServeLoginCSS writes the login page stylesheet.
func ServeLoginCSS(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/css; charset=utf-8")
	h.Set("Content-Length", strconv.Itoa(len(loginCSS)))
	h.Set("Cache-Control", "public, max-age=3600")
	if r.Method != http.MethodHead {
		_, _ = w.Write(loginCSS)
	}
}
