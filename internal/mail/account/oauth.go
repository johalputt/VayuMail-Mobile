package account

import (
	"fmt"

	"github.com/emersion/go-sasl"
)

// Authentication mechanisms. An empty mechanism means classic password
// (LOGIN for IMAP, AUTH PLAIN for SMTP). The token mechanisms authenticate
// with a bearer token instead of a password — the modern-auth / 2FA path,
// where the token is minted by an identity provider (or your VayuPress
// site) and the mail password never exists on the device.
const (
	// AuthPassword is username + password (the default).
	AuthPassword = ""
	// AuthOAuthBearer is SASL OAUTHBEARER (RFC 7628), the IETF standard.
	AuthOAuthBearer = "oauthbearer"
	// AuthXOAuth2 is Google/Microsoft-style XOAUTH2, for servers that
	// implement that older mechanism instead of OAUTHBEARER.
	AuthXOAuth2 = "xoauth2"
)

// IsTokenAuth reports whether a mechanism uses a bearer token rather than a
// password.
func IsTokenAuth(mech string) bool {
	return mech == AuthOAuthBearer || mech == AuthXOAuth2
}

// SASLClient builds the SASL client for a token mechanism. host and port
// identify the server (OAUTHBEARER carries them in its gs2 header). It is
// an error to call this for AuthPassword.
func SASLClient(mech, username, token, host string, port int) (sasl.Client, error) {
	switch mech {
	case AuthOAuthBearer:
		return sasl.NewOAuthBearerClient(&sasl.OAuthBearerOptions{
			Username: username,
			Token:    token,
			Host:     host,
			Port:     port,
		}), nil
	case AuthXOAuth2:
		return &xoauth2Client{username: username, token: token}, nil
	default:
		return nil, fmt.Errorf("account: %q is not a token mechanism", mech)
	}
}

// xoauth2Client implements sasl.Client for the XOAUTH2 mechanism. The
// initial response is the single line
//
//	user=<username>^Aauth=Bearer <token>^A^A
//
// where ^A is 0x01, base64-encoded by the transport.
type xoauth2Client struct {
	username string
	token    string
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	ir = []byte("user=" + c.username + "\x01auth=Bearer " + c.token + "\x01\x01")
	return "XOAUTH2", ir, nil
}

func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	// A challenge here means the server rejected the token and is sending an
	// error payload; an empty client response ends the exchange with a
	// clean authentication failure rather than leaking anything.
	return nil, fmt.Errorf("account: XOAUTH2 rejected: %s", challenge)
}
