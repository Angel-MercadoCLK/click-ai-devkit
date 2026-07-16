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
