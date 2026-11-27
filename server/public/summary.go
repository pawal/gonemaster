package public

import (
	"fmt"
	"html"
	"slices"
	"strings"
	"time"
)

// Must match ui-public/src/i18n (see TestHreflangLangsMatchShippedLocales).
var hreflangLangs = []string{"cs", "da", "de", "en", "es", "fi", "fr", "ja", "nb", "nl", "sl", "sv"}

// ShippedLocales is the single list the sitemap and hreflang block share.
func ShippedLocales() []string {
	return slices.Clone(hreflangLangs)
}

// DefaultUIPath is where the public UI sits under the site root unless a
// deployment serves it somewhere else, typically the root itself.
const DefaultUIPath = "public/"

// Site builds the URLs a deployment advertises. base is the site root; uiPath
// is where visitors reach the public UI under it.
type Site struct {
	base   string
	uiPath string
}

func NewSite(base, uiPath string) Site {
	uiPath = strings.Trim(strings.TrimSpace(uiPath), "/")
	if uiPath != "" {
		uiPath += "/"
	}
	return Site{base: base, uiPath: uiPath}
}

func (s Site) Home() string { return s.base + s.uiPath }

// ClientBase is the path prefix the browser sees, which the SPA router needs.
func (s Site) ClientBase() string { return "/" + s.uiPath }

func (s Site) Result(id string) string { return s.Home() + "result/" + id }

// English shares the x-default URL rather than duplicating it under ?lang=en.
func (s Site) Locale(locale string) string {
	if locale == "" || locale == "en" {
		return s.Home()
	}
	return s.Home() + "?lang=" + locale
}

// PageURLs is every distinct indexable public UI URL.
func (s Site) PageURLs() []string {
	urls := []string{s.Home()}
	for _, lang := range hreflangLangs {
		if u := s.Locale(lang); u != s.Home() {
			urls = append(urls, u)
		}
	}
	return urls
}

const MaxSummaryFindings = 25

type Finding struct {
	Level    string
	Message  string
	Testcase string
}

type ResultSummary struct {
	Domain     string
	Grade      string // "" when scoring is hidden
	Score      int
	FinishedAt time.Time
	Warnings   int
	Errors     int
	Criticals  int
	Findings   []Finding // capped at MaxSummaryFindings
}

// Issues counts WARNING and above, including findings past the cap.
func (s ResultSummary) Issues() int {
	return s.Warnings + s.Errors + s.Criticals
}

type LookupStatus int

const (
	LookupFound    LookupStatus = iota // finished, summary valid
	LookupPending                      // job exists, not finished
	LookupNotFound                     // unknown, failed, or expired
)

type LookupResult func(publicID, locale string) (ResultSummary, LookupStatus)

// titleGrade, titleResult and heading are copies of the ui-public catalog keys,
// pinned by TestSummaryStringsMatchCatalogs.
type summaryStrings struct {
	titleGrade      string
	titleResult     string
	heading         string
	gradeClause     string
	issues          string
	tested          string
	more            string
	homeTitle       string
	homeDescription string
}

