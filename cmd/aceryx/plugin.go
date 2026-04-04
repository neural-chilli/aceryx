package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/tetratelabs/wazero"
	"gopkg.in/yaml.v3"
)

const (
	aceryxHostVersion = "0.0.1"
)

var (
	pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

	pluginExecRunner = defaultPluginExecRunner
	pluginWASMCheck  = validateWASMFile
)

//go:embed templates/*/*.tmpl
var pluginTemplateFS embed.FS

type pluginManifestForCLI struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	Version         string   `yaml:"version"`
	Type            string   `yaml:"type"`
	Category        string   `yaml:"category"`
	Tier            string   `yaml:"tier"`
	Maturity        string   `yaml:"maturity"`
	MinHostVersion  string   `yaml:"min_host_version"`
	ToolCapable     bool     `yaml:"tool_capable"`
	ToolDescription string   `yaml:"tool_description"`
	ToolSafety      string   `yaml:"tool_safety"`
	HostFunctions   []string `yaml:"host_functions"`
}

func runPlugin(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: aceryx plugin [init|build|test|validate]")
	}

	switch args[0] {
	case "init":
		return runPluginInit(args[1:], os.Stdout)
	case "build":
		return runPluginBuild(args[1:], os.Stdout)
	case "test":
		return runPluginTest(args[1:], os.Stdout)
	case "validate":
		return runPluginValidate(args[1:], os.Stdout)
	default:
		return fmt.Errorf("unknown plugin subcommand: %s", args[0])
	}
}

func runPluginInit(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("plugin init", flag.ContinueOnError)
	lang := fs.String("lang", "go", "plugin language (go|rust)")
	pluginType := fs.String("type", "step", "plugin type (step|trigger)")
	name := fs.String("name", "", "plugin name/id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required")
	}
	if !pluginIDPattern.MatchString(*name) {
		return fmt.Errorf("invalid plugin name: must match %s", pluginIDPattern.String())
	}

	key, err := templateKey(*lang, *pluginType)
	if err != nil {
		return err
	}
	files, err := templateFiles(key)
	if err != nil {
		return err
	}
	targetDir := *name
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create plugin directory: %w", err)
	}

	data := map[string]any{
		"Name": *name,
		"ID":   *name,
		"Type": *pluginType,
	}

	for _, rel := range files {
		templatePath := filepath.ToSlash(filepath.Join("templates", key, rel))
		raw, err := pluginTemplateFS.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("read template %s: %w", templatePath, err)
		}
		parsed, err := template.New(rel).Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", templatePath, err)
		}

		var rendered bytes.Buffer
		if err := parsed.Execute(&rendered, data); err != nil {
			return fmt.Errorf("execute template %s: %w", templatePath, err)
		}

		outRel := outputPathFromTemplate(rel)
		targetPath := filepath.Join(targetDir, outRel)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", targetPath, err)
		}
		if err := os.WriteFile(targetPath, rendered.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}
	}

	_, _ = fmt.Fprintf(out, "created plugin scaffold: %s\n", targetDir)
	return nil
}

func runPluginBuild(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("plugin build", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	lang := detectPluginLanguage(".")
	manifest, err := readPluginManifest("manifest.yaml")
	if err != nil {
		return err
	}

	switch lang {
	case "go":
		if err := pluginExecRunner(".", "tinygo", "build", "-o", "plugin.wasm", "-target", "wasi", "./"); err != nil {
			return err
		}
	case "rust":
		if err := pluginExecRunner(".", "cargo", "build", "--target", "wasm32-wasi", "--release"); err != nil {
			return err
		}
		wasm, err := findRustWASM(".")
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(wasm)
		if err != nil {
			return fmt.Errorf("read built wasm: %w", err)
		}
		if err := os.WriteFile("plugin.wasm", raw, 0o644); err != nil {
			return fmt.Errorf("write plugin.wasm: %w", err)
		}
	default:
		return fmt.Errorf("unsupported plugin language")
	}

	size, hash, err := wasmSizeAndHash("plugin.wasm")
	if err != nil {
		return err
	}
	if err := pluginWASMCheck("plugin.wasm", manifest.Type); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "output: plugin.wasm (%d bytes)\n", size)
	_, _ = fmt.Fprintf(out, "sha-256: %s\n", hash)
	return nil
}

