package main

// The release criteria of the first release (Increment 4): `go test -run TestRelease ./...` is the
// checkpoint. The first two run the real binary built by TestMain against a fresh SQLite file; the
// third checks that every test name the requirements model plans exists as a Go test function
// (ADR-0022).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v2"

	"github.com/adverax/booking-demo/internal/httpapi"
)

// AC-031 (NFR-001, ADR-0021): 50 simultaneous requests for one free future slot on a fresh
// database yield exactly one successful booking. A correctness criterion proving INV-001 under
// concurrency, not a scale target (NG-1).
func TestRelease_ConcurrencyExactlyOneWinner(t *testing.T) {
	assertExactlyOneOf50Wins(t)
}

// AC-032 (NFR-002): on a fresh database, the whole first-release scenario runs through the HTTP
// API of the real binary: create a resource, list availability, book, repeat the booking, cancel.
// Requests and responses use the types generated from api/openapi.yaml.
func TestRelease_FullAPIEndToEndOnFreshDatabase(t *testing.T) {
	svc := startService(t, filepath.Join(t.TempDir(), "booking.db"))
	client := &http.Client{Timeout: 10 * time.Second}
	slot := futureMondayNine()
	day := slot.Format(time.DateOnly)

	// Create a resource working Monday 09:00-17:00 UTC.
	var res httpapi.Resource
	call(t, client, http.MethodPost, svc.url+"/resources", httpapi.CreateResourceRequest{
		Name:        "Dr. Lee",
		WeeklyHours: httpapi.WeeklyHours{{Weekday: httpapi.WeekdayMonday, Start: "09:00", End: "17:00"}},
	}, http.StatusCreated, &res)
	if res.Id == "" || res.Name != "Dr. Lee" {
		t.Fatalf("created resource = %+v", res)
	}

	availabilityURL := fmt.Sprintf("%s/resources/%s/availability?from=%s&to=%s", svc.url, res.Id, day, day)
	freeStarts := func() []time.Time {
		t.Helper()
		var a httpapi.Availability
		call(t, client, http.MethodGet, availabilityURL, nil, http.StatusOK, &a)
		starts := make([]time.Time, len(a.Slots))
		for i, s := range a.Slots {
			if !s.End.Equal(s.Start.Add(30 * time.Minute)) {
				t.Fatalf("slot %v-%v is not 30 minutes", s.Start, s.End)
			}
			starts[i] = s.Start
		}
		return starts
	}

	// List availability: 16 half-hour slots, the first at 09:00.
	before := freeStarts()
	if len(before) != 16 || !before[0].Equal(slot) {
		t.Fatalf("free slots before booking = %v, want 16 starting at %v", before, slot)
	}

	// Book the 09:00 slot.
	req := httpapi.BookSlotRequest{ResourceId: res.Id, SlotStart: slot, CustomerName: "Ann", IdempotencyKey: "release-e2e-1"}
	var booked httpapi.Booking
	call(t, client, http.MethodPost, svc.url+"/bookings", req, http.StatusCreated, &booked)
	if booked.Id == "" || booked.Status != httpapi.BookingStatusActive || !booked.SlotStart.Equal(slot) {
		t.Fatalf("booking = %+v", booked)
	}
	if free := freeStarts(); len(free) != 15 || slices.ContainsFunc(free, slot.Equal) {
		t.Fatalf("free slots after booking = %v, want 15 without %v", free, slot)
	}

	// Repeat the booking: the original is returned and nothing new is created.
	var replayed httpapi.Booking
	call(t, client, http.MethodPost, svc.url+"/bookings", req, http.StatusOK, &replayed)
	if replayed.Id != booked.Id || replayed.Status != httpapi.BookingStatusActive {
		t.Fatalf("replay = %+v, want the original booking %s", replayed, booked.Id)
	}

	// Cancel: the booking is cancelled and its slot is free again.
	var cancelled httpapi.Booking
	call(t, client, http.MethodPost, svc.url+"/bookings/"+booked.Id+"/cancel", nil, http.StatusOK, &cancelled)
	if cancelled.Id != booked.Id || cancelled.Status != httpapi.BookingStatusCancelled {
		t.Fatalf("cancel = %+v", cancelled)
	}
	var read httpapi.Booking
	call(t, client, http.MethodGet, svc.url+"/bookings/"+booked.Id, nil, http.StatusOK, &read)
	if read.Status != httpapi.BookingStatusCancelled {
		t.Fatalf("booking after cancel = %+v", read)
	}
	if free := freeStarts(); !slices.EqualFunc(free, before, time.Time.Equal) {
		t.Fatalf("free slots after cancel = %v, want %v", free, before)
	}

	if s := strings.TrimSpace(svc.stderr.String()); s != "" {
		t.Errorf("unexpected stderr output: %s", s)
	}
}

