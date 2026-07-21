package guard

import "testing"

func TestRedTeamBlocksForbiddenPayloads(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		category string
	}{
		{name: "email", payload: `{"content":"Customer email juan.perez@clickseguros.com.ar"}`, category: "pii"},
		{name: "dni", payload: `{"content":"DNI 12345678 asociado al caso"}`, category: "pii"},
		{name: "cuit", payload: `{"content":"CUIT 20-12345678-3 validado"}`, category: "pii"},
		{name: "phone", payload: `{"content":"Teléfono +54 11 5555-1234 del asegurado"}`, category: "pii"},
		{name: "amount peso", payload: `{"content":"Reserva aprobada por $ 123.456,78"}`, category: "amounts"},
		{name: "amount ars", payload: `{"content":"Monto ARS 250000 pendiente"}`, category: "amounts"},
		{name: "policy number", payload: `{"title":"Guardar póliza POL-ABC12345"}`, category: "policy-numbers"},
		{name: "claim id", payload: `{"content":"Siniestro SIN-2026-000123 escalado"}`, category: "claim-ids"},
		{name: "customer identifier", payload: `{"topic_key":"customer-id CLI-998877"}`, category: "customer-identifiers"},
		// R1-001 coverage guard: the domain's day-to-day abbreviated forms — "Sin" for siniestro,
		// "Pol" for póliza — with a real digit-bearing ID must still block. These are the exact
		// abbreviations dropping the bare sin/pol alternatives would have silently let through.
		{name: "claim abbrev sin+digits", payload: `{"content":"Sin 123456 fue rechazado, revisar"}`, category: "claim-ids"},
		{name: "policy abbrev pol+digits", payload: `{"content":"Pol 12-345-6 vigente al día"}`, category: "policy-numbers"},
		// R1-002 coverage guard: an ID token whose only digit is at the START (index 0) must still
		// block — the digit requirement means "contains a digit ANYWHERE", not "at position >= 3".
		{name: "claim digit-first token", payload: `{"content":"siniestro 1ABCDE escalado"}`, category: "claim-ids"},
		// Finding 1 (CRITICAL): the secrets category previously did not exist at all, so any
		// credential pasted into a mem_save payload flowed straight into persistent memory. These
		// cases cover the high-confidence, low-false-positive credential formats.
		{name: "aws access key id", payload: `{"content":"AWS access key AKIAIOSFODNN7EXAMPLE leaked in commit"}`, category: "secrets"},
		{name: "aws secret access key", payload: `{"content":"aws_secret_access_key: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`, category: "secrets"},
		{name: "github personal token", payload: `{"content":"leaked token ghp_16C7e42F292c6912E7710c838347Ae178B4a in logs"}`, category: "secrets"},
		{name: "github fine-grained pat", payload: `{"content":"leaked github_pat_11AAAAAAA0abcdefghijklmnopqrstuvwxyz0123456789 in logs"}`, category: "secrets"},
		{name: "private key pem block", payload: `{"content":"-----BEGIN RSA PRIVATE KEY-----MIIEowIBAAKCAQEA-----END RSA PRIVATE KEY-----"}`, category: "secrets"},
		{name: "jwt", payload: `{"content":"session eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U leaked"}`, category: "secrets"},
		// Fixture is deliberately non-realistic (no long digit groups): a lifelike Slack token here
		// trips GitHub push protection and blocks the repo's own pushes. The rule only needs the
		// xox[baprs]- prefix plus [A-Za-z0-9-]+, so this still exercises the pattern faithfully.
		{name: "slack token", payload: `{"content":"slack webhook xoxb-EXAMPLE-FAKE-TOKEN-FOR-TESTS-ONLY exposed"}`, category: "secrets"},
		{name: "openai key", payload: `{"content":"OPENAI_API_KEY sk-1234567890abcdefghijklmnopqrstuvwxyzABCDEF committed"}`, category: "secrets"},
		{name: "anthropic key", payload: `{"content":"ANTHROPIC_API_KEY sk-ant-api03-1234567890abcdefghijklmnopqrstuvwxyz committed"}`, category: "secrets"},
		{name: "generic api_key assignment", payload: `{"content":"api_key: abcdef1234567890 in .env"}`, category: "secrets"},
		{name: "generic password assignment", payload: `{"content":"password=Str0ngP@ssw0rd12026 in config"}`, category: "secrets"},
		{name: "bearer token", payload: `{"content":"Authorization: Bearer AbCdEfGhIjKlMnOpQrStUvWxYz0123456789 sent"}`, category: "secrets"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := ScanWithError(tt.payload)
			if err != nil {
				t.Fatalf("ScanWithError() error = %v", err)
			}
			if !decision.Blocked {
				t.Fatalf("Scan() blocked = false, want true; decision = %+v", decision)
			}
			if decision.Category != tt.category {
				t.Fatalf("Scan() category = %q, want %q", decision.Category, tt.category)
			}
			if decision.Reason == "" {
				t.Fatal("Scan() returned an empty reason")
			}
		})
	}
}

