package management

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestPathsRejectTraversalAndAlternateEncodings(t *testing.T) {
	for _, p := range []string{"", "relative", "/..", "/../x", "/a/../b", "/./x", "/a//b", "/x/", "/%2e%2e/x", "/%252e%252e/x", "/x\\y", "/x\x00y", "/x\ny", "/"} {
		if _, err := Path(p, false); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
	for _, p := range []string{"/server.properties", "/world/region/r.0.0.mca", "/plugins/Example.jar", "/a b/日本語.txt"} {
		if got, err := Path(p, false); err != nil || got != p {
			t.Errorf("rejected %q: %v", p, err)
		}
	}
	if _, err := Path("/", true); err != nil {
		t.Fatal(err)
	}
}
func TestJavaPropertiesParsingAndEscaping(t *testing.T) {
	lines, err := parseProperties("# keep\r\nlevel-name : My\\ World\r\nmo\\td=Hello\\u0021\nmotd=Last\\\n  value\nunknown\\ key = preserve\\:me\n")
	if err != nil {
		t.Fatal(err)
	}
	props := map[string]string{}
	for _, v := range lines {
		if v.key != "" {
			props[v.key] = v.value
		}
	}
	if props["level-name"] != "My World" || props["motd"] != "Lastvalue" || props["unknown key"] != "preserve:me" {
		t.Fatal(props)
	}
	for _, v := range []string{"Hi friends! # : = \\", "こんにちは 🌍", " leading and trailing "} {
		decoded, err := unescape(escapeProperty(v))
		if err != nil || decoded != v {
			t.Fatalf("round trip %q => %q %v", v, decoded, err)
		}
	}
	if _, err = parseProperties("motd=\\uZZZZ"); err == nil {
		t.Fatal("invalid escape accepted")
	}
	if _, err = unescape("\\ud800"); err == nil {
		t.Fatal("unpaired surrogate accepted")
	}
	if lines[0].raw != "# keep\r\n" || !strings.Contains(lines[len(lines)-2].raw, "unknown") {
		t.Fatal("unmanaged lines changed")
	}
}
func pluginJar(t *testing.T, descriptor string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("name: Test\nmain: dev.test.Plugin\nversion: 1.0\n"))
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestPluginValidation(t *testing.T) {
	if err := validatePlugin("test.jar", pluginJar(t, "plugin.yml")); err != nil {
		t.Fatal(err)
	}
	if err := validatePlugin("test.jar", pluginJar(t, "paper-plugin.yml")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		name string
		data []byte
	}{{"evil.txt", pluginJar(t, "plugin.yml")}, {"fake.jar", []byte("shell command")}, {"library.jar", pluginJar(t, "META-INF/MANIFEST.MF")}} {
		if validatePlugin(v.name, v.data) == nil {
			t.Fatal("invalid plugin accepted", v.name)
		}
	}
}
