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

		{"bathrooms combined", string(ParseBathrooms("combined")), string(BathroomsCombined)},
		{"bathrooms separate", string(ParseBathrooms("separate")), string(BathroomsSeparate)},
		{"bathrooms two", string(ParseBathrooms("two")), string(BathroomsTwo)},

		{"bathrooms count", string(ParseBathrooms("1")), unknown},
		{"bathrooms empty", string(ParseBathrooms("")), ""},

		{"utilities included", string(ParseUtilities("included")), string(UtilitiesIncluded)},
		{"utilities meters", string(ParseUtilities("meters")), string(UtilitiesMeters)},
		{"utilities junk", string(ParseUtilities("free")), unknown},
		{"utilities empty", string(ParseUtilities("")), ""},

		{"restriction no_pets", string(ParseRestriction("no_pets")), string(RestrictionNoPets)},
		{"restriction no_couples", string(ParseRestriction("no_couples")), string(RestrictionNoCouples)},
		{"restriction no_smoking", string(ParseRestriction("no_smoking")), string(RestrictionNoSmoking)},
		{"restriction label", string(ParseRestriction("без животных")), unknown},
		{"restriction empty", string(ParseRestriction("")), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("parsed %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestGoneThreshold(t *testing.T) {
	tests := []struct {
		pool int
		want int
	}{
		{0, 2}, {1, 2}, {2, 2}, {3, 2}, {4, 2}, {5, 2}, {6, 2},
		{7, 3}, {10, 3}, {20, 6},
	}
	for _, tt := range tests {
		if got := GoneThreshold(tt.pool); got != tt.want {
			t.Errorf("GoneThreshold(%d) = %d, want %d", tt.pool, got, tt.want)
		}
	}
}

func TestVotable(t *testing.T) {
	url := "https://avito.ru/1"
	tests := []struct {
		name  string
		point Point
		want  bool
	}{
		{"flat with a link", Point{Type: PointFlat, URLNorm: &url}, true},
		{"flat without a link", Point{Type: PointFlat}, false},
		{"intent_home", Point{Type: PointIntentHome, URLNorm: &url}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.point.Votable(); got != tt.want {
				t.Errorf("Votable() = %v, want %v", got, tt.want)
			}
		})
	}
}
