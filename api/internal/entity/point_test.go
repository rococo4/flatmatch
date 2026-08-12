package entity

import "testing"

func TestParsePointEnums(t *testing.T) {
	tests := []struct {
		name      string
		got, want string
	}{
		{"type flat", string(ParsePointType("flat")), string(PointFlat)},
		{"type intent_home", string(ParsePointType("intent_home")), string(PointIntentHome)},
		{"type junk", string(ParsePointType("hostel")), unknown},
		{"type empty", string(ParsePointType("")), ""},

		{"stage live", string(ParsePointStage("live")), string(StageLive)},
		{"stage moving_in", string(ParsePointStage("moving_in")), string(StageMovingIn)},
		{"stage junk", string(ParsePointStage("landlord")), unknown},
		{"stage empty", string(ParsePointStage("")), ""},

		{"status pending", string(ParsePointStatus("pending")), string(PointPending)},
		{"status active", string(ParsePointStatus("active")), string(PointActive)},
		{"status paused", string(ParsePointStatus("paused")), string(PointPaused)},
		{"status closed", string(ParsePointStatus("closed")), string(PointClosed)},
		{"status junk", string(ParsePointStatus("archived")), unknown},
		{"status empty", string(ParsePointStatus("")), ""},

		{"source tg_channel", string(ParsePointSource("tg_channel")), string(SourceTGChannel)},
		{"source user_link", string(ParsePointSource("user_link")), string(SourceUserLink)},
		{"source manual", string(ParsePointSource("manual")), string(SourceManual)},
		{"source realtor", string(ParsePointSource("realtor")), string(SourceRealtor)},
		{"source junk", string(ParsePointSource("avito_parser")), unknown},
		{"source empty", string(ParsePointSource("")), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("parsed %q, want %q", tt.got, tt.want)
			}
		})
	}
}
