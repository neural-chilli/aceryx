package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginInitGoStep(t *testing.T) {
	withTempCWD(t, func() {
		var out bytes.Buffer
		if err := runPluginInit([]string{"--lang=go", "--type=step", "--name=my-plugin"}, &out); err != nil {
			t.Fatalf("runPluginInit failed: %v", err)
		}
		mustExist(t, "my-plugin/manifest.yaml")
		mustExist(t, "my-plugin/main.go")
		mustExist(t, "my-plugin/main_test.go")
		mustExist(t, "my-plugin/Makefile")
		mustExist(t, "my-plugin/README.md")
		mustExist(t, "my-plugin/.gitignore")

		manifest := mustRead(t, "my-plugin/manifest.yaml")
		if !strings.Contains(manifest, "type: step") {
			t.Fatalf("manifest missing step type:\n%s", manifest)
		}
	})
}

func TestPluginInitGoTrigger(t *testing.T) {
	withTempCWD(t, func() {
		var out bytes.Buffer
		if err := runPluginInit([]string{"--lang=go", "--type=trigger", "--name=my-trigger"}, &out); err != nil {
			t.Fatalf("runPluginInit failed: %v", err)
		}
		mustExist(t, "my-trigger/manifest.yaml")
		mustExist(t, "my-trigger/main.go")
		manifest := mustRead(t, "my-trigger/manifest.yaml")
		if !strings.Contains(manifest, "type: trigger") || !strings.Contains(manifest, "trigger_contract:") {
			t.Fatalf("manifest missing trigger fields:\n%s", manifest)
		}
	})
}

func TestPluginInitRustStep(t *testing.T) {
	withTempCWD(t, func() {
		var out bytes.Buffer
		if err := runPluginInit([]string{"--lang=rust", "--type=step", "--name=rusty"}, &out); err != nil {
			t.Fatalf("runPluginInit failed: %v", err)
		}
		mustExist(t, "rusty/manifest.yaml")
		mustExist(t, "rusty/Cargo.toml")
		mustExist(t, "rusty/src/lib.rs")
	})
}

func TestPluginValidateManifestRules(t *testing.T) {
	withTempCWD(t, func() {
		valid := `
id: good-plugin
name: Good
version: 1.2.3
type: step
category: Custom
tier: open_source
maturity: community
min_host_version: 0.0.1
tool_capable: false
host_functions: [host_log]
`
		if err := os.WriteFile("manifest.yaml", []byte(valid), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		var out bytes.Buffer
		if err := validateManifestFile("manifest.yaml", &out); err != nil {
			t.Fatalf("expected valid manifest, got %v\n%s", err, out.String())
		}

		invalid := `
name: Bad
version: bad
type: trigger
category: Custom
tier: open_source
maturity: community
min_host_version: x
tool_capable: true
host_functions: [host_unknown]
`
		if err := os.WriteFile("invalid.yaml", []byte(invalid), 0o644); err != nil {
			t.Fatalf("write invalid manifest: %v", err)
		}
		out.Reset()
		if err := validateManifestFile("invalid.yaml", &out); err == nil {
			t.Fatalf("expected validation failure")
		}
		if !strings.Contains(out.String(), "✗") {
			t.Fatalf("expected failed checks output, got:\n%s", out.String())
		}
	})
}

func TestPluginBuildInvokesGoToolchain(t *testing.T) {
	withTempCWD(t, func() {
		manifest := `
id: build-plugin
name: Build
version: 1.0.0
type: step
category: Custom
tier: open_source
maturity: community
min_host_version: 0.0.1
host_functions: [host_log]
`
		if err := os.WriteFile("manifest.yaml", []byte(manifest), 0o644); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		if err := os.WriteFile("main.go", []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
			t.Fatalf("write main.go: %v", err)
		}

		origExec := pluginExecRunner
		origCheck := pluginWASMCheck
		t.Cleanup(func() {
			pluginExecRunner = origExec
			pluginWASMCheck = origCheck
		})

		calls := make([]string, 0, 1)
		pluginExecRunner = func(dir, name string, args ...string) error {
			calls = append(calls, name+" "+strings.Join(args, " "))
			return os.WriteFile(filepath.Join(dir, "plugin.wasm"), []byte("wasm-bytes"), 0o644)
		}
		pluginWASMCheck = func(path, pluginType string) error { return nil }

		var out bytes.Buffer
		if err := runPluginBuild(nil, &out); err != nil {
			t.Fatalf("runPluginBuild failed: %v", err)
		}
		if len(calls) != 1 || !strings.Contains(calls[0], "tinygo build") {
			t.Fatalf("unexpected toolchain calls: %v", calls)
		}
		if !strings.Contains(out.String(), "sha-256:") {
			t.Fatalf("expected hash output, got %s", out.String())
		}
	})
}

func mustExist(t *testing.T, rel string) {
	t.Helper()
	if _, err := os.Stat(rel); err != nil {
		t.Fatalf("expected %s to exist: %v", rel, err)
	}
}

func mustRead(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

func withTempCWD(t *testing.T, fn func()) {
	t.Helper()
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		_ = os.Chdir(old)
	}()
	fn()
}
