// BLI-STARTER-COMMUNITY-027 / PRI-STARTER-COMMUNITY-027 coverage elevation
package httpheaders

import "testing"

func TestHeaderNameConstants(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		Authorization: "Authorization",
		ContentType:   "Content-Type",
		UserAgent:     "User-Agent",
		XAPIKey:       "X-API-Key",
		XEventType:    "X-Event-Type",
		XSource:       "X-Source",
		XTimestamp:    "X-Timestamp",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
