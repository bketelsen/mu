package auth

// MessageAccess describes whether an inbound channel message may use the owner.
type MessageAccess int

const (
	AccessIgnore MessageAccess = iota
	AccessNeedsLink
	AccessOwner
)

// ClassifyMessage rejects shared contexts and links that are absent or stale.
func ClassifyMessage(isDirect bool, linkedAccount string) MessageAccess {
	if !isDirect {
		return AccessIgnore
	}
	if linkedAccount == "" || !IsOwner(linkedAccount) {
		return AccessNeedsLink
	}
	return AccessOwner
}
