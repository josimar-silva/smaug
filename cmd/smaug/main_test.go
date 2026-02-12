package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	// When: The run function is called
	result := run()

	// Then: It returns the expected application name
	expected := "Smaug - Power Aware Reverse Proxy"
	assert.Equal(t, expected, result)
}
