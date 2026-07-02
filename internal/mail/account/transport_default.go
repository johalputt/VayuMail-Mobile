//go:build !testonly

package account

// allowPlainTransport is false in every production build: a provisioning
// payload requesting plaintext IMAP or SMTP is rejected with
// ErrInsecureTransport. The testonly build tag flips this for integration
// tests against local plaintext servers only.
const allowPlainTransport = false
