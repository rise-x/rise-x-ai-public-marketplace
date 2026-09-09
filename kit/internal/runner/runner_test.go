package runner

import "testing"

func TestArgv(t *testing.T) {
	got := Argv("claude", []string{"plugin", "install", "rise-x apps@rise-x-public"})
	want := `claude plugin install "rise-x apps@rise-x-public"`
	if got != want {
		t.Fatalf("Argv() = %q, want %q", got, want)
	}
}

func TestRedact(t *testing.T) {
	// Every case the review's redaction probe listed, plus the lines that
	// must survive untouched.
	cases := map[string]string{
		"token=abc123":                             "[redacted]",
		"secret=abc123":                            "[redacted]",
		"Authorization abc.def":                    "[redacted]",
		"Authorization: Bearer eyJhbGciOi":         "[redacted]",
		"ANTHROPIC_API_KEY=sk-ant-api03-XXXXXXXX":  "ANTHROPIC_[redacted]",
		"apiKey=abcdef123456":                      "[redacted]",
		"api-key: abcdef123456":                    "[redacted]",
		"password: hunter2":                        "[redacted]",
		"passwd=hunter2":                           "[redacted]",
		"credential=zzz":                           "[redacted]",
		"--client-secret zzz":                      "--client-[redacted]",
		"ghp_0123456789abcdefghijklmnopqrstuvwxyz": "[redacted]",
		"github_pat_11ABCDEFG0123456789":           "[redacted]",
		"//npm.pkg.github.com/:_authToken=ghp_xxx": "//npm.pkg.github.com/:_auth[redacted]",
		"//packages.example.com/_packaging/x/npm/:_password=BASE64SECRET==": "//packages.example.com/_packaging/x/npm/:_[redacted]",

		"plain line, nothing sensitive":                   "plain line, nothing sensitive",
		"npx -y @upstash/context7-mcp":                    "npx -y @upstash/context7-mcp",
		"claude mcp list":                                 "claude mcp list",
		"claude plugin install rise-x-apps@rise-x-public": "claude plugin install rise-x-apps@rise-x-public",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
