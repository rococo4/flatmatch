package entity

import (
	"slices"
	"testing"
)

func TestParseNotificationEnums(t *testing.T) {
	tests := []struct {
		name      string
		got, want string
	}{
		{"zone_new", string(ParseNotificationType("zone_new")), string(NotifZoneNew)},
		{"pool_new", string(ParseNotificationType("pool_new")), string(NotifPoolNew)},
		{"liked", string(ParseNotificationType("liked")), string(NotifLiked)},
		{"matched", string(ParseNotificationType("matched")), string(NotifMatched)},
		{"profile_ok", string(ParseNotificationType("profile_ok")), string(NotifProfileOK)},
		{"photo_rejected", string(ParseNotificationType("photo_rejected")), string(NotifPhotoRejected)},
		{"about_rejected", string(ParseNotificationType("about_rejected")), string(NotifAboutRejected)},
		{"point_ok", string(ParseNotificationType("point_ok")), string(NotifPointOK)},
		{"point_rejected", string(ParseNotificationType("point_rejected")), string(NotifPointRejected)},
		{"still_searching", string(ParseNotificationType("still_searching")), string(NotifStillSearching)},
		{"post_match", string(ParseNotificationType("post_match")), string(NotifPostMatch)},

		{"type junk", string(ParseNotificationType("point_changed")), unknown},
		{"type empty", string(ParseNotificationType("")), ""},

		{"poll moving", string(ParsePollAnswer("moving")), string(PollMoving)},
		{"poll talking", string(ParsePollAnswer("talking")), string(PollTalking)},
		{"poll failed", string(ParsePollAnswer("failed")), string(PollFailed)},
		{"poll junk", string(ParsePollAnswer("maybe")), unknown},
		{"poll empty", string(ParsePollAnswer("")), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("parsed %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestPersonalPushTypesHoldNoAggregates(t *testing.T) {
	for _, typ := range PersonalPushTypes() {
		if typ.Aggregated() {
			t.Errorf("%q is both personal and aggregated", typ)
		}
	}
	if !slices.Contains(PersonalPushTypes(), NotifLiked) {
		t.Error("liked is not personal, and the tail is built out of exactly those")
	}

	for _, typ := range []NotificationType{NotifStillSearching, NotifPostMatch} {
		if slices.Contains(PersonalPushTypes(), typ) {
			t.Errorf("%q should not be counted by the safeguard", typ)
		}
	}
}
