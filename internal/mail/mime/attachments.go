package mime

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/emersion/go-message/mail"
)

// ExtractAttachment returns the metadata and decoded bytes of the
// index-th attachment (0-based, in part order) of a raw message. The
// transfer encoding is undone by go-message, so the returned bytes are
// the file content.
func ExtractAttachment(raw []byte, index int) (AttachmentRef, []byte, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && mr == nil {
		return AttachmentRef{}, nil, fmt.Errorf("mime: unreadable message: %w", err)
	}
	defer mr.Close()

	seen := 0
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return AttachmentRef{}, nil, fmt.Errorf("mime: walk parts: %w", err)
		}
		h, ok := part.Header.(*mail.AttachmentHeader)
		if !ok {
			continue
		}
		if seen != index {
			seen++
			continue
		}
		filename, err := h.Filename()
		if err != nil || filename == "" {
			filename = fmt.Sprintf("attachment-%d", index+1)
		}
		ct, _, err := h.ContentType()
		if err != nil {
			ct = "application/octet-stream"
		}
		// 64 MiB cap: larger attachments need streaming to disk, which
		// the saver upstream can add without changing this contract.
		data, err := io.ReadAll(io.LimitReader(part.Body, 64<<20))
		if err != nil {
			return AttachmentRef{}, nil, fmt.Errorf("mime: read attachment: %w", err)
		}
		return AttachmentRef{Filename: filename, ContentType: ct}, data, nil
	}
	return AttachmentRef{}, nil, fmt.Errorf("mime: attachment %d not found", index)
}