func TestScan_AllowsBenignTechnicalKnowledge(t *testing.T) {
	cases := []string{
		`{"title":"ADR: switch installer copy strategy","content":"Store only architecture decisions and Go packaging gotchas."}`,
		`{"content":"Bugfix: Normalize Cobra exit handling for doctor and uninstall commands."}`,
		`{"topic_key":"architecture/installer-hooks","content":"Pattern: resolve Claude home via env override for tests."}`,
		// DNI tuning: the pii/dni rule now requires a document keyword (dni/documento/cedula/carnet)
		// before the digit run, mirroring the policy/claim/customer discriminator. A bare 7-8 digit
		// number with no such keyword — a release date (20260716), a build counter (1234567), an
		// issue number — is ordinary technical content and must NOT be blocked. The old bare
		// `\b[0-9]{7,8}\b` pattern false-positived on all of these.
		`{"content":"Release tag cut on 20260716, build counter 1234567 deployed to prod."}`,
		// Finding 1 negative-corpus guard: real technical content from this repo's own domain that
		// the new secrets/* rules must NOT block. If any new pattern is too broad, one of these will
		// start failing — fix the pattern, not the test.
		`{"content":"Root-caused the regression to commit 96b996d2f2af8baca22ac712d7712a4156c26692."}`,
		`{"content":"Correlation UUID 550e8400-e29b-41d4-a716-446655440000 attached to the trace."}`,
		`{"content":"Bumped the guard module to v0.5.0 in go.mod."}`,
		`{"content":"See internal/guard/patterns.yaml for the compiled rule set."}`,
		`{"content":"the token vocabulary is documented in result-contract.md"}`,
		`{"content":"Icon asset blob: iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="}`,
		`{"content":"Never log the password field in plaintext; hash it before storage."}`,
		`{"content":"There is no secret sauce here, just deterministic pattern matching."}`,
	}

	for _, payload := range cases {
		decision, err := ScanWithError(payload)
		if err != nil {
			t.Fatalf("ScanWithError() error = %v", err)
		}
		if decision.Blocked {
			t.Fatalf("Scan() blocked benign payload %q with decision %+v", payload, decision)
		}
	}
}

// TestScan_AllowsReviewAndSecurityVocabulary is the T2-3 regression corpus: ordinary English/Spanish
// technical prose that contains the placeholder keywords (claim, policy, customer, siniestro, and the
// former bare "sin"/"pol" alternatives) WITHOUT any real ID token must NOT be blocked. Before this
// fix the claim-ids/policy-numbers/customer-identifiers placeholder rules matched "<keyword> <any
// word>" (zero required delimiter, no digit), so normal review/security/memory-policy writing tripped
// the guard constantly (e.g. "claim identifier", "policy layer", "customer data", "sin conflicto").
// The tuned rules require the ID token to contain a digit — real IDs do, prose words do not — so this
// whole corpus must pass. If any of these blocks, the tuning has regressed toward the old
// over-matching behavior.
func TestScan_AllowsReviewAndSecurityVocabulary(t *testing.T) {
	cases := []string{
		`{"content":"The reviewer will claim a finding is real only with concrete evidence."}`,
		`{"content":"This finding is a claim that must be verified before the fix loop."}`,
		`{"content":"The claim-ids category blocks siniestro numbers with a digit token."}`,
		`{"content":"The memory policy is deny-by-default and the guard enforces it deterministically."}`,
		`{"content":"Update the CORS policy layer and the security policy enforcement middleware."}`,
		`{"topic_key":"architecture/policy-engine","content":"Refactor the policy resolution path."}`,
		`{"content":"Never persist customer data, customer identifiers, or customer records."}`,
		`{"content":"The customer boundary is enforced at the service layer, not the frontend."}`,
		`{"content":"Resolved sin conflicto; single source of truth since the last release."}`,
		`{"content":"Add a policy check and a claim validation step to the pipeline."}`,
	}

	for _, payload := range cases {
		decision, err := ScanWithError(payload)
		if err != nil {
			t.Fatalf("ScanWithError() error = %v", err)
		}
		if decision.Blocked {
			t.Fatalf("Scan() blocked benign review/security vocabulary %q with decision %+v", payload, decision)
		}
	}
}

func BenchmarkScanTypicalPayload(b *testing.B) {
	payload := `{"title":"Architecture decision","content":"Keep the installer deterministic, use Cobra, and write tests with TempDir.","topic_key":"architecture/memory-guard"}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Scan(payload)
	}
}