// call sends body (if any) as JSON, requires status want and decodes the response into out.
func call(t *testing.T, client *http.Client, method, url string, body any, want int, out any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, url, resp.StatusCode, want, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s %s: decode %s: %v", method, url, raw, err)
	}
}

// requirementsModel is the requirements model the planned test names are read from (ADR-0022).
const requirementsModel = "meta/architecture/requirements.yml"

// AC-033 (NFR-003, ADR-0022): every test name the requirements model plans exists as a Go test
// function in the repository. It checks existence, not whether the test is meaningful (NG-2).
func TestRelease_EveryRuleHasANamedTest(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, requirementsModel))
	if err != nil {
		t.Fatal(err)
	}
	planned, err := plannedTestNames(raw)
	if err != nil {
		t.Fatalf("%s: %v", requirementsModel, err)
	}
	defined, err := goTestFunctions(root)
	if err != nil {
		t.Fatal(err)
	}
	missing := missingTests(planned, defined)
	for _, p := range missing {
		t.Errorf("%s: %s names %q, which is not a Go test function in the repository", requirementsModel, p.at, p.name)
	}
	t.Logf("%d planned test names, %d Go test functions, %d missing", len(planned), len(defined), len(missing))
}

// repoRoot walks up from the working directory to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// plannedTest is one test name the model plans, with where it was read.
type plannedTest struct {
	name string
	at   string // e.g. functional[0].acceptance[2].test
}