var summaryText = map[string]summaryStrings{
	"en": {
		titleGrade: "{domain} - grade {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Results for {domain}", gradeClause: "Grade {grade}, score {score}/100.",
		issues: "Issues", tested: "Tested", more: "and {n} more",
		homeTitle:       "Gonemaster: free DNS health check and DNSSEC test",
		homeDescription: "Free DNS health check for any domain: tests delegation, DNSSEC, connectivity, nameserver behavior and zone consistency on every authoritative nameserver.",
	},
	"sv": {
		titleGrade: "{domain} - betyg {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Resultat för {domain}", gradeClause: "Betyg {grade}, poäng {score}/100.",
		issues: "Problem", tested: "Testad", more: "och {n} fler",
		homeTitle:       "Gonemaster: gratis DNS-hälsokontroll och DNSSEC-test",
		homeDescription: "Gratis DNS-hälsokontroll för alla domäner: testar delegering, DNSSEC, anslutning, namnserverbeteende och zonkonsekvens på varje auktoritativ namnserver.",
	},
	"da": {
		titleGrade: "{domain} - karakter {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Resultater for {domain}", gradeClause: "Karakter {grade}, point {score}/100.",
		issues: "Problemer", tested: "Testet", more: "og {n} flere",
		homeTitle:       "Gonemaster: gratis DNS-helbredstjek og DNSSEC-test",
		homeDescription: "Gratis DNS-helbredstjek for alle domæner: tester delegering, DNSSEC, forbindelse, navneserveradfærd og zonekonsistens på hver autoritativ navneserver.",
	},
	"nb": {
		titleGrade: "{domain} - karakter {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Resultater for {domain}", gradeClause: "Karakter {grade}, poeng {score}/100.",
		issues: "Problemer", tested: "Testet", more: "og {n} flere",
		homeTitle:       "Gonemaster: gratis DNS-helsesjekk og DNSSEC-test",
		homeDescription: "Gratis DNS-helsesjekk for alle domener: tester delegering, DNSSEC, tilkobling, navnetjeneratferd og sonekonsistens på hver autoritativ navnetjener.",
	},
	"de": {
		titleGrade: "{domain} - Note {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Ergebnisse für {domain}", gradeClause: "Note {grade}, Punktzahl {score}/100.",
		issues: "Probleme", tested: "Getestet", more: "und {n} weitere",
		homeTitle:       "Gonemaster: kostenloser DNS-Check und DNSSEC-Test",
		homeDescription: "Kostenloser DNS-Check für jede Domain: testet Delegation, DNSSEC, Konnektivität, Nameserver-Verhalten und Zonenkonsistenz auf jedem autoritativen Nameserver.",
	},
	"nl": {
		titleGrade: "{domain} - cijfer {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Resultaten voor {domain}", gradeClause: "Cijfer {grade}, score {score}/100.",
		issues: "Problemen", tested: "Getest", more: "en nog {n}",
		homeTitle:       "Gonemaster: gratis DNS-gezondheidscontrole en DNSSEC-test",
		homeDescription: "Gratis DNS-gezondheidscontrole voor elk domein: test delegatie, DNSSEC, connectiviteit, naamservergedrag en zoneconsistentie op elke autoritatieve naamserver.",
	},
	"fr": {
		titleGrade: "{domain} - note {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Résultats pour {domain}", gradeClause: "Note {grade}, score {score}/100.",
		issues: "Problèmes", tested: "Testé", more: "et {n} de plus",
		homeTitle:       "Gonemaster : contrôle de santé DNS et test DNSSEC gratuits",
		homeDescription: "Contrôle de santé DNS gratuit pour tout domaine : teste la délégation, DNSSEC, la connectivité, le comportement des serveurs de noms et la cohérence de zone sur chaque serveur de noms faisant autorité.",
	},
	"es": {
		titleGrade: "{domain} - nota {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Resultados para {domain}", gradeClause: "Nota {grade}, puntuación {score}/100.",
		issues: "Problemas", tested: "Probado", more: "y {n} más",
		homeTitle:       "Gonemaster: verificación de salud DNS y prueba DNSSEC gratuitas",
		homeDescription: "Verificación de salud DNS gratuita para cualquier dominio: prueba la delegación, DNSSEC, la conectividad, el comportamiento de los servidores de nombres y la coherencia de zona en cada servidor de nombres autoritativo.",
	},
	"fi": {
		titleGrade: "{domain} - arvosana {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Tulokset verkkotunnukselle {domain}", gradeClause: "Arvosana {grade}, pisteet {score}/100.",
		issues: "Ongelmat", tested: "Testattu", more: "ja {n} muuta",
		homeTitle:       "Gonemaster: ilmainen DNS-kuntotarkistus ja DNSSEC-testi",
		homeDescription: "Ilmainen DNS-kuntotarkistus mille tahansa verkkotunnukselle: testaa delegoinnin, DNSSEC:n, yhteydet, nimipalvelimen toiminnan ja vyöhykkeen yhtenäisyyden jokaisella auktoritatiivisella nimipalvelimella.",
	},
	"cs": {
		titleGrade: "{domain} - známka {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Výsledky pro {domain}", gradeClause: "Známka {grade}, skóre {score}/100.",
		issues: "Problémy", tested: "Otestováno", more: "a {n} dalších",
		homeTitle:       "Gonemaster: bezplatná kontrola zdraví DNS a test DNSSEC",
		homeDescription: "Bezplatná kontrola zdraví DNS pro jakoukoli doménu: testuje delegaci, DNSSEC, konektivitu, chování jmenných serverů a konzistenci zóny na každém autoritativním jmenném serveru.",
	},
	"sl": {
		titleGrade: "{domain} - ocena {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "Rezultati za {domain}", gradeClause: "Ocena {grade}, točke {score}/100.",
		issues: "Težave", tested: "Preizkušeno", more: "in {n} več",
		homeTitle:       "Gonemaster: brezplačna preveritev zdravja DNS in test DNSSEC",
		homeDescription: "Brezplačna preveritev zdravja DNS za katero koli domeno: preizkusi delegiranje, DNSSEC, povezljivost, vedenje imenskih strežnikov in doslednost cone na vsakem avtoritativnem imenskem strežniku.",
	},
	"ja": {
		titleGrade: "{domain} - 評価 {grade} - Gonemaster", titleResult: "{domain} - Gonemaster",
		heading: "{domain} の結果", gradeClause: "評価 {grade}、スコア {score}/100。",
		issues: "問題", tested: "テスト日時", more: "他 {n} 件",
		homeTitle:       "Gonemaster: 無料の DNS ヘルスチェックと DNSSEC テスト",
		homeDescription: "あらゆるドメインに使える無料の DNS ヘルスチェック。すべての権威ネームサーバーで委任、DNSSEC、接続性、ネームサーバーの動作、ゾーンの一貫性をテストします。",
	},
}

