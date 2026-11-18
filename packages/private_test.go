package packages

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseNpmrc(t *testing.T) {
	p, _ := parseNpmrc([]byte(`registry=https://registry.npmjs.org/
@corp:registry=https://npm.corp.example/
  @Other:registry = https://npm.other.example/
; @commented:registry=https://npm.old.example/
//npm.corp.example/:_authToken=${NPM_TOKEN}
`))
	if want := []string{"@corp", "@Other"}; !slices.Equal(p.private, want) {
		t.Fatalf("scopes = %v, want %v", p.private, want)
	}
}

func TestParseYarnrc(t *testing.T) {
	p, _ := parseYarnrc([]byte(`nodeLinker: node-modules
npmScopes:
  acme:
    npmRegistryServer: "https://npm.acme.example"
    npmAuthToken: "${NPM_TOKEN}"
  "@beta":
    npmRegistryServer: "https://npm.beta.example"
yarnPath: .yarn/releases/yarn.cjs
`))
	if want := []string{"@acme", "@beta"}; !slices.Equal(p.private, want) {
		t.Fatalf("scopes = %v, want %v", p.private, want)
	}
}

func TestGoNoProxy(t *testing.T) {
	file := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(file, []byte("GOPROXY=https://proxy.golang.org,direct\nGOPRIVATE=*.corp.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", file)
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "")
	if got := goNoProxy(); got != "*.corp.example" {
		t.Errorf("from go env file: got %q", got)
	}
	t.Setenv("GOPRIVATE", "git.env.example")
	if got := goNoProxy(); got != "git.env.example" {
		t.Errorf("environment should win over the file: got %q", got)
	}
	t.Setenv("GONOPROXY", "git.noproxy.example")
	if got := goNoProxy(); got != "git.noproxy.example" {
		t.Errorf("GONOPROXY should win over GOPRIVATE: got %q", got)
	}
}

func TestCollectDropsLocalAndPrivatePackages(t *testing.T) {
	t.Setenv("GOENV", "off")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOPRIVATE", "git.corp.example")
	root := t.TempDir()
	write(t, root, "package.json", `{"name": "monorepo", "workspaces": ["packages/*"]}`)
	write(t, root, ".npmrc", "@corp:registry=https://npm.corp.example/\n")
	write(t, root, ".yarnrc.yml", "npmScopes:\n  other:\n    npmRegistryServer: \"https://npm.other.example\"\n")
	write(t, root, "packages/a/package.json", `{"name": "@acme/a", "dependencies": {
		"@acme/b": "^1.0.0", "@corp/secret": "^2.0.0", "@other/thing": "^1.0.0", "left-pad": "^1.3.0"}}`)
	write(t, root, "packages/b/package.json", `{"name": "@acme/b"}`)
	write(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n\nrequire (\n"+
		"\texample.com/app/lib v0.0.0\n\tgit.corp.example/team/tool v1.0.0\n\tgithub.com/google/uuid v1.6.0\n)\n")
	write(t, root, "lib/go.mod", "module example.com/app/lib\n\ngo 1.22\n")
	write(t, root, "py/pyproject.toml", "[project]\nname = \"Corp_Utils\"\n")
	write(t, root, "requirements.txt", "corp-utils\nrequests\n")

	deps, bad, err := Collect(root)
	if err != nil || len(bad) != 0 {
		t.Fatalf("Collect: %v, bad = %+v", err, bad)
	}
	if got, want := names(deps), []string{"github.com/google/uuid", "left-pad", "requests"}; !slices.Equal(got, want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
}
