package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
)

func TestOllamaProvider_GetCorrection(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the request
		assert.Equal(t, "/api/generate", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		// Send a mock response
		w.Header().Set("Content-Type", "application/json")
		mockResponse := api.GenerateResponse{
			Response: `{"explanation": "mock explanation", "corrected_command": "mock command"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	// Configure the client to use the mock server
	parsedURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	client := api.NewClient(parsedURL, http.DefaultClient)

	provider := &OllamaProvider{client: client, model: "deepseek-r1:1.5b"}

	// Call the method being tested
	correction, err := provider.GetCorrection(context.Background(), "lş -l", "lş: invalid option -- 'l'", 1, false)

	// Check the result
	assert.NoError(t, err)
	assert.NotNil(t, correction)
	assert.Equal(t, "mock explanation", correction.Explanation)
	assert.Equal(t, "mock command", correction.CorrectedCommand)
}

func TestNewOllamaProvider_HTTPS(t *testing.T) {
	// Create a mock TLS server to simulate an HTTPS endpoint.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The provider's health check sends a HEAD request to the root.
		assert.Equal(t, http.MethodHead, r.Method)
		assert.Equal(t, "/", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// The server's client is used to handle the self-signed certificate.
	provider, err := newOllamaProviderWithClient(server.URL, "test-model", server.Client())

	// Assert that the provider was created successfully without any errors.
	assert.NoError(t, err)
	assert.NotNil(t, provider)
}