func runPluginTest(args []string, _ io.Writer) error {
	fs := flag.NewFlagSet("plugin test", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	lang := detectPluginLanguage(".")
	if lang == "rust" {
		return pluginExecRunner(".", "cargo", "test")
	}
	return pluginExecRunner(".", "go", "test", "./...")
}

func runPluginValidate(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("plugin validate", flag.ContinueOnError)
	manifestPath := fs.String("manifest", "manifest.yaml", "manifest path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return validateManifestFile(*manifestPath, out)
}

func validateManifestFile(path string, out io.Writer) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest pluginManifestForCLI
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("parse manifest yaml: %w", err)
	}
	meta := map[string]any{}
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("parse manifest metadata: %w", err)
	}

	type check struct {
		name string
		ok   bool
		msg  string
	}
	results := make([]check, 0, 8)

	required := []string{"id", "name", "version", "type", "category", "tier", "maturity", "min_host_version"}
	missing := make([]string, 0)
	for _, key := range required {
		value, ok := meta[key]
		if !ok || strings.TrimSpace(fmt.Sprintf("%v", value)) == "" {
			missing = append(missing, key)
		}
	}
	results = append(results, check{
		name: "required fields present",
		ok:   len(missing) == 0,
		msg:  strings.Join(missing, ", "),
	})

	results = append(results, check{
		name: "id format valid",
		ok:   pluginIDPattern.MatchString(manifest.ID),
		msg:  "must match " + pluginIDPattern.String(),
	})

	_, versionErr := parseSemver(manifest.Version)
	results = append(results, check{name: "version is semver", ok: versionErr == nil, msg: "invalid semver"})

	typeOK := manifest.Type == "step" || manifest.Type == "trigger"
	results = append(results, check{name: "type is step or trigger", ok: typeOK, msg: "type must be step or trigger"})

	_, minErr := parseSemver(manifest.MinHostVersion)
	compatible := minErr == nil
	if compatible {
		cmp, err := compareSemver(manifest.MinHostVersion, aceryxHostVersion)
		compatible = err == nil && cmp <= 0
	}
	results = append(results, check{
		name: "min_host_version valid and compatible",
		ok:   compatible,
		msg:  fmt.Sprintf("requires Aceryx >= %s", manifest.MinHostVersion),
	})

	_, hasTriggerContract := meta["trigger_contract"]
	triggerRulesOK := true
	triggerRuleMsg := ""
	if manifest.Type == "trigger" && !hasTriggerContract {
		triggerRulesOK = false
		triggerRuleMsg = "trigger plugins must include trigger_contract"
	}
	if manifest.Type == "step" && hasTriggerContract {
		triggerRulesOK = false
		triggerRuleMsg = "step plugins must not include trigger_contract"
	}
	results = append(results, check{name: "trigger contract rules", ok: triggerRulesOK, msg: triggerRuleMsg})

	toolRulesOK := true
	toolRuleMsg := ""
	if manifest.ToolCapable {
		if strings.TrimSpace(manifest.ToolDescription) == "" {
			toolRulesOK = false
			toolRuleMsg = "tool_description required when tool_capable is true"
		}
		switch manifest.ToolSafety {
		case "read_only", "idempotent_write", "side_effect":
		default:
			toolRulesOK = false
			if toolRuleMsg == "" {
				toolRuleMsg = "tool_safety required when tool_capable is true"
			}
		}
	}
	results = append(results, check{name: "tool_capable rules", ok: toolRulesOK, msg: toolRuleMsg})

	hostRulesOK := true
	hostRuleMsg := ""
	for _, fn := range manifest.HostFunctions {
		if _, ok := validHostFunctions()[fn]; !ok {
			hostRulesOK = false
			hostRuleMsg = "invalid host function: " + fn
			break
		}
	}
	results = append(results, check{name: "host_functions valid", ok: hostRulesOK, msg: hostRuleMsg})

	failed := false
	for _, r := range results {
		mark := "✓"
		if !r.ok {
			mark = "✗"
			failed = true
		}
		if r.msg != "" && !r.ok {
			_, _ = fmt.Fprintf(out, "%s %s (%s)\n", mark, r.name, r.msg)
			continue
		}
		_, _ = fmt.Fprintf(out, "%s %s\n", mark, r.name)
	}
	if failed {
		return fmt.Errorf("manifest validation failed")
	}
	_, _ = fmt.Fprintln(out, "manifest valid")
	return nil
}

