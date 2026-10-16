package ownedprobe

import (
	"bytes"
	"strings"
	"testing"
)

func TestProofRejectsHostCredentialKeysWithoutExposingValues(t *testing.T) {
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "SSH_AUTH_SOCK", "DOCKER_AUTH_CONFIG", "GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CLIENT_SECRET"} {
		t.Run(key, func(t *testing.T) {
			s, _, _, calls := dynamicAdapter()
			const value = "owned-test-credential-not-a-real-secret"
			s.proof.lookup = func(name string) (string, bool) { *calls = append(*calls, "lookup:"+name); return value, name == key }
			var out, stderr bytes.Buffer
			code := Run([]string{"--fixture=smoke-v1", "--config=owned-proof-v1"}, s, &out, &stderr)
			if code != 1 || stderr.String() != "owned-smoke: owned stderr\nowned-smoke: unavailable\n" || !strings.Contains(out.String(), `"name":"host_credentials_absent","state":"UNVERIFIABLE"`) || strings.Contains(out.String(), value) || strings.Contains(stderr.String(), value) {
				t.Fatalf("credential rule failed: exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			for _, call := range *calls {
				if call == "cpu" || strings.HasPrefix(call, "child:") {
					t.Fatal("resource demand after credential rejection")
				}
			}
		})
	}
}
