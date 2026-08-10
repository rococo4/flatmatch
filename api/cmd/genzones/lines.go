package main

type lineMeta struct {
	Slug    string
	Name    string // «красная ветка»
	NameIn  string // «на красной ветке»
	NameGen string // «красной ветки»
}

type stationKey struct {
	Line    string // official hh line name
	Station string // station name as the feed spells it
}

type stationFix struct {
	Lat, Lng float64
	Why      string
}

var linesByOfficialName = map[string]lineMeta{
	"Сокольническая":             {"line-1", "красная ветка", "на красной ветке", "красной ветки"},
	"Замоскворецкая":             {"line-2", "зелёная ветка", "на зелёной ветке", "зелёной ветки"},
	"Арбатско-Покровская":        {"line-3", "синяя ветка", "на синей ветке", "синей ветки"},
	"Филевская":                  {"line-4", "голубая ветка", "на голубой ветке", "голубой ветки"},
	"Кольцевая":                  {"line-5", "кольцевая ветка", "на кольцевой ветке", "кольцевой ветки"},
	"Калужско-Рижская":           {"line-6", "оранжевая ветка", "на оранжевой ветке", "оранжевой ветки"},
	"Таганско-Краснопресненская": {"line-7", "фиолетовая ветка", "на фиолетовой ветке", "фиолетовой ветки"},
	"Калининская":                {"line-8", "жёлтая ветка", "на жёлтой ветке", "жёлтой ветки"},
	// Солнцевская is the other yellow line: same colour on the map, a separate
	// zone in the directory. Its declensions carry the qualifier too — without
	// it the two produce word-for-word identical pool headlines («N человек
	// ищут на жёлтой ветке») and the user cannot tell which pool they opened.
	"Солнцевская": {"line-8a", "жёлтая ветка (Солнцевская)",
		"на жёлтой ветке (Солнцевской)", "жёлтой ветки (Солнцевской)"},
	"Серпуховско-Тимирязевская": {"line-9", "серая ветка", "на серой ветке", "серой ветки"},
	"Люблинско-Дмитровская":     {"line-10", "салатовая ветка", "на салатовой ветке", "салатовой ветки"},
	"Большая кольцевая линия":   {"line-11", "БКЛ", "на БКЛ", "БКЛ"},
	"Бутовская":                 {"line-12", "Бутовская ветка", "на Бутовской ветке", "Бутовской ветки"},
	"Некрасовская":              {"line-15", "розовая ветка", "на розовой ветке", "розовой ветки"},
	"Троицкая":                  {"line-16", "Троицкая ветка", "на Троицкой ветке", "Троицкой ветки"},
	"МЦК":                       {"line-mck", "МЦК", "на МЦК", "МЦК"},
	"МЦД-1":                     {"line-d1", "МЦД-1", "на МЦД-1", "МЦД-1"},
	"МЦД-2":                     {"line-d2", "МЦД-2", "на МЦД-2", "МЦД-2"},
	"МЦД-3":                     {"line-d3", "МЦД-3", "на МЦД-3", "МЦД-3"},
	"МЦД-4":                     {"line-d4", "МЦД-4", "на МЦД-4", "МЦД-4"},
}

var skippedLines = map[string]string{
	"Каховская": "absorbed into the Большая кольцевая линия in 2023",

	"Московская монорельсовая дорога": "dismantled, no longer a transport zone",
	"Монорельс": "dismantled, no longer a transport zone",
}

var stationOverrides = map[stationKey]stationFix{
	// hh ships МЦД-2 «Покровское» (order 27, between Царицыно 55.618 and
	// Красный строитель 55.589 — the southern Курское направление) with the
	// coordinates of «Покровское-Стрешнево» (order 9, north-west), byte for
	// byte. Uncorrected it lands ~25 km off, inside Покровское-Стрешнево and
	// Щукино. Real position from OSM (nodes 1994609761 / 1994609766 /
	// 1994609769, railway=halt/stop «Покровское», all within 4 m of each
	// other); the value below is the railway=halt node and it sits between its
	// two МЦД-2 neighbours by latitude, as it must.
	{"МЦД-2", "Покровское"}: {55.602761, 37.631924, "hh duplicates the Покровское-Стрешнево coordinates"},
}

// stationSkips drop a platform from ONE line without touching the others.
// Unlike skippedLines this is not about a line disappearing — the station is
// alive, it just stopped belonging here, and the source feed still says it does.
var stationSkips = map[stationKey]string{
	// Handed over to the Большая кольцевая линия in 2023, when the БКЛ closed
	// its ring; hh still lists them on both. All three are already in line-11,
	// so nothing is lost — line-15 simply stops claiming a stretch it no longer
	// runs. Нижегородская stays: it is the line's genuine terminus.
	{"Некрасовская", "Авиамоторная"}:     "transferred to the БКЛ in 2023",
	{"Некрасовская", "Лефортово"}:        "transferred to the БКЛ in 2023",
	{"Некрасовская", "Электрозаводская"}: "transferred to the БКЛ in 2023",
}

// mcdLineSlugs are the МЦД diameters: commuter rail that runs far past the city
// limits. Only their in-Moscow stations become part of the directory (§8 — the
// market is Moscow), so these are the lines the boundary filter applies to.
var mcdLineSlugs = map[string]bool{
	"line-d1": true,
	"line-d2": true,
	"line-d3": true,
	"line-d4": true,
}
