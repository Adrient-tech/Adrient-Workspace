package cloudimport

import (
	"testing"
	"time"
)

// FakeGoogleEndpoints starts the fixture Google API + Gmail IMAP servers and
// returns OAuth endpoints pointing at them (for the external integration test).
func FakeGoogleEndpoints(t *testing.T, token string) OAuthEndpoints {
	src, fg, _ := googleFixtureSource(t, token)
	ep := src.EP
	return OAuthEndpoints{GoogleAuth: fg.URL + "/auth", GoogleToken: fg.URL + "/token", Google: &ep, MSLogin: fg.URL, Graph: fg.URL}
}

// FastRetry keeps throttling tests quick.
var FastRetry = RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}
