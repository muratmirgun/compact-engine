package token

import (
	"testing"

	"github.com/muratmirgun/compact-engine/compact"
)

func TestCountingIsAdditiveAndSupportsUnicode(t *testing.T) {
	t.Parallel()
	for _, encoding := range []string{"o200k_base", "cl100k_base"} {
		t.Run(encoding, func(t *testing.T) {
			t.Parallel()
			c, err := New(encoding)
			if err != nil {
				t.Fatal(err)
			}
			a := compact.Message{ID: "a", Role: "user", Text: "Türkçe metin 🧠"}
			b := compact.Message{ID: "b", Role: "assistant", Text: "Hello"}
			x, err := c.Count([]compact.Message{a})
			if err != nil {
				t.Fatal(err)
			}
			y, err := c.Count([]compact.Message{b})
			if err != nil {
				t.Fatal(err)
			}
			z, err := c.Count([]compact.Message{a, b})
			if err != nil {
				t.Fatal(err)
			}
			if z != x+y || x <= 8 {
				t.Errorf("Count() x=%d y=%d z=%d, want additive nonempty counts", x, y, z)
			}
		})
	}
}
