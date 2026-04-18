package intake

import "testing"

func TestMainEntryPolicyNormalizeRequestedAgent(t *testing.T) {
	policy := MainEntryPolicy{
		ResolveMainAgentName: func() string { return "AnyClaw" },
	}

	tests := []struct {
		name      string
		agent     string
		assistant string
		want      string
		wantErr   bool
	}{
		{name: "empty defaults to main", want: "AnyClaw"},
		{name: "main alias", agent: "main-agent", want: "AnyClaw"},
		{name: "main exact name", agent: "AnyClaw", want: "AnyClaw"},
		{name: "assistant alias", assistant: "default agent", want: "AnyClaw"},
		{name: "specialist rejected", agent: "Go Expert", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := policy.NormalizeRequestedAgent(tc.agent, tc.assistant)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMainEntryPolicyNormalizeSelectionList(t *testing.T) {
	policy := MainEntryPolicy{
		ResolveMainAgentName: func() string { return "AnyClaw" },
	}

	items, err := policy.NormalizeSelectionList("", "main", "AnyClaw")
	if err != nil {
		t.Fatalf("NormalizeSelectionList: %v", err)
	}
	if len(items) != 1 || items[0] != "AnyClaw" {
		t.Fatalf("expected [AnyClaw], got %#v", items)
	}

	items, err = policy.NormalizeSelectionList()
	if err != nil {
		t.Fatalf("NormalizeSelectionList empty: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty selection list, got %#v", items)
	}

	if _, err := policy.NormalizeSelectionList("Go Expert"); err == nil {
		t.Fatal("expected specialist selection to fail")
	}
}
