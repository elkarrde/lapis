package rename

import (
	"testing"
)

func TestScramble(t *testing.T) {
	name, err := NewName(ModeScramble)
	if err != nil {
		t.Fatal(err)
	}
	if len(name) != 8 {
		t.Errorf("scramble: got %q (len %d), want 8 chars", name, len(name))
	}

	// two calls should differ (with overwhelming probability)
	name2, _ := NewName(ModeScramble)
	if name == name2 {
		t.Error("scramble: two calls returned identical values")
	}
}

func TestUUID4(t *testing.T) {
	name, err := NewName(ModeUUID)
	if err != nil {
		t.Fatal(err)
	}
	// xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	if len(name) != 36 {
		t.Errorf("uuid4: got %q (len %d), want 36", name, len(name))
	}
	if name[14] != '4' {
		t.Errorf("uuid4: version nibble at pos 14 = %c, want '4'", name[14])
	}
	variant := name[19]
	if variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("uuid4: variant nibble at pos 19 = %c, want 8/9/a/b", variant)
	}
}
