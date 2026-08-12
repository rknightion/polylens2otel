package phonetarget

import (
	"encoding/json"
	"fmt"
)

// Secret prevents accidental string formatting of a credential. The backing
// string is indirect so fmt's special %p handling cannot dump the value when
// it diagnoses a non-pointer operand without calling Format.
type Secret struct{ value *string }

func NewSecret(value string) Secret { return Secret{value: &value} }

func (s *Secret) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	s.value = &value
	return nil
}

func (Secret) String() string   { return "[redacted]" }
func (Secret) GoString() string { return "[redacted]" }

// Format ignores the requested verb and flags so even unusual diagnostic
// formatting cannot bypass String or expose the backing value.
func (Secret) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte("[redacted]"))
}

func (s Secret) reveal() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

func (s Secret) empty() bool { return s.value == nil || *s.value == "" }
