package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSecretStringString(t *testing.T) {
	secret := SecretString{value: "super-secret-api-key"}

	result := secret.String()

	assert.Equal(t, "***REDACTED***", result, "String() should return redacted value")
}

func TestSecretStringStringEmpty(t *testing.T) {
	secret := SecretString{value: ""}

	result := secret.String()

	assert.Equal(t, "", result, "String() should return empty string for empty secrets")
}

func TestSecretStringMarshalText(t *testing.T) {
	secret := SecretString{value: "super-secret-token"}

	result, err := secret.MarshalText()

	require.NoError(t, err, "MarshalText should not return an error")
	assert.Equal(t, []byte("***REDACTED***"), result, "MarshalText() should return redacted value")
}

func TestSecretStringMarshalTextEmpty(t *testing.T) {
	secret := SecretString{value: ""}

	result, err := secret.MarshalText()

	require.NoError(t, err, "MarshalText should not return an error")
	assert.Equal(t, []byte(""), result, "MarshalText() should return empty bytes for empty secrets")
}

func TestSecretStringValue(t *testing.T) {
	expectedSecret := "my-actual-secret-value"
	secret := SecretString{value: expectedSecret}

	result := secret.Value()

	assert.Equal(t, expectedSecret, result, "Value() should return the actual secret value")
}

func TestSecretStringValueEmpty(t *testing.T) {
	secret := SecretString{value: ""}

	result := secret.Value()

	assert.Equal(t, "", result, "Value() should return empty string for empty secrets")
}

func TestSecretStringUnmarshalYAML(t *testing.T) {
	yamlContent := `apiKey: "my-secret-api-key"`

	type testConfig struct {
		APIKey SecretString `yaml:"apiKey"`
	}

	var config testConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)

	require.NoError(t, err, "UnmarshalYAML should not return an error")
	assert.Equal(t, "my-secret-api-key", config.APIKey.Value(), "UnmarshalYAML should set the actual secret value")
	assert.Equal(t, "***REDACTED***", config.APIKey.String(), "String() should still return redacted value after unmarshal")
}

func TestSecretStringUnmarshalYAMLEmpty(t *testing.T) {
	yamlContent := `apiKey: ""`

	type testConfig struct {
		APIKey SecretString `yaml:"apiKey"`
	}

	var config testConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)

	require.NoError(t, err, "UnmarshalYAML should not return an error")
	assert.Equal(t, "", config.APIKey.Value(), "UnmarshalYAML should set empty value")
	assert.Equal(t, "", config.APIKey.String(), "String() should return empty for empty secrets")
}

func TestSecretStringLogValuer(t *testing.T) {
	secret := SecretString{value: "super-secret-password"}

	logValue := secret.LogValue()

	assert.Equal(t, slog.StringValue("***REDACTED***"), logValue, "LogValue() should return redacted value")
}

func TestSecretStringLogValuerEmpty(t *testing.T) {
	secret := SecretString{value: ""}

	logValue := secret.LogValue()

	assert.Equal(t, slog.StringValue(""), logValue, "LogValue() should return empty string for empty secrets")
}

func TestSecretStringJSONMarshal(t *testing.T) {
	type testStruct struct {
		APIKey SecretString `json:"apiKey"`
	}
	testObj := testStruct{
		APIKey: SecretString{value: "super-secret-key"},
	}

	jsonBytes, err := json.Marshal(testObj)

	require.NoError(t, err, "JSON marshaling should not return an error")
	assert.Contains(t, string(jsonBytes), "***REDACTED***", "JSON should contain redacted value")
	assert.NotContains(t, string(jsonBytes), "super-secret-key", "JSON should not contain actual secret")
}

func TestSecretStringFmtPrintf(t *testing.T) {
	secret := SecretString{value: "super-secret-token"}

	type testStruct struct {
		Token SecretString
	}
	obj := testStruct{Token: secret}
	result := fmt.Sprintf("%+v", obj)

	assert.Contains(t, result, "***REDACTED***", "fmt.Sprintf should use redacted value")
	assert.NotContains(t, result, "super-secret-token", "fmt.Sprintf should not expose actual secret")
}

func TestSecretStringFmtPrintfVerb(t *testing.T) {
	secret := SecretString{value: "super-secret-password"}

	result := fmt.Sprintf("%v", secret)

	assert.Equal(t, "***REDACTED***", result, "fmt.Sprintf with %%v should use redacted value")
}

func TestGwaihirConfigSecretRedaction(t *testing.T) {
	config := GwaihirConfig{
		URL:    "http://example.com",
		APIKey: SecretString{value: "sensitive-api-key"},
	}

	result := fmt.Sprintf("%+v", config)

	assert.Contains(t, result, "***REDACTED***", "GwaihirConfig should show redacted API key")
	assert.NotContains(t, result, "sensitive-api-key", "GwaihirConfig should not expose actual API key")
}

func TestSleepOnLanConfigSecretRedaction(t *testing.T) {
	config := SleepOnLanConfig{
		Enabled:   true,
		Endpoint:  "http://example.com/sleep",
		AuthToken: SecretString{value: "sensitive-auth-token"},
	}

	result := fmt.Sprintf("%+v", config)

	assert.Contains(t, result, "***REDACTED***", "SleepOnLanConfig should show redacted auth token")
	assert.NotContains(t, result, "sensitive-auth-token", "SleepOnLanConfig should not expose actual auth token")
}
