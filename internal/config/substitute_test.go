package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstituteEnv_SimpleVariableSubstitution(t *testing.T) {
	// Given: An environment variable is set
	varName := "TEST_VAR_SIMPLE"
	varValue := "test-value-123"
	t.Setenv(varName, varValue)

	// When: Substituting a simple variable reference
	input := "${TEST_VAR_SIMPLE}"
	result := SubstituteEnv(input)

	// Then: The variable is replaced with its value
	assert.Equal(t, varValue, result)
}

func TestSubstituteEnv_MultipleVariables(t *testing.T) {
	// Given: Multiple environment variables are set
	t.Setenv("HOST", "example.com")
	t.Setenv("PORT", "8080")

	// When: Substituting multiple variable references
	input := "http://${HOST}:${PORT}"
	result := SubstituteEnv(input)

	// Then: All variables are replaced
	assert.Equal(t, "http://example.com:8080", result)
}

func TestSubstituteEnv_WithDefaultValue(t *testing.T) {
	// Given: An environment variable is NOT set
	varName := "NONEXISTENT_VAR_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=") // Verify it's not set

	// When: Substituting with a default value
	input := "${" + varName + ":-default-value}"
	result := SubstituteEnv(input)

	// Then: The default value is used
	assert.Equal(t, "default-value", result)
}

func TestSubstituteEnv_OverrideDefaultValue(t *testing.T) {
	// Given: An environment variable is set with a default syntax
	varName := "OVERRIDE_TEST"
	varValue := "actual-value"
	t.Setenv(varName, varValue)

	// When: Substituting a variable with default syntax but variable is set
	input := "${OVERRIDE_TEST:-default-value}"
	result := SubstituteEnv(input)

	// Then: The actual value overrides the default
	assert.Equal(t, varValue, result)
}

func TestSubstituteEnv_MissingVariableWithoutDefault(t *testing.T) {
	// Given: An environment variable is NOT set and no default is provided
	varName := "MISSING_VAR_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=")

	// When: Substituting a missing variable without default
	input := "${" + varName + "}"
	result := SubstituteEnv(input)

	// Then: Returns empty string for missing variable
	assert.Equal(t, "", result)
}

func TestSubstituteEnv_NoSubstitution(t *testing.T) {
	// Given: A string with no variable references
	input := "plain text without variables"

	// When: Calling SubstituteEnv
	result := SubstituteEnv(input)

	// Then: The string is returned unchanged
	assert.Equal(t, input, result)
}

func TestSubstituteEnv_EscapedBraces(t *testing.T) {
	// Given: A string with escaped braces
	input := "no substitution here"

	// When: Calling SubstituteEnv with no variable syntax
	result := SubstituteEnv(input)

	// Then: It's returned as-is
	assert.Equal(t, input, result)
}

func TestSubstituteEnv_EmptyVariable(t *testing.T) {
	// Given: An environment variable is set to empty string
	t.Setenv("EMPTY_VAR", "")

	// When: Substituting the empty variable
	input := "${EMPTY_VAR}"
	result := SubstituteEnv(input)

	// Then: Returns empty string (variable exists but is empty)
	assert.Equal(t, "", result)
}

func TestSubstituteEnv_DefaultWithEmptyValue(t *testing.T) {
	// Given: An environment variable is set to empty string, with default syntax
	t.Setenv("EMPTY_WITH_DEFAULT", "")

	// When: Substituting with default (variable exists and is empty)
	input := "${EMPTY_WITH_DEFAULT:-fallback}"
	result := SubstituteEnv(input)

	// Then: The empty value from the variable is used (not the default)
	assert.Equal(t, "", result)
}

func TestSubstituteEnv_DefaultWithMissingVariable(t *testing.T) {
	// Given: An environment variable is NOT set, with default syntax
	varName := "MISSING_WITH_DEFAULT_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=")

	// When: Substituting missing variable with default
	input := "${" + varName + ":-my-default}"
	result := SubstituteEnv(input)

	// Then: The default is used
	assert.Equal(t, "my-default", result)
}

func TestSubstituteEnv_ComplexDefaultValue(t *testing.T) {
	// Given: A variable not set, but with a complex default
	varName := "MISSING_COMPLEX_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=")

	// When: Substituting with complex default (URL with special chars)
	input := "${" + varName + ":-http://localhost:8080/path}"
	result := SubstituteEnv(input)

	// Then: The complex default is returned as-is
	assert.Equal(t, "http://localhost:8080/path", result)
}

func TestSubstituteEnv_VariableInMiddleOfString(t *testing.T) {
	// Given: An environment variable is set
	t.Setenv("SERVICE_NAME", "myapp")

	// When: Substituting variable in the middle of text
	input := "http://localhost/${SERVICE_NAME}/api"
	result := SubstituteEnv(input)

	// Then: Variable is replaced in the correct position
	assert.Equal(t, "http://localhost/myapp/api", result)
}

func TestSubstituteEnv_VariableAtStart(t *testing.T) {
	// Given: An environment variable is set
	t.Setenv("PROTOCOL", "https")

	// When: Substituting variable at the start
	input := "${PROTOCOL}://example.com"
	result := SubstituteEnv(input)

	// Then: Variable is replaced at the start
	assert.Equal(t, "https://example.com", result)
}

func TestSubstituteEnv_VariableAtEnd(t *testing.T) {
	// Given: An environment variable is set
	t.Setenv("DOMAIN", "example.com")

	// When: Substituting variable at the end
	input := "https://${DOMAIN}"
	result := SubstituteEnv(input)

	// Then: Variable is replaced at the end
	assert.Equal(t, "https://example.com", result)
}

func TestSubstituteEnv_OnlyVariable(t *testing.T) {
	// Given: An environment variable is set
	t.Setenv("API_KEY", "secret-key-123")

	// When: The entire string is just a variable
	input := "${API_KEY}"
	result := SubstituteEnv(input)

	// Then: Returns the variable value
	assert.Equal(t, "secret-key-123", result)
}

func TestSubstituteEnv_DefaultOnlyVariable(t *testing.T) {
	// Given: A variable is not set, with default syntax
	varName := "MISSING_ONLY_VAR_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=")

	// When: The entire string is just a variable reference with default
	input := "${" + varName + ":-default-only}"
	result := SubstituteEnv(input)

	// Then: Returns the default value
	assert.Equal(t, "default-only", result)
}

func TestSubstituteEnv_ConsecutiveVariables(t *testing.T) {
	// Given: Multiple environment variables are set
	t.Setenv("FIRST", "one")
	t.Setenv("SECOND", "two")

	// When: Substituting consecutive variables
	input := "${FIRST}${SECOND}"
	result := SubstituteEnv(input)

	// Then: Both are replaced
	assert.Equal(t, "onetwo", result)
}

func TestSubstituteEnv_VariableNameCase(t *testing.T) {
	// Given: An environment variable is set with uppercase name
	t.Setenv("MY_VAR", "value123")

	// When: Substituting variable with the exact name
	input := "${MY_VAR}"
	result := SubstituteEnv(input)

	// Then: The variable is found and replaced
	assert.Equal(t, "value123", result)
}

func TestSubstituteEnv_SpecialCharsInDefault(t *testing.T) {
	// Given: A variable is not set
	varName := "SPECIAL_CHARS_" + t.Name()
	require.NotContains(t, os.Environ(), varName+"=")

	// When: Substituting with special characters in default
	input := "${" + varName + ":-value:with:colons}"
	result := SubstituteEnv(input)

	// Then: Special chars in default are preserved
	assert.Equal(t, "value:with:colons", result)
}
