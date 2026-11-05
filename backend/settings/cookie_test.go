package settings

import "testing"

func TestNormalizeCookieAcceptsFullCookieHeader(t *testing.T) {
	got, err := normalizeCookie(" Cookie: ageVerified=true; fc2cmadb-session=fixture ", "FC2CMADB", maxFC2CookieLength)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ageVerified=true; fc2cmadb-session=fixture" {
		t.Fatalf("normalized cookie=%q", got)
	}
}
