package programme

import (
	"errors"
	"testing"
)

func TestActivitySuggestions(t *testing.T) {
	st, _ := newStore(t)
	if set, _ := st.GetSuggestSettings(1); set.HasToken || set.QROnPDF {
		t.Fatalf("a new squadron has suggestions on: %+v", set)
	}
	if err := st.SetSuggestQROnPDF(1, true); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("QR on the PDF without a link: %v", err)
	}
	token, err := st.NewSuggestToken(1)
	if err != nil || len(token) < 20 {
		t.Fatalf("token %q, %v", token, err)
	}
	if sq, err := st.SquadronBySuggestToken(token); err != nil || sq.ID != 1 {
		t.Errorf("by token: %v", err)
	}
	if _, err := st.SquadronBySuggestToken("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown token: %v", err)
	}
	if err := st.SetSuggestQROnPDF(1, true); err != nil {
		t.Fatal(err)
	}
	if set, _ := st.GetSuggestSettings(1); !set.QROnPDF {
		t.Error("the QR option was not kept")
	}
	token2, _ := st.NewSuggestToken(1)
	if token2 == token {
		t.Error("a new link has the old token")
	}
	if _, err := st.SquadronBySuggestToken(token); !errors.Is(err, ErrNotFound) {
		t.Error("the old link still works")
	}

	add := func(name string) bool {
		t.Helper()
		ok, err := st.AddSuggestion(1, name)
		if err != nil {
			t.Fatalf("AddSuggestion(%q): %v", name, err)
		}
		return ok
	}
	if !add("  Paintballing ") || add("paintballing") || add("Bowling") {
		t.Error("new, repeated or built-in suggestions were not handled")
	}
	for _, bad := range []string{"", "x", "bad\x07name"} {
		if _, err := st.AddSuggestion(1, bad); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("AddSuggestion(%q) = %v", bad, err)
		}
	}
	add("Kayaking")
	list, _ := st.ListSuggestions(1)
	if len(list) != 2 || list[0].Name != "Kayaking" || list[0].Added {
		t.Fatalf("list = %+v", list)
	}
	if other, _ := st.ListSuggestions(2); len(other) != 0 {
		t.Error("another squadron sees them")
	}
	if _, err := st.MakeSuggestionActivity(2, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron made an activity: %v", err)
	}

	got, err := st.MakeSuggestionActivity(1, list[1].ID) // Paintballing
	if err != nil || !got.Added || got.ActivityTitle != "Paintballing" || got.ActivityID == 0 {
		t.Fatalf("make = %+v, %v", got, err)
	}
	if opts, _ := st.ActivityOptions(1); opts[len(opts)-1] != "Paintballing" {
		t.Errorf("options = %v", opts)
	}
	if _, err := st.MakeSuggestionActivity(1, list[1].ID); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("made an activity twice: %v", err)
	}
	if set, _ := st.GetSuggestSettings(1); set.Pending != 1 {
		t.Errorf("pending = %d, want 1", set.Pending)
	}
	// Added ones come after the waiting ones and show the activity's current name.
	custom, _ := st.ListCustomActivities(1)
	if err := st.UpdateActivity(1, custom[0].ID, "Paintball day"); err != nil {
		t.Fatal(err)
	}
	list, _ = st.ListSuggestions(1)
	if list[0].Added || !list[1].Added || list[1].ActivityTitle != "Paintball day" {
		t.Errorf("list after rename = %+v", list)
	}
	// If the activity is removed the suggestion still says what it became.
	if err := st.DeleteActivity(1, custom[0].ID); err != nil {
		t.Fatal(err)
	}
	list, _ = st.ListSuggestions(1)
	if !list[1].Added || list[1].ActivityTitle != "Paintballing" || list[1].ActivityID != 0 {
		t.Errorf("list after removal = %+v", list[1])
	}
	// A suggestion that matches an activity added meanwhile is linked to it.
	if err := st.AddActivity(1, "Kayaking"); err != nil {
		t.Fatal(err)
	}
	if got, err := st.MakeSuggestionActivity(1, list[0].ID); err != nil || got.ActivityTitle != "Kayaking" || got.ActivityID == 0 {
		t.Errorf("linking to an existing activity: %+v, %v", got, err)
	}
	if c, _ := st.ListCustomActivities(1); len(c) != 1 {
		t.Errorf("a copy was added: %+v", c)
	}
	if err := st.DeleteSuggestion(2, list[1].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("another squadron deleted one: %v", err)
	}
	if err := st.DeleteSuggestion(1, list[1].ID); err != nil {
		t.Fatal(err)
	}

	if err := st.DisableSuggestions(1); err != nil {
		t.Fatal(err)
	}
	if set, _ := st.GetSuggestSettings(1); set.HasToken || set.QROnPDF {
		t.Errorf("disabled = %+v", set)
	}
}

func TestSuggestionsAreLimited(t *testing.T) {
	st, _ := newStore(t)
	for i := 0; i < maxPendingSuggestions; i++ {
		if _, err := st.AddSuggestion(1, "Idea number "+string(rune('A'+i%26))+string(rune('a'+i/26%26))+string(rune('0'+i/676))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.AddSuggestion(1, "One too many"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("past the limit: %v", err)
	}
}