// plannedTestNames reads every value of a `test` key anywhere in the model. The model's only
// test-name field is `test:` with one name; any other shape is refused rather than skipped (a
// list, an empty value, or another key naming tests such as `tests:`), so that a schema change
// of the model fails the check instead of silently shrinking it. Zero names is an error too.
func plannedTestNames(raw []byte) ([]plannedTest, error) {
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	var out []plannedTest
	var walk func(v any, at string) error
	walk = func(v any, at string) error {
		switch v := v.(type) {
		case map[any]any:
			for k, child := range v {
				key, ok := k.(string)
				if !ok {
					return fmt.Errorf("%s: non-string key %v", at, k)
				}
				where := key
				if at != "" {
					where = at + "." + key
				}
				switch {
				case key == "test":
					name, ok := child.(string)
					if !ok || name == "" {
						return fmt.Errorf("%s: want one test name, got %#v", where, child)
					}
					out = append(out, plannedTest{name: name, at: where})
				case strings.Contains(strings.ToLower(key), "test"):
					return fmt.Errorf("%s: unrecognised test field; only `test:` is read", where)
				default:
					if err := walk(child, where); err != nil {
						return err
					}
				}
			}
		case []any:
			for i, child := range v {
				if err := walk(child, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(doc, ""); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no planned test names found")
	}
	slices.SortFunc(out, func(a, b plannedTest) int { return strings.Compare(a.at, b.at) })
	return out, nil
}

// goTestFunctions returns the names of the Go test functions in the *_test.go files under root,
// skipping the directories the go tool ignores (names starting with "." or "_", testdata).
func goTestFunctions(root string) (map[string]bool, error) {
	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		testingName := testingImportName(f)
		if testingName == "" {
			return nil
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && isTestFunc(fn, testingName) {
				found[fn.Name.Name] = true
			}
		}
		return nil
	})
	return found, err
}

// testingImportName is the name the file refers to package testing by, or "" if not imported.
func testingImportName(f *ast.File) string {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"testing"` {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				return ""
			}
			return imp.Name.Name
		}
		return "testing"
	}
	return ""
}

// isTestFunc reports whether fn is a test the go tool runs: `func TestXxx(t *testing.T)` at top
// level, no type parameters, no results, and a valid test name.
func isTestFunc(fn *ast.FuncDecl, testingName string) bool {
	if fn.Recv != nil || fn.Type.TypeParams != nil || fn.Type.Results != nil || !isTestName(fn.Name.Name) {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == testingName
}

// isTestName applies the go tool's rule: an identifier "Test" followed by nothing or by a
// character that is not a lower-case letter.
func isTestName(name string) bool {
	if !token.IsIdentifier(name) || !strings.HasPrefix(name, "Test") {
		return false
	}
	rest := name[len("Test"):]
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// missingTests returns the planned tests whose name is not a defined Go test function. A name
// that is not a valid test name can never be defined, so it is always missing.
func missingTests(planned []plannedTest, defined map[string]bool) []plannedTest {
	var missing []plannedTest
	for _, p := range planned {
		if !isTestName(p.name) || !defined[p.name] {
			missing = append(missing, p)
		}
	}
	return missing
}

// The rule check fails on a missing or invalid name and on a model it cannot read in full: a
// synthetic model and a synthetic Go file stand in for the real ones.
func TestEveryRuleHasANamedTest_DetectsMissingAndInvalidNames(t *testing.T) {
	dir := t.TempDir()
	src := `package x

import tt "testing"

func TestPresent(t *tt.T)              {}
func Test(t *tt.T)                     {}
func Testlower(t *tt.T)                {}
func TestWrongParam(b *tt.B)           {}
func TestWithResult(t *tt.T) error     { return nil }
type s struct{}
func (s) TestMethod(t *tt.T)           {}
`
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("x/x_test.go", src)
	write("x/testdata/y_test.go", "package y\nimport \"testing\"\nfunc TestInTestdata(t *testing.T) {}\n")
	write(".hidden/z_test.go", "package z\nimport \"testing\"\nfunc TestHidden(t *testing.T) {}\n")
	write("x/notatest.go", "package x\nimport \"testing\"\nfunc TestInNonTestFile(t *testing.T) {}\n")

	defined, err := goTestFunctions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"TestPresent": true, "Test": true}; !maps.Equal(defined, want) {
		t.Fatalf("defined = %v, want %v", defined, want)
	}

	model := `
functional:
  - id: FR-1
    acceptance:
      - {id: AC-1, test: TestPresent}
      - {id: AC-2, test: TestAbsent}
      - {id: AC-3, test: PropertyTest_Old}
      - {id: AC-4, test: Testlower}
      - {id: AC-5, test: TestMethod}
      - {id: AC-6, test: TestWrongParam}
      - {id: AC-7, test: TestInTestdata}
non_functional:
  - id: NFR-1
    acceptance:
      - {id: AC-8, test: TestPresent}
`
	planned, err := plannedTestNames([]byte(model))
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 8 {
		t.Fatalf("planned = %v, want 8 names", planned)
	}
	var got []string
	for _, p := range missingTests(planned, defined) {
		got = append(got, p.name)
	}
	want := []string{"TestAbsent", "PropertyTest_Old", "Testlower", "TestMethod", "TestWrongParam", "TestInTestdata"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("missing = %v, want %v", got, want)
	}

	for name, bad := range map[string]string{
		"no names":       "functional:\n  - id: FR-1\n",
		"empty model":    "",
		"list value":     "functional:\n  - acceptance:\n      - test: [TestA, TestB]\n",
		"empty value":    "functional:\n  - acceptance:\n      - test: \"\"\n",
		"unknown field":  "functional:\n  - acceptance:\n      - test: TestA\n        tests: [TestB]\n",
		"malformed yaml": "functional: [\n",
	} {
		if _, err := plannedTestNames([]byte(bad)); err == nil {
			t.Errorf("%s: model accepted, want an error", name)
		}
	}
}
