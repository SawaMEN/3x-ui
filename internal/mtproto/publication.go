package mtproto

import "encoding/json"

// Old inbounds remain visible until the administrator explicitly hides them.
func Publication(settings string) (onParent, inSubscription bool) {
	var value struct {
		ShowOnParent       *bool `json:"showOnParent"`
		ShowInSubscription *bool `json:"showInSubscription"`
	}
	if json.Unmarshal([]byte(settings), &value) != nil {
		return false, false
	}
	return value.ShowOnParent == nil || *value.ShowOnParent, value.ShowInSubscription == nil || *value.ShowInSubscription
}
