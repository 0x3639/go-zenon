// Package mailbox tracks unreceived sends destined for each account.
//
// # Overview
//
// In Zenon's send/receive model, a send block sits in the recipient's mailbox
// until the recipient (a user or an embedded contract) authors a matching
// receive block. mailbox is the index that lets the recipient enumerate its
// pending sends efficiently.
//
// Per-package documentation is being filled in incrementally. See
// docs/STYLE.md for the full template applied in subsequent PRs.
package mailbox