func templateKey(lang, pluginType string) (string, error) {
	switch lang {
	case "go":
		if pluginType == "step" {
			return "go-step", nil
		}
		if pluginType == "trigger" {
			return "go-trigger", nil
		}
	case "rust":
		if pluginType == "step" {
			return "rust-step", nil
		}
		return "", fmt.Errorf("rust trigger scaffolding is not available yet")
	}
	return "", fmt.Errorf("unsupported plugin template: lang=%s type=%s", lang, pluginType)
}

func templateFiles(key string) ([]string, error) {
	dir := filepath.ToSlash(filepath.Join("templates", key))
	entries, err := fs.ReadDir(pluginTemplateFS, dir)
	if err != nil {
		return nil, fmt.Errorf("read template directory %s: %w", key, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	return files, nil
}

func outputPathFromTemplate(name string) string {
	switch name {
	case "gitignore.tmpl":
		return ".gitignore"
	case "lib.rs.tmpl":
		return filepath.Join("src", "lib.rs")
	default:
		return strings.TrimSuffix(name, ".tmpl")
	}
}

func detectPluginLanguage(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "rust"
	}
	return "go"
}

func readPluginManifest(path string) (pluginManifestForCLI, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return pluginManifestForCLI{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest pluginManifestForCLI
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return pluginManifestForCLI{}, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Type != "step" && manifest.Type != "trigger" {
		return pluginManifestForCLI{}, fmt.Errorf("manifest type must be step or trigger")
	}
	return manifest, nil
}

func defaultPluginExecRunner(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func findRustWASM(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "target", "wasm32-wasi", "release", "*.wasm"))
	if err != nil {
		return "", fmt.Errorf("find rust wasm output: %w", err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", fmt.Errorf("no wasm output found in target/wasm32-wasi/release")
	}
	return matches[0], nil
}

func wasmSizeAndHash(path string) (int64, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, "", fmt.Errorf("read wasm: %w", err)
	}
	sum := sha256.Sum256(raw)
	return int64(len(raw)), hex.EncodeToString(sum[:]), nil
}

func validateWASMFile(path, pluginType string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read wasm: %w", err)
	}
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer func() { _ = rt.Close(ctx) }()

	mod, err := rt.CompileModule(ctx, raw)
	if err != nil {
		return fmt.Errorf("compile wasm: %w", err)
	}
	exports := mod.ExportedFunctions()

	missing := make([]string, 0, 3)
	if pluginType == "step" {
		if _, ok := exports["Execute"]; !ok {
			missing = append(missing, "Execute")
		}
	}
	if pluginType == "trigger" {
		if _, ok := exports["Start"]; !ok {
			missing = append(missing, "Start")
		}
		if _, ok := exports["Stop"]; !ok {
			missing = append(missing, "Stop")
		}
	}
	if _, ok := exports["aceryx_abi_version"]; !ok {
		missing = append(missing, "aceryx_abi_version")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing wasm exports: %s", strings.Join(missing, ", "))
	}
	return nil
}

func validHostFunctions() map[string]struct{} {
	return map[string]struct{}{
		"host_http_request":   {},
		"host_call_connector": {},
		"host_case_get":       {},
		"host_case_set":       {},
		"host_vault_read":     {},
		"host_vault_write":    {},
		"host_secret_get":     {},
		"host_log":            {},
		"host_config_get":     {},
		"host_create_case":    {},
		"host_emit_event":     {},
		"host_queue_consume":  {},
		"host_queue_ack":      {},
		"host_file_watch":     {},
	}
}

type semver struct {
	major int
	minor int
	patch int
}

func parseSemver(v string) (semver, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}, errors.New("invalid semver")
	}
	var out semver
	_, err := fmt.Sscanf(v, "%d.%d.%d", &out.major, &out.minor, &out.patch)
	if err != nil {
		return semver{}, err
	}
	return out, nil
}

func compareSemver(a, b string) (int, error) {
	sa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	sb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	if sa.major != sb.major {
		if sa.major < sb.major {
			return -1, nil
		}
		return 1, nil
	}
	if sa.minor != sb.minor {
		if sa.minor < sb.minor {
			return -1, nil
		}
		return 1, nil
	}
	if sa.patch != sb.patch {
		if sa.patch < sb.patch {
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
}
