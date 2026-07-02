//go:build testonly

package account

// allowPlainTransport permits plaintext transports under the testonly
// build tag so integration tests can run against local unencrypted
// servers. Never ship a binary built with this tag.
const allowPlainTransport = true
