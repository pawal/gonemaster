package i18n

import (
	"errors"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// poEntry renders a single .po entry keyed by msgctxt.
func poEntry(ctx string, msgid string, msgstr string) []byte {
	return []byte("msgctxt " + strconv.Quote(ctx) + "\n" +
		"msgid " + strconv.Quote(msgid) + "\n" +
		"msgstr " + strconv.Quote(msgstr) + "\n")
}

// errFS reports one file in the listing but fails to read it.
type errFS struct{ name string }

var errUnreadable = errors.New("unreadable")

func (e errFS) Open(name string) (fs.File, error) { return nil, errUnreadable }

func (e errFS) ReadFile(name string) ([]byte, error) { return nil, errUnreadable }

func (e errFS) Glob(pattern string) ([]string, error) { return []string{e.name}, nil }

// A consumer with its own catalog registers it and translates from it, using
// the embedded conventions for keys and placeholders.
func TestRegisterCatalogsAddsLocaleAndEntry(t *testing.T) {
	fsys := fstest.MapFS{
		"zz.po": &fstest.MapFile{Data: poEntry("TESTREG:SAMPLE", "{count} servers", "zz {count} servers")},
	}
	if err := RegisterCatalogs(fsys, "*.po"); err != nil {
		t.Fatalf("RegisterCatalogs: %v", err)
	}

	got, ok := TranslateWithStatus("zz", "testreg", "SAMPLE", map[string]any{"count": 3})
	if !ok {
		t.Fatal("registered entry not found")
	}
	if got != "zz 3 servers" {
		t.Errorf("Translate = %q, want %q", got, "zz 3 servers")
	}

	if !slices.Contains(AvailableLocales(), "zz") {
		t.Errorf("registered locale missing from AvailableLocales: %v", AvailableLocales())
	}

	// The msgid serves as the English fallback, as it does for embedded files.
	if english, ok := TranslateWithStatus("en", "testreg", "SAMPLE", map[string]any{"count": 3}); !ok || english != "3 servers" {
		t.Errorf("English fallback = %q (found=%v), want %q", english, ok, "3 servers")
	}
}

// Later registrations win, so a consumer can layer catalogs.
func TestRegisterCatalogsLaterRegistrationWins(t *testing.T) {
	first := fstest.MapFS{"zz.po": &fstest.MapFile{Data: poEntry("TESTREG:LAYERED", "original", "first")}}
	second := fstest.MapFS{"zz.po": &fstest.MapFile{Data: poEntry("TESTREG:LAYERED", "original", "second")}}

	if err := RegisterCatalogs(first, "*.po"); err != nil {
		t.Fatalf("RegisterCatalogs first: %v", err)
	}
	if got := Translate("zz", "testreg", "LAYERED", nil); got != "first" {
		t.Fatalf("after first registration = %q, want %q", got, "first")
	}
	if err := RegisterCatalogs(second, "*.po"); err != nil {
		t.Fatalf("RegisterCatalogs second: %v", err)
	}
	if got := Translate("zz", "testreg", "LAYERED", nil); got != "second" {
		t.Errorf("after second registration = %q, want %q", got, "second")
	}
}

// A registered entry shadows the embedded one for the same key. The original is
// registered back afterwards so the rest of the package sees the shipped text.
func TestRegisterCatalogsOverridesEmbedded(t *testing.T) {
	const (
		locale = "da"
		module = "basic"
		tag    = "B01_ROOT_HAS_NO_PARENT"
		key    = "BASIC:B01_ROOT_HAS_NO_PARENT"
	)

	englishText := Translate("en", module, tag, nil)
	original := Translate(locale, module, tag, nil)
	if original == "" || original == key {
		t.Fatalf("no embedded %s translation to override: %q", locale, original)
	}

	t.Cleanup(func() {
		restore := fstest.MapFS{locale + ".po": &fstest.MapFile{Data: poEntry(key, englishText, original)}}
		if err := RegisterCatalogs(restore, "*.po"); err != nil {
			t.Fatalf("restore embedded catalog: %v", err)
		}
		if got := Translate(locale, module, tag, nil); got != original {
			t.Fatalf("restore left %q, want %q", got, original)
		}
	})

	override := fstest.MapFS{locale + ".po": &fstest.MapFile{Data: poEntry(key, englishText, "registered override")}}
	if err := RegisterCatalogs(override, "*.po"); err != nil {
		t.Fatalf("RegisterCatalogs: %v", err)
	}
	if got := Translate(locale, module, tag, nil); got != "registered override" {
		t.Errorf("Translate = %q, want the registered override", got)
	}
}

// Parsing happens before merging, so a failed read cannot leave the catalogs
// half-updated.
func TestRegisterCatalogsReadErrorKeepsCatalogs(t *testing.T) {
	before := Translate("da", "basic", "B01_ROOT_HAS_NO_PARENT", nil)

	err := RegisterCatalogs(errFS{name: "da.po"}, "*.po")
	if err == nil {
		t.Fatal("expected an error for an unreadable catalog file")
	}
	if !errors.Is(err, errUnreadable) {
		t.Errorf("error = %v, want it to wrap the read failure", err)
	}
	if after := Translate("da", "basic", "B01_ROOT_HAS_NO_PARENT", nil); after != before {
		t.Errorf("catalogs changed after a failed registration: %q, want %q", after, before)
	}
}

func TestRegisterCatalogsRejectsBadPatternAndNilFS(t *testing.T) {
	if err := RegisterCatalogs(fstest.MapFS{}, "["); err == nil {
		t.Error("expected an error for a malformed pattern")
	}
	if err := RegisterCatalogs(nil, "*.po"); err == nil {
		t.Error("expected an error for a nil filesystem")
	}
}

// A pattern matching nothing is a no-op, not an error.
func TestRegisterCatalogsEmptyMatchIsNoOp(t *testing.T) {
	if err := RegisterCatalogs(fstest.MapFS{}, "*.po"); err != nil {
		t.Errorf("RegisterCatalogs on an empty filesystem: %v", err)
	}
}

// Translate reads the catalogs while RegisterCatalogs writes them; meaningful
// under -race.
func TestRegisterCatalogsConcurrentWithTranslate(t *testing.T) {
	seed := fstest.MapFS{"zc.po": &fstest.MapFile{Data: poEntry("TESTREG:CONCURRENT", "value", "value seed")}}
	if err := RegisterCatalogs(seed, "*.po"); err != nil {
		t.Fatalf("seed RegisterCatalogs: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			fsys := fstest.MapFS{
				"zc.po": &fstest.MapFile{Data: poEntry("TESTREG:CONCURRENT", "value", "value "+strconv.Itoa(i))},
			}
			if err := RegisterCatalogs(fsys, "*.po"); err != nil {
				t.Errorf("RegisterCatalogs: %v", err)
				return
			}
		}
	}()

	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(50 * time.Millisecond)
			for time.Now().Before(deadline) {
				if got := Translate("zc", "testreg", "CONCURRENT", nil); !strings.HasPrefix(got, "value") {
					t.Errorf("Translate = %q, want a registered value", got)
					return
				}
				_ = AvailableLocales()
			}
		}()
	}

	time.Sleep(60 * time.Millisecond)
	close(stop)
	wg.Wait()
}