func textFor(locale string) summaryStrings {
	if s, ok := summaryText[locale]; ok {
		return s
	}
	return summaryText["en"]
}

func fill(template string, pairs ...string) string {
	return strings.NewReplacer(pairs...).Replace(template)
}

// Browsers send Accept-Language in descending q order, so q values are ignored.
func negotiateLocale(param, acceptLanguage string) string {
	if l := matchLocale(param); l != "" {
		return l
	}
	for _, tag := range strings.Split(acceptLanguage, ",") {
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = tag[:i]
		}
		if l := matchLocale(tag); l != "" {
			return l
		}
	}
	return "en"
}

func matchLocale(tag string) string {
	base := strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(base, "-_"); i >= 0 {
		base = base[:i]
	}
	if base != "" && slices.Contains(hreflangLangs, base) {
		return base
	}
	return ""
}

func summaryTitle(s ResultSummary, locale string) string {
	t := textFor(locale)
	if s.Grade == "" {
		return fill(t.titleResult, "{domain}", s.Domain)
	}
	return fill(t.titleGrade, "{domain}", s.Domain, "{grade}", s.Grade)
}

func summaryDescription(s ResultSummary, locale string) string {
	t := textFor(locale)
	parts := []string{fill(t.heading, "{domain}", s.Domain) + "."}
	if s.Grade != "" {
		parts = append(parts, fill(t.gradeClause, "{grade}", s.Grade, "{score}", fmt.Sprint(s.Score)))
	}
	parts = append(parts, fmt.Sprintf("%s: %d", t.issues, s.Issues()))
	return strings.Join(parts, " ")
}

// Domains and message args are user input, so every value is escaped.
func renderSummary(s ResultSummary, locale string) string {
	t := textFor(locale)
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(fill(t.heading, "{domain}", s.Domain)))
	if s.Grade != "" {
		clause := fill(t.gradeClause, "{grade}", s.Grade, "{score}", fmt.Sprint(s.Score))
		fmt.Fprintf(&b, "      <p>%s</p>\n", html.EscapeString(clause))
	}
	if !s.FinishedAt.IsZero() {
		stamp := s.FinishedAt.UTC().Format("2006-01-02 15:04 MST")
		fmt.Fprintf(&b, "      <p>%s: %s</p>\n", html.EscapeString(t.tested), html.EscapeString(stamp))
	}
	fmt.Fprintf(&b, "      <h2>%s: %d</h2>\n", html.EscapeString(t.issues), s.Issues())
	if len(s.Findings) > 0 {
		b.WriteString("      <ul>\n")
		for _, f := range s.Findings {
			label := f.Level
			if f.Testcase != "" {
				label += " " + f.Testcase
			}
			fmt.Fprintf(&b, "        <li>%s: %s</li>\n",
				html.EscapeString(label), html.EscapeString(f.Message))
		}
		b.WriteString("      </ul>\n")
	}
	if more := s.Issues() - len(s.Findings); more > 0 {
		fmt.Fprintf(&b, "      <p>%s</p>\n", html.EscapeString(fill(t.more, "{n}", fmt.Sprint(more))))
	}
	return b.String()
}
