package services

import "testing"

func TestDefaultImagePullPolicy_Default(t *testing.T) {
	t.Setenv("IMAGE_PULL_POLICY", "")
	got := defaultImagePullPolicy()
	if got != "IfNotPresent" {
		t.Fatalf("expected IfNotPresent, got %q", got)
	}
}

func TestDefaultImagePullPolicy_IgnoresEnvOverride(t *testing.T) {
	for _, envValue := range []string{"Always", "Never", "IfNotPresent", "   "} {
		t.Run(envValue, func(t *testing.T) {
			t.Setenv("IMAGE_PULL_POLICY", envValue)
			got := defaultImagePullPolicy()
			if got != "IfNotPresent" {
				t.Fatalf("expected IfNotPresent, got %q", got)
			}
		})
	}
}

func TestBuildRuntimeConfig_HermesUsesWebtopDefaults(t *testing.T) {
	config := buildRuntimeConfig("hermes", "hermes", "latest", nil, nil)

	if config.Port != 3001 {
		t.Fatalf("expected Hermes port 3001, got %d", config.Port)
	}
	if config.MountPath != "/config" {
		t.Fatalf("expected Hermes mount path /config, got %q", config.MountPath)
	}
	if config.Env["HERMES_HOME"] != "/config/.hermes" {
		t.Fatalf("expected Hermes HERMES_HOME /config/.hermes, got %q", config.Env["HERMES_HOME"])
	}
	if config.Env["SUBFOLDER"] != "/" {
		t.Fatalf("expected Hermes default SUBFOLDER /, got %q", config.Env["SUBFOLDER"])
	}
	if config.Env["KASM_SVC_SEND_CUT_TEXT"] != kasmClipboardSendEnabled {
		t.Fatalf("expected Hermes to enable outbound clipboard sync, got %q", config.Env["KASM_SVC_SEND_CUT_TEXT"])
	}
	if config.Env["KASM_SVC_ACCEPT_CUT_TEXT"] != kasmClipboardAcceptEnabled {
		t.Fatalf("expected Hermes to enable inbound clipboard sync, got %q", config.Env["KASM_SVC_ACCEPT_CUT_TEXT"])
	}
	assertSelkiesClipboardEnabled(t, config.Env)
	if !usesWebtopImage("hermes") {
		t.Fatalf("expected Hermes to use webtop proxy behavior")
	}
}

func TestBuildRuntimeConfig_OpenClawEnablesDesktopClipboardSync(t *testing.T) {
	config := buildRuntimeConfig("openclaw", "openclaw", "latest", nil, nil)

	if config.Env["KASM_SVC_SEND_CUT_TEXT"] != kasmClipboardSendEnabled {
		t.Fatalf("expected OpenClaw to enable outbound clipboard sync, got %q", config.Env["KASM_SVC_SEND_CUT_TEXT"])
	}
	if config.Env["KASM_SVC_ACCEPT_CUT_TEXT"] != kasmClipboardAcceptEnabled {
		t.Fatalf("expected OpenClaw to enable inbound clipboard sync, got %q", config.Env["KASM_SVC_ACCEPT_CUT_TEXT"])
	}
	assertSelkiesClipboardEnabled(t, config.Env)
}

func TestBuildRuntimeConfig_WorkbuddyUsesWindowsDefaults(t *testing.T) {
	config := buildRuntimeConfig("workbuddy", "workbuddy", "latest", nil, nil)

	if config.Image != defaultSystemImageSettings["workbuddy"] {
		t.Fatalf("expected Workbuddy default image %q, got %q", defaultSystemImageSettings["workbuddy"], config.Image)
	}
	if config.Port != 8006 {
		t.Fatalf("expected Workbuddy port 8006, got %d", config.Port)
	}
	if config.MountPath != "/storage" {
		t.Fatalf("expected Workbuddy mount path /storage, got %q", config.MountPath)
	}
	for key, want := range map[string]string{
		"VERSION":   "10l",
		"DISK_SIZE": "64G",
		"DISK_FMT":  "qcow2",
		"SHUTDOWN":  "Y",
	} {
		if got := config.Env[key]; got != want {
			t.Fatalf("expected Workbuddy %s=%q, got %q", key, want, got)
		}
	}
	if usesWebtopImage("workbuddy") {
		t.Fatalf("expected Workbuddy to use Windows proxy behavior")
	}
	if usesHTTPSUpstream("workbuddy", 8006) {
		t.Fatalf("expected Workbuddy to use HTTP upstream proxying")
	}
}

func TestBuildRuntimeConfig_CodexUsesWindowsDefaults(t *testing.T) {
	config := buildRuntimeConfig("codex", "codex", "latest", nil, nil)

	if config.Image != defaultSystemImageSettings["codex"] {
		t.Fatalf("expected Codex default image %q, got %q", defaultSystemImageSettings["codex"], config.Image)
	}
	if config.Port != 8006 || config.MountPath != "/storage" {
		t.Fatalf("unexpected Windows Codex config: %#v", config)
	}
	for key, want := range map[string]string{
		"VERSION":   "11",
		"LANGUAGE":  "Chinese",
		"REGION":    "zh-CN",
		"KEYBOARD":  "zh-CN",
		"DISK_SIZE": "80G",
		"DISK_FMT":  "qcow2",
	} {
		if got := config.Env[key]; got != want {
			t.Fatalf("expected Codex %s=%q, got %q", key, want, got)
		}
	}
	if usesWebtopImage("codex") || usesHTTPSUpstream("codex", 8006) {
		t.Fatal("Windows Codex must use noVNC HTTP proxy behavior")
	}
}

func assertSelkiesClipboardEnabled(t *testing.T, env map[string]string) {
	t.Helper()
	for _, key := range []string{
		"SELKIES_CLIPBOARD_ENABLED",
		"SELKIES_CLIPBOARD_IN_ENABLED",
		"SELKIES_CLIPBOARD_OUT_ENABLED",
	} {
		if got := env[key]; got != selkiesClipboardEnabled {
			t.Fatalf("expected %s=%q, got %q", key, selkiesClipboardEnabled, got)
		}
	}
}
