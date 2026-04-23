// Package domain holds the notification bounded context's aggregates and
// value objects. The notification BC reads (not writes) the `users`
// Firestore collection owned by the user BC, plus its own `notifier_dedup`
// collection for idempotency.
package domain

// UserProfile is the notification view of a Firestore `users/{uid}` document.
// Only the fields we actually read are declared — extra fields such as
// favorite_country_cds are ignored by our query path.
type UserProfile struct {
	UID                    string
	FCMTokens              []string
	NotificationPreference NotificationPreference
}

// NotificationPreference mirrors the nested object on the user doc.
type NotificationPreference struct {
	Enabled          bool
	TargetCountryCds []string // 通知を受けたい国の alpha-2 コード
	InfoTypes        []string // 受信する info_type (空 = 全種別)
}

// WantsInfoType reports whether this preference allows the given info_type
// through. Empty InfoTypes means "all types", matching the design default.
func (p NotificationPreference) WantsInfoType(infoType string) bool {
	if len(p.InfoTypes) == 0 {
		return true
	}
	for _, t := range p.InfoTypes {
		if t == infoType {
			return true
		}
	}
	return false
}

// IsDeliverable reports whether this user has any way to receive a push:
// preferences enabled and at least one FCM token registered. The query layer
// filters by Enabled; this is a safety check at the usecase boundary.
func (u UserProfile) IsDeliverable() bool {
	return u.NotificationPreference.Enabled && len(u.FCMTokens) > 0
}
