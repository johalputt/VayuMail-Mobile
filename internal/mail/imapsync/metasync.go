package imapsync

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// MetaFolder holds VayuMail's cross-device state (encrypted settings
// blobs) inside the user's own mailbox — multi-device sync with no cloud
// account (ADR-0008). The folder is created on first use.
const MetaFolder = "VayuMail.Meta"

// SaveSettingsBlob appends an (already sealed) settings blob as a message
// in the meta folder. The newest message always wins; older blobs are
// left for the server's own retention.
func SaveSettingsBlob(client *imapclient.Client, sealed []byte) error {
	if err := ensureMetaFolder(client); err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(sealed)
	raw := strings.Join([]string{
		"From: vayumail@localhost",
		"Subject: vayumail-settings",
		"Content-Type: application/octet-stream",
		"",
		encoded,
	}, "\r\n")
	return AppendMessage(client, MetaFolder, []byte(raw),
		[]imap.Flag{imap.FlagSeen})
}

// LoadSettingsBlob fetches the newest settings blob from the meta
// folder. A missing folder or empty folder returns (nil, nil).
func LoadSettingsBlob(client *imapclient.Client) ([]byte, error) {
	selected, err := client.Select(MetaFolder, nil).Wait()
	if err != nil {
		// No meta folder yet: nothing synced from another device.
		return nil, nil //nolint:nilerr // absence is a valid state
	}
	if selected.NumMessages == 0 {
		return nil, nil
	}
	raw, err := fetchNewestRaw(client)
	if err != nil {
		return nil, err
	}
	// Body follows the blank header separator line.
	idx := bytes.Index(raw, []byte("\r\n\r\n"))
	if idx < 0 {
		return nil, fmt.Errorf("imapsync: malformed settings blob message")
	}
	body := bytes.TrimSpace(raw[idx+4:])
	sealed, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, fmt.Errorf("imapsync: decode settings blob: %w", err)
	}
	return sealed, nil
}

// ensureMetaFolder creates the meta folder, tolerating pre-existence.
func ensureMetaFolder(client *imapclient.Client) error {
	err := client.Create(MetaFolder, nil).Wait()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "exist") {
		return fmt.Errorf("imapsync: create %s: %w", MetaFolder, err)
	}
	return nil
}

// fetchNewestRaw returns the raw bytes of the highest-UID message in the
// selected folder.
func fetchNewestRaw(client *imapclient.Client) ([]byte, error) {
	var uidSet imap.UIDSet
	uidSet.AddRange(1, 0)
	bufs, err := client.Fetch(uidSet, &imap.FetchOptions{UID: true}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapsync: list meta uids: %w", err)
	}
	if len(bufs) == 0 {
		return nil, fmt.Errorf("imapsync: meta folder empty")
	}
	newest := bufs[len(bufs)-1].UID
	for _, b := range bufs {
		if b.UID > newest {
			newest = b.UID
		}
	}
	return FetchRaw(client, uint32(newest))
}
