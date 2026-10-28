package mail

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"strings"
	"time"
)

// stdHeaders are the headers every message should carry: a Date and a unique
// Message-ID in the sender's own domain. Receivers (and spam filters) expect
// both, and some relays reject or penalise messages without them.
func (s Sender) stdHeaders() string {
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := "rafactools-planner.local"
	if _, d, ok := strings.Cut(s.From, "@"); ok && d != "" {
		domain = d
	}
	return fmt.Sprintf("Date: %s\r\nMessage-ID: <%s@%s>\r\n",
		time.Now().Format(time.RFC1123Z), hex.EncodeToString(id), domain)
}

// encodeSubject makes a subject safe for any characters (a squadron name with
// an accent, say) by Q-encoding it only when it has to be.
func encodeSubject(subject string) string {
	return mime.QEncoding.Encode("utf-8", subject)
}
