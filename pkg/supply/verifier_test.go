package supply

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifier(t *testing.T) {
	v := NewVerifier()
	assert.NotNil(t, v)
}
