package packages

import (
	"slices"
	"testing"
)

func TestParseTSConfigWithComments(t *testing.T) {
	p, err := parseTSConfig([]byte(`{
  // comment
  "compilerOptions": {
    "baseUrl": ".", /* inline */
    "paths": {
      "@app/*": ["src/app/*"],
      "url": ["https://example.com//not-a-comment"],
    },
  },
}`))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(p.aliases)
	if want := []string{"@app/*", "url"}; !slices.Equal(p.aliases, want) {
		t.Fatalf("aliases = %v, want %v", p.aliases, want)
	}
}

func TestParseImportMap(t *testing.T) {
	p, _ := parseImportMap([]byte(`{"imports": {"@std/assert": "jsr:@std/assert@1", "lodash/": "https://esm.sh/lodash/"}}`))
	slices.Sort(p.aliases)
	if want := []string{"@std/assert", "lodash/"}; !slices.Equal(p.aliases, want) {
		t.Fatalf("aliases = %v, want %v", p.aliases, want)
	}
}

func TestAliased(t *testing.T) {
	aliases := []string{"@app/*", "lib/", "exact", "*"}
	for spec, want := range map[string]bool{
		"@app/core": true,
		"lib/x":     true,
		"exact":     true,
		"exactly":   false,
		"react":     false, // the catch-all "*" does not count
	} {
		if got := aliased(spec, aliases); got != want {
			t.Errorf("aliased(%q) = %v, want %v", spec, got, want)
		}
	}
}
