package normalize

import "testing"

func TestPlate_WithSeparators_CompactUppercase(t *testing.T) {
	tests := map[string]string{
		"LK53 WUO":     "LK53WUO",
		"lk53-wuo":     "LK53WUO",
		"WW–412–DX":    "WW412DX",
		" KLE QH 426 ": "KLEQH426",
	}
	for value, expected := range tests {
		actual := Plate(value)
		if actual != expected {
			t.Errorf("Plate(%q): expected %q, got %q", value, expected, actual)
		}
	}
}
