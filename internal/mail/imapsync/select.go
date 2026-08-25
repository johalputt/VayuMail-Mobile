package imapsync

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// SelectFolder selects a mailbox, turning on CONDSTORE tracking when the
// server advertises it (RFC 7162).
//
// The returned HighestModSeq is the anchor every delta decision hangs off:
// without it the stored modseq stays zero and RefreshFlags must re-read
// the flags of an entire mailbox on every unilateral notification —
// O(mailbox) per flag change, per folder, per account (audit M3). With it,
// CHANGEDSINCE narrows that fetch to exactly the messages that changed.
// Servers without CONDSTORE get a plain SELECT and the old behaviour.
func SelectFolder(client *imapclient.Client, mailbox string) (*imap.SelectData, error) {
	var opts *imap.SelectOptions
	if client.Caps().Has(imap.CapCondStore) {
		opts = &imap.SelectOptions{CondStore: true}
	}
	data, err := client.Select(mailbox, opts).Wait()
	if err != nil {
		return nil, fmt.Errorf("imapsync: select %q: %w", mailbox, err)
	}
	return data, nil
}
