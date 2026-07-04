package audio

import "testing"

func TestPickSearchCandidatesSkipsNullAndEmptyEntries(t *testing.T) {
	data := []byte(`{"id":"query-id","entries":[null,{"id":"","duration":10},{"id":"abc123","duration":215.0}]}`)
	got := pickSearchCandidates(data, 0)
	if len(got) != 1 || got[0].ID != "abc123" {
		t.Fatalf("expected single candidate abc123, got %+v", got)
	}
}

func TestPickSearchCandidatesDurationRanking(t *testing.T) {
	data := []byte(`{"entries":[
		{"id":"compilation","duration":3600},
		{"id":"match","duration":214},
		{"id":"short","duration":60}
	]}`)
	got := pickSearchCandidates(data, 215000)
	if len(got) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(got))
	}
	if got[0].ID != "match" || got[1].ID != "short" || got[2].ID != "compilation" {
		t.Fatalf("wrong ranking order: %+v", got)
	}
}

func TestPickSearchCandidatesNoDurationKeepsSearchOrder(t *testing.T) {
	data := []byte(`{"entries":[
		{"id":"first","duration":3600},
		{"id":"second","duration":214}
	]}`)
	got := pickSearchCandidates(data, 0)
	if len(got) != 2 || got[0].ID != "first" || got[1].ID != "second" {
		t.Fatalf("expected search order preserved, got %+v", got)
	}
}

func TestPickSearchCandidatesEmptyOrInvalid(t *testing.T) {
	if got := pickSearchCandidates([]byte(`{"entries":[]}`), 0); len(got) != 0 {
		t.Fatalf("expected no candidates, got %+v", got)
	}
	if got := pickSearchCandidates([]byte(`not json`), 0); len(got) != 0 {
		t.Fatalf("expected no candidates for invalid json, got %+v", got)
	}
	if got := pickSearchCandidates([]byte(`{"id":"only-playlist-id"}`), 0); len(got) != 0 {
		t.Fatalf("playlist id must not become candidate, got %+v", got)
	}
}
