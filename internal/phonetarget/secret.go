package phonetarget

import (
	"encoding/json"
	"fmt"
)

// Secret prevents accidental string formatting of a credential.
type Secret struct{ value string }

func NewSecret(value string) Secret { return Secret{value: value} }

func (s *Secret) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &s.value)
}

func (Secret) String() string   { return "[redacted]" }
func (Secret) GoString() string { return "[redacted]" }

// Format ignores the requested verb and flags so even unusual diagnostic
// formatting cannot bypass String or expose the backing value.
func (Secret) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted]"))
}

func (s Secret) reveal() string { return s.value }
func (s Secret) empty() bool    { return s.value == "" }
