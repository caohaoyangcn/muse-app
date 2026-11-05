package providers

import "testing"

func TestRoutedProviderNameIsNotAnUnderlyingProvider(t *testing.T) {
	provider := NewRoutedProvider(&countingProvider{name: "fc2cmadb"}, &countingProvider{name: "javbus"})
	if provider.Name() != "jav-router" {
		t.Fatalf("router name=%q, want jav-router", provider.Name())
	}
}
