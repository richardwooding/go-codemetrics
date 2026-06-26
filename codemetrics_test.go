package codemetrics

import (
	"errors"
	"testing"
)

// firstFunc parses src via ParseGo and returns the metrics of its first
// function. It fails the test if there is no function.
func firstFunc(t *testing.T, src string) FunctionMetrics {
	t.Helper()
	fns, err := ParseGo([]byte(src))
	if err != nil {
		t.Fatalf("ParseGo: %v", err)
	}
	if len(fns) == 0 {
		t.Fatal("no function in source")
	}
	return fns[0]
}

func TestCognitiveComplexity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{
			// SonarSource reference: nested loops + labelled continue = 7.
			name: "sumOfPrimes",
			src: `package p
func sumOfPrimes(max int) int {
	total := 0
OUT:
	for i := 1; i <= max; i++ {
		for j := 2; j < i; j++ {
			if i%j == 0 {
				continue OUT
			}
		}
		total += i
	}
	return total
}`,
			want: 7,
		},
		{
			// SonarSource reference: a flat switch = 1.
			name: "switch",
			src: `package p
func getWords(number int) string {
	switch number {
	case 1:
		return "one"
	case 2:
		return "a couple"
	default:
		return "lots"
	}
}`,
			want: 1,
		},
		{
			name: "else-if-chain",
			src: `package p
func f(n int) int {
	if n == 1 {
		return 1
	} else if n == 2 {
		return 2
	} else {
		return 3
	}
}`,
			want: 3,
		},
		{
			name: "nested-if",
			src: `package p
func f(a, b bool) int {
	if a {
		if b {
			return 1
		}
	}
	return 0
}`,
			want: 3,
		},
		{
			name: "flat-ifs",
			src: `package p
func f(a, b bool) int {
	if a {
		return 1
	}
	if b {
		return 2
	}
	return 0
}`,
			want: 2,
		},
		{
			name: "and-run",
			src: `package p
func f(a, b, c, d bool) bool {
	return a && b && c && d
}`,
			want: 1,
		},
		{
			name: "mixed-bool",
			src: `package p
func f(a, b, c bool) bool {
	return a && b || c
}`,
			want: 2,
		},
		{
			name: "paren-bool",
			src: `package p
func f(a, b, c bool) bool {
	return a && (b || c)
}`,
			want: 2,
		},
		{
			name: "recursion",
			src: `package p
func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}`,
			want: 3,
		},
		{
			name: "method-recursion",
			src: `package p
type T struct{ n int }
func (t *T) fib(n int) int {
	if n < 2 {
		return n
	}
	return t.fib(n-1) + t.fib(n-2)
}`,
			want: 3,
		},
		{
			name: "trivial",
			src: `package p
func f() int { return 41 }`,
			want: 0,
		},
		{
			name: "func-lit-nesting",
			src: `package p
func f(items []int) {
	for range items {
		g := func(x int) {
			if x > 0 {
				println(x)
			}
		}
		g(1)
	}
}`,
			want: 4,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := firstFunc(t, tc.src)
			if m.Cognitive == nil {
				t.Fatal("Cognitive is nil; want a value for Go")
			}
			if *m.Cognitive != tc.want {
				t.Errorf("cognitive complexity = %d, want %d", *m.Cognitive, tc.want)
			}
		})
	}
}

func TestCyclomaticComplexity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
	}{
		{"trivial", "package p\nfunc f() {}", 1},
		{
			name: "if-and-loop",
			src: `package p
func f(a, b bool) {
	if a {
	}
	for b {
	}
}`,
			want: 3, // 1 + if + for
		},
		{
			name: "switch-cases",
			src: `package p
func f(n int) {
	switch n {
	case 1:
	case 2:
	default:
	}
}`,
			want: 4, // 1 + three case clauses (gocyclo counts default too)
		},
		{
			name: "logical-ops",
			src: `package p
func f(a, b, c bool) bool {
	return a && b || c
}`,
			want: 3, // 1 + && + ||
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := firstFunc(t, tc.src)
			if m.Cyclomatic != tc.want {
				t.Errorf("cyclomatic complexity = %d, want %d", m.Cyclomatic, tc.want)
			}
		})
	}
}

func TestParseGo_MultipleFunctionsAndSpans(t *testing.T) {
	src := `package p

func first() int {
	return 1
}

func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}
`
	fns, err := ParseGo([]byte(src))
	if err != nil {
		t.Fatalf("ParseGo: %v", err)
	}
	if len(fns) != 2 {
		t.Fatalf("got %d functions, want 2", len(fns))
	}

	if fns[0].Name != "first" || fns[0].Receiver != "" {
		t.Errorf("fn0 = %q recv %q, want first/\"\"", fns[0].Name, fns[0].Receiver)
	}
	if got := fns[0].StartLine; got != 3 {
		t.Errorf("first StartLine = %d, want 3", got)
	}
	if got := fns[0].EndLine; got != 5 {
		t.Errorf("first EndLine = %d, want 5", got)
	}
	if got := fns[0].Lines(); got != 3 {
		t.Errorf("first Lines() = %d, want 3", got)
	}

	if fns[1].Name != "Push" || fns[1].Receiver != "*Stack" {
		t.Errorf("fn1 = %q recv %q, want Push/*Stack", fns[1].Name, fns[1].Receiver)
	}
	if got := fns[1].QualifiedName(); got != "(*Stack).Push" {
		t.Errorf("Push QualifiedName() = %q, want (*Stack).Push", got)
	}
}

func TestParseGo_SkipsBodylessDecls(t *testing.T) {
	src := `package p

type I interface { M() }      // interface method has no body
func external()                // external decl, no body

func real() {}
`
	fns, err := ParseGo([]byte(src))
	if err != nil {
		t.Fatalf("ParseGo: %v", err)
	}
	if len(fns) != 1 || fns[0].Name != "real" {
		t.Fatalf("got %+v, want only func real", fns)
	}
}

func TestParseGo_PartialParseIsBestEffort(t *testing.T) {
	// A trailing syntax error must not lose the well-formed function above it.
	src := `package p

func good() int { return 1 }

func bad( {
`
	fns, err := ParseGo([]byte(src))
	if err != nil {
		t.Fatalf("ParseGo returned error on partial parse: %v", err)
	}
	found := false
	for _, f := range fns {
		if f.Name == "good" {
			found = true
		}
	}
	if !found {
		t.Errorf("good() not recovered from partial parse; got %+v", fns)
	}
}

func TestParse_Dispatch(t *testing.T) {
	src := []byte("package p\nfunc f() {}")

	for _, lang := range []string{"go", "Go", "golang", " go "} {
		if _, err := Parse(lang, src); err != nil {
			t.Errorf("Parse(%q) = %v, want nil", lang, err)
		}
	}

	_, err := Parse("rust", src)
	if !errors.Is(err, ErrUnsupportedLanguage) {
		t.Errorf("Parse(rust) error = %v, want ErrUnsupportedLanguage", err)
	}
}

func TestSupportedLanguages(t *testing.T) {
	got := SupportedLanguages()
	if len(got) != 1 || got[0] != "go" {
		t.Errorf("SupportedLanguages() = %v, want [go]", got)
	}
}
